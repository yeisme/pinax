package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeShareStartTest(t *testing.T, vault string, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	fullArgs := append([]string{"--vault", vault, "share", "start"}, args...)
	cmd.SetArgs(fullArgs)
	err := cmd.Execute()
	return out.String(), err
}

func writeShareExploreVault(t *testing.T, files map[string]string) string {
	t.Helper()
	vault := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(vault, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir fixture dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	return vault
}

func TestShareStartExploreViewRequiresVaultReadonlyScope(t *testing.T) {
	vault := writeShareExploreVault(t, map[string]string{})
	out, err := executeShareStartTest(t, vault, "--scope", "published", "--view", "explore", "--host", "127.0.0.1", "--port", "0", "--readonly", "--out", vault, "--json")
	if err == nil || !strings.Contains(out, "share_view_scope_invalid") {
		t.Fatalf("explore view + published scope must fail with stable error: err=%v out=%s", err, out)
	}
}

func TestShareStartRejectsUnknownView(t *testing.T) {
	vault := writeShareExploreVault(t, map[string]string{})
	out, err := executeShareStartTest(t, vault, "--scope", "vault-readonly", "--view", "graph", "--host", "127.0.0.1", "--port", "0", "--readonly", "--token-file", filepath.Join(t.TempDir(), "missing"), "--json")
	if err == nil || !strings.Contains(out, "share_view_invalid") {
		t.Fatalf("unknown view must fail with stable error: err=%v out=%s", err, out)
	}
}

func TestShareStartExploreOnceServesAllEndpoints(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "share-token")
	if err := os.WriteFile(tokenFile, []byte("explore-share-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	vault := writeShareExploreVault(t, map[string]string{
		"notes/auth-design.md":  "---\nschema_version: pinax.note.v1\nnote_id: note_auth_design\ntitle: Auth Design\nkind: reference\ntags: [auth]\nsummary: rotation summary\nupdated_at: 2026-09-01T00:00:00+00:00\nverified: [{by: human:ye, at: 2026-09-01T01:00:00+00:00}]\n---\n\nEXPLORE_BODY_SENTINEL see [[Auth Runbook]] and [[Missing Page]].",
		"notes/auth-runbook.md": "---\nschema_version: pinax.note.v1\nnote_id: note_auth_runbook\ntitle: Auth Runbook\nkind: runbook\nupdated_at: 2026-09-02T00:00:00+00:00\n---\n\nEXPLORE_BODY_SENTINEL",
	})

	out, err := executeShareStartTest(t, vault, "--scope", "vault-readonly", "--view", "explore", "--host", "127.0.0.1", "--port", "0", "--readonly", "--token-file", tokenFile, "--once", "--json")
	if err != nil {
		t.Fatalf("share start explore once: %v\n%s", err, out)
	}
	var envelope struct {
		Command string            `json:"command"`
		Status  string            `json:"status"`
		Facts   map[string]string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &envelope); err != nil {
		t.Fatalf("decode share envelope: %v\n%s", err, out)
	}
	if envelope.Command != "share.start" || envelope.Status != "success" {
		t.Fatalf("envelope = %#v", envelope)
	}
	for key, want := range map[string]string{
		"scope":             "vault-readonly",
		"view":              "explore",
		"auth":              "token-file",
		"explore_nodes":     "2",
		"explore_edges":     "2",
		"explore_truncated": "false",
		"explore_embed":     "true",
		"web_smoke":         "true",
		"api_smoke":         "true",
	} {
		if envelope.Facts[key] != want {
			t.Fatalf("facts[%q] = %q want %q (facts %#v)", key, envelope.Facts[key], want, envelope.Facts)
		}
	}
	for _, leak := range []string{"EXPLORE_BODY_SENTINEL", "explore-share-token", vault, "Missing Page"} {
		if strings.Contains(out, leak) {
			t.Fatalf("explore share output leaked %q:\n%s", leak, out)
		}
	}
}

func TestShareStartDefaultViewOutputUnchanged(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "share-token")
	if err := os.WriteFile(tokenFile, []byte("share-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	vault := writeShareExploreVault(t, map[string]string{
		"notes/private.md": "---\nschema_version: pinax.note.v1\nnote_id: note_private\ntitle: Private\nkind: concept\n---\n\nbody",
	})
	out, err := executeShareStartTest(t, vault, "--scope", "vault-readonly", "--host", "127.0.0.1", "--port", "0", "--readonly", "--token-file", tokenFile, "--once", "--json")
	if err != nil {
		t.Fatalf("default share start once: %v\n%s", err, out)
	}
	if strings.Contains(out, "explore") {
		t.Fatalf("default view must not register explore facts:\n%s", out)
	}
	var envelope struct {
		Facts map[string]string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &envelope); err != nil {
		t.Fatalf("decode share envelope: %v\n%s", err, out)
	}
	for _, required := range []string{"scope", "host", "port", "readonly", "auth", "web_url", "api_url", "served", "web_smoke", "api_smoke"} {
		if _, ok := envelope.Facts[required]; !ok {
			t.Fatalf("default facts missing %q: %#v", required, envelope.Facts)
		}
	}
}
