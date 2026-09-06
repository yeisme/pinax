package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func writeExploreNote(t *testing.T, root, rel, frontmatter, body string) {
	t.Helper()
	content := "---\n" + frontmatter + "\n---\n\n" + body
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(rel)), content)
}

func TestExploreBundleAssemblyBoundedProjection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/auth-design.md", strings.Join([]string{
		"schema_version: pinax.note.v1",
		"note_id: note_auth_design",
		"title: Auth Design",
		"kind: reference",
		"tags: [auth, security]",
		"summary: Token rotation must invalidate refresh grants.",
		"updated_at: 2026-09-01T10:00:00+00:00",
		"generated: {by: agent:pinax/0.9.0, at: 2026-09-01T08:00:00+00:00}",
		"verified: [{by: human:ye, at: 2026-09-01T10:30:00+00:00}]",
		"stale_after: 2030-01-01T00:00:00+00:00",
	}, "\n"), "# Auth Design\n\nVAULT_BODY_SENTINEL\n\nSee [[Auth Runbook]] and [[Missing Target]].")

	writeExploreNote(t, root, "notes/auth-runbook.md", strings.Join([]string{
		"schema_version: pinax.note.v1",
		"note_id: note_auth_runbook",
		"title: Auth Runbook",
		"kind: runbook",
		"tags: [auth, ops, extra1, extra2, extra3, extra4, extra5, extra6, extra7, extra8, extra9]",
		"updated_at: 2026-09-02T10:00:00+00:00",
		"verified: {by: machine:pinax/0.9.0, at: 2026-09-02T10:00:00+00:00}",
	}, "\n"), "# Auth Runbook\n\nVAULT_BODY_SENTINEL")

	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	bundle, err := buildExploreBundle(root, defaultExploreBundleLimits(), now)
	if err != nil {
		t.Fatalf("build explore bundle: %v", err)
	}

	if bundle.SchemaVersion != ExploreBundleSchemaVersion {
		t.Fatalf("schema version = %q", bundle.SchemaVersion)
	}
	if bundle.Counts.Nodes != 2 || bundle.Counts.Edges != 2 || bundle.Counts.Truncated {
		t.Fatalf("counts = %#v", bundle.Counts)
	}

	byID := map[string]ExploreNode{}
	for _, node := range bundle.Nodes {
		byID[node.ID] = node
	}
	design := byID["note_auth_design"]
	if design.Title != "Auth Design" || design.Kind != "reference" || design.Trust != "human" || design.Fresh != "fresh" {
		t.Fatalf("auth-design node = %#v", design)
	}
	if design.Summary != "Token rotation must invalidate refresh grants." {
		t.Fatalf("summary = %q", design.Summary)
	}
	runbook := byID["note_auth_runbook"]
	if runbook.Trust != "machine" {
		t.Fatalf("runbook trust = %q (bare mapping must count as single verified event)", runbook.Trust)
	}
	if len(runbook.Tags) != 8 {
		t.Fatalf("runbook tags = %#v (want bounded to 8)", runbook.Tags)
	}

	edges := map[string]ExploreEdge{}
	for _, edge := range bundle.Edges {
		edges[edge.From+"->"+edge.To] = edge
	}
	resolved, ok := edges["note_auth_design->note_auth_runbook"]
	if !ok || resolved.Broken {
		t.Fatalf("resolved edge missing or broken flag wrong: %#v", bundle.Edges)
	}
	broken, ok := edges["note_auth_design->Missing Target"]
	if !ok || !broken.Broken {
		t.Fatalf("broken edge missing: %#v", bundle.Edges)
	}
}

func TestExploreBundleRecursivelyOmitsBodiesAndPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/private.md", strings.Join([]string{
		"schema_version: pinax.note.v1",
		"note_id: note_private",
		"title: Private Note EXPLORE_BODY_SENTINEL",
		"summary: Safe summary line.",
	}, "\n"), "# Private\n\nEXPLORE_BODY_SENTINEL body line that must never ship.")

	bundle, err := BuildExploreBundle(root)
	if err != nil {
		t.Fatalf("build explore bundle: %v", err)
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	payload := string(body)
	if strings.Contains(payload, "EXPLORE_BODY_SENTINEL body line") {
		t.Fatalf("bundle leaked note body: %s", payload)
	}
	if strings.Contains(payload, root) {
		t.Fatalf("bundle leaked absolute vault path: %s", payload)
	}
	assertExploreNoBodyKeys(t, payload)
}

