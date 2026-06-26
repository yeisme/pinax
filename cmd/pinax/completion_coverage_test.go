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
	runCLI(t, "plugin", "install", writePluginFixture(t, stateRoot, "project-dashboard"), "--scope", "vault", "--vault", root, "--json")
	bundle := writeContentBundleFixture(t, stateRoot)
	runCLI(t, "collection", "import", "--from", bundle, "--yes", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "research", "alpha.20260625101010.conflict.md"), "# Conflict\n")

	assertCompletionContains(t, runCLI(t, "__complete", "--color", ""), "auto\tcolor", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "--theme", "h"), "high-contrast\ttheme", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "--markdown-style", "d"), "dark\tmarkdown-style", "ShellCompDirectiveNoFileComp")

	assertCompletionContains(t, runCLI(t, "__complete", "project", "switch", "--vault", root, ""), "research\tResearch", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "project", "subproject", "show", "research", "--vault", root, ""), "stock-learning\tStock Learning", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "project", "board", "show", "research", "--vault", root, "--subproject", ""), "stock-learning\tStock Learning", "ShellCompDirectiveNoFileComp")

	assertCompletionContains(t, runCLI(t, "__complete", "folder", "show", "--vault", root, ""), "notes/research\tnotes", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "folder", "list", "--vault", root, "--under", "notes/r"), "notes/research\tnotes", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "profile", "show", ""), "cloud-work\tprofile workspace=ws_test", "ShellCompDirectiveNoFileComp")
	assertCompletionDoesNotContain(t, runCLI(t, "__complete", "profile", "show", ""), "PINAX_TEST_TOKEN", "https://pinax.example.test")
	assertCompletionContains(t, runCLI(t, "__complete", "backend", "show", "--vault", root, ""), "local-dev\tlocal", "ShellCompDirectiveNoFileComp")

	assertCompletionContains(t, runCLI(t, "__complete", "prompt", "show", "--vault", root, ""), "storyboard_prompt_v1\tNovel character portrait", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "prompt", "lifecycle", "storyboard_prompt_v1", "--vault", root, "--to", ""), "tested\tlifecycle", "accepted\tlifecycle", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "plugin", "inspect", "--vault", root, ""), "project-dashboard\tProject Dashboard", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "plugin", "run", "project-dashboard", "--vault", root, ""), "render_dashboard\tview.render", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "collection", "export", "--vault", root, "--format", ""), "eikona.prompt_bundle.v1\tformat", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "graph", "query", "--vault", root, "--kind", ""), "technique\tgraph node kind", "prompt\tgraph node kind", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "sync", "conflicts", "show", "--vault", root, ""), "notes/research/alpha.20260625101010.conflict.md\tconflict", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "note", "list", "--limit", ""), "10\tlimit", "25\tlimit", "50\tlimit", "ShellCompDirectiveNoFileComp")
	assertCompletionContains(t, runCLI(t, "__complete", "note", "list", "--period", ""), "5h\tperiod", "daily\tperiod", "weekly\tperiod", "monthly\tperiod", "ShellCompDirectiveNoFileComp")
}

func TestPathLikeCompletionKeepsFileCompletionCLI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "token.txt"), []byte("token"), 0o600); err != nil {
		t.Fatalf("write token fixture: %v", err)
	}
	out := runCLIInDir(t, root, "__complete", "note", "add", "Title", "--from", "tok")
	if strings.Contains(out, "ShellCompDirectiveNoFileComp") {
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
