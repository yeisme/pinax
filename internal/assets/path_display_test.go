package assets

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetPathDisplayStyles(t *testing.T) {
	root := t.TempDir()
	assetPath := "attachments/note_alpha/diagram.png"
	notePath := "notes/projects/alpha.md"

	vaultRel, err := DisplayPath(PathDisplayRequest{Root: root, AssetPath: assetPath, Style: PathStyleVaultRelative})
	if err != nil || vaultRel != assetPath {
		t.Fatalf("vault-relative display = %q err=%v", vaultRel, err)
	}
	noteRel, err := DisplayPath(PathDisplayRequest{Root: root, AssetPath: assetPath, ContextNotePath: notePath, Style: PathStyleNoteRelative})
	if err != nil || noteRel != "../../attachments/note_alpha/diagram.png" {
		t.Fatalf("note-relative display = %q err=%v", noteRel, err)
	}
	markdown, err := DisplayPath(PathDisplayRequest{Root: root, AssetPath: assetPath, ContextNotePath: notePath, MediaType: "image/png", Style: PathStyleMarkdown})
	if err != nil || markdown != "![diagram.png](../../attachments/note_alpha/diagram.png)" {
		t.Fatalf("markdown display = %q err=%v", markdown, err)
	}
	wiki, err := DisplayPath(PathDisplayRequest{Root: root, AssetPath: assetPath, Style: PathStyleWiki})
	if err != nil || wiki != "![[attachments/note_alpha/diagram.png]]" {
		t.Fatalf("wiki display = %q err=%v", wiki, err)
	}
	absolute, err := DisplayPath(PathDisplayRequest{Root: root, AssetPath: assetPath, Style: PathStyleAbsolute})
	if err != nil || !strings.HasPrefix(absolute, filepath.Clean(root)) || !strings.HasSuffix(filepath.ToSlash(absolute), assetPath) {
		t.Fatalf("absolute display = %q err=%v", absolute, err)
	}
	if _, err := DisplayPath(PathDisplayRequest{Root: root, AssetPath: assetPath, Style: PathStyleNoteRelative}); err == nil || !strings.Contains(err.Error(), "path_context_required") {
		t.Fatalf("note-relative without context err = %v", err)
	}
}
