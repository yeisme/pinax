package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDailyNoteCompletionUsesShellFriendlyTitle(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "daily", "2026-06-09.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_daily_legacy\ntitle: Daily 2026-06-09\ntags: [daily]\nfolder: daily\nkind: daily\nstatus: journal\n---\n\n# 2026-06-09\n")

	completion := runCLI(t, "__complete", "note", "show", "--vault", root, "Daily")
	if !strings.Contains(completion, "Daily-2026-06-09\tnote") || strings.Contains(completion, "Daily 2026-06-09") {
		t.Fatalf("daily note completion should use shell-friendly title:\n%s", completion)
	}
	shown := runCLI(t, "note", "show", "Daily-2026-06-09", "--vault", root, "--json")
	if !strings.Contains(shown, `"path":"daily/2026-06-09.md"`) {
		t.Fatalf("daily shell-friendly alias should resolve to the journal note:\n%s", shown)
	}
}
