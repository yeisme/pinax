package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDailyReviewFollowthroughModesAndFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PINAX_API_URL", "")
	root := t.TempDir()
	if _, _, err := runCLIWithStdin(t, "", "init", root, "--title", "Review", "--json"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLIWithStdin(t, "fixture #tag", "note", "add", "Target", "--dir", "index", "--stdin", "--vault", root, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Facts map[string]string `json:"facts"`
	}
	if err = json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatal(err)
	}
	id := p.Facts["note_id"]
	for _, mode := range []string{"--json", "--agent", "--events", "--explain"} {
		for _, args := range [][]string{{"organize", "plan", "--note", id}, {"agent", "note", "proposal", "list", "--status", "approval_required", "--limit", "5"}} {
			args = append(args, "--vault", root, mode)
			out, stderr, err := runCLIWithStdin(t, "", args...)
			if err != nil || stderr != "" || strings.Contains(out, "\x1b") {
				t.Fatal("mode failed", mode, err)
			}
			if mode == "--json" && !json.Valid([]byte(out)) {
				t.Fatal("invalid JSON")
			}
			if mode == "--events" {
				for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
					if !json.Valid([]byte(line)) {
						t.Fatal("invalid NDJSON")
					}
				}
			}
		}
	}
	for _, args := range [][]string{{"organize", "plan", "--note", ""}, {"agent", "note", "proposal", "list", "--limit", "0"}, {"agent", "note", "proposal", "list", "--kind", "unknown"}, {"agent", "note", "proposal", "list", "--api-url", "https://unreachable.invalid"}} {
		if _, _, err := runCLIWithStdin(t, "", append(args, "--json")...); err == nil {
			t.Fatal("invalid/remote input accepted")
		}
	}
	out, _, err = runCLIWithStdin(t, "", "organize", "plan", "--help")
	if err != nil || !strings.Contains(out, "--note") {
		t.Fatal("note help missing")
	}
	out, _, err = runCLIWithStdin(t, "", "agent", "note", "proposal", "list", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--status", "--kind", "--limit", "--cursor"} {
		if !strings.Contains(out, flag) {
			t.Fatal("missing help", flag)
		}
	}
}
