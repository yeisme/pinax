package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestShareStartPublishedLoopbackProjection(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	writeCLIFixture(t, filepath.Join(outDir, "index.html"), "<html>published</html>")

	out := runCLI(t, "share", "start", "--scope", "published", "--host", "127.0.0.1", "--port", "0", "--readonly", "--out", outDir, "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "share.start" || envelope["status"] != "success" || facts["scope"] != "published" || facts["readonly"] != "true" || facts["web_url"] == "" || facts["api_url"] == "" {
		t.Fatalf("share start envelope = %#v", envelope)
	}
	if strings.Contains(out, root) {
		t.Fatalf("share output leaked local root:\n%s", out)
	}
}

func TestSharePublishedScopeStartServesOnce(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	writeCLIFixture(t, filepath.Join(outDir, "index.html"), "<html>published</html>")
	writeCLIFixture(t, filepath.Join(outDir, "pinax-data", "search-index.json"), `{"entries":[{"id":"note_public","title":"Public","body":"PRIVATE BODY MUST NOT LEAK"}]}`)

	out := runCLI(t, "share", "start", "--scope", "published", "--host", "127.0.0.1", "--port", "0", "--readonly", "--once", "--out", outDir, "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "share.start" || envelope["status"] != "success" || facts["served"] != "true" || facts["web_smoke"] != "true" || facts["api_smoke"] != "true" {
		t.Fatalf("share once envelope = %#v", envelope)
	}
	for _, forbidden := range []string{root, "PRIVATE BODY MUST NOT LEAK"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("share once output leaked %q:\n%s", forbidden, out)
		}
	}
}

func TestShareStartSecurityGates(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	writeCLIFixture(t, filepath.Join(outDir, "index.html"), "<html>published</html>")

	lanOut, lanErr := runCLIExpectError("share", "start", "--scope", "published", "--host", "0.0.0.0", "--port", "8787", "--readonly", "--out", outDir, "--vault", root, "--json")
	if lanErr == nil || !strings.Contains(lanOut, "share_allow_lan_required") {
		t.Fatalf("LAN share should require --allow-lan: out=%s err=%v", lanOut, lanErr)
	}
	authOut, authErr := runCLIExpectError("share", "start", "--scope", "vault-readonly", "--host", "0.0.0.0", "--port", "8787", "--allow-lan", "--readonly", "--out", outDir, "--vault", root, "--json")
	if authErr == nil || !strings.Contains(authOut, "share_auth_required") {
		t.Fatalf("vault-readonly share should require auth: out=%s err=%v", authOut, authErr)
	}
	writeOut, writeErr := runCLIExpectError("share", "start", "--scope", "published", "--host", "127.0.0.1", "--port", "0", "--out", outDir, "--vault", root, "--json")
	if writeErr == nil || !strings.Contains(writeOut, "share_readonly_required") {
		t.Fatalf("share should require readonly: out=%s err=%v", writeOut, writeErr)
	}
}

func TestShareVaultReadonlyScopeStartsWithTokenFileOnce(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/private.md", map[string]string{"note_id": "note_private", "title": "Private", "kind": "concept", "status": "active", "tags": "team"}, "# Private\n\nPRIVATE BODY MUST NOT LEAK")
	tokenFile := filepath.Join(t.TempDir(), "share-token")
	writeCLIFixture(t, tokenFile, "share-token\n")

	out := runCLI(t, "share", "start", "--scope", "vault-readonly", "--host", "127.0.0.1", "--port", "0", "--readonly", "--token-file", tokenFile, "--once", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "share.start" || envelope["status"] != "success" || facts["scope"] != "vault-readonly" || facts["auth"] != "token-file" || facts["api_smoke"] != "true" {
		t.Fatalf("vault-readonly share envelope = %#v", envelope)
	}
	for _, forbidden := range []string{root, tokenFile, "share-token", "PRIVATE BODY MUST NOT LEAK"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("vault-readonly share output leaked %q:\n%s", forbidden, out)
		}
	}
}
