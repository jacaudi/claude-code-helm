// claude-pod-logger streams Claude Code's per-session JSONL conversation
// files to stdout so claude-pod activity is visible in `kubectl logs`.
//
// It polls the log directory (default ~/.claude/projects) at a fixed
// interval, picks up new files as they appear, and writes new content
// from each file to stdout. It does not depend on tmux, claude, or any
// other process being alive — it just watches files.
//
// Default behaviour skips the historical backlog at startup: files that
// already exist when the logger starts have their current size recorded
// as the starting position. New conversations and appended content are
// streamed in full.
package main

import (
	"bufio"
	"context"
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

func main() {
	dir := flag.String("dir", defaultLogDir(),
		"Root directory holding Claude session JSONL files (scanned recursively)")
	interval := flag.Duration("interval", 2*time.Second,
		"Polling interval between directory scans")
	tail := flag.Bool("tail", true,
		"Skip existing content at startup; only stream content appended after the logger starts")
	flag.Parse()

	setupLogger()
	slog.Info("starting", "dir", *dir, "interval", *interval, "tail", *tail)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	err := run(ctx, *dir, *interval, *tail)
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

func run(ctx context.Context, dir string, interval time.Duration, tail bool) error {
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
		if err := scanAndStream(dir, positions, out); err != nil {
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

// scanAndStream walks root, finds every .jsonl file, and copies any
// content past the last-seen position to w. New files (not yet in
// positions) are streamed from offset 0.
func scanAndStream(root string, positions map[string]int64, w io.Writer) error {
	return walkJSONL(root, func(path string, info fs.FileInfo) {
		size := info.Size()
		pos := positions[path] // zero for unseen files
		if size < pos {
			// File was truncated/rotated; restart from the beginning.
			pos = 0
		}
		if size <= pos {
			return
		}
		if err := streamRange(path, pos, w); err != nil {
			slog.Warn("read failed", "path", path, "err", err)
			return
		}
		positions[path] = size
	})
}

func walkJSONL(root string, visit func(path string, info fs.FileInfo)) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Unreadable entry — skip but keep going.
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
		// Root may not exist yet on a fresh PVC; treat as empty.
		return nil
	}
	return err
}

func streamRange(path string, offset int64, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
