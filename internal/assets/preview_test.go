package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderedNoteEmbedsMarkdownAttachmentAndStopsCycle(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "spec.md"), []byte("# Spec\n\n![[root.md]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# Root\n\n![[spec.md]]"
	result, err := RenderEmbeddedPreview(RenderPreviewRequest{Root: root, SourcePath: "notes/root.md", Body: body, Mode: "markdown", MaxDepth: 2, MaxBytes: 4096})
	if err != nil {
		t.Fatalf("render preview: %v", err)
	}
	if !strings.Contains(result.Body, "# Root") || !strings.Contains(result.Body, "# Spec") || !strings.Contains(result.Body, "attachment_embed_cycle") {
		t.Fatalf("preview body = %s", result.Body)
	}
	if len(result.EmbeddedAssets) < 2 || result.EmbeddedAssets[len(result.EmbeddedAssets)-1].Path != "notes/spec.md" {
		t.Fatalf("embedded assets = %#v", result.EmbeddedAssets)
	}
}

func TestRenderedNoteEmbedsTextAttachmentWithBoundsAndImagePlaceholder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "transcript.txt"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# Root\n\n![[transcript.txt]]\n![[diagram.png]]"
	result, err := RenderEmbeddedPreview(RenderPreviewRequest{Root: root, SourcePath: "notes/root.md", Body: body, Mode: "text", MaxDepth: 1, MaxBytes: 3})
	if err != nil {
		t.Fatalf("render preview: %v", err)
	}
	if !strings.Contains(result.Body, "abc") || strings.Contains(result.Body, "def") || !strings.Contains(result.Body, "pinax asset show diagram.png") {
		t.Fatalf("preview body = %s", result.Body)
	}
	if len(result.EmbeddedAssets) != 2 || !result.EmbeddedAssets[0].Truncated || result.EmbeddedAssets[1].Status != "placeholder" {
		t.Fatalf("embedded assets = %#v", result.EmbeddedAssets)
	}
}
