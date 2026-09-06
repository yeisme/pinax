package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func createVerifyVault(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	created := runCLI(t, "note", "new", "Auth Design", "--body", "token rotation must invalidate refresh grants", "--tags", "auth", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(created), &envelope); err != nil {
		t.Fatalf("note new json invalid: %v\n%s", err, created)
	}
	path := envelope["facts"].(map[string]any)["path"].(string)
	return root, path
}

func TestNoteVerifyAppendsEventAndIsIdempotentCLI(t *testing.T) {
	t.Parallel()
	root, path := createVerifyVault(t)

	stdout, stderr, err := runCLISeparate("note", "verify", path, "--actor", "human:ye", "--note", "reviewed", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("note verify json err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("note verify json invalid: %v\n%s", err, stdout)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{"actor": "human:ye", "idempotent": "false", "trust": "human", "index_updated": "true"} {
		if facts[key] != want {
			t.Fatalf("fact %s = %#v, want %q; envelope=%#v", key, facts[key], want, envelope)
		}
	}
	body := readCLIFile(t, filepath.Join(root, path))
	if !strings.Contains(body, "verified:") || !strings.Contains(body, "by: human:ye") || !strings.Contains(body, "note: reviewed") {
		t.Fatalf("verified event missing from frontmatter:\n%s", body)
	}
	if !strings.Contains(body, "token rotation must invalidate refresh grants") {
		t.Fatalf("note body must stay intact:\n%s", body)
	}
	count := strings.Count(body, "by: human:ye")

	// 幂等：同 actor 同日第二次调用不追加，返回既有事件。
	second, err := runCLIExpectError("note", "verify", path, "--actor", "human:ye", "--vault", root, "--json")
	if err != nil {
		t.Fatalf("idempotent verify must not fail: %v\n%s", err, second)
	}
	var secondEnvelope map[string]any
	if err := json.Unmarshal([]byte(second), &secondEnvelope); err != nil {
		t.Fatalf("idempotent verify json invalid: %v\n%s", err, second)
	}
	secondFacts := secondEnvelope["facts"].(map[string]any)
	if secondFacts["idempotent"] != "true" || secondFacts["writes"] != "false" {
		t.Fatalf("idempotent facts = %#v", secondFacts)
	}
	if got := strings.Count(readCLIFile(t, filepath.Join(root, path)), "by: human:ye"); got != count {
		t.Fatalf("idempotent verify appended another event: %d vs %d", got, count)
	}

	// agent 输出携带关键 facts。
	agent := runCLI(t, "note", "verify", path, "--actor", "human:ye", "--vault", root, "--agent")
	for _, want := range []string{"command=note.verify", "fact.idempotent=true", "fact.trust=human"} {
		if !strings.Contains(agent, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agent)
		}
	}
	assertMachineOutputClean(t, agent)
}

func TestNoteVerifyRequiresActorWithoutIdentityCLI(t *testing.T) {
	// 不并行：Setenv 与并行测试互斥。
	stateRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(stateRoot, "config"))
	root, path := createVerifyVault(t)

	out, err := runCLIExpectError("note", "verify", path, "--vault", root, "--json")
	if err == nil || !strings.Contains(out, `"code":"actor_required"`) {
		t.Fatalf("verify without actor/identity should fail closed: err=%v out=%s", err, out)
	}
	if strings.Contains(readCLIFile(t, filepath.Join(root, path)), "verified") {
		t.Fatalf("fail-closed verify must not write events")
	}

	// 配置 identity 后默认 actor 生效（human:<id>）。
	runCLI(t, "config", "set", "identity", "ye", "--scope", "user", "--json")
	defaulted := runCLI(t, "note", "verify", path, "--vault", root, "--json")
	if !strings.Contains(defaulted, `"actor":"human:ye"`) {
		t.Fatalf("identity default actor missing:\n%s", defaulted)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, path)), "by: human:ye") {
		t.Fatalf("identity default actor should write human:ye event")
	}
}

func TestNoteVerifyInvalidExistingTimestampFailsClosedCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "broken.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_broken\ntitle: Broken\nstale_after: 2026-12-01\n---\n\n# Broken\n")
	out, err := runCLIExpectError("note", "verify", "broken", "--actor", "human:ye", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "trust_field_invalid") {
		t.Fatalf("invalid stale_after must fail closed: err=%v out=%s", err, out)
	}
	if got := readCLIFile(t, filepath.Join(root, "notes", "broken.md")); !strings.Contains(got, "stale_after: 2026-12-01\n") {
		t.Fatalf("fail-closed verify must not rewrite the note:\n%s", got)
	}
}

func TestNoteVerifyAmbiguousRefFailsClosedCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "a", "auth.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_a\ntitle: Auth One\n---\n\n# Auth\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "b", "auth.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_b\ntitle: Auth Two\n---\n\n# Auth\n")
	out, err := runCLIExpectError("note", "verify", "notes/a/auth.md", "--actor", "human:ye", "--vault", root, "--json")
	if err != nil || !strings.Contains(out, "note.verify") {
		t.Fatalf("unique path verify should succeed: err=%v out=%s", err, out)
	}
	ambiguous, err := runCLIExpectError("note", "verify", "auth", "--actor", "human:ye", "--vault", root, "--json")
	if err == nil || !strings.Contains(ambiguous, "note_ref_ambiguous") {
		t.Fatalf("ambiguous ref must fail closed: err=%v out=%s", err, ambiguous)
	}
}
