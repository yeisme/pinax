package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/render"
)

func TestReaderDocumentDailyFixedPagesAndIsolation(t *testing.T) {
	root := t.TempDir()
	svc := NewService()
	content := navigationNote(t, root, "daily/2026-08-20.md", "daily-body", "Day", "daily", "daily", strings.Repeat("Bounded journal paragraph.\n\n", 15000))
	revision := collaborationRevision(content)
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)
	doc := ReaderDocumentRequest{VaultPath: root, Session: session, ObjectID: "daily-body", Revision: revision, CursorSecret: readerCursorSecret()}
	req := ReaderDailyDocumentRequest{Document: doc, Date: "2026-08-20", Timezone: "Asia/Shanghai"}
	if _, err := svc.ReaderDocument(context.Background(), doc); err == nil {
		t.Fatal("ordinary document admitted a system journal")
	}
	projection, err := svc.ReaderDailyDocument(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if page.ObjectID != doc.ObjectID || page.Revision != revision || page.NextCursor == "" || page.Complete || page.ContentBytes > render.ReaderPageMaxTextBytes {
		t.Fatal("daily fixed-page contract drift")
	}
	req.Document.Cursor = page.NextCursor
	next, err := svc.ReaderDailyDocument(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second := next.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if second.Revision != revision || len(second.Nodes) == 0 || second.Nodes[0].Anchor == page.Nodes[0].Anchor {
		t.Fatal("daily continuation did not advance")
	}
	if _, err = os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
		t.Fatal("reading daily created state")
	}
	for _, kind := range []string{"metadata", "body", "date", "id", "revision", "session"} {
		t.Run(kind, func(t *testing.T) {
			bad := req
			switch kind {
			case "metadata":
				bad.Document.Session.Scopes = []string{ReaderScopeMetadata}
			case "body":
				bad.Document.Session.Scopes = []string{ReaderScopeBody}
			case "date":
				bad.Date = "2026-08-21"
			case "id":
				bad.Document.ObjectID = "other"
			case "revision":
				bad.Document.Revision = "sha256:" + strings.Repeat("0", 64)
			case "session":
				bad.Document.Session.SessionID = "another-session"
			}
			if _, err := svc.ReaderDailyDocument(context.Background(), bad); err == nil {
				t.Fatal("daily boundary bypass", kind)
			}
		})
	}
	if err = os.WriteFile(filepath.Join(root, "daily/2026-08-20.md"), []byte(content+"\nExternal change.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReaderDailyDocument(context.Background(), req)
	requireEditorSourceError(t, err, "revision_unavailable")
}

func TestReaderDocumentDailyLegacyAndLiteralContent(t *testing.T) {
	root := t.TempDir()
	content := navigationNote(t, root, "notes/daily/2026-08-20.md", "legacy-day", "Legacy", "daily", "daily", "# Legacy heading\n\n<script>untrusted()</script>\n\n```shell\nnever-run\n```\n")
	req := ReaderDailyDocumentRequest{Document: ReaderDocumentRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeBody), ObjectID: "legacy-day", Revision: collaborationRevision(content), Anchor: "legacy-heading", CursorSecret: readerCursorSecret()}, Date: "2026-08-20", Timezone: "UTC"}
	projection, err := NewService().ReaderDailyDocument(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if len(page.Nodes) < 3 || page.Nodes[0].Anchor != "legacy-heading" {
		t.Fatal("legacy daily anchor missing")
	}
	code, html := false, false
	for _, node := range page.Nodes {
		code = code || (node.Type == "code" && strings.Contains(node.Text, "never-run"))
		html = html || (node.Type == "unsupported" && strings.Contains(node.Text, "untrusted"))
	}
	if !code || !html {
		t.Fatal("unsafe content was not inert")
	}
}