// assertExploreNoBodyKeys 递归扫描 bundle 任意深度，禁止 body 类字段。
func assertExploreNoBodyKeys(t *testing.T, payload string) {
	t.Helper()
	var decoded any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	var walk func(value any, path string)
	walk = func(value any, path string) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				for _, banned := range []string{"body", "note_body", "raw_body", "frontmatter"} {
					if lower == banned || strings.HasSuffix(lower, "_"+banned) {
						t.Fatalf("bundle key %q at %s is body-shaped", key, path)
					}
				}
				walk(child, path+"."+key)
			}
		case []any:
			for _, child := range typed {
				walk(child, path+"[]")
			}
		}
	}
	walk(decoded, "$")
}

func TestExploreBundleTruncationFailSafe(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		writeExploreNote(t, root, "notes/n"+string(rune('a'+i))+".md", strings.Join([]string{
			"schema_version: pinax.note.v1",
			"note_id: note_" + string(rune('a'+i)),
			"title: Note " + string(rune('A'+i)),
			"updated_at: 2026-09-01T00:00:00+00:00",
		}, "\n"), "body links to [[Note B]]")
	}
	limits := defaultExploreBundleLimits()
	limits.maxNodes = 3
	limits.maxEdges = 1
	bundle, err := buildExploreBundle(root, limits, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build explore bundle: %v", err)
	}
	if !bundle.Counts.Truncated {
		t.Fatalf("counts must report truncated: %#v", bundle.Counts)
	}
	if bundle.Counts.Nodes != 3 {
		t.Fatalf("nodes = %d (want capped at 3)", bundle.Counts.Nodes)
	}
	if bundle.Counts.Edges != 1 {
		t.Fatalf("edges = %d (want capped at 1)", bundle.Counts.Edges)
	}
}

func TestExploreNotePreviewBoundedAndRedacted(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("lorem ipsum ", 600)
	note := mustScanSingleExploreNote(t, strings.Join([]string{
		"schema_version: pinax.note.v1",
		"note_id: note_preview",
		"title: Preview",
	}, "\n"), "first line\n\nsecond line token=super-secret "+long)
	preview := exploreNotePreview(note)
	if strings.Contains(preview, "\n") {
		t.Fatalf("preview must collapse whitespace: %q", preview[:80])
	}
	if strings.Contains(preview, "super-secret") {
		t.Fatalf("preview leaked token value: %q", preview[:120])
	}
	if len(preview) > exploreNotePreviewBytes+16 {
		t.Fatalf("preview length %d exceeds bound", len(preview))
	}
	if !strings.HasPrefix(preview, "first line second line") {
		t.Fatalf("preview start = %q", preview[:60])
	}
}

func TestExploreResolveNoteFailClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/a.md", "schema_version: pinax.note.v1\nnote_id: note_a\ntitle: A", "body")
	writeExploreNote(t, root, "notes/b.md", "schema_version: pinax.note.v1\ntitle: B", "body")
	notes, err := scanNotes(root)
	if err != nil {
		t.Fatalf("scan notes: %v", err)
	}
	if _, err := exploreResolveNote(notes, "note_a"); err != nil {
		t.Fatalf("resolve note_a: %v", err)
	}
	for _, unsafe := range []string{"", "note_token", "../etc/passwd", "note_a/extra", "note_missing"} {
		if _, err := exploreResolveNote(notes, unsafe); err == nil {
			t.Fatalf("resolve %q must fail closed", unsafe)
		}
	}
}

func TestExploreNodeIDStableWithoutNoteID(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/b.md", "schema_version: pinax.note.v1\ntitle: B", "body")
	first, err := BuildExploreBundle(root)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	second, err := BuildExploreBundle(root)
	if err != nil {
		t.Fatalf("build bundle again: %v", err)
	}
	if len(first.Nodes) != 1 || first.Nodes[0].ID != second.Nodes[0].ID {
		t.Fatalf("derived node id not stable: %#v vs %#v", first.Nodes, second.Nodes)
	}
	if !strings.HasPrefix(first.Nodes[0].ID, "n") || len(first.Nodes[0].ID) != 17 {
		t.Fatalf("derived id shape unexpected: %q", first.Nodes[0].ID)
	}
}

