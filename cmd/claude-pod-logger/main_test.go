package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    format
		wantErr bool
	}{
		{"text", formatText, false},
		{"TEXT", formatText, false},
		{"json", formatJSON, false},
		{"JSON", formatJSON, false},
		{"yaml", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseFormat(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFormat(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseFormat(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestShouldEmit(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"user message", `{"type":"user","message":{"content":"hi"}}`, true},
		{"assistant message", `{"type":"assistant","message":{"content":[]}}`, true},
		{"summary", `{"type":"summary","summary":"recap"}`, true},
		{"attachment", `{"type":"attachment","attachment":{"type":"deferred_tools_delta"}}`, false},
		{"file-history-snapshot", `{"type":"file-history-snapshot"}`, false},
		{"system", `{"type":"system","subtype":"turn_duration"}`, false},
		{"isMeta true", `{"type":"user","message":{"content":"hi"},"isMeta":true}`, false},
		{"unknown type", `{"type":"mystery"}`, false},
		{"missing type", `{"foo":"bar"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tt.in), &m); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if got := shouldEmit(m); got != tt.want {
				t.Errorf("shouldEmit() = %v, want %v\ninput: %s", got, tt.want, tt.in)
			}
		})
	}
}

func TestRenderUser(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain prompt", `{"type":"user","message":{"role":"user","content":"hello"}}`, "👤 hello"},
		{"whitespace stripped", `{"type":"user","message":{"role":"user","content":"  hi there  "}}`, "👤 hi there"},
		{"empty string", `{"type":"user","message":{"content":""}}`, ""},
		{"only whitespace", `{"type":"user","message":{"content":"   \n  "}}`, ""},
		{"array content (tool_result)", `{"type":"user","message":{"content":[{"type":"tool_result","content":"..."}]}}`, ""},
		{"missing message", `{"type":"user"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tt.in), &m); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if got := renderUser(m); got != tt.want {
				t.Errorf("renderUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderAssistant(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"text block",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello!"}]}}`,
			"🦀 Hello!",
		},
		{
			"tool use",
			`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{}}]}}`,
			"🔧 Bash",
		},
		{
			"text + tool",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"running it"},{"type":"tool_use","name":"Read"}]}}`,
			"🦀 running it\n🔧 Read",
		},
		{
			"empty text block dropped",
			`{"type":"assistant","message":{"content":[{"type":"text","text":""},{"type":"text","text":"second"}]}}`,
			"🦀 second",
		},
		{"missing message", `{"type":"assistant"}`, ""},
		{"empty content array", `{"type":"assistant","message":{"content":[]}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tt.in), &m); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if got := renderAssistant(m); got != tt.want {
				t.Errorf("renderAssistant() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"user", `{"type":"user","message":{"content":"hi"}}`, "👤 hi"},
		{"assistant text", `{"type":"assistant","message":{"content":[{"type":"text","text":"hey"}]}}`, "🦀 hey"},
		{"summary field", `{"type":"summary","summary":"recap"}`, "📝 recap"},
		{"summary content fallback", `{"type":"summary","content":"older form"}`, "📝 older form"},
		{"empty summary", `{"type":"summary"}`, ""},
		{"unknown type", `{"type":"mystery"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tt.in), &m); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if got := renderText(m); got != tt.want {
				t.Errorf("renderText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		format  format
		verbose bool
		want    string
		emit    bool
	}{
		{
			"verbose passes everything",
			`{"type":"attachment"}`, formatText, true,
			`{"type":"attachment"}` + "\n", true,
		},
		{
			"verbose adds newline when missing",
			`{"type":"system"}`, formatJSON, true,
			`{"type":"system"}` + "\n", true,
		},
		{
			"verbose keeps existing newline",
			`{"type":"system"}` + "\n", formatJSON, true,
			`{"type":"system"}` + "\n", true,
		},
		{
			"text format filters noise",
			`{"type":"attachment","attachment":{}}`, formatText, false,
			"", false,
		},
		{
			"text format renders user",
			`{"type":"user","message":{"content":"hi"}}`, formatText, false,
			"👤 hi\n", true,
		},
		{
			"json format passes filtered line as-is",
			`{"type":"user","message":{"content":"hi"}}`, formatJSON, false,
			`{"type":"user","message":{"content":"hi"}}` + "\n", true,
		},
		{
			"invalid JSON is dropped in filtered mode",
			`not json`, formatText, false,
			"", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, emit := renderLine([]byte(tt.line), tt.format, tt.verbose)
			if emit != tt.emit {
				t.Errorf("emit = %v, want %v", emit, tt.emit)
			}
			if emit && string(got) != tt.want {
				t.Errorf("rendered = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestAppendNewline(t *testing.T) {
	if got := appendNewline([]byte("hello")); string(got) != "hello\n" {
		t.Errorf("appendNewline without trailing nl: got %q", string(got))
	}
	if got := appendNewline([]byte("hello\n")); string(got) != "hello\n" {
		t.Errorf("appendNewline preserves existing nl: got %q", string(got))
	}
}

func TestStreamRangeBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	content := `{"type":"user","message":{"content":"hi"}}` + "\n" +
		`{"type":"attachment"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"yo"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	next, err := streamRange(path, 0, &buf, formatText, false)
	if err != nil {
		t.Fatalf("streamRange: %v", err)
	}
	if got := next; got != int64(len(content)) {
		t.Errorf("next offset = %d, want %d", got, len(content))
	}
	got := buf.String()
	if !strings.Contains(got, "👤 hi") {
		t.Errorf("missing user line in output: %q", got)
	}
	if !strings.Contains(got, "🦀 yo") {
		t.Errorf("missing assistant line in output: %q", got)
	}
	if strings.Contains(got, "attachment") {
		t.Errorf("attachment leaked through filter: %q", got)
	}
}

func TestStreamRangePartialTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	// Two complete lines + a partial third (no trailing \n).
	content := `{"type":"user","message":{"content":"one"}}` + "\n" +
		`{"type":"user","message":{"content":"two"}}` + "\n" +
		`{"type":"user","message":{"content":"three"`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	next, err := streamRange(path, 0, &buf, formatText, false)
	if err != nil {
		t.Fatalf("streamRange: %v", err)
	}

	want := int64(strings.LastIndex(content, "\n") + 1)
	if next != want {
		t.Errorf("next offset = %d, want %d (start of partial trailing line)", next, want)
	}
	out := buf.String()
	if !strings.Contains(out, "👤 one") || !strings.Contains(out, "👤 two") {
		t.Errorf("missing complete lines: %q", out)
	}
	if strings.Contains(out, "three") {
		t.Errorf("partial line leaked: %q", out)
	}
}

func TestStreamRangeNoNewlineYet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	// File exists with content but no \n yet — should emit nothing and
	// return the offset unchanged so the next scan retries.
	if err := os.WriteFile(path, []byte(`{"type":"user"`), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	next, err := streamRange(path, 0, &buf, formatText, false)
	if err != nil {
		t.Fatalf("streamRange: %v", err)
	}
	if next != 0 {
		t.Errorf("next offset = %d, want 0 (no complete lines)", next)
	}
	if buf.Len() != 0 {
		t.Errorf("output should be empty: %q", buf.String())
	}
}

func TestScanAndStreamTruncationResets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")

	// Initial content (deliberately longer than the replacement below
	// so positions[path] > new file size triggers the truncation reset).
	first := `{"type":"user","message":{"content":"a much longer first prompt that takes up a bunch of bytes"}}` + "\n"
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	positions := map[string]int64{}
	var buf bytes.Buffer
	if err := scanAndStream(dir, positions, &buf, formatText, false); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if !strings.Contains(buf.String(), "a much longer first prompt") {
		t.Fatalf("expected first prompt in output: %q", buf.String())
	}
	buf.Reset()

	// Truncate + write shorter new content. positions[path] is now > new
	// size — code should reset pos to 0 and stream the whole new file.
	second := `{"type":"user","message":{"content":"hi"}}` + "\n"
	if err := os.WriteFile(path, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}
	if int64(len(second)) >= positions[path] {
		t.Fatalf("test setup bug: replacement %d bytes not shorter than original %d", len(second), positions[path])
	}
	if err := scanAndStream(dir, positions, &buf, formatText, false); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if !strings.Contains(buf.String(), "👤 hi") {
		t.Errorf("expected second prompt after truncation reset: %q", buf.String())
	}
}

func TestSnapshotSizes(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "top.jsonl")
	b := filepath.Join(subdir, "nested.jsonl")
	c := filepath.Join(dir, "ignored.txt")
	for _, p := range []string{a, b, c} {
		if err := os.WriteFile(p, []byte("xyz\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	positions := map[string]int64{}
	if err := snapshotSizes(dir, positions); err != nil {
		t.Fatalf("snapshotSizes: %v", err)
	}
	if _, ok := positions[a]; !ok {
		t.Errorf("top-level jsonl missing from snapshot")
	}
	if _, ok := positions[b]; !ok {
		t.Errorf("nested jsonl missing from snapshot")
	}
	if _, ok := positions[c]; ok {
		t.Errorf(".txt file should be ignored")
	}
	if positions[a] != 4 {
		t.Errorf("position for %s = %d, want 4", a, positions[a])
	}
}

func TestScanAndStreamMissingDir(t *testing.T) {
	// walkJSONL swallows fs.ErrNotExist so missing dirs are no-ops.
	positions := map[string]int64{}
	if err := scanAndStream("/does/not/exist", positions, io.Discard, formatText, false); err != nil {
		t.Errorf("scanAndStream on missing dir should not error, got: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions should remain empty: %v", positions)
	}
}
