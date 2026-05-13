// claude-pod-config overlays JSON config fragments mounted from
// Kubernetes ConfigMaps onto Claude Code's writable home-directory
// files, preserving anything Claude itself writes there.
//
// Usage:
//
//	claude-pod-config merge SOURCE DEST
//
// Both SOURCE and DEST must contain a JSON object (when DEST exists).
// Each top-level key in SOURCE is assigned onto DEST, overwriting any
// existing value at that key; DEST keys not mentioned in SOURCE are
// preserved verbatim. If DEST does not exist, SOURCE is re-encoded and
// written to DEST (creating parent directories as needed). Writes go
// through a temp file + rename in DEST's directory so a crash mid-write
// cannot leave a half-written config.
//
// Used by claude-pod-init to overlay ConfigMap-mounted fragments onto
// Claude Code's writable state:
//
//	/etc/claude-pod/mcp.json      → ~/.claude.json          (mcpServers, etc.)
//	/etc/claude-pod/settings.json → ~/.claude/settings.json (permissions, env, etc.)
//
// Replaces an earlier jq-based shell pipeline; behaviour matches the
// original `.mcpServers = $src.mcpServers` semantics (top-level
// replacement, not deep merge) generalised to every key in SOURCE.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claude-pod-config:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing subcommand")
	}
	switch args[0] {
	case "merge":
		if len(args) != 3 {
			usage()
			return errors.New("merge: expected SOURCE and DEST")
		}
		return mergeFile(args[1], args[2])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: claude-pod-config merge SOURCE DEST")
}

// mergeFile overlays the top-level keys of the SOURCE JSON object onto
// DEST, writing the result back to DEST atomically. If DEST does not
// exist, SOURCE is re-encoded and written verbatim. Parent directories
// for DEST are created if missing.
func mergeFile(srcPath, dstPath string) error {
	src, err := readJSONObject(srcPath)
	if err != nil {
		return fmt.Errorf("read source %s: %w", srcPath, err)
	}

	dst, err := readJSONObject(dstPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		dst = map[string]any{}
	case err != nil:
		return fmt.Errorf("read destination %s: %w", dstPath, err)
	}

	for k, v := range src {
		dst[k] = v
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	return writeJSONAtomic(dstPath, dst)
}

// readJSONObject reads path and decodes it as a JSON object. A JSON
// `null` or non-object top-level value is rejected so we don't silently
// nuke the destination on bad input.
func readJSONObject(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if m == nil {
		return nil, errors.New("top-level value must be a JSON object, got null")
	}
	return m, nil
}

// writeJSONAtomic encodes v to path via a temp file in the same
// directory followed by rename, so readers never see a partial file.
// Output is 2-space indented to match the jq default the script used.
func writeJSONAtomic(path string, v any) (retErr error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".claude-pod-config.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if retErr != nil {
			os.Remove(tmpPath)
		}
	}()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		return fmt.Errorf("encode: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp to %s: %w", path, err)
	}
	return nil
}