func TestExploreBundleJSONViaExploreHandler(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/a.md", strings.Join([]string{
		"schema_version: pinax.note.v1",
		"note_id: note_a",
		"title: A",
		"summary: card summary",
		"updated_at: 2026-09-01T00:00:00+00:00",
	}, "\n"), "A body EXPLORE_HANDLER_SENTINEL")
	bundle, err := BuildExploreBundle(root)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	server := httptest.NewServer(shareVaultExploreHandler(root, "", bundle, false, time.Now().UTC()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/explore/data.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("data.json status = %d", resp.StatusCode)
	}
	var decoded ExploreBundle
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode data.json: %v", err)
	}
	if decoded.SchemaVersion != ExploreBundleSchemaVersion || decoded.Counts.Nodes != 1 {
		t.Fatalf("data.json bundle = %#v", decoded)
	}
}

func TestExploreHandlerTokenQueryParamAuthAndBoundedPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/a.md", strings.Join([]string{
		"schema_version: pinax.note.v1",
		"note_id: note_a",
		"title: A",
		"summary: card summary",
		"updated_at: 2026-09-01T00:00:00+00:00",
	}, "\n"), "A preview line token=super-secret-42\n\nsecond line")
	bundle, err := BuildExploreBundle(root)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	server := httptest.NewServer(shareVaultExploreHandler(root, "share-token", bundle, true, time.Now().UTC()))
	defer server.Close()

	// 未认证：401 且不泄漏 bundle。
	unauth, err := http.Get(server.URL + "/explore/data.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(io.LimitReader(unauth.Body, 4096))
	_ = unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized || strings.Contains(string(raw), ExploreBundleSchemaVersion) {
		t.Fatalf("unauthenticated data.json = %d %s", unauth.StatusCode, string(raw))
	}

	// 浏览器导航路径：?token= 查询参数认证（页面 HTML 内嵌 bundle）。
	page, err := http.Get(server.URL + "/explore?token=share-token")
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(io.LimitReader(page.Body, 1<<20))
	_ = page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("token query param page status = %d", page.StatusCode)
	}
	if !strings.Contains(string(pageBody), "note_a") || strings.Contains(string(pageBody), "A preview line token=super-secret-42") {
		t.Fatalf("embedded page must carry bundle metadata without bodies")
	}
	if refs := ScanExploreExternalRefs(pageBody); len(refs) != 0 {
		t.Fatalf("served page leaked external refs: %v", refs)
	}

	// 错误 token：仍 401。
	wrong, err := http.Get(server.URL + "/explore?token=wrong")
	if err != nil {
		t.Fatal(err)
	}
	_ = wrong.Body.Close()
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", wrong.StatusCode)
	}

	// note 预览端点：有界 + 脱敏。
	preview, err := http.Get(server.URL + "/explore/note/note_a?token=share-token")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = preview.Body.Close() }()
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("note preview status = %d", preview.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(preview.Body).Decode(&payload); err != nil {
		t.Fatalf("decode preview payload: %v", err)
	}
	previewText, _ := payload["preview"].(string)
	if !strings.Contains(previewText, "A preview line") {
		t.Fatalf("preview text = %q", previewText)
	}
	if strings.Contains(previewText, "super-secret-42") {
		t.Fatalf("preview leaked secret value: %q", previewText)
	}
	if _, ok := payload["frontmatter"]; ok {
		t.Fatalf("preview must not expose frontmatter map")
	}

	// 未知 id fail-closed 404；不安全 id 404。
	for _, bad := range []string{"note_missing", "note_token", "..%2F..%2Fetc"} {
		resp, err := http.Get(server.URL + "/explore/note/" + bad + "?token=share-token")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("bad id %q status = %d", bad, resp.StatusCode)
		}
	}
}

func mustScanSingleExploreNote(t *testing.T, frontmatter, body string) domain.Note {
	t.Helper()
	root := t.TempDir()
	writeExploreNote(t, root, "notes/n.md", frontmatter, body)
	notes, err := scanNotes(root)
	if err != nil || len(notes) != 1 {
		t.Fatalf("scan notes: %v (%d)", err, len(notes))
	}
	return notes[0]
}
