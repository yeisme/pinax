package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	noterender "github.com/yeisme/pinax/internal/render"
	"github.com/yeisme/pinax/internal/sqlitedsn"
)

func renderVaultFingerprint(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRenderedNoteFencesReferencesBadgeAndReadOnly(t *testing.T) {
	s, root, _ := noteProposalFixture(t)
	ctx := context.Background()
	if _, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Target", Body: "Linked note", Dir: "index"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDatabaseView(ctx, ViewRequest{VaultPath: root, Name: "active", Query: "SELECT title FROM notes", Language: "sql", Display: "table"}); err != nil {
		t.Fatal(err)
	}
	body := "# Main\n\nSome text [[Target]] [[Missing]] [External](drivebridge://file/opaque-id) [Unknown](custom://literal)\n\n```pinax-database-view active\n```\n\n```pinax-graph hops=2\n```\n\n```pinax-board missing\n```\n\n```pinax-timeline\n```\n"
	created, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Main", Body: body, Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Facts["note_id"]
	sqlitedsn.CloseAll()
	before := renderVaultFingerprint(t, root)
	p, err := s.RenderNote(ctx, RenderNoteRequest{VaultPath: root, NoteRef: id, Display: "body"})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.Data.(noterender.View)
	if !ok || v.Badge.ObjectID != id || v.Badge.Revision == "" || v.Badge.Version == 0 {
		t.Fatal("proof facts absent")
	}
	b, _ := json.Marshal(v)
	for _, want := range []string{"external_ref", "custom://literal", "broken", "pinax-database-view", "hops", "fence_failed", "board_not_found", "pinax-timeline"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %s", want)
		}
	}
	for _, node := range v.Tree {
		if node.Fence == "pinax-database-view" && (node.Type != "fence" || node.Payload == nil) {
			t.Fatal("valid database view was incorrectly reported as a failure")
		}
	}
	if strings.Contains(string(b), root) || strings.Contains(string(b), "notes/index/") {
		t.Fatal("projection leaked filesystem paths")
	}
	sqlitedsn.CloseAll()
	if !reflect.DeepEqual(before, renderVaultFingerprint(t, root)) {
		t.Fatal("render mutated the vault")
	}
}

func TestRenderedPinaxRefFloatingStaleExactHistoricalAndUnresolved(t *testing.T) {
	s, root, _ := noteProposalFixture(t)
	ctx := context.Background()
	created, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "History", Body: "old revision body", Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Facts["note_id"]
	p, err := s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: id, Floating: true})
	if err != nil {
		t.Fatal(err)
	}
	original := p.Data.(NoteRefResult)
	if _, err := s.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "Reference history"}); err != nil {
		t.Fatal(err)
	}
	note, _, _ := s.CollaborationRead(ctx, root, id, "read", CollaborationPolicy{AllowBody: true})
	path := filepath.Join(root, note.Path)
	content, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(content), "old revision body", "new revision body")), 0o644); err != nil {
		t.Fatal(err)
	}
	sqlitedsn.CloseAll()
	before := renderVaultFingerprint(t, root)
	p, err = s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: original.Ref.URI, ResolvedRevision: original.ResolvedRevision})
	if err != nil {
		t.Fatal(err)
	}
	r := p.Data.(NoteRefResult)
	if !r.Stale || r.ResolvedRevision != original.ResolvedRevision || r.CurrentRevision == r.ResolvedRevision {
		t.Fatal("floating revision silently advanced")
	}
	p, err = s.RenderNote(ctx, RenderNoteRequest{VaultPath: root, NoteRef: noterender.NoteURI(id, original.ResolvedRevision), Display: "body"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p.Data)
	if !strings.Contains(string(b), "old revision body") || strings.Contains(string(b), "new revision body") {
		t.Fatal("fixed reference read a different version")
	}
	unavailable := "sha256:" + strings.Repeat("0", 64)
	p, err = s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: noterender.NoteURI(id, unavailable)})
	if err != nil || p.Facts["reference_status"] != "unresolved" {
		t.Fatalf("missing exact revision: %v", err)
	}
	sqlitedsn.CloseAll()
	if !reflect.DeepEqual(before, renderVaultFingerprint(t, root)) {
		t.Fatal("reference resolution mutated the vault")
	}
	for _, bad := range []string{"pinax://note/../outside", "pinax://note/token-id", "pinax://note/abc?v=wrong", "pinax://note/abc?x=y"} {
		if _, err := s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: bad}); err == nil {
			t.Fatal("unsafe reference accepted")
		}
	}
}

