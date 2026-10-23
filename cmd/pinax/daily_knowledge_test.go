package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func TestDailyKnowledgeCapture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--json")
	body := "# 中文记录\n\n" + strings.Repeat("保留原文与来源。", 150) + "\n\n```go\nfmt.Println(\"hello\")\n```\n\n[来源](https://example.org)\n\n| 列 | 值 |\n|---|---|\n| A | B |\n"
	output := runCLIWithInput(t, body, "inbox", "capture", "记录", "--stdin", "--slug", "capture", "--vault", root, "--json")
	var projection domain.Projection
	if err := json.Unmarshal([]byte(output), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.Facts["readback_status"] != "verified" || projection.Facts["note_id"] == "" {
		t.Fatalf("missing readback: %s", output)
	}
	stored, err := os.ReadFile(filepath.Join(root, projection.Facts["path"]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), strings.TrimSpace(body)) {
		t.Fatal("capture changed content")
	}
	if !strings.Contains(runCLI(t, "note", "show", projection.Facts["note_id"], "--display", "body", "--vault", root, "--json"), "来源") {
		t.Fatal("cannot read capture by identity")
	}
	for _, action := range projection.Actions {
		if !strings.Contains(action.Command, projection.Facts["note_id"]) {
			t.Fatalf("unstable action: %v", action)
		}
	}
	for _, args := range [][]string{
		{"--body", "duplicate", "--stdin"}, {"--body", "", "--stdin"}, {"--stdin"},
	} {
		cmd := newRootCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(append([]string{"inbox", "capture", "Rejected", "--vault", root, "--json"}, args...))
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted invalid sources: %v", args)
		}
		if !strings.Contains(out.String(), "note_source_conflict") && !strings.Contains(out.String(), "stdin_empty") {
			t.Fatal(out.String())
		}
	}
	preview := runCLIWithInput(t, body, "inbox", "capture", "Preview", "--stdin", "--dry-run", "--slug", "preview", "--vault", root, "--json")
	if strings.Contains(preview, `"readback_status":"verified"`) {
		t.Fatal("dry run claimed readback")
	}
	if fileExists(filepath.Join(root, "inbox", "preview.md")) || fileExists(filepath.Join(root, "notes", "inbox", "preview.md")) {
		t.Fatal("preview wrote note")
	}
}

func TestDailyKnowledgeReview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--json")
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		runCLI(t, "note", "add", name, "--dir", "index", "--body", "中文有界摘要", "--tags", "reading", "--vault", root, "--json")
	}
	runCLI(t, "inbox", "capture", "raw", "--body", "Raw source", "--vault", root, "--json")
	runCLI(t, "note", "add", "archived", "--dir", "index", "--status", "archived", "--vault", root, "--json")
	runCLI(t, "note", "add", "classified", "--dir", "index", "--project", "chosen", "--vault", root, "--json")
	var projection struct {
		Facts map[string]string     `json:"facts"`
		Data  app.InboxReviewResult `json:"data"`
	}
	output := runCLI(t, "inbox", "review", "--vault", root, "--json")
	if err := json.Unmarshal([]byte(output), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.Data.Total != 7 || len(projection.Data.Items) != 5 || projection.Data.Remaining != 2 {
		t.Fatal(output)
	}
	if projection.Facts["writes"] != "false" || projection.Data.NextAfter == "" {
		t.Fatal(output)
	}
	second := runCLI(t, "inbox", "review", "--after", projection.Data.NextAfter, "--vault", root, "--json")
	if !strings.Contains(second, `"returned":"2"`) {
		t.Fatal(second)
	}
	for _, flags := range [][]string{{"--limit", "0"}, {"--limit", "21"}, {"--after", "missing"}} {
		_, err := runCLIExpectError(append([]string{"inbox", "review", "--vault", root, "--json"}, flags...)...)
		if err == nil {
			t.Fatalf("accepted invalid review: %v", flags)
		}
	}
	for _, mode := range []string{"--agent", "--events", "--explain"} {
		got := runCLI(t, "inbox", "review", "--vault", root, mode)
		want := "inbox.review"
		if mode == "--explain" {
			want = "Conclusion: Pending notes ready for review"
		}
		if !strings.Contains(got, want) {
			t.Fatalf("missing command in %s: %s", mode, got)
		}
	}
	missing, err := runCLIExpectError("inbox", "review", "--vault", t.TempDir(), "--json")
	if err == nil || !strings.Contains(missing, "vault_not_initialized") {
		t.Fatalf("missing vault disguised as empty: %s", missing)
	}
}

func TestDailyKnowledgeStableReading(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--json")
	var created domain.Projection
	body := strings.Repeat("中文摘要", 100)
	out := runCLI(t, "note", "add", "Same title", "--slug", "first", "--body", body, "--vault", root, "--json")
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "note", "add", "Same title", "--slug", "second", "--vault", root, "--json")
	var card struct {
		Actions []domain.Action      `json:"actions"`
		Data    app.SearchShowResult `json:"data"`
	}
	shown := runCLI(t, "search", "show", created.Facts["note_id"], "--vault", root, "--json")
	if err := json.Unmarshal([]byte(shown), &card); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(card.Data.Snippet) || strings.ContainsRune(card.Data.Snippet, utf8.RuneError) || len(card.Data.Snippet) > 200 {
		t.Fatalf("broken Unicode snippet: %q", card.Data.Snippet)
	}
	for _, action := range card.Actions {
		if !strings.Contains(action.Command, created.Facts["note_id"]) {
			t.Fatalf("title-based action: %+v", action)
		}
	}
	logPath := filepath.Join(t.TempDir(), "editor.log")
	editor := writeFakeEditor(t, t.TempDir(), logPath)
	runCLI(t, "note", "open", created.Facts["note_id"], "--editor", editor, "--vault", root, "--json")
	opened, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(opened), "first.md") {
		t.Fatalf("wrong editor target: %s %v", opened, err)
	}
}
