package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestAssetProjectionListAndFind(t *testing.T) {
	root := t.TempDir()
	asset := domain.Asset{ID: "asset_index", Path: "assets/from-index.png", Filename: "from-index.png", Stem: "from-index", Extension: "png", MediaType: "image/png", Size: 12, SHA256: "abc123", ManagedStatus: domain.ManagedStatusManaged, Width: 3, Height: 2}
	if err := ReplaceAssetProjection(root, []domain.Asset{asset}); err != nil {
		t.Fatalf("replace asset projection: %v", err)
	}
	assets, status, err := ListAssets(root)
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if status.Status != "fresh" || len(assets) != 1 || assets[0].Path != asset.Path || assets[0].Width != 3 || assets[0].Height != 2 {
		t.Fatalf("assets=%#v status=%#v", assets, status)
	}
	found, status, err := FindAsset(root, "from-index")
	if err != nil {
		t.Fatalf("find asset: %v", err)
	}
	if status.Status != "fresh" || found.ID != asset.ID {
		t.Fatalf("found=%#v status=%#v", found, status)
	}
}

func TestVaultObjectLookupSchemaHasResolverCandidateFields(t *testing.T) {
	root := t.TempDir()
	note := domain.Note{ID: "note_alpha", Title: "Alpha", Path: "notes/projects/alpha-note.md", Body: "# Alpha"}
	if _, err := Rebuild(root, []domain.Note{note}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	for _, column := range []string{"filename", "stem", "object_kind", "managed_status"} {
		if !db.Migrator().HasColumn(&NoteRecord{}, column) {
			t.Fatalf("note_records missing resolver candidate column %s", column)
		}
	}
	var record NoteRecord
	if err := db.First(&record, &NoteRecord{Path: note.Path}).Error; err != nil {
		t.Fatalf("load note record: %v", err)
	}
	if record.Filename != "alpha-note.md" || record.Stem != "alpha-note" || record.ObjectKind != string(domain.VaultObjectKindNote) || record.ManagedStatus != string(domain.ManagedStatusRegistered) {
		t.Fatalf("note resolver fields = %#v", record)
	}
}

func TestAssetLinksAndVaultFilesProjectionFromRebuild(t *testing.T) {
	root := t.TempDir()
	writeIndexFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeIndexFixture(t, filepath.Join(root, "attachments", "spec.pdf"), "pdf")
	writeIndexFixture(t, filepath.Join(root, "notes", "media", "demo.mp4"), "mp4")
	writeIndexFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha")
	note := domain.Note{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Body: "![Diagram](../assets/diagram.png)\n[Spec](attachments/spec.pdf)\n![[media/demo.mp4|demo]]\n[Plan](project-plan.md)\n"}

	if _, err := Rebuild(root, []domain.Note{note}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	links, status, err := ListAssetLinks(root)
	if err != nil {
		t.Fatalf("list asset links: %v", err)
	}
	if status.Status != "fresh" || len(links) != 3 {
		t.Fatalf("links=%#v status=%#v", links, status)
	}
	assertAssetLinkRecord(t, links[0], "assets/diagram.png", "notes/alpha.md", "![Diagram](../assets/diagram.png)", "markdown", "embed", 1, "resolved")
	assertAssetLinkRecord(t, links[1], "attachments/spec.pdf", "notes/alpha.md", "[Spec](attachments/spec.pdf)", "markdown", "link", 2, "resolved")
	assertAssetLinkRecord(t, links[2], "notes/media/demo.mp4", "notes/alpha.md", "![[media/demo.mp4|demo]]", "wiki", "embed", 3, "resolved")

	files, status, err := ListVaultFiles(root)
	if err != nil {
		t.Fatalf("list vault files: %v", err)
	}
	if status.Status != "fresh" || !hasVaultFile(files, "assets/diagram.png") || !hasVaultFile(files, "attachments/spec.pdf") || !hasVaultFile(files, "notes/alpha.md") {
		t.Fatalf("files=%#v status=%#v", files, status)
	}
	assets, status, err := ListAssets(root)
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if status.Status != "fresh" || !hasAssetPath(assets, "assets/diagram.png") || !hasAssetPath(assets, "attachments/spec.pdf") {
		t.Fatalf("assets=%#v status=%#v", assets, status)
	}
}

func TestRebuildAndRefreshProjectRegisteredUnmanagedAndAssets(t *testing.T) {
	root := t.TempDir()
	writeIndexFixture(t, filepath.Join(root, "notes", "registered.md"), "# Registered")
	writeIndexFixture(t, filepath.Join(root, "notes", "unmanaged.md"), "# Unmanaged")
	writeIndexFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	notes := []domain.Note{{ID: "note_registered", Title: "Registered", Path: "notes/registered.md", Body: "![Diagram](../assets/diagram.png)\n"}}

	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	files, status, err := ListVaultFiles(root)
	if err != nil {
		t.Fatalf("list vault files after rebuild: %v", err)
	}
	if status.Status != "fresh" {

		t.Fatalf("vault file status after rebuild = %#v", status)
	}
	registered := findVaultFile(files, "notes/registered.md")
	if registered == nil || registered.ObjectKind != string(domain.VaultObjectKindNote) || registered.ManagedStatus != string(domain.ManagedStatusRegistered) {
		t.Fatalf("registered note vault file = %#v", registered)
	}
	unmanaged := findVaultFile(files, "notes/unmanaged.md")
	if unmanaged == nil || unmanaged.ObjectKind != string(domain.VaultObjectKindNote) || unmanaged.ManagedStatus != string(domain.ManagedStatusUnmanaged) {
		t.Fatalf("unmanaged markdown vault file = %#v", unmanaged)
	}
	assetFile := findVaultFile(files, "assets/diagram.png")
	if assetFile == nil || assetFile.ObjectKind != string(domain.VaultObjectKindAsset) {
		t.Fatalf("asset vault file = %#v", assetFile)
	}

	writeIndexFixture(t, filepath.Join(root, "attachments", "fresh.pdf"), "pdf")
	if _, err := Refresh(root, notes, RefreshOptions{}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	files, status, err = ListVaultFiles(root)
	if err != nil {
		t.Fatalf("list vault files after refresh: %v", err)
	}
	if status.Status != "fresh" || findVaultFile(files, "attachments/fresh.pdf") == nil {
		t.Fatalf("refresh vault files=%#v status=%#v", files, status)
	}
	assets, status, err := ListAssets(root)
	if err != nil {
		t.Fatalf("list assets after refresh: %v", err)
	}
	if status.Status != "fresh" || !hasAssetPath(assets, "attachments/fresh.pdf") {
		t.Fatalf("refresh assets=%#v status=%#v", assets, status)
	}
}

func assertAssetLinkRecord(t *testing.T, link AssetLinkRecord, assetPath, sourcePath, raw, style, kind string, line int, status string) {
	t.Helper()
	if link.AssetPath != assetPath || link.SourcePath != sourcePath || link.RawReference != raw || link.LinkStyle != style || link.LinkKind != kind || link.Line != line || link.Status != status {
		t.Fatalf("unexpected asset link: %#v", link)
	}
}

func hasVaultFile(files []VaultFileRecord, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func findVaultFile(files []VaultFileRecord, path string) *VaultFileRecord {
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
}

func hasAssetPath(assets []domain.Asset, path string) bool {
	for _, asset := range assets {
		if asset.Path == path {
			return true
		}
	}
	return false
}

func writeIndexFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