func TestRenderedNoteRefDisambiguationAndBodyDisplayBoundary(t *testing.T) {
	s, root, _ := noteProposalFixture(t)
	ctx := context.Background()
	for _, dir := range []string{"index", "other"} {
		if _, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Same", Body: strings.Repeat("paragraph ", 100), Dir: dir}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: "Same"})
	if err != nil || p.Facts["reference_status"] != "ambiguous" {
		t.Fatalf("ambiguity: %v", err)
	}
	p, err = s.NoteReference(ctx, NoteRefRequest{VaultPath: root, NoteRef: "Same", Pick: 1})
	if err != nil || p.Facts["reference_status"] != "resolved" {
		t.Fatalf("pick: %v", err)
	}
	id := p.Data.(NoteRefResult).Ref.ObjectID
	p, err = s.RenderNote(ctx, RenderNoteRequest{VaultPath: root, NoteRef: id})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p.Data)
	if strings.Contains(string(b), "paragraph paragraph") || p.Facts["display"] != "card" {
		t.Fatal("default includes the body")
	}
	if _, err := s.RenderNote(ctx, RenderNoteRequest{VaultPath: root, NoteRef: id, Display: "invalid"}); domain.ErrorCode(err) != "display_invalid" {
		t.Fatalf("display: %v", err)
	}
}

func TestRenderedAssetMetadataBoardAndGraphBounds(t *testing.T) {
	s, root, _ := noteProposalFixture(t)
	ctx := context.Background()
	if _, err := s.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "Research"}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(file, []byte("asset bytes must not be fetched"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset, err := s.AssetAdd(ctx, AssetRequest{VaultPath: root, Source: file})
	if err != nil {
		t.Fatal(err)
	}
	assetPath := asset.Facts["asset_path"]
	if assetPath == "" {
		assetPath = "assets/sample.txt"
	}
	if err := os.Remove(filepath.Join(root, assetPath)); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Hub", Dir: "index", Body: "# Hub\n\n[Asset](../../assets/sample.txt)\n\n```pinax-board research\n```\n\n```pinax-graph hops=2\n```\n"})
	if err != nil {
		t.Fatal(err)
	}
	id := p.Facts["note_id"]
	// Construct owner-parsed fixtures without indexing every incremental note.
	for i := 0; i < 55; i++ {
		rel := fmt.Sprintf("notes/index/link-%02d.md", i)
		content := buildNoteContentWithObjectID(fmt.Sprintf("01982d84-2b48-7000-8000-%012x", i+1), fmt.Sprintf("Link %d", i), "", "", "", nil, "active", "2026-01-01T00:00:00Z", "[[Hub]]")
		writeAppFixture(t, filepath.Join(root, rel), content)
	}
	sqlitedsn.CloseAll()
	before := renderVaultFingerprint(t, root)
	p, err = s.RenderNote(ctx, RenderNoteRequest{VaultPath: root, NoteRef: id, Display: "body"})
	if err != nil {
		t.Fatal(err)
	}
	v := p.Data.(noterender.View)
	b, _ := json.Marshal(v)
	if !strings.Contains(string(b), "pinax-asset://") || !strings.Contains(string(b), "text/plain") || strings.Contains(string(b), "board_not_found") || !v.Truncated || len(v.Panels["backlinks"].Entities) != 50 {
		t.Fatal("asset, board or graph bounds incorrect")
	}
	after := renderVaultFingerprint(t, root)
	if !reflect.DeepEqual(before, after) {
		for file, hash := range after {
			if before[file] != hash {
				t.Log("changed fixture file:", file)
			}
		}
		t.Fatal("asset or projection render performed IO writes")
	}
}
