package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentNoteProposalMachineModeAndRenderedHelp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PINAX_API_URL", "")
	root := t.TempDir()
	if _, _, err := runCLIWithStdin(t, "", "init", root, "--title", "CLI", "--json"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--json", "--agent", "--events", "--explain"} {
		out, stderr, err := runCLIWithStdin(t, "PRIVATE_BODY_SENTINEL", "agent", "note", "propose", "--kind", "note_draft", "--title", "Fixture", "--stdin", "--vault", root, mode)
		if err != nil || stderr != "" || strings.Contains(out, "PRIVATE_BODY_SENTINEL") || strings.Contains(out, "\x1b") {
			t.Fatalf("mode %s failed: %v", mode, err)
		}
		if mode == "--json" && !json.Valid([]byte(out)) {
			t.Fatal("JSON invalid")
		}
		if mode == "--agent" && !strings.Contains(out, "command=agent.note.propose") {
			t.Fatal("agent contract missing")
		}
		if mode == "--events" {
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				if !json.Valid([]byte(line)) {
					t.Fatal("events invalid")
				}
			}
		}
	}
	for _, args := range [][]string{{"note", "preview", "--help"}, {"note", "ref", "--help"}, {"agent", "note", "proposal", "--help"}, {"mcp", "serve", "--help"}} {
		out, _, err := runCLIWithStdin(t, "", args...)
		if err != nil || out == "" {
			t.Fatal("help failed")
		}
	}
}

func TestRenderedPreviewExplicitOptInModesAndClipboardFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PINAX_API_URL", "")
	root := t.TempDir()
	if _, _, err := runCLIWithStdin(t, "", "init", root, "--title", "CLI", "--json"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLIWithStdin(t, "# Header\n\nvisible-body-marker\n\n```html\n<script>evil</script>\n```\n", "note", "add", "Fixture", "--dir", "index", "--stdin", "--vault", root, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Facts map[string]string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Facts["note_id"]
	out, _, err = runCLIWithStdin(t, "", "note", "preview", id, "--vault", root)
	if err != nil || !strings.Contains(out, "visible-body-marker") {
		t.Fatal("legacy preview changed")
	}
	for _, mode := range []string{"--json", "--agent", "--events", "--explain"} {
		out, _, err = runCLIWithStdin(t, "", "note", "preview", id, "--view", "rendered", "--vault", root, mode)
		if err != nil || strings.Contains(out, "visible-body-marker") || strings.Contains(out, "\x1b") {
			t.Fatalf("render mode %s: %v", mode, err)
		}
	}
	out, _, err = runCLIWithStdin(t, "", "note", "preview", id, "--view", "rendered", "--display", "body", "--format", "html", "--vault", root)
	if err != nil || strings.Contains(out, "<script>") || !strings.Contains(out, "&lt;script&gt;") {
		t.Fatal("HTML escaping failed")
	}
	t.Setenv("PATH", t.TempDir())
	out, _, err = runCLIWithStdin(t, "", "note", "ref", id, "--copy", "--vault", root, "--json")
	if err != nil || !strings.Contains(out, "unavailable") || !strings.Contains(out, "wikilink") {
		t.Fatal("clipboard fallback absent")
	}
	if _, _, err = runCLIWithStdin(t, "", "note", "preview", id, "--view", "rendered", "--api-url", "https://unreachable.invalid", "--vault", root, "--json"); err == nil {
		t.Fatal("new flags silently dispatched old remote API")
	}
}
