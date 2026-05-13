package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeFile writes data to a path under dir and returns the full path.
func writeFile(t *testing.T, dir, name, data string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// readJSON reads a path and decodes the JSON object back to a map for
// structural comparison (so whitespace differences don't matter).
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return m
}

func TestMergeFile_DestExistsKeyOverwritten(t *testing.T) {
	// Mirrors the MCP use case: dest is ~/.claude.json with various
	// Claude-written keys, source supplies a fresh mcpServers block.
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{
		"mcpServers": {"fs": {"command": "fs-server"}}
	}`)
	dst := writeFile(t, dir, "dst.json", `{
		"mcpServers": {"old": {"command": "stale"}},
		"projects": {"/a": {"trustDialog": true}},
		"statsigSdkUserId": "abc"
	}`)

	if err := mergeFile(src, dst); err != nil {
		t.Fatalf("mergeFile: %v", err)
	}

	got := readJSON(t, dst)
	want := map[string]any{
		"mcpServers": map[string]any{
			"fs": map[string]any{"command": "fs-server"},
		},
		"projects": map[string]any{
			"/a": map[string]any{"trustDialog": true},
		},
		"statsigSdkUserId": "abc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged dst = %v, want %v", got, want)
	}
}

func TestMergeFile_DestExistsMultipleKeysOverlaid(t *testing.T) {
	// Mirrors the settings.json use case: source provides several
	// top-level keys (permissions, env, model), dest may already have
	// some of them.
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{
		"permissions": {"allow": ["Bash(ls)"]},
		"env": {"FOO": "1"},
		"model": "claude-opus-4-7"
	}`)
	dst := writeFile(t, dir, "dst.json", `{
		"permissions": {"deny": ["Bash(rm)"]},
		"theme": "dark"
	}`)

	if err := mergeFile(src, dst); err != nil {
		t.Fatalf("mergeFile: %v", err)
	}

	got := readJSON(t, dst)
	want := map[string]any{
		// Shallow merge: permissions is replaced wholesale, not deep-merged.
		"permissions": map[string]any{"allow": []any{"Bash(ls)"}},
		"env":         map[string]any{"FOO": "1"},
		"model":       "claude-opus-4-7",
		"theme":       "dark",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged dst = %v, want %v", got, want)
	}
}

func TestMergeFile_DestMissingCreatesIt(t *testing.T) {
	// ConfigMap is mounted but Claude hasn't written settings.json yet:
	// the dest should be created with the source content.
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{"model": "claude-opus-4-7"}`)
	dst := filepath.Join(dir, "nested", "deep", "settings.json")

	if err := mergeFile(src, dst); err != nil {
		t.Fatalf("mergeFile: %v", err)
	}
	got := readJSON(t, dst)
	want := map[string]any{"model": "claude-opus-4-7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("created dst = %v, want %v", got, want)
	}
}

func TestMergeFile_DestEmptyKeepsSource(t *testing.T) {
	// An empty {} dest is valid JSON; merge should turn it into source.
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{"a": 1}`)
	dst := writeFile(t, dir, "dst.json", `{}`)

	if err := mergeFile(src, dst); err != nil {
		t.Fatalf("mergeFile: %v", err)
	}
	got := readJSON(t, dst)
	if !reflect.DeepEqual(got, map[string]any{"a": float64(1)}) {
		t.Errorf("got %v", got)
	}
}

func TestMergeFile_SourceMissingErrors(t *testing.T) {
	dir := t.TempDir()
	dst := writeFile(t, dir, "dst.json", `{}`)
	err := mergeFile(filepath.Join(dir, "nope.json"), dst)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "read source") {
		t.Errorf("error = %q, want it to mention source", err.Error())
	}
}

func TestMergeFile_InvalidJSONSource(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{not json`)
	dst := writeFile(t, dir, "dst.json", `{}`)
	err := mergeFile(src, dst)
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error = %q, want it to mention invalid JSON", err.Error())
	}
}

func TestMergeFile_InvalidJSONDest(t *testing.T) {
	// Bad existing dest should hard-fail, not silently overwrite — the
	// dest could be a user's hand-edited config we don't want to lose.
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{"a": 1}`)
	dst := writeFile(t, dir, "dst.json", `{not json`)
	err := mergeFile(src, dst)
	if err == nil {
		t.Fatal("expected error for invalid dest")
	}
	if !strings.Contains(err.Error(), "read destination") {
		t.Errorf("error = %q, want it to mention destination", err.Error())
	}
	// And dest must be untouched.
	b, _ := os.ReadFile(dst)
	if string(b) != `{not json` {
		t.Errorf("dest was modified: %s", b)
	}
}

func TestMergeFile_NullSourceRejected(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `null`)
	dst := writeFile(t, dir, "dst.json", `{"a": 1}`)
	if err := mergeFile(src, dst); err == nil {
		t.Fatal("expected error for null source")
	}
}

func TestMergeFile_ArraySourceRejected(t *testing.T) {
	// A JSON array is not a top-level object; refuse it.
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `[1, 2, 3]`)
	dst := writeFile(t, dir, "dst.json", `{}`)
	if err := mergeFile(src, dst); err == nil {
		t.Fatal("expected error for array source")
	}
}

func TestMergeFile_NoTempFilesLeftBehind(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{"a": 1}`)
	dst := writeFile(t, dir, "dst.json", `{"b": 2}`)
	if err := mergeFile(src, dst); err != nil {
		t.Fatalf("mergeFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".claude-pod-config.") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestRun_UsageOnMissingArgs(t *testing.T) {
	if err := run(nil); err == nil {
		t.Error("expected error on missing subcommand")
	}
	if err := run([]string{"merge"}); err == nil {
		t.Error("expected error on missing operands")
	}
	if err := run([]string{"merge", "only-one"}); err == nil {
		t.Error("expected error on missing dest")
	}
	if err := run([]string{"unknown"}); err == nil {
		t.Error("expected error on unknown subcommand")
	}
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("--help should not error: %v", err)
	}
}

func TestRun_MergeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src.json", `{"mcpServers": {"x": 1}}`)
	dst := writeFile(t, dir, "dst.json", `{"projects": {}}`)
	if err := run([]string{"merge", src, dst}); err != nil {
		t.Fatalf("run merge: %v", err)
	}
	got := readJSON(t, dst)
	want := map[string]any{
		"mcpServers": map[string]any{"x": float64(1)},
		"projects":   map[string]any{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
