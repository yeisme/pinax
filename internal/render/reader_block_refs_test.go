package render

import (
	"strings"
	"testing"
)

func TestReaderDocumentBlockReferences(t *testing.T) {
	body := "# Heading\n\nExact **paragraph** ^stable-1\n\nSecond line\ncontinues ^two\n\n`literal ^code`\n\n```text\nfence ^fenced\n```\n\n> quote ^nested\n\n- item ^listed\n\nEscaped \\^escaped\n\nRepeated ^dup\n\nOther ^dup\n\nLink [label ^link](https://example.invalid)\n\nUnicode ^中文\n"
	doc, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"stable-1", "two"} {
		anchor := doc.BlockRefs[id]
		index := LocateReaderAnchor(doc, anchor)
		if anchor == "" || index < 0 || !strings.Contains(doc.Blocks[index].Text, "^"+id) {
			t.Fatalf("missing block %q: %q", id, anchor)
		}
	}
	for _, id := range []string{"code", "fenced", "nested", "listed", "escaped", "dup", "link", "中文"} {
		if doc.BlockRefs[id] != "" {
			t.Fatalf("unsafe marker resolved: %s", id)
		}
	}
	if len(doc.Outline) != 1 {
		t.Fatal("block markers became headings")
	}
}

func TestReaderDocumentBlockReferenceFragmentPagination(t *testing.T) {
	text := strings.Repeat("长", ReaderPageMaxTextBytes/2) + " ^large"
	doc, err := BuildReaderDocument("Before\n\n" + text + "\n\nAfter\n")
	if err != nil {
		t.Fatal(err)
	}
	anchor := doc.BlockRefs["large"]
	index := LocateReaderAnchor(doc, anchor)
	if index != 1 {
		t.Fatalf("target index = %d", index)
	}
	position := ReaderPagePosition{BlockIndex: index}
	var restored strings.Builder
	pages := 0
	for {
		page := PaginateReaderDocument(doc, position)
		pages++
		if pages > 10 || page.ContentBytes > ReaderPageMaxTextBytes || page.NodeCount > ReaderPageMaxNodes {
			t.Fatal("pagination budget failure")
		}
		for _, node := range page.Nodes {
			if node.Anchor == anchor || node.BlockAnchor == anchor {
				restored.WriteString(node.Text)
			}
		}
		if page.Next == nil {
			break
		}
		position = *page.Next
	}
	if pages < 2 || restored.String() != text {
		t.Fatal("fixed block fragments did not reconstruct exact text")
	}
}

func TestReaderDocumentBlockReferenceGrammar(t *testing.T) {
	for _, test := range []struct {
		source, id string
		valid      bool
	}{
		{"Text ^A-09", "A-09", true},
		{"Text ^" + strings.Repeat("x", 64), strings.Repeat("x", 64), true},
		{"Text ^" + strings.Repeat("x", 65), strings.Repeat("x", 65), false},
		{"Text ^a_b", "a_b", false},
		{"Text ^a trailing", "a", false},
		{"Text^joined", "joined", false},
		{"Text \\x5c^escaped", "escaped", false},
		{"Text `^code`", "code", false},
		{"Text **^strong**", "strong", false},
		{"<div>Text ^html</div>", "html", false},
		{"# Heading ^heading", "heading", false},
		{"Text ^terminal  ", "terminal", true},
	} {
		t.Run(test.source, func(t *testing.T) {
			doc, err := BuildReaderDocument(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if (doc.BlockRefs[test.id] != "") != test.valid {
				t.Fatalf("unexpected block resolution: %#v", doc.BlockRefs)
			}
		})
	}
}
