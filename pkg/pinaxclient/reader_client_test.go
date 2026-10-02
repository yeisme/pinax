package pinaxclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
)

// TestReaderClient covers the typed human reading surface end to end against
// a real Pinax server (SDK + service integration): typed calls, manifest
// digest discovery, unknown-version safe failure, permission failures,
// redirect rejection, response size limits, and no local fallback.

func readerClientServer(t *testing.T, scopes ...string) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\n# Heading\n\nAlpha body zq-client-marker.\n\nLinks [[note_beta]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "beta.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-client", BindingID: "binding-client", Scopes: scopes,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, bearer
}

func TestReaderClientTypedCallsAgainstRealServer(t *testing.T) {
	t.Parallel()
	server, bearer := readerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody, app.ReaderScopeRelations)
	client, err := New(Config{BaseURL: server.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	source, err := client.ReaderSource(ctx)
	if err != nil {
		t.Fatalf("ReaderSource: %v", err)
	}
	if source.SchemaVersion != ReaderSourceSchemaV1Alpha1 || source.BindingID != "binding-client" {
		t.Fatalf("source = %#v", source)
	}
	if source.Limits.ListMaxLimit != 200 || source.Limits.DocumentPageMaxTextBytes != 262144 {
		t.Fatalf("limits = %#v", source.Limits)
	}

	notes, err := client.ReaderNotes(ctx, ReaderPageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ReaderNotes: %v", err)
	}
	if len(notes.Items) != 2 || notes.Items[0].ObjectID != "note_alpha" {
		t.Fatalf("notes = %#v", notes.Items)
	}
	if notes.Items[0].URI != "pinax://note/note_alpha?v="+notes.Items[0].Revision {
		t.Fatalf("note card uri = %q", notes.Items[0].URI)
	}

	search, err := client.ReaderSearch(ctx, "zq-client-marker", ReaderPageQuery{})
	if err != nil {
		t.Fatalf("ReaderSearch: %v", err)
	}
	if len(search.Items) != 1 || search.Items[0].ObjectID != "note_alpha" || len(search.Snippets) != 1 {
		t.Fatalf("search = %#v", search)
	}

	document, err := client.ReaderDocument(ctx, ReaderDocumentRequest{ObjectID: "note_alpha"})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	if document.SchemaVersion != ReaderDocumentSchemaV1Alpha1 {
		t.Fatalf("document = %#v", document)
	}
	if len(document.Outline) != 1 || document.Outline[0].Title != "Heading" {
		t.Fatalf("outline = %#v", document.Outline)
	}
	// Walk remaining pages via the typed cursor.
	current := document
	pages := 1
	for !current.Complete {
		next, err := client.ReaderDocument(ctx, ReaderDocumentRequest{ObjectID: "note_alpha", Revision: current.Revision, Cursor: current.NextCursor})
		if err != nil {
			t.Fatalf("ReaderDocument page %d: %v", pages+1, err)
		}
		current = next
		pages++
		if pages > 20 {
			t.Fatal("document pagination does not terminate")
		}
	}

	links, err := client.ReaderLinks(ctx, "note_alpha", notes.Items[0].Revision, "outgoing", ReaderPageQuery{})
	if err != nil {
		t.Fatalf("ReaderLinks: %v", err)
	}
	if len(links.Items) != 1 || links.Items[0].ObjectID != "note_beta" {
		t.Fatalf("links = %#v", links.Items)
	}

	descriptor, body, err := client.ReaderSourceExport(ctx, "note_alpha", notes.Items[0].Revision)
	if err != nil {
		t.Fatalf("ReaderSourceExport: %v", err)
	}
	if !strings.Contains(string(body), "zq-client-marker") || !strings.HasPrefix(descriptor.MediaType, "text/markdown") {
		t.Fatalf("export = %q / %q", body, descriptor.MediaType)
	}

	// Unknown object ids fail typed, without falling back to local reads.
	if _, err := client.ReaderDocument(ctx, ReaderDocumentRequest{ObjectID: "note_missing"}); err == nil {
		t.Fatal("missing note succeeded")
	} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != "reader_object_not_found" {
		t.Fatalf("missing note err = %v", err)
	}
}

func TestReaderClientRangedSourceExport(t *testing.T) {
	t.Parallel()
	// A note body above the 1 MiB export window: the typed client must be
	// able to discover the total size from the first range reply and fetch
	// the whole source in explicit windows, byte-exact.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString("---\nschema_version: pinax.note.v1\nnote_id: note_big\ntitle: Big\nstatus: active\n---\n\n")
	for i := 0; i < 300000; i++ {
		body.WriteString("abcdefgh")
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "big.md"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-range", BindingID: "binding-range", Scopes: []string{app.ReaderScopeMetadata, app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: bearer, MaxResponseBytes: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	notes, err := client.ReaderNotes(ctx, ReaderPageQuery{})
	if err != nil {
		t.Fatalf("notes: %v", err)
	}
	revision := notes.Items[0].Revision

	// No-range export of an oversized body must fail closed with 416.
	if _, _, err := client.ReaderSourceExport(ctx, "note_big", revision); err == nil {
		t.Fatal("oversized no-range export accepted")
	} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != "reader_range_invalid" {
		t.Fatalf("oversized export err = %v", err)
	}

	// First window reveals the total size via Content-Range.
	window := int64(1024 * 1024)
	first, firstBody, err := client.ReaderSourceExportRange(ctx, "note_big", revision, ReaderByteRange{Start: 0, End: window - 1, Set: true})
	if err != nil {
		t.Fatalf("first range: %v", err)
	}
	if len(firstBody) != int(window) || !strings.Contains(first.ContentRange, "/") {
		t.Fatalf("first range len=%d content-range=%q", len(firstBody), first.ContentRange)
	}
	// The export streams the note body (markdown after frontmatter).
	expectedBody := strings.Repeat("abcdefgh", 300000)
	var total int64
	if _, err := fmt.Sscanf(first.ContentRange, "bytes 0-%d/%d", new(int64), &total); err != nil || total != int64(len(expectedBody)) {
		t.Fatalf("total from content-range: %q err=%v want %d", first.ContentRange, err, len(expectedBody))
	}

	// Walk the remaining windows and reassemble byte-exact.
	reassembled := make([]byte, 0, total)
	reassembled = append(reassembled, firstBody...)
	for offset := window; offset < total; offset += window {
		end := offset + window - 1
		if end >= total {
			end = total - 1
		}
		_, chunk, err := client.ReaderSourceExportRange(ctx, "note_big", revision, ReaderByteRange{Start: offset, End: end, Set: true})
		if err != nil {
			t.Fatalf("range %d-%d: %v", offset, end, err)
		}
		reassembled = append(reassembled, chunk...)
	}
	if string(reassembled) != expectedBody {
		t.Fatalf("ranged export mismatch: got %d bytes want %d", len(reassembled), len(expectedBody))
	}
}

func TestReaderClientByteRangeWindowSemantics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "win.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_win\ntitle: Win\nstatus: active\n---\n\n0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-win", BindingID: "binding-win", Scopes: []string{app.ReaderScopeMetadata, app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: bearer, MaxResponseBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	notes, err := client.ReaderNotes(ctx, ReaderPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	revision := notes.Items[0].Revision

	// The legal one-byte window {0,0} must reach the wire as bytes=0-0 —
	// never silently degrade to a full-body download.
	descriptor, body, err := client.ReaderSourceExportRange(ctx, "note_win", revision, ReaderByteRange{Start: 0, End: 0, Set: true})
	if err != nil {
		t.Fatalf("zero window: %v", err)
	}
	if string(body) != "0" || !strings.HasPrefix(descriptor.ContentRange, "bytes 0-0/") {
		t.Fatalf("zero window body=%q content-range=%q", body, descriptor.ContentRange)
	}
	// The descriptor carries the resource identity: TOTAL size (10, not
	// the 1-byte window) and the content digest from the ETag header.
	if descriptor.SizeBytes != 10 {
		t.Fatalf("descriptor size = %d want total 10", descriptor.SizeBytes)
	}
	fullDigest := sha256.Sum256([]byte("0123456789"))
	if descriptor.Digest != "sha256:"+hex.EncodeToString(fullDigest[:]) {
		t.Fatalf("descriptor digest = %q", descriptor.Digest)
	}

	// Invalid windows fail the call safely without issuing any request.
	for _, invalid := range []ReaderByteRange{
		{Start: -1, End: 10, Set: true},
		{Start: 10, End: 5, Set: true},
	} {
		if _, _, err := client.ReaderSourceExportRange(ctx, "note_win", revision, invalid); err == nil {
			t.Fatalf("invalid window %+v accepted", invalid)
		} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != "request_invalid" {
			t.Fatalf("invalid window err = %v", err)
		}
	}

	// Zero value keeps its meaning: no Range header, whole (small) body.
	wholeDescriptor, whole, err := client.ReaderSourceExport(ctx, "note_win", revision)
	if err != nil {
		t.Fatalf("zero value export: %v", err)
	}
	if string(whole) != "0123456789" {
		t.Fatalf("zero value body = %q", whole)
	}
	if wholeDescriptor.SizeBytes != 10 || wholeDescriptor.Digest != "sha256:"+hex.EncodeToString(fullDigest[:]) {
		t.Fatalf("full-body descriptor = %#v", wholeDescriptor)
	}
}

func TestReaderClientPermissionFailureAndNoFallback(t *testing.T) {
	t.Parallel()
	server, bearer := readerClientServer(t, app.ReaderScopeMetadata)
	client, err := New(Config{BaseURL: server.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	// Body surface denied: typed insufficient_scope, never a local vault read.
	if _, err := client.ReaderSearch(context.Background(), "anything", ReaderPageQuery{}); err == nil {
		t.Fatal("search without body scope succeeded")
	} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != "insufficient_scope" {
		t.Fatalf("search err = %v", err)
	}
	if _, err := client.ReaderDocument(context.Background(), ReaderDocumentRequest{ObjectID: "note_alpha"}); err == nil {
		t.Fatal("document without body scope succeeded")
	}
}

func TestReaderClientUnknownVersionFailsSafely(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spec_version": "1.0", "mode": "json", "command": "reader.source",
			"status": "success",
			"data": map[string]any{"source": map[string]any{
				"schema_version": "pinax.reader.source.v999", "session_id": "s", "binding_id": "b",
				"display_name": "n", "capabilities": []string{}, "limits": map[string]any{},
				"index_state": map[string]any{"status": "fresh"}, "observed_at": "2026-10-01T00:00:00Z",
			}},
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReaderSource(context.Background()); err == nil {
		t.Fatal("unknown schema version accepted")
	} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != CodeUpstreamInvalidResponse {
		t.Fatalf("unknown version err = %v", err)
	}
}

func TestReaderClientRedirectAndSizeLimits(t *testing.T) {
	t.Parallel()
	server, bearer := readerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody)

	// Redirects are never followed.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", server.URL+"/v1/reader/source")
		w.WriteHeader(http.StatusFound)
	}))
	defer redirector.Close()
	client, err := New(Config{BaseURL: redirector.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReaderSource(context.Background()); err == nil {
		t.Fatal("redirect followed")
	} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != CodeRedirectRejected {
		t.Fatalf("redirect err = %v", err)
	}

	// Response bodies beyond the configured limit fail closed.
	bloated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spec_version":"1.0","mode":"json","command":"reader.notes","status":"success","pad":"` + strings.Repeat("x", 4096) + `"}`))
	}))
	defer bloated.Close()
	limited, err := New(Config{BaseURL: bloated.URL, Token: bearer, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.ReaderNotes(context.Background(), ReaderPageQuery{}); err == nil {
		t.Fatal("oversized response accepted")
	} else if clientErr, ok := err.(*Error); !ok || clientErr.Code != CodeResponseTooLarge {
		t.Fatalf("oversize err = %v", err)
	}
}

func TestReaderClientDiscoversRealBindingDigest(t *testing.T) {
	t.Parallel()
	server, bearer := readerClientServer(t, app.ReaderScopeMetadata)
	client, err := New(Config{BaseURL: server.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := client.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if !strings.HasPrefix(manifest.Digest, "sha256:") || manifest.SchemaVersion != TransportManifestSchemaV1 {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	available := 0
	for _, capability := range manifest.Capabilities {
		if !strings.HasPrefix(capability.ID, "reader.") {
			continue
		}
		if len(capability.AvailableSurfaces) == 1 && capability.AvailableSurfaces[0] == "rest" {
			available++
		}
	}
	if available != 7 {
		t.Fatalf("reader capabilities with REST availability = %d, want 7", available)
	}
}

func TestReaderClient413RecoveryChainToSourceExport(t *testing.T) {
	t.Parallel()
	// The complete recovery contract: an oversized document read through the
	// typed client surfaces the machine-recoverable fields, and the consumer
	// follows them straight into a ranged source export — HTTP → typed
	// client → recovery, with no out-of-band knowledge.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString("---\nschema_version: pinax.note.v1\nnote_id: note_huge\ntitle: Huge\nstatus: active\n---\n\n")
	for i := 0; i < 1100000; i++ {
		body.WriteString("abcdefgh")
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "huge.md"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-413", BindingID: "binding-413",
		Scopes:    []string{app.ReaderScopeMetadata, app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: bearer, MaxResponseBytes: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	_, err = client.ReaderDocument(ctx, ReaderDocumentRequest{ObjectID: "note_huge"})
	if err == nil {
		t.Fatal("oversized document accepted")
	}
	clientErr, ok := err.(*Error)
	if !ok || clientErr.Code != "document_too_large" {
		t.Fatalf("document err = %v", err)
	}
	revision := clientErr.Facts["revision"]
	size := clientErr.Facts["source_size_bytes"]
	if !strings.HasPrefix(revision, "sha256:") || size == "" || size == "0" {
		t.Fatalf("recovery facts = %#v", clientErr.Facts)
	}
	foundAction := false
	for _, action := range clientErr.Actions {
		if action.Name == "source_export" && strings.Contains(action.Command, "note_huge") {
			foundAction = true
		}
	}
	if !foundAction {
		t.Fatalf("source_export action missing: %#v", clientErr.Actions)
	}

	// Follow the action: ranged export by the surfaced revision.
	descriptor, firstChunk, err := client.ReaderSourceExportRange(ctx, "note_huge", revision, ReaderByteRange{Start: 0, End: 4095, Set: true})
	if err != nil {
		t.Fatalf("recovery export: %v", err)
	}
	if len(firstChunk) != 4096 || !strings.HasPrefix(descriptor.MediaType, "text/markdown") {
		t.Fatalf("recovery export descriptor = %#v chunk=%d", descriptor, len(firstChunk))
	}
	if !strings.Contains(descriptor.ContentRange, revision[:16]) && descriptor.ContentRange == "" {
		t.Fatal("content range missing")
	}
}

func TestReaderClient413RecoveryFollowsActionToPinnedSnapshotBody(t *testing.T) {
	t.Parallel()
	// The complete pinned-revision recovery chain over HTTP: an over-budget
	// old revision surfaces a 413 whose action names the pinned revision;
	// following it through the typed client must deliver the OLD body, not
	// the shrunken live one.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	var big strings.Builder
	for i := 0; i < 70000; i++ {
		big.WriteString("- item\n")
	}
	bigBody := big.String()
	if err := os.WriteFile(filepath.Join(root, "notes", "flood.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_flood\ntitle: Flood\nstatus: active\n---\n\n"+bigBody), 0o644); err != nil {
		t.Fatal(err)
	}
	service := app.NewService()
	if _, err := service.IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	// Discover the pinned revision from the public listing.
	listing, err := service.ReaderNotes(context.Background(), app.ReaderNotesRequest{
		VaultPath: root, Session: app.ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{app.ReaderScopeMetadata}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pinnedRevision := ""
	for _, card := range listing.Data.(map[string]any)["notes"].(app.ReaderNoteList).Items {
		if card.ObjectID == "note_flood" {
			pinnedRevision = card.Revision
		}
	}
	if pinnedRevision == "" {
		t.Fatal("flood note not listed")
	}
	if _, err := service.VersionSnapshot(context.Background(), app.SnapshotRequest{VaultPath: root, Message: "flood state"}); err != nil {
		t.Fatalf("VersionSnapshot: %v", err)
	}
	// Shrink the live body; only the pinned revision stays over budget.
	if err := os.WriteFile(filepath.Join(root, "notes", "flood.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_flood\ntitle: Flood2\nstatus: active\n---\n\ntiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-pin", BindingID: "binding-pin", Scopes: []string{app.ReaderScopeMetadata, app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(service, root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// The pinned document read fails with the machine-recoverable 413.
	_, err = client.ReaderDocument(ctx, ReaderDocumentRequest{ObjectID: "note_flood", Revision: pinnedRevision})
	if err == nil {
		t.Fatal("over-budget pinned document accepted")
	}
	clientErr, ok := err.(*Error)
	if !ok || clientErr.Code != "document_budget_exceeded" {
		t.Fatalf("pinned document err = %v", err)
	}
	if clientErr.Facts["revision"] != pinnedRevision {
		t.Fatalf("recovery revision = %#v", clientErr.Facts)
	}

	// Follow the action: export the pinned revision in windows and verify
	// the reassembled bytes are the OLD body. The first window reveals the
	// total via Content-Range; later windows clamp to total-1.
	window := int64(128 * 1024)
	descriptor, firstChunk, err := client.ReaderSourceExportRange(ctx, "note_flood", pinnedRevision, ReaderByteRange{Start: 0, End: window - 1, Set: true})
	if err != nil {
		t.Fatalf("first window: %v", err)
	}
	var total int64
	if _, err := fmt.Sscanf(descriptor.ContentRange, "bytes 0-%d/%d", new(int64), &total); err != nil || total == 0 {
		t.Fatalf("content range = %q err=%v", descriptor.ContentRange, err)
	}
	reassembled := append([]byte(nil), firstChunk...)
	for offset := window; offset < total; offset += window {
		end := offset + window - 1
		if end >= total {
			end = total - 1
		}
		_, chunk, chunkErr := client.ReaderSourceExportRange(ctx, "note_flood", pinnedRevision, ReaderByteRange{Start: offset, End: end, Set: true})
		if chunkErr != nil {
			t.Fatalf("window %d-%d: %v", offset, end, chunkErr)
		}
		reassembled = append(reassembled, chunk...)
	}
	want := strings.TrimRight(bigBody, "\n")
	if string(reassembled) != want {
		t.Fatalf("pinned export mismatch: got %d bytes want %d", len(reassembled), len(want))
	}
	if strings.Contains(string(reassembled), "tiny") {
		t.Fatal("pinned export returned live body")
	}
}

func TestReaderClientDecodesFenceNodes(t *testing.T) {
	t.Parallel()
	// The typed client must carry the fence semantics (payload and failure
	// diagnostics), not silently drop the new fields.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spec_version": "1.0", "mode": "json", "command": "reader.document", "status": "success",
			"data": map[string]any{"document": map[string]any{
				"schema_version": "pinax.reader.document.v1alpha1", "object_id": "o", "revision": "r", "current_revision": "r",
				"uri": "u", "nodes": []any{
					map[string]any{"type": "fence", "anchor": "f-1", "fence": "pinax-graph", "payload": map[string]any{"row_count": 3}},
					map[string]any{"type": "fence_failed", "anchor": "f-2", "fence": "pinax-timeline", "error_code": "timeline_unavailable", "next_action": "pinax record history <object-id>"},
				},
				"outline": []any{}, "complete": true, "content_bytes": 1, "observed_at": "2026-10-02T00:00:00Z",
			}},
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ReaderDocument(context.Background(), ReaderDocumentRequest{ObjectID: "o"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 2 {
		t.Fatalf("nodes = %d", len(page.Nodes))
	}
	fence := page.Nodes[0]
	if fence.Type != "fence" || fence.Fence != "pinax-graph" || !strings.Contains(string(fence.Payload), "row_count") {
		t.Fatalf("fence node = %#v payload=%s", fence, string(fence.Payload))
	}
	failed := page.Nodes[1]
	if failed.Type != "fence_failed" || failed.Fence != "pinax-timeline" || failed.ErrorCode != "timeline_unavailable" || failed.NextAction == "" {
		t.Fatalf("fence_failed node = %#v", failed)
	}
}

func TestReaderClientSurfacesIndexStaleState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := func(body string) string {
		return "---\nschema_version: pinax.note.v1\nnote_id: note_st\ntitle: St\nstatus: active\n---\n\n" + body + "\n"
	}
	notePath := filepath.Join(root, "notes", "st.md")
	if err := os.WriteFile(notePath, []byte(fixture("fresh needle body")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-st", BindingID: "binding-st", Scopes: []string{app.ReaderScopeMetadata, app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: bearer, MaxResponseBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	freshList, err := client.ReaderNotes(ctx, ReaderPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if freshList.IndexState.Status != "ready" {
		t.Fatalf("fresh index state = %#v", freshList.IndexState)
	}
	// Edit without refresh: the typed results expose stale explicitly.
	if err := os.WriteFile(notePath, []byte(fixture("edited needle body")), 0o644); err != nil {
		t.Fatal(err)
	}
	staleList, err := client.ReaderNotes(ctx, ReaderPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if staleList.IndexState.Status != "stale" {
		t.Fatalf("stale index state = %#v", staleList.IndexState)
	}
	staleSearch, err := client.ReaderSearch(ctx, "needle", ReaderPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if staleSearch.IndexState.Status != "stale" {
		t.Fatalf("stale search index state = %#v", staleSearch.IndexState)
	}
}
