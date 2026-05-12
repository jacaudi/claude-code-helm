// claude-pod-logger streams Claude Code's per-session JSONL conversation
// files to stdout so claude-pod activity is visible in `kubectl logs`.
//
// It polls the log directory (default ~/.claude/projects) at a fixed
// interval, picks up new files as they appear, and emits one entry per
// detected line. By default it filters out noise (file-history snapshots,
// attachment events, system events, meta lines) and renders the signal
// (user prompts, assistant responses, summaries) as compact text. See
// --format json for structured output, --verbose for raw passthrough.
//
// Default behaviour skips the historical backlog at startup: files that
// already exist when the logger starts have their current size recorded
// as the starting position. New conversations and appended content are
// streamed in full.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type format int

const (
	formatText format = iota
	formatJSON
)

func parseFormat(s string) (format, error) {
	switch strings.ToLower(s) {
	case "text":
		return formatText, nil
	case "json":
		return formatJSON, nil
	default:
		return 0, fmt.Errorf("unknown format %q (want text|json)", s)
	}
}

func main() {
	dir := flag.String("dir", defaultLogDir(),
		"Root directory holding Claude session JSONL files (scanned recursively)")
	interval := flag.Duration("interval", 2*time.Second,
		"Polling interval between directory scans")
	tail := flag.Bool("tail", true,
		"Skip existing content at startup; only stream content appended after the logger starts")
	formatStr := flag.String("format", "text",
		"Output format: text (compact human-readable) or json (one filtered JSONL per line)")
	verbose := flag.Bool("verbose", false,
		"Disable filtering and rendering; emit every JSONL line verbatim")
	flag.Parse()

	fmt_, err := parseFormat(*formatStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	setupLogger()
	slog.Info("starting",
		"dir", *dir, "interval", *interval, "tail", *tail,
		"format", *formatStr, "verbose", *verbose)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	err = run(ctx, *dir, *interval, *tail, fmt_, *verbose)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("stopped with error", "err", err)
		os.Exit(1)
	}
	slog.Info("stopped")
}

func defaultLogDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/home/claude"
	}
	return filepath.Join(home, ".claude", "projects")
}

func setupLogger() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
}

func run(ctx context.Context, dir string, interval time.Duration, tail bool, f format, verbose bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil && !errors.Is(err, fs.ErrPermission) {
		slog.Warn("could not ensure log dir", "dir", dir, "err", err)
	}

	positions := map[string]int64{}
	if tail {
		if err := snapshotSizes(dir, positions); err != nil {
			slog.Warn("startup snapshot failed", "err", err)
		}
		slog.Info("baseline captured", "files", len(positions))
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := scanAndStream(dir, positions, out, f, verbose); err != nil {
			slog.Warn("scan failed", "err", err)
		}
		if err := out.Flush(); err != nil {
			return fmt.Errorf("stdout flush: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// snapshotSizes records the current size of every JSONL file under root
// in positions, so that scanAndStream skips the backlog and only emits
// content that appears after this call returns.
func snapshotSizes(root string, positions map[string]int64) error {
	return walkJSONL(root, func(path string, info fs.FileInfo) {
		positions[path] = info.Size()
	})
}

// scanAndStream walks root, finds every .jsonl file, parses the bytes
// past the last-seen position, and emits filtered/rendered lines to w.
// New files (not yet in positions) are streamed from offset 0.
func scanAndStream(root string, positions map[string]int64, w io.Writer, f format, verbose bool) error {
	return walkJSONL(root, func(path string, info fs.FileInfo) {
		size := info.Size()
		pos := positions[path]
		if size < pos {
			pos = 0 // truncated, restart
		}
		if size <= pos {
			return
		}
		next, err := streamRange(path, pos, w, f, verbose)
		if err != nil {
			slog.Warn("read failed", "path", path, "err", err)
			return
		}
		positions[path] = next
	})
}

func walkJSONL(root string, visit func(path string, info fs.FileInfo)) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		visit(path, info)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// streamRange reads from path starting at offset, processes complete
// lines through renderLine, and writes emitted output to w. Returns the
// offset of the first byte after the last complete line — partial
// trailing content (if any) is re-read on the next scan when it's
// complete.
func streamRange(path string, offset int64, w io.Writer, f format, verbose bool) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}

	buf, err := io.ReadAll(file)
	if err != nil {
		return offset, err
	}

	lastNl := bytes.LastIndexByte(buf, '\n')
	if lastNl < 0 {
		// No complete lines yet — re-read on next scan.
		return offset, nil
	}

	complete := buf[:lastNl+1]
	for _, line := range bytes.Split(bytes.TrimRight(complete, "\n"), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		rendered, emit := renderLine(line, f, verbose)
		if !emit {
			continue
		}
		if _, err := w.Write(rendered); err != nil {
			return offset, err
		}
	}

	return offset + int64(lastNl+1), nil
}

// renderLine decides whether to emit a JSONL line and how to render it.
// Returns (output bytes including trailing \n, true) to emit, or
// (nil, false) to skip.
func renderLine(line []byte, f format, verbose bool) ([]byte, bool) {
	if verbose {
		return appendNewline(line), true
	}

	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		return nil, false
	}
	if !shouldEmit(m) {
		return nil, false
	}

	switch f {
	case formatJSON:
		return appendNewline(line), true
	case formatText:
		text := renderText(m)
		if text == "" {
			return nil, false
		}
		return []byte(text + "\n"), true
	}
	return nil, false
}

// shouldEmit returns true for JSONL entries that represent signal
// (user prompts, assistant responses, summaries). Everything else
// (attachments, system events, file-history snapshots, meta lines) is
// dropped.
func shouldEmit(m map[string]any) bool {
	if v, ok := m["isMeta"].(bool); ok && v {
		return false
	}
	typ, _ := m["type"].(string)
	switch typ {
	case "user", "assistant", "summary":
		return true
	}
	return false
}

func renderText(m map[string]any) string {
	switch typ, _ := m["type"].(string); typ {
	case "user":
		return renderUser(m)
	case "assistant":
		return renderAssistant(m)
	case "summary":
		s, _ := m["summary"].(string)
		if s == "" {
			s, _ = m["content"].(string)
		}
		if s == "" {
			return ""
		}
		return "[summary] " + s
	}
	return ""
}

func renderUser(m map[string]any) string {
	msg, _ := m["message"].(map[string]any)
	if msg == nil {
		return ""
	}
	switch c := msg["content"].(type) {
	case string:
		s := strings.TrimSpace(c)
		if s == "" {
			return ""
		}
		return "> " + s
	}
	// Array-shaped content on the user side is almost always tool_result
	// blocks routed back to the model — not signal worth showing.
	return ""
}

func renderAssistant(m map[string]any) string {
	msg, _ := m["message"].(map[string]any)
	if msg == nil {
		return ""
	}
	blocks, _ := msg["content"].([]any)
	var parts []string
	for _, b := range blocks {
		bb, _ := b.(map[string]any)
		switch t, _ := bb["type"].(string); t {
		case "text":
			if s, _ := bb["text"].(string); strings.TrimSpace(s) != "" {
				parts = append(parts, "< "+s)
			}
		case "tool_use":
			name, _ := bb["name"].(string)
			if name != "" {
				parts = append(parts, "· tool: "+name)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func appendNewline(b []byte) []byte {
	if bytes.HasSuffix(b, []byte{'\n'}) {
		return b
	}
	out := make([]byte, len(b)+1)
	copy(out, b)
	out[len(b)] = '\n'
	return out
}
