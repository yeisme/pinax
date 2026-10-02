package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/mcpserver"
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

// TestReaderComponentE2E is the P07 Pinax component run: a real temp vault,
// the real reader HTTP surface and the typed SDK drive the complete reading
// chain, while the normal CLI and the MCP server operate the same vault
// concurrently. Long-document paging, asset bytes, revision drift failures
// and supervised-session EOF are all exercised. Fixture-only: this proves
// the Pinax surface, not DriveBridge platforms or natural-use readiness.

func readerComponentVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	var long strings.Builder
	long.WriteString("# 长文标题\n\n")
	for i := 0; i < 8000; i++ {
		fmt.Fprintf(&long, "第 %d 段：包含中文、路径 /Users/yeisme/notes 和 token 类词与 zq-e2e-marker。\n\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "long.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_long\ntitle: Long Doc\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\n"+long.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "beta.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta links back [[note_long]] and embeds ![[attachments/pic.png]] plus an empty-alt markdown image ![](attachments/pic.png).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 120)...)
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_pic","path":"attachments/pic.png","filename":"pic.png","stem":"pic","extension":".png","media_type":"image/png","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(png), hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	return root
}

func TestReaderComponentE2E(t *testing.T) {
	root := readerComponentVault(t)

	// Reader HTTP surface with a fully scoped session.
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-e2e", BindingID: "binding-e2e",
		Scopes:    []string{app.ReaderScopeMetadata, app.ReaderScopeBody, app.ReaderScopeRelations, app.ReaderScopeAssets},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	service := app.NewService()
	server := httptest.NewServer(api.NewServerWithOptions(service, root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client, err := pinaxclient.New(pinaxclient.Config{BaseURL: server.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 1. Source descriptor: session identity and frozen budgets.
	source, err := client.ReaderSource(ctx)
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if source.SessionID != "sess-e2e" || source.BindingID != "binding-e2e" || source.IndexState.Status != "ready" {
		t.Fatalf("source = %#v", source)
	}

	// 2. Notes listing drives the rest of the chain.
	notes, err := client.ReaderNotes(ctx, pinaxclient.ReaderPageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("notes: %v", err)
	}
	byID := map[string]pinaxclient.ReaderNoteCard{}
	for _, card := range notes.Items {
		byID[card.ObjectID] = card
	}
	if len(notes.Items) != 2 || byID["note_long"].Revision == "" {
		t.Fatalf("notes = %#v", notes.Items)
	}

	// 3. Search finds the marker without leaking it into errors.
	search, err := client.ReaderSearch(ctx, "zq-e2e-marker", pinaxclient.ReaderPageQuery{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(search.Items) != 1 || search.Items[0].ObjectID != "note_long" {
		t.Fatalf("search = %#v", search.Items)
	}

	// 4. Long document: every page pins one revision; no anchor repeats and
	// no text is silently dropped; the walk terminates.
	first, err := client.ReaderDocument(ctx, pinaxclient.ReaderDocumentRequest{ObjectID: "note_long"})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	revision := first.Revision
	seenAnchors := map[string]bool{}
	paragraphs := 0
	current := first
	pages := 1
	for {
		for _, node := range current.Nodes {
			if seenAnchors[node.Anchor] {
				t.Fatalf("page %d repeated anchor %q", pages, node.Anchor)
			}
			seenAnchors[node.Anchor] = true
			if node.Type == "paragraph" && strings.Contains(node.Text, "zq-e2e-marker") {
				paragraphs++
			}
		}
		if current.Complete {
			break
		}
		if current.NextCursor == "" {
			t.Fatal("incomplete page without cursor")
		}
		current, err = client.ReaderDocument(ctx, pinaxclient.ReaderDocumentRequest{ObjectID: "note_long", Revision: revision, Cursor: current.NextCursor})
		if err != nil {
			t.Fatalf("document page %d: %v", pages+1, err)
		}
		if current.Revision != revision {
			t.Fatalf("page %d revision drifted", pages+1)
		}
		pages++
		if pages > 50 {
			t.Fatal("pagination does not terminate")
		}
	}
	if paragraphs == 0 {
		t.Fatal("marker paragraphs never surfaced")
	}

	// 5. Links for the pinned revision.
	links, err := client.ReaderLinks(ctx, "note_long", revision, "backlinks", pinaxclient.ReaderPageQuery{})
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(links.Items) != 1 || links.Items[0].ObjectID != "note_beta" {
		t.Fatalf("links = %#v", links.Items)
	}

	// 6. Asset bytes round-trip through the note-bound entry — and the
	// asset ref is DISCOVERED from the public document response, not from
	// any private manifest knowledge.
	beta := byID["note_beta"]
	betaDocument, err := client.ReaderDocument(ctx, pinaxclient.ReaderDocumentRequest{ObjectID: "note_beta"})
	if err != nil {
		t.Fatalf("beta document: %v", err)
	}
	discoveredRef := findAssetRef(betaDocument.Nodes)
	if discoveredRef == "" {
		t.Fatalf("no pinax-asset ref discovered in document nodes: %#v", betaDocument.Nodes)
	}
	assetID := strings.TrimPrefix(discoveredRef, "pinax-asset://")
	asset, err := client.ReaderAsset(ctx, assetID, "note_beta", beta.Revision)
	if err != nil {
		t.Fatalf("asset %q: %v", discoveredRef, err)
	}
	if len(asset.Body) != 128 || asset.Body[0] != 0x89 || !strings.HasPrefix(asset.Descriptor.Mime, "image/png") {
		t.Fatalf("asset = %d bytes mime=%q", len(asset.Body), asset.Descriptor.Mime)
	}

	// 7. Explicit source export streams the markdown bytes.
	_, exported, err := client.ReaderSourceExport(ctx, "note_beta", beta.Revision)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(string(exported), "[[note_long]]") {
		t.Fatalf("export body = %q", exported)
	}

	// 8. Revision drift: edit the note; the next pinned cursor fails typed.
	if err := os.WriteFile(filepath.Join(root, "notes", "beta.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta2\nstatus: active\n---\n\nEdited body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReaderLinks(ctx, "note_beta", beta.Revision, "outgoing", pinaxclient.ReaderPageQuery{}); err == nil {
		t.Fatal("stale revision accepted after edit")
	} else if clientErr, ok := err.(*pinaxclient.Error); !ok || clientErr.Code != "revision_unavailable" {
		t.Fatalf("drift err = %v", err)
	}

	// 9. CLI concurrency: the compiled binary reads the same vault while the
	// reader server keeps it open — no lock stealing, no process killing.
	pinaxBin := filepath.Join(sharedBinDir, "pinax")
	if out, err := exec.Command(pinaxBin, "note", "list", "--vault", root, "--json").CombinedOutput(); err != nil {
		t.Fatalf("cli note list during reader serve: %v\n%s", err, out)
	}
	if out, err := exec.Command(pinaxBin, "index", "status", "--vault", root, "--json").CombinedOutput(); err != nil {
		t.Fatalf("cli index status during reader serve: %v\n%s", err, out)
	}

	// 10. MCP concurrency: the same vault answers an MCP note search while
	// the reader surface is live.
	mcp := mcpserver.NewServerWithOptions(service, root, mcpserver.ServerOptions{Collaboration: true})
	response, err := mcp.Handle(ctx, mcpserver.Request{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call",
		Params: map[string]any{"name": "pinax.interaction.search", "arguments": map[string]any{"query": "Beta2"}},
	})
	if err != nil {
		t.Fatalf("mcp handle: %v", err)
	}
	encoded, _ := json.Marshal(response)
	if !strings.Contains(string(encoded), "Beta2") {
		t.Fatalf("mcp search response missing result: %s", encoded)
	}

	// 11. Old-permission regression: the legacy Agent route still serves the
	// projection on a legacy-mode server whose token store holds a known
	// long-lived credential; the reader bearer never unlocks that surface
	// and the legacy credential never unlocks the reader surface.
	storeDir := t.TempDir()
	legacyRecord, legacySecret := api.GenerateTokenRecord("", nil, "", "e2e")
	legacyRecord.Scope = map[api.TokenScope]api.ScopeTarget{api.ScopeRead: {}}
	legacyJSON, _ := json.Marshal(map[string]*api.TokenRecord{legacyRecord.ID: legacyRecord})
	storePath := filepath.Join(storeDir, "tokens.json")
	if err := os.WriteFile(storePath, legacyJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{AuthMode: api.AuthModeTokenStore, TokenStore: storePath}).Handler())
	defer legacy.Close()
	legacyRequest, _ := http.NewRequest(http.MethodGet, legacy.URL+"/v1/notes/note_beta", nil)
	legacyRequest.Header.Set("Authorization", "Bearer "+legacySecret)
	legacyResponse, err := legacy.Client().Do(legacyRequest)
	if err != nil {
		t.Fatalf("legacy note read: %v", err)
	}
	legacyBody, _ := io.ReadAll(legacyResponse.Body)
	_ = legacyResponse.Body.Close()
	if legacyResponse.StatusCode != http.StatusOK {
		t.Fatalf("legacy note status = %d body=%s", legacyResponse.StatusCode, legacyBody)
	}
	legacyRequest, _ = http.NewRequest(http.MethodGet, legacy.URL+"/v1/notes/note_beta", nil)
	legacyRequest.Header.Set("Authorization", "Bearer "+bearer)
	legacyResponse, err = legacy.Client().Do(legacyRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = legacyResponse.Body.Close()
	if legacyResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reader bearer reached legacy surface: %d", legacyResponse.StatusCode)
	}
	crossRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/reader/source", nil)
	crossRequest.Header.Set("Authorization", "Bearer "+legacySecret)
	crossResponse, err := server.Client().Do(crossRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = crossResponse.Body.Close()
	if crossResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy token reached reader surface: %d", crossResponse.StatusCode)
	}

	// 12. Supervised session EOF: boot, read once, close stdin, shutdown.
	supervised := makeSupervisedSession(t, root)
	supervisedRequest, _ := http.NewRequest(http.MethodGet, supervised.address+"/v1/reader/source", nil)
	supervisedRequest.Header.Set("Authorization", "Bearer "+supervised.bearer)
	supervisedResponse, err := http.DefaultClient.Do(supervisedRequest)
	if err != nil {
		t.Fatalf("supervised source: %v", err)
	}
	_ = supervisedResponse.Body.Close()
	if supervisedResponse.StatusCode != http.StatusOK {
		t.Fatalf("supervised source status = %d", supervisedResponse.StatusCode)
	}
	supervised.closeStdin()
	if err := supervised.wait(); err != nil {
		t.Fatalf("supervised shutdown: %v", err)
	}
}

// findAssetRef walks document nodes (including children) for the first
// opaque pinax-asset:// reference resolved by the owner.
func findAssetRef(nodes []pinaxclient.ReaderDocumentNode) string {
	for _, node := range nodes {
		if node.Type == "image" && strings.HasPrefix(node.Ref, "pinax-asset://") {
			return node.Ref
		}
		if ref := findAssetRef(node.Children); ref != "" {
			return ref
		}
	}
	return ""
}

type supervisedE2E struct {
	address    string
	bearer     string
	stdout     io.Reader
	closeStdin func()
	wait       func() error
}

func makeSupervisedSession(t *testing.T, root string) *supervisedE2E {
	t.Helper()
	frame := fmt.Sprintf(`{"schema_version":"pinax.local_session.bootstrap.v1alpha1","session_nonce":"nonce-0123456789abcdef","bearer":%q,"binding_id":"binding-eof","vault_root":%q,"scopes":["reader.metadata","reader.body"],"readonly":true}`, "3q2-7e2e-bearer-supervised-aaaaaaaaaaaaaaaa", root)
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stdinWriter.Write([]byte(frame + "\n")); err != nil {
		t.Fatal(err)
	}
	// Production shape: the frame is consumed from the SAME pipe reader
	// the session drains, byte-precisely; the session polices the rest.
	parsed, err := api.ReadSessionBootstrap(stdinReader)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- api.ServeSupervisedSession(context.Background(), api.ServeSupervisedSessionOptions{
			Service: app.NewService(), Frame: parsed,
			Stdin: stdinReader, Stdout: stdoutWriter, Stderr: io.Discard,
		})
	}()
	scanner := readOneEvent(t, stdoutReader)
	var ready struct {
		Type    string `json:"type"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(scanner), &ready); err != nil || ready.Type != "ready" {
		t.Fatalf("supervised ready = %q err=%v", scanner, err)
	}
	return &supervisedE2E{
		address:    ready.Address,
		bearer:     parsed.Bearer,
		stdout:     stdoutReader,
		closeStdin: func() { _ = stdinWriter.Close() },
		wait:       func() error { return <-done },
	}
}

func readOneEvent(t *testing.T, reader io.Reader) string {
	t.Helper()
	buffer := make([]byte, 4096)
	n, err := reader.Read(buffer)
	if err != nil && n == 0 {
		t.Fatalf("read ready event: %v", err)
	}
	return strings.TrimSpace(string(buffer[:n]))
}
