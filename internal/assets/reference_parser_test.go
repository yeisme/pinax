package assets

import "testing"

func TestExtractAssetLinksParsesMarkdownAndWikiReferences(t *testing.T) {
	body := "intro\n" +
		"![Diagram](../assets/diagram.png)\n" +
		"[Spec](attachments/spec.pdf)\n" +
		"![[media/demo.mp4|demo]]\n" +
		"[[assets/My%20File.pdf]]\n" +
		"[Plan](project-plan.md)\n" +
		"[[Project Plan]]\n" +
		"[External](https://example.com/a.png)\n" +
		"![Secret](../../secret.png)\n"
	original := body

	links := ExtractLinks(LinkExtractionRequest{SourcePath: "notes/note.md", Body: body})

	if body != original {
		t.Fatalf("ExtractLinks rewrote note body")
	}
	if len(links) != 4 {
		t.Fatalf("expected 4 asset links, got %d: %#v", len(links), links)
	}
	assertAssetLink(t, links[0], "assets/diagram.png", "notes/note.md", "![Diagram](../assets/diagram.png)", "markdown", "embed", 2, "unresolved")
	assertAssetLink(t, links[1], "attachments/spec.pdf", "notes/note.md", "[Spec](attachments/spec.pdf)", "markdown", "link", 3, "unresolved")
	assertAssetLink(t, links[2], "notes/media/demo.mp4", "notes/note.md", "![[media/demo.mp4|demo]]", "wiki", "embed", 4, "unresolved")
	assertAssetLink(t, links[3], "assets/My File.pdf", "notes/note.md", "[[assets/My%20File.pdf]]", "wiki", "link", 5, "unresolved")
}

func TestExtractAssetLinksRejectsParentTraversalFromNestedNotes(t *testing.T) {
	body := "![Secret](../../secret.png)\n![OK](../assets/ok.gif)\n"

	links := ExtractLinks(LinkExtractionRequest{SourcePath: "notes/project/note.md", Body: body})

	if len(links) != 1 {
		t.Fatalf("expected only the single-level parent asset link, got %d: %#v", len(links), links)
	}
	assertAssetLink(t, links[0], "notes/assets/ok.gif", "notes/project/note.md", "![OK](../assets/ok.gif)", "markdown", "embed", 2, "unresolved")
}

func TestExtractAssetLinksIgnoresExternalUnsafeAndNoteReferences(t *testing.T) {
	body := "[HTTP](https://example.com/a.png)\n" +
		"[Mail](mailto:user@example.com)\n" +
		"![Data](data:image/png;base64,abcd)\n" +
		"[Heading](#section)\n" +
		"[Note](../other.md)\n" +
		"[[Project Plan]]\n" +
		"[[notes/other.md]]\n" +
		"![Secret](../../secret.png)\n" +
		"![OK](assets/ok.gif)\n"

	links := ExtractLinks(LinkExtractionRequest{SourcePath: "notes/note.md", Body: body})

	if len(links) != 1 {
		t.Fatalf("expected 1 safe asset link, got %d: %#v", len(links), links)
	}
	assertAssetLink(t, links[0], "assets/ok.gif", "notes/note.md", "![OK](assets/ok.gif)", "markdown", "embed", 9, "unresolved")
}

func assertAssetLink(t *testing.T, link AssetLink, assetPath, sourcePath, raw, style, kind string, line int, status string) {
	t.Helper()
	if link.AssetPath != assetPath || link.SourcePath != sourcePath || link.RawReference != raw || link.LinkStyle != style || link.LinkKind != kind || link.Line != line || link.Status != status {
		t.Fatalf("unexpected asset link:\n got: %#v\nwant: asset_path=%q source_path=%q raw=%q style=%q kind=%q line=%d status=%q", link, assetPath, sourcePath, raw, style, kind, line, status)
	}
}
