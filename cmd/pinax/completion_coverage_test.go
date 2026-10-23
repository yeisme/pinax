package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHighValueCompletionCoverageCLI(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(stateRoot, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(stateRoot, "cache"))

	root := filepath.Join(stateRoot, "vault")
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "notes/research", "--vault", root, "--json")
	runCLI(t, "project", "subproject", "create", "research", "stock-learning", "--title", "Stock Learning", "--vault", root, "--json")
	runCLI(t, "folder", "create", "notes/research", "--purpose", "notes", "--vault", root, "--json")
	runCLI(t, "backend", "add", "local", "local-dev", "--root", filepath.Join(stateRoot, "backend"), "--vault", root, "--json")
	runCLI(t, "profile", "add", "cloud-work", "--endpoint", "https://pinax.example.test", "--workspace", "ws_test", "--device", "laptop", "--secret-ref", "env://PINAX_TEST_TOKEN")
	runCLI(t, "prompt", "import", "--from", writePromptAssetFixture(t, stateRoot, "storyboard_prompt_v1"), "--vault", root, "--json")
	runCLI(t, "plugin", "install", writePluginFixture(t, stateRoot), "--scope", "vault", "--vault", root, "--json")
	bundle := writeContentBundleFixture(t, stateRoot)
	runCLI(t, "collection", "import", "--from", bundle, "--yes", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "research", "alpha.20260625101010.conflict.md"), "# Conflict\n")

	assertCompletionContains(t, runCLI(t, "__complete", "--color", ""), "auto\tcolor", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "--theme", "h"), "high-contrast\ttheme", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "--markdown-style", "d"), "dark\tmarkdown-style", ":4\n")

	assertCompletionContains(t, runCLI(t, "__complete", "project", "switch", "--vault", root, ""), "research\tResearch", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "project", "subproject", "show", "research", "--vault", root, ""), "stock-learning\tStock Learning", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "project", "board", "show", "research", "--vault", root, "--subproject", ""), "stock-learning\tStock Learning", ":4\n")

	assertCompletionContains(t, runCLI(t, "__complete", "folder", "show", "--vault", root, ""), "notes/research\tnotes", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "folder", "list", "--vault", root, "--under", "notes/r"), "notes/research\tnotes", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "profile", "show", ""), "cloud-work\tprofile workspace=ws_test", ":4\n")
	assertCompletionDoesNotContain(t, runCLI(t, "__complete", "profile", "show", ""), "PINAX_TEST_TOKEN", "https://pinax.example.test")
	assertCompletionContains(t, runCLI(t, "__complete", "backend", "show", "--vault", root, ""), "local-dev\tlocal", ":4\n")

	assertCompletionContains(t, runCLI(t, "__complete", "prompt", "show", "--vault", root, ""), "storyboard_prompt_v1\tNovel character portrait", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "prompt", "lifecycle", "storyboard_prompt_v1", "--vault", root, "--to", ""), "tested\tlifecycle", "accepted\tlifecycle", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "plugin", "inspect", "--vault", root, ""), "project-dashboard\tProject Dashboard", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "plugin", "run", "project-dashboard", "--vault", root, ""), "render_dashboard\tview.render", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "collection", "export", "--vault", root, "--format", ""), "eikona.prompt_bundle.v1\tformat", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "graph", "query", "--vault", root, "--kind", ""), "technique\tgraph node kind", "prompt\tgraph node kind", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "sync", "conflicts", "show", "--vault", root, ""), "notes/research/alpha.20260625101010.conflict.md\tconflict", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "note", "list", "--limit", ""), "10\tlimit", "25\tlimit", "50\tlimit", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "note", "list", "--period", ""), "5h\tperiod", "daily\tperiod", "weekly\tperiod", "monthly\tperiod", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "share", "start", "--view", ""), "published\tview", "explore\tview", ":4\n")
}

func TestNoteOperationReferenceCompletionCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Alpha Note", "--body", "body", "--vault", root, "--json")

	commands := [][]string{
		{"note", "read"},
		{"note", "refresh"},
		{"note", "links"},
		{"note", "backlinks"},
		{"note", "attachments"},
		{"note", "edit"},
		{"note", "open"},
		{"note", "archive"},
		{"note", "delete"},
		{"note", "property", "set"},
		{"note", "property", "remove"},
		{"note", "tag", "add"},
		{"note", "tag", "remove"},
		{"note", "tag", "set"},
	}
	for _, command := range commands {
		args := append([]string{"__complete"}, command...)
		args = append(args, "--vault", root, "")
		assertCompletionContains(t, runCLI(t, args...), "Alpha Note\tnote", ":4\n")
	}
}

func TestInboxDraftReferenceAndFlagCompletionCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "inbox", "capture", "Inbox Alpha", "--body", "body", "--vault", root, "--json")
	runCLI(t, "draft", "create", "Draft Alpha", "--body", "body", "--vault", root, "--json")

	for _, command := range [][]string{
		{"inbox", "triage"},
		{"inbox", "show"},
		{"inbox", "promote"},
		{"inbox", "discard"},
		{"draft", "show"},
		{"draft", "promote"},
		{"draft", "archive"},
		{"draft", "discard"},
	} {
		args := append([]string{"__complete"}, command...)
		args = append(args, "--vault", root, "")
		out := runCLI(t, args...)
		assertCompletionContains(t, out, ":4\n")
		if !strings.Contains(out, "Inbox Alpha\tnote") && !strings.Contains(out, "Draft Alpha\tnote") {
			t.Fatalf("%v completion missing note candidates:\n%s", command, out)
		}
	}

	assertCompletionContains(t, runCLI(t, "__complete", "inbox", "show", "Inbox", "--vault", root, "--view", ""), "source\tview", "rendered\tview", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "inbox", "promote", "Inbox", "--vault", root, "--to", ""), "draft\tstatus", "active\tstatus", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "draft", "promote", "Draft", "--vault", root, "--status", ""), "active\tstatus", "archived\tstatus", "discarded\tstatus", ":4\n")
}

func TestAssetNoteFlagCompletionCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Asset Context", "--body", "body", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	assertCompletionContains(t, runCLI(t, "__complete", "asset", "link", "diagram", "--vault", root, "--note", ""), "Asset Context\tnote", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "asset", "show", "diagram", "--vault", root, "--context-note", ""), "Asset Context\tnote", ":4\n")
	assertCompletionContains(t, runCLI(t, "__complete", "asset", "preview", "diagram", "--vault", root, "--context-note", ""), "Asset Context\tnote", ":4\n")
}

func TestRootHelpGroupsCurrentTopLevelCommandsCLI(t *testing.T) {
	t.Parallel()
	help := runCLI(t, "--help")
	if strings.Contains(help, "Other\n") {
		t.Fatalf("root help should not leave current product commands in Other:\n%s", help)
	}
	for _, want := range []string{"Start here", "Capture and write", "Find and organize", "Local safety", "More commands"} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing core group %q:\n%s", want, help)
		}
	}
	catalog := runCLI(t, "commands")
	for _, want := range []string{"draft", "graph", "monitor", "prompt", "proof", "api", "token", "profile"} {
		if !strings.Contains(catalog, want) {
			t.Fatalf("command catalog missing advanced command %q:\n%s", want, catalog)
		}
	}
}

func TestPathLikeCompletionKeepsFileCompletionCLI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "token.txt"), []byte("token"), 0o600); err != nil {
		t.Fatalf("write token fixture: %v", err)
	}
	out := runCLIInDir(t, root, "__complete", "note", "add", "Title", "--from", "tok")
	if !strings.HasSuffix(out, ":0\n") {
		t.Fatalf("--from should keep file completion enabled:\n%s", out)
	}
}

func assertCompletionContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("completion missing %q:\n%s", want, out)
		}
	}
}

func assertCompletionDoesNotContain(t *testing.T, out string, forbidden ...string) {
	t.Helper()
	for _, item := range forbidden {
		if strings.Contains(out, item) {
			t.Fatalf("completion leaked %q:\n%s", item, out)
		}
	}
}
