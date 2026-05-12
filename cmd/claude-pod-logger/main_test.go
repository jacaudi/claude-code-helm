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

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"valid", "2026-05-12T18:11:56.518Z", "18:11:56"},
		{"valid no fractional", "2026-05-12T18:11:56Z", "18:11:56"},
		{"valid with TZ", "2026-05-12T13:11:56-05:00", "18:11:56"},
		{"missing", nil, ""},
		{"empty string", "", ""},
		{"non-string", 12345, ""},
		{"malformed", "not-a-date", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTimestamp(tt.in); got != tt.want {
				t.Errorf("formatTimestamp(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPrefixLines(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		text string
		want string
	}{
		{"single line with ts", "18:11:56", "👤 hi", "18:11:56 👤 hi"},
		{"empty text", "18:11:56", "", ""},
		{"empty ts (no prefix)", "", "👤 hi", "👤 hi"},
		{
			"multi-line indented",
			"18:11:56", "🦀 line one\nline two",
			"18:11:56 🦀 line one\n         line two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prefixLines(tt.ts, tt.text); got != tt.want {
				t.Errorf("prefixLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderUserPrompt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain prompt", `{"type":"user","message":{"role":"user","content":"hello"}}`, "👤 hello"},
		{"whitespace stripped", `{"type":"user","message":{"role":"user","content":"  hi there  "}}`, "👤 hi there"},
		{"empty string", `{"type":"user","message":{"content":""}}`, ""},
		{"only whitespace", `{"type":"user","message":{"content":"   \n  "}}`, ""},
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

func TestRenderUserToolResult(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"plain string result",
			`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"x","content":"hello world"}]}}`,
			"↩ hello world",
		},
		{
			"multi-line result picks first non-blank",
			`{"type":"user","message":{"content":[{"type":"tool_result","content":"\n\nfirst\nsecond"}]}}`,
			"↩ first",
		},
		{
			"error",
			`{"type":"user","message":{"content":[{"type":"tool_result","content":"oops","is_error":true}]}}`,
			"↩ ERR: oops",
		},
		{
			"structured content (text block)",
			`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"hi from block"}]}]}}`,
			"↩ hi from block",
		},
		{
			"empty result dropped",
			`{"type":"user","message":{"content":[{"type":"tool_result","content":""}]}}`,
			"",
		},
		{
			"empty error annotated",
			`{"type":"user","message":{"content":[{"type":"tool_result","content":"","is_error":true}]}}`,
			"↩ (empty error)",
		},
		{
			"non-tool_result blocks skipped",
			`{"type":"user","message":{"content":[{"type":"something_else","data":"..."}]}}`,
			"",
		},
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
			"tool use with input",
			`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
			`🔧 Bash: {"command":"ls"}`,
		},
		{
			"tool use no input",
			`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"LS"}]}}`,
			"🔧 LS",
		},
		{
			"tool use empty-object input",
			`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Glob","input":{}}]}}`,
			"🔧 Glob",
		},
		{
			"text + tool",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"running it"},{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/a"}}]}}`,
			"🦀 running it\n" + `🔧 Read: {"file_path":"/tmp/a"}`,
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

func TestRenderTextWithTimestamp(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"user with ts",
			`{"type":"user","message":{"content":"hi"},"timestamp":"2026-05-12T18:11:56Z"}`,
			"18:11:56 👤 hi",
		},
		{
			"assistant multi-line indented",
			`{"type":"assistant","message":{"content":[{"type":"text","text":"line one\nline two"}]},"timestamp":"2026-05-12T18:11:56Z"}`,
			"18:11:56 🦀 line one\n         line two",
		},
		{
			"summary with ts",
			`{"type":"summary","summary":"recap","timestamp":"2026-05-12T18:11:56Z"}`,
			"18:11:56 📝 recap",
		},
		{
			"no ts falls back to bare prefix",
			`{"type":"user","message":{"content":"hi"}}`,
			"👤 hi",
		},
		{
			"unknown type returns empty",
			`{"type":"mystery"}`, "",
		},
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
		name     string
		line     string
		format   format
		verbose  bool
		want     string
		wantRole string
		emit     bool
	}{
		{
			"verbose passes everything",
			`{"type":"attachment"}`, formatText, true,
			`{"type":"attachment"}` + "\n", "", true,
		},
		{
			"text format filters noise",
			`{"type":"attachment","attachment":{}}`, formatText, false,
			"", "", false,
		},
		{
			"text format renders user",
			`{"type":"user","message":{"content":"hi"},"timestamp":"2026-05-12T18:11:56Z"}`, formatText, false,
			"18:11:56 👤 hi\n", "user", true,
		},
		{
			"json format passes filtered line as-is",
			`{"type":"user","message":{"content":"hi"}}`, formatJSON, false,
			`{"type":"user","message":{"content":"hi"}}` + "\n", "user", true,
		},
		{
			"invalid JSON is dropped in filtered mode",
			`not json`, formatText, false,
			"", "", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, role, emit := renderLine([]byte(tt.line), tt.format, tt.verbose)
			if emit != tt.emit {
				t.Errorf("emit = %v, want %v", emit, tt.emit)
			}
			if emit && string(got) != tt.want {
				t.Errorf("rendered = %q, want %q", string(got), tt.want)
			}
			if role != tt.wantRole {
				t.Errorf("role = %q, want %q", role, tt.wantRole)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Errorf("truncate over: got %q", got)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate under: got %q", got)
	}
	if got := truncate("abc", 0); got != "abc" {
		t.Errorf("truncate 0 cap: got %q", got)
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

func TestStreamRangeTurnBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	content := `{"type":"user","message":{"content":"hi"},"timestamp":"2026-05-12T18:11:56Z"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"yo"}]},"timestamp":"2026-05-12T18:11:57Z"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"more"}]},"timestamp":"2026-05-12T18:11:58Z"}` + "\n" +
		`{"type":"user","message":{"content":"again"},"timestamp":"2026-05-12T18:11:59Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	st := &emissionState{}
	if _, err := streamRange(path, 0, &buf, formatText, false, st); err != nil {
		t.Fatalf("streamRange: %v", err)
	}
	got := buf.String()
	// User -> assistant should have a blank line; assistant -> assistant should not;
	// assistant -> user should.
	want := "18:11:56 👤 hi\n\n18:11:57 🦀 yo\n18:11:58 🦀 more\n\n18:11:59 👤 again\n"
	if got != want {
		t.Errorf("turn boundary output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestStreamRangePartialTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	content := `{"type":"user","message":{"content":"one"}}` + "\n" +
		`{"type":"user","message":{"content":"two"}}` + "\n" +
		`{"type":"user","message":{"content":"three"`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	st := &emissionState{}
	next, err := streamRange(path, 0, &buf, formatText, false, st)
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
	if err := os.WriteFile(path, []byte(`{"type":"user"`), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	st := &emissionState{}
	next, err := streamRange(path, 0, &buf, formatText, false, st)
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

	first := `{"type":"user","message":{"content":"a much longer first prompt that takes up a bunch of bytes"}}` + "\n"
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	positions := map[string]int64{}
	st := &emissionState{}
	var buf bytes.Buffer
	if err := scanAndStream(dir, positions, &buf, formatText, false, st); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if !strings.Contains(buf.String(), "a much longer first prompt") {
		t.Fatalf("expected first prompt in output: %q", buf.String())
	}
	buf.Reset()

	second := `{"type":"user","message":{"content":"hi"}}` + "\n"
	if err := os.WriteFile(path, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}
	if int64(len(second)) >= positions[path] {
		t.Fatalf("test setup bug: replacement %d bytes not shorter than original %d", len(second), positions[path])
	}
	if err := scanAndStream(dir, positions, &buf, formatText, false, st); err != nil {
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
	positions := map[string]int64{}
	st := &emissionState{}
	if err := scanAndStream("/does/not/exist", positions, io.Discard, formatText, false, st); err != nil {
		t.Errorf("scanAndStream on missing dir should not error, got: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions should remain empty: %v", positions)
	}
}
