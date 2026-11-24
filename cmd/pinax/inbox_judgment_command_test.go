package main

import (
	"strings"
	"testing"
)

// inbox_judgment_command_test.go 覆盖实验性判断入口的 CLI 合同：默认 off、
// dry-run 零外部调用、非法参数 fail closed，以及命令树可见性。

func TestInboxJudgmentCommand_HelpExists(t *testing.T) {
	t.Parallel()
	help := runCLI(t, "inbox", "judgment", "--help")
	for _, want := range []string{"Evaluate one pending inbox note", "Suggestions are advisory", "--dry-run", "--attempt", "--note"} {
		if !strings.Contains(help, want) {
			t.Fatalf("inbox judgment --help missing %q:\n%s", want, help)
		}
	}
}

func TestInboxJudgmentCommand_InCommandTree(t *testing.T) {
	t.Parallel()
	tree := runCLI(t, "inbox", "--help")
	if !strings.Contains(tree, "judgment") {
		t.Fatalf("inbox command tree should list the judgment entry:\n%s", tree)
	}
}

func TestInboxJudgmentCommandDefaultsToOff(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "inbox", "capture", "Judgment candidate", "--body", "inbox body", "--tags", "deep-work", "--vault", root, "--json")
	runCLI(t, "note", "new", "Related reference", "--body", "related body", "--tags", "deep-work", "--vault", root, "--json")

	out := runCLI(t, "inbox", "judgment", "--vault", root, "--json")
	for _, want := range []string{`"command":"inbox.judgment"`, `"judgment_mode":"off"`, `"provider_dispatch":"none"`, `"credential_reads":"none"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("dormant judgment output missing %q:\n%s", want, out)
		}
	}
}

func TestInboxJudgmentCommandDryRunStaysOffline(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "inbox", "capture", "Judgment candidate", "--body", "inbox body", "--tags", "deep-work", "--vault", root, "--json")
	runCLI(t, "note", "new", "Related reference", "--body", "related body", "--tags", "deep-work", "--vault", root, "--json")

	t.Setenv("PINAX_JUDGMENT_ENABLED", "true")
	t.Setenv("PINAX_JUDGMENT_MODE", "shadow")
	t.Setenv("PINAX_JUDGMENT_PROFILE", "review")
	t.Setenv("PINAX_JUDGMENT_PROVIDER", "openai_decisions")
	t.Setenv("PINAX_JUDGMENT_MODEL", "gpt-6-luna")
	t.Setenv("PINAX_JUDGMENT_CREDENTIAL_SOURCE", "env")
	t.Setenv("PINAX_JUDGMENT_CREDENTIAL_ENV", "PINAX_JUDGMENT_TEST_CREDENTIAL")

	out := runCLI(t, "inbox", "judgment", "--dry-run", "--vault", root, "--json")
	for _, want := range []string{`"judgment_status":"planned"`, `"provider_dispatch":"none"`, `"credential_reads":"none"`, `"candidates":"1"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run judgment output missing %q:\n%s", want, out)
		}
	}
}

func TestInboxJudgmentCommandRejectsOutOfRangeLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("inbox", "judgment", "--limit", "9", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("out-of-range limit must fail closed:\n%s", out)
	}
	if !strings.Contains(out, "invalid_limit") {
		t.Fatalf("expected invalid_limit, got:\n%s", out)
	}
}
