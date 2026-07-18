package publishdocast

import (
	"testing"
)

func TestPublishDocASTHeadings(t *testing.T) {
	doc := Parse("Title", "# H1\n\n## H2\n\n### H3\n\ntext")
	if len(doc.Blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d: %+v", len(doc.Blocks), doc.Blocks)
	}
	if doc.Blocks[0].Type != BlockHeading || doc.Blocks[0].Level != 1 || doc.Blocks[0].Text != "H1" {
		t.Fatalf("block 0 = %+v", doc.Blocks[0])
	}
	if doc.Blocks[1].Level != 2 || doc.Blocks[1].Text != "H2" {
		t.Fatalf("block 1 = %+v", doc.Blocks[1])
	}
	if doc.Blocks[2].Level != 3 || doc.Blocks[2].Text != "H3" {
		t.Fatalf("block 2 = %+v", doc.Blocks[2])
	}
}

func TestPublishDocASTParagraphInlineMarks(t *testing.T) {
	doc := Parse("T", "This is **bold** and *italic* and `code` and [link](https://example.com).")
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockParagraph {
		t.Fatalf("expected 1 paragraph, got %+v", doc.Blocks)
	}
	text := doc.Blocks[0].Text
	for _, want := range []string{"**bold**", "*italic*", "`code`", "[link](https://example.com)"} {
		if !containsPas(text, want) {
			t.Fatalf("paragraph text missing %q: %q", want, text)
		}
	}
}

func TestPublishDocASTCodeBlock(t *testing.T) {
	body := "```go\nfunc main() {}\n```\n"
	doc := Parse("T", body)
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockCode {
		t.Fatalf("expected code block, got %+v", doc.Blocks)
	}
	if doc.Blocks[0].Language != "go" {
		t.Fatalf("language = %q", doc.Blocks[0].Language)
	}
	if doc.Blocks[0].Code != "func main() {}" {
		t.Fatalf("code = %q", doc.Blocks[0].Code)
	}
}

func TestPublishDocASTMermaidBlock(t *testing.T) {
	body := "```mermaid\ngraph TD\n  A-->B\n```\n"
	doc := Parse("T", body)
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockMermaid {
		t.Fatalf("expected mermaid block, got %+v", doc.Blocks)
	}
	if doc.Blocks[0].Code == "" {
		t.Fatalf("mermaid source empty: %+v", doc.Blocks[0])
	}
}

func TestPublishDocASTList(t *testing.T) {
	body := "- alpha\n- beta\n- gamma\n"
	doc := Parse("T", body)
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockList {
		t.Fatalf("expected list, got %+v", doc.Blocks)
	}
	if doc.Blocks[0].Ordered {
		t.Fatalf("expected unordered list")
	}
	if len(doc.Blocks[0].Items) != 3 || doc.Blocks[0].Items[0].Text != "alpha" {
		t.Fatalf("items = %+v", doc.Blocks[0].Items)
	}
}

func TestPublishDocASTOrderedList(t *testing.T) {
	doc := Parse("T", "1. first\n2. second\n")
	if len(doc.Blocks) != 1 || !doc.Blocks[0].Ordered {
		t.Fatalf("expected ordered list, got %+v", doc.Blocks)
	}
}

func TestPublishDocASTBlockquote(t *testing.T) {
	doc := Parse("T", "> wisdom here\n")
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockBlockquote {
		t.Fatalf("expected blockquote, got %+v", doc.Blocks)
	}
	if !containsPas(doc.Blocks[0].Text, "wisdom") {
		t.Fatalf("blockquote text = %q", doc.Blocks[0].Text)
	}
}

func TestPublishDocASTTable(t *testing.T) {
	body := "| Name | Value |\n| --- | --- |\n| a | 1 |\n| b | 2 |\n"
	doc := Parse("T", body)
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockTable {
		t.Fatalf("expected table, got %+v", doc.Blocks)
	}
	tbl := doc.Blocks[0]
	if len(tbl.Headers) != 2 || tbl.Headers[0] != "Name" {
		t.Fatalf("headers = %+v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 || tbl.Rows[0][0] != "a" || tbl.Rows[1][1] != "2" {
		t.Fatalf("rows = %+v", tbl.Rows)
	}
}

func TestPublishDocASTLocalImage(t *testing.T) {
	doc := Parse("T", "![diagram](assets/diagram.png)\n")
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockImage {
		t.Fatalf("expected image block, got %+v", doc.Blocks)
	}
	if doc.Blocks[0].ImageSrc != "assets/diagram.png" || doc.Blocks[0].IsRemoteImage {
		t.Fatalf("image = %+v", doc.Blocks[0])
	}
	if doc.Blocks[0].Alt != "diagram" {
		t.Fatalf("alt = %q", doc.Blocks[0].Alt)
	}
}

func TestPublishDocASTRemoteImage(t *testing.T) {
	doc := Parse("T", "![logo](https://example.com/logo.png)\n")
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockImage || !doc.Blocks[0].IsRemoteImage {
		t.Fatalf("expected remote image, got %+v", doc.Blocks)
	}
}

func TestPublishDocASTInlineSVG(t *testing.T) {
	body := "<svg><rect/></svg>\n"
	doc := Parse("T", body)
	if len(doc.Blocks) != 1 || doc.Blocks[0].Type != BlockHTML {
		t.Fatalf("expected HTML block for SVG, got %+v", doc.Blocks)
	}
	if !containsPas(doc.Blocks[0].RawHTML, "<svg>") {
		t.Fatalf("raw html = %q", doc.Blocks[0].RawHTML)
	}
}

func TestPublishDocASTThematicBreak(t *testing.T) {
	doc := Parse("T", "above\n\n---\n\nbelow\n")
	found := false
	for _, b := range doc.Blocks {
		if b.Type == BlockThematicBreak {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected thematic break in blocks: %+v", doc.Blocks)
	}
}

func TestPublishDocASTEmptyBody(t *testing.T) {
	doc := Parse("Title", "")
	if len(doc.Blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty body, got %d", len(doc.Blocks))
	}
}

func TestMarkdownPublishASTSourceLine(t *testing.T) {
	body := "intro\n\n## Heading\n\ntext"
	doc := Parse("T", body)
	for _, b := range doc.Blocks {
		if b.Type == BlockHeading && b.Text == "Heading" {
			if b.SourceLine < 1 {
				t.Fatalf("heading source line should be >= 1, got %d", b.SourceLine)
			}
			return
		}
	}
	t.Fatalf("heading block not found")
}

func containsPas(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && indexOfPas(s, substr) >= 0)
}

func indexOfPas(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
