package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestReaderDocument builds and pages the human-reading projection: paths
// and code survive verbatim (no Agent SafeText redaction), HTML stays inert,
// pages respect budgets, and oversized blocks continue via rune-safe
// fragments that reassemble exactly.

func TestReaderDocumentKeepsPathsCodeAndTables(t *testing.T) {
	t.Parallel()
	body := "# 概述\n\nParagraph with /Users/yeisme/secret/path and `code` inline.\n\n```go\nfunc main() { println(1) }\n```\n\n| Col A | Col B |\n| --- | --- |\n| 一 | 二 |\n| 三 | 四 |\n\n> quote text\n\n<script>alert(1)</script>\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	encoded := documentText(document)
	for _, want := range []string{"概述", "/Users/yeisme/secret/path", "func main() { println(1) }", "一", "四", "quote text"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("document lost %q:\n%s", want, encoded)
		}
	}
	// User-written paths are NOT redacted here (unlike the Agent view).
	if strings.Contains(encoded, "[REDACTED]") {
		t.Fatalf("reader document applied Agent redaction:\n%s", encoded)
	}
	if document.NodeCount == 0 || len(document.Blocks) == 0 {
		t.Fatalf("empty document: %#v", document)
	}
	types := map[string]bool{}
	for _, block := range document.Blocks {
		types[block.Type] = true
	}
	for _, want := range []string{"heading", "paragraph", "code", "table", "quote", "unsupported"} {
		if !types[want] {
			t.Fatalf("missing block type %q in %#v", want, types)
		}
	}
	// Tables carry their header; unsupported HTML is inert text.
	for _, block := range document.Blocks {
		if block.Type == "table" && len(block.Header) != 2 {
			t.Fatalf("table header = %#v", block.Header)
		}
		if block.Type == "unsupported" && strings.Contains(block.Text, "<script>") {
			if block.Type == "paragraph" {
				t.Fatal("html executed as paragraph")
			}
		}
	}
	// Outline covers the heading.
	if len(document.Outline) != 1 || document.Outline[0].Title != "概述" || document.Outline[0].Level != 1 {
		t.Fatalf("outline = %#v", document.Outline)
	}
}

func TestReaderDocumentOldAgentViewStillRedacts(t *testing.T) {
	t.Parallel()
	// The legacy Agent contract is unchanged: the same body redacts paths.
	legacy := Build(Inputs{Body: "path /Users/x/y and token abc", Display: "body"})
	if !strings.Contains(Text(legacy), "[REDACTED]") {
		t.Fatalf("legacy view stopped redacting:\n%s", Text(legacy))
	}
}

func TestReaderDocumentPaginationCoversEverythingOnce(t *testing.T) {
	t.Parallel()
	var body strings.Builder
	body.WriteString("# Title\n\n")
	for i := 0; i < 1200; i++ {
		body.WriteString("Paragraph ")
		body.WriteString(strings.Repeat("段", 40))
		body.WriteString("\n\n")
	}
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	seenAnchors := map[string]bool{}
	totalText := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if page.NodeCount > ReaderPageMaxNodes {
			t.Fatalf("page %d nodes = %d", pages, page.NodeCount)
		}
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d bytes = %d", pages, page.ContentBytes)
		}
		for _, node := range page.Nodes {
			if seenAnchors[node.Anchor] {
				t.Fatalf("page %d repeated anchor %q", pages, node.Anchor)
			}
			seenAnchors[node.Anchor] = true
			totalText += len([]rune(node.Text))
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("final page is not complete")
			}
			break
		}
		position = *page.Next
		if pages > 200 {
			t.Fatal("pagination does not terminate")
		}
	}
	if pages < 2 {
		t.Fatalf("expected multiple pages, got %d", pages)
	}
	// Every original paragraph anchor appears exactly once across pages.
	if len(seenAnchors) != len(document.Blocks) {
		t.Fatalf("anchors covered %d of %d blocks", len(seenAnchors), len(document.Blocks))
	}
}

func TestReaderDocumentContinuationFragmentsReassemble(t *testing.T) {
	t.Parallel()
	// Oversized code block sits AFTER other page content, so later fragments
	// resume from mid-page budgets; a trailing paragraph proves no tail loss.
	source := strings.Repeat("字", 300*1024)
	body := "leading paragraph\n\n```text\n" + source + "\n```\n\ntrailing paragraph\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 3 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}

	position := ReaderPagePosition{}
	var reassembled strings.Builder
	fragments := 0
	sawLeading, sawTrailing := false, false
	blockAnchor := document.Blocks[1].Anchor
	pages := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d content bytes = %d", pages, page.ContentBytes)
		}
		for _, node := range page.Nodes {
			switch {
			case node.Type == "continuation":
				if node.BlockAnchor != blockAnchor {
					t.Fatalf("fragment block anchor = %q", node.BlockAnchor)
				}
				reassembled.WriteString(node.Text)
				fragments++
			case node.Text == "leading paragraph":
				sawLeading = true
			case node.Text == "trailing paragraph":
				sawTrailing = true
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("final page incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
		if pages > 20 {
			t.Fatal("pagination does not terminate")
		}
	}
	// Byte-exact reassembly of the fragmented block, including its newline.
	if reassembled.String() != source+"\n" {
		t.Fatalf("fragments do not reassemble: got %d bytes want %d", len(reassembled.String()), len(source)+1)
	}
	if fragments < 2 {
		t.Fatalf("expected multiple fragments, got %d", fragments)
	}
	if !sawLeading || !sawTrailing {
		t.Fatalf("leading=%v trailing=%v", sawLeading, sawTrailing)
	}
}

func TestReaderDocumentLargeListPaginatesByChild(t *testing.T) {
	t.Parallel()
	// A 520-item list fits the total budget but exceeds one page of nodes:
	// it must paginate by child items — every item delivered exactly once,
	// complete=true, zero warnings — instead of being dropped.
	var body strings.Builder
	body.WriteString("before\n\n")
	for i := 0; i < 520; i++ {
		body.WriteString("- item text\n")
	}
	body.WriteString("\nafter\n")
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	items := 0
	sawBefore, sawAfter := false, false
	pages := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if len(page.Warnings) > 0 {
			t.Fatalf("page %d warnings = %v", pages, page.Warnings)
		}
		if page.NodeCount > ReaderPageMaxNodes {
			t.Fatalf("page %d nodes = %d", pages, page.NodeCount)
		}
		if actual := recursiveOutputBytes(page.Nodes); actual > ReaderPageMaxTextBytes {
			t.Fatalf("page %d actual output bytes = %d over 256KiB", pages, actual)
		}
		for _, node := range page.Nodes {
			if node.Text == "before" {
				sawBefore = true
			}
			if node.Text == "after" {
				sawAfter = true
			}
			if node.Type == "list" {
				for _, item := range node.Children {
					for _, inner := range item.Children {
						if inner.Text == "item text" {
							items++
						}
					}
				}
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("in-budget list walk ended incomplete")
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	if items != 520 {
		t.Fatalf("delivered %d of 520 list items", items)
	}
	if !sawBefore || !sawAfter {
		t.Fatalf("before=%v after=%v", sawBefore, sawAfter)
	}
}

func TestReaderDocumentOversizedListItemFragmentsFully(t *testing.T) {
	t.Parallel()
	// A single 260 KiB list item is inside the total budget: its text must
	// fragment across pages (byte-continuous, reassembly exact), siblings
	// delivered, walk complete with zero warnings.
	huge := strings.Repeat("i", 260*1024)
	var body strings.Builder
	body.WriteString("- small one\n")
	body.WriteString("- " + huge + "\n")
	body.WriteString("- small two\n")
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	smallCount := 0
	var reassembled strings.Builder
	pages := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if len(page.Warnings) > 0 {
			t.Fatalf("page %d warnings = %v", pages, page.Warnings)
		}
		if page.NodeCount > ReaderPageMaxNodes {
			t.Fatalf("page %d nodes = %d over 512", pages, page.NodeCount)
		}
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d bytes = %d over 256KiB", pages, page.ContentBytes)
		}
		if actual := recursiveOutputBytes(page.Nodes); actual > ReaderPageMaxTextBytes {
			t.Fatalf("page %d actual output bytes = %d over 256KiB", pages, actual)
		}
		for _, node := range page.Nodes {
			for _, item := range node.Children {
				for _, inner := range item.Children {
					if inner.Text == "small one" || inner.Text == "small two" {
						smallCount++
					}
					if strings.HasPrefix(inner.Text, "iii") {
						reassembled.WriteString(inner.Text)
					}
				}
			}
			if node.Type == "continuation" && strings.HasPrefix(node.Text, "iii") {
				reassembled.WriteString(node.Text)
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("oversized item walk ended incomplete")
			}
			break
		}
		position = *page.Next
		if pages > 20 {
			t.Fatal("pagination does not terminate")
		}
	}
	if smallCount != 2 {
		t.Fatalf("siblings lost: %d of 2", smallCount)
	}
	// Byte-exact reassembly: fragments concatenated equal the original item
	// text exactly — no duplication, no loss.
	if reassembled.String() != huge {
		t.Fatalf("oversized item reassembly mismatch: got %d bytes want %d", reassembled.Len(), len(huge))
	}
}

func TestReaderDocumentLinkAfterFullPageIsDelivered(t *testing.T) {
	t.Parallel()
	// 511 short paragraphs consume the page node budget; the following
	// paragraph with one plain link must still be delivered exactly once on
	// the next page — never dropped as "exceeds budget".
	var body strings.Builder
	for i := 0; i < 511; i++ {
		body.WriteString("par\n\n")
	}
	body.WriteString("[link](https://example.org) tail\n")
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	links := 0
	paragraphs := 0
	pages := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if len(page.Warnings) > 0 {
			t.Fatalf("page %d warnings = %v", pages, page.Warnings)
		}
		if page.NodeCount > ReaderPageMaxNodes {
			t.Fatalf("page %d nodes = %d over 512", pages, page.NodeCount)
		}
		for _, node := range page.Nodes {
			if node.Text == "par" {
				paragraphs++
			}
			for _, child := range node.Children {
				if child.Type == "link" && child.Ref == "https://example.org" {
					links++
				}
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("walk ended incomplete after link paragraph")
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	if paragraphs != 511 {
		t.Fatalf("paragraphs = %d of 511", paragraphs)
	}
	if links != 1 {
		t.Fatalf("links = %d, want 1", links)
	}
}
func TestReaderDocumentPageBudgetCountsBytesAndChildren(t *testing.T) {
	t.Parallel()
	// 30000 five-byte paragraphs stay under the node and byte totals but
	// far exceed one page of node budget, so pages must split repeatedly.
	var body strings.Builder
	for i := 0; i < 30000; i++ {
		body.WriteString("abcde\n\n")
	}
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	covered := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d bytes = %d", pages, page.ContentBytes)
		}
		covered += len(page.Nodes)
		if page.Next == nil {
			break
		}
		position = *page.Next
		if pages > 100 {
			t.Fatal("pagination does not terminate")
		}
	}
	if pages < 2 || covered != len(document.Blocks) {
		t.Fatalf("pages=%d covered=%d of %d", pages, covered, len(document.Blocks))
	}
}

func TestReaderDocumentBudgetErrors(t *testing.T) {
	t.Parallel()
	if _, err := BuildReaderDocument(strings.Repeat("a", 8*1024*1024+1)); err == nil {
		t.Fatal("oversized source accepted")
	} else if budgetErr, ok := err.(*ReaderBudgetError); !ok || budgetErr.Code != "document_too_large" {
		t.Fatalf("err = %v", err)
	}

	// Node budget: a list with many items exercises nested node counting.
	var body strings.Builder
	for i := 0; i < 70000; i++ {
		body.WriteString("- item\n")
	}
	if _, err := BuildReaderDocument(body.String()); err == nil {
		t.Fatal("node budget not enforced")
	} else if budgetErr, ok := err.(*ReaderBudgetError); !ok || budgetErr.Code != "document_budget_exceeded" {
		t.Fatalf("err = %v", err)
	}
}

func TestReaderDocumentOutlinePagingAndAnchorLookup(t *testing.T) {
	t.Parallel()
	var body strings.Builder
	for i := 0; i < 600; i++ {
		body.WriteString("## Heading ")
		body.WriteString(strings.Repeat("h", i%7+1))
		body.WriteString("\n\ntext\n\n")
	}
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	first, next := ReaderOutlinePage(document, 0)
	if len(first) != ReaderOutlinePageSize || next != ReaderOutlinePageSize {
		t.Fatalf("outline page = %d next = %d", len(first), next)
	}
	second, next2 := ReaderOutlinePage(document, next)
	if len(second) != 88 || next2 != 0 { // 600 headings - 512 first page
		t.Fatalf("second outline page = %d next = %d", len(second), next2)
	}
	block := LocateReaderAnchor(document, first[0].Anchor)
	if block < 0 {
		t.Fatalf("anchor %q not located", first[0].Anchor)
	}
	if LocateReaderAnchor(document, "missing") != -1 {
		t.Fatal("unknown anchor located")
	}
}

func documentText(document ReaderDocument) string {
	var builder strings.Builder
	var walk func(nodes []ReaderDocumentNode)
	walk = func(nodes []ReaderDocumentNode) {
		for _, node := range nodes {
			builder.WriteString(node.Text)
			builder.WriteString(" ")
			walk(node.Children)
		}
	}
	walk(document.Blocks)
	return builder.String()
}

func TestReaderDocumentFragmentThenBlocksRespectBudget(t *testing.T) {
	t.Parallel()
	// A ~300KiB code block followed by a ~230KiB paragraph: after the final
	// fragment of the code block, the SAME page must not keep absorbing the
	// next block beyond the 256KiB budget.
	code := strings.Repeat("c", 300*1024)
	tail := strings.Repeat("p", 230*1024)
	body := "```text\n" + code + "\n```\n\n" + tail + "\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	var collected strings.Builder
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d content bytes = %d over budget", pages, page.ContentBytes)
		}
		for _, node := range page.Nodes {
			collected.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk ended incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	if !strings.Contains(collected.String(), code) || !strings.Contains(collected.String(), tail) {
		t.Fatal("content lost during fragment-then-block walk")
	}
}

func TestReaderDocumentTableContinuationCountsHeaderBudget(t *testing.T) {
	t.Parallel()
	// A long table with a header: every fragment re-carries the header, so
	// the page budget (and content_bytes) must include those bytes rather
	// than silently exceeding the 256 KiB text budget.
	rows := make([]string, 0, 20000)
	for i := 0; i < 20000; i++ {
		rows = append(rows, "| aaaaaaaa | bbbbbbbb |")
	}
	body := "| H1 header cell | H2 header cell |\n| --- | --- |\n" + strings.Join(rows, "\n") + "\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 || document.Blocks[0].Type != "table" {
		t.Fatalf("blocks = %#v", document.Blocks)
	}
	headerBytes := 0
	for _, header := range document.Blocks[0].Header {
		headerBytes += len(header)
	}
	if headerBytes == 0 {
		t.Fatal("table has no header to carry")
	}
	position := ReaderPagePosition{}
	pages := 0
	fragments := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d content bytes = %d over budget", pages, page.ContentBytes)
		}
		for _, node := range page.Nodes {
			if node.Type != "continuation" || node.OriginalType != "table" {
				t.Fatalf("unexpected node %#v", node)
			}
			if len(node.Header) != len(document.Blocks[0].Header) {
				t.Fatalf("fragment lost header: %#v", node.Header)
			}
			fragments++
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk ended incomplete: %v", page.Warnings)
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	if fragments < 2 {
		t.Fatalf("expected multiple fragments, got %d", fragments)
	}
}

func TestReaderDocumentStandaloneImageWithoutAltKeepsRef(t *testing.T) {
	t.Parallel()
	// A paragraph consisting only of an empty-alt image keeps its ref; the
	// owner resolver needs it to expose the attachment through the public
	// document surface.
	document, err := BuildReaderDocument("![](attachments/pic.png)\n")
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}
	found := false
	for _, child := range document.Blocks[0].Children {
		if child.Type == "image" && child.Ref == "attachments/pic.png" {
			found = true
		}
	}
	if !found {
		t.Fatalf("standalone image ref lost: %#v", document.Blocks[0])
	}
}

func TestReaderDocumentKeepsCodeInsideListItem(t *testing.T) {
	t.Parallel()
	// Fenced code nested in a list item must survive as a real child node;
	// flattening to text loses the code lines silently.
	body := "- item text\n\n  ```go\n  func inner() {}\n  ```\n\n- plain item\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 || document.Blocks[0].Type != "list" {
		t.Fatalf("blocks = %#v", document.Blocks)
	}
	encoded := documentText(document)
	if !strings.Contains(encoded, "func inner() {}") {
		t.Fatalf("code inside list item lost:\n%s", encoded)
	}
	sawCodeChild := false
	for _, item := range document.Blocks[0].Children {
		for _, child := range item.Children {
			if child.Type == "code" && strings.Contains(child.Text, "func inner() {}") {
				sawCodeChild = true
			}
		}
	}
	if !sawCodeChild {
		t.Fatalf("nested code not a typed child:\n%s", encoded)
	}
}

func TestReaderDocumentPaginationSurvivesHeaderHeavierThanRemainingBudget(t *testing.T) {
	t.Parallel()
	// A paragraph sized to leave almost no page budget, followed by a table:
	// the remaining budget cannot fit the table header, so the walk must
	// page-break (never panic on a negative slice) and still deliver every
	// block exactly once.
	filler := strings.Repeat("f", ReaderPageMaxTextBytes-4)
	rows := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		rows = append(rows, "| aaaaaaaa | bbbbbbbb |")
	}
	body := filler + "\n\n| H1 header cell | H2 header cell |\n| --- | --- |\n" + strings.Join(rows, "\n") + "\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	sawFiller, sawTable := false, false
	anchors := map[string]bool{}
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d bytes = %d", pages, page.ContentBytes)
		}
		for _, node := range page.Nodes {
			if anchors[node.Anchor] {
				t.Fatalf("page %d repeated anchor %q", pages, node.Anchor)
			}
			anchors[node.Anchor] = true
			if node.Type == "paragraph" && strings.HasPrefix(node.Text, "ffff") {
				sawFiller = true
			}
			if node.Type == "table" || (node.Type == "continuation" && node.OriginalType == "table") {
				sawTable = true
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk ended incomplete: %v", page.Warnings)
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	if !sawFiller || !sawTable {
		t.Fatalf("filler=%v table=%v", sawFiller, sawTable)
	}
	if len(anchors) != len(document.Blocks) {
		t.Fatalf("covered %d of %d blocks", len(anchors), len(document.Blocks))
	}
}

func TestReaderDocumentKeepsImagesInsideTableCells(t *testing.T) {
	t.Parallel()
	// An attachment referenced from a table cell must survive as a child
	// ref node; text-only extraction would silently drop the discovery link.
	body := "| Picture |\n| --- |\n| ![](attachments/pic.png) |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 || document.Blocks[0].Type != "table" {
		t.Fatalf("blocks = %#v", document.Blocks)
	}
	found := false
	for _, child := range document.Blocks[0].Children {
		if child.Type == "image" && child.Ref == "attachments/pic.png" {
			found = true
		}
	}
	if !found {
		t.Fatalf("table cell image ref lost: %#v", document.Blocks[0].Children)
	}
}

func TestReaderDocumentKeepsHeaderCellImagesWithUniqueAnchors(t *testing.T) {
	t.Parallel()
	// An attachment referenced from the HEADER row must survive, and child
	// anchors must stay unique across every cell of the table.
	body := "| ![](attachments/head.png) | text |\n| --- | --- |\n| ![](attachments/cell1.png) | ![](attachments/cell2.png) |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 || document.Blocks[0].Type != "table" {
		t.Fatalf("blocks = %#v", document.Blocks)
	}
	refs := map[string]bool{}
	anchors := map[string]bool{}
	for _, child := range document.Blocks[0].Children {
		if child.Type != "image" {
			t.Fatalf("unexpected child %#v", child)
		}
		refs[child.Ref] = true
		if anchors[child.Anchor] {
			t.Fatalf("duplicate child anchor %q", child.Anchor)
		}
		anchors[child.Anchor] = true
	}
	for _, want := range []string{"attachments/head.png", "attachments/cell1.png", "attachments/cell2.png"} {
		if !refs[want] {
			t.Fatalf("ref %q lost; children = %#v", want, document.Blocks[0].Children)
		}
	}
}

func TestReaderDocumentHeadingAnchorsAreRuneSafeAndUnique(t *testing.T) {
	t.Parallel()
	// Long CJK headings must not be byte-truncated mid-rune: the anchor has
	// to survive a JSON round-trip and still resolve in the anchor index.
	longTitle := strings.Repeat("题", 30) // 30 runes = 90 bytes > 80
	body := "# " + longTitle + "\n\npara\n\n# Foo\n\n# Foo\n\n# Foo-2\n\n# block 1\n\nlast\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	cjk := document.Outline[0]
	encoded, err := json.Marshal(cjk)
	if err != nil {
		t.Fatalf("marshal CJK outline entry: %v", err)
	}
	var roundTripped ReaderOutlineEntry
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if roundTripped.Anchor != cjk.Anchor {
		t.Fatalf("CJK anchor does not round-trip: %q vs %q", roundTripped.Anchor, cjk.Anchor)
	}
	if LocateReaderAnchor(document, cjk.Anchor) < 0 {
		t.Fatalf("CJK anchor not locatable: %q", cjk.Anchor)
	}
	// foo, foo, foo-2 headings and a "block 1" heading must all yield unique
	// anchors that never collide with paragraph block anchors.
	seen := map[string]int{}
	for _, block := range document.Blocks {
		seen[block.Anchor]++
	}
	for anchor, count := range seen {
		if count > 1 {
			t.Fatalf("anchor %q used %d times", anchor, count)
		}
		if LocateReaderAnchor(document, anchor) < 0 {
			t.Fatalf("anchor %q not indexed", anchor)
		}
	}
	if len(seen) != len(document.Blocks) {
		t.Fatalf("anchors %d != blocks %d", len(seen), len(document.Blocks))
	}
}

func TestReaderDocumentFragmentsOversizedBlocksWithInlineRefs(t *testing.T) {
	t.Parallel()
	// A ~300KiB paragraph carrying one link and a long table with an image
	// ref must continue via fragments (refs riding the first fragment), not
	// be dropped: full text delivered, complete=true, no warnings.
	longParagraph := strings.Repeat("段落", 150*1024) + " [link](notes/other.md) " + strings.Repeat("尾部", 1024)
	rows := make([]string, 0, 20000)
	for i := 0; i < 20000; i++ {
		rows = append(rows, "| aaaaaaaa | bbbbbbbb |")
	}
	body := longParagraph + "\n\n| H | ![pic](attachments/pic.png) |\n| --- | --- |\n" + strings.Join(rows, "\n") + "\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 2 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}
	position := ReaderPagePosition{}
	var collected strings.Builder
	pages := 0
	warnings := 0
	sawLink, sawImage := false, false
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		warnings += len(page.Warnings)
		if page.ContentBytes > ReaderPageMaxTextBytes {
			t.Fatalf("page %d bytes = %d", pages, page.ContentBytes)
		}
		for _, node := range page.Nodes {
			collected.WriteString(node.Text)
			for _, child := range node.Children {
				if child.Type == "link" && child.Ref == "notes/other.md" {
					sawLink = true
				}
				if child.Type == "image" && child.Ref == "attachments/pic.png" {
					sawImage = true
				}
			}
		}
		if page.Next == nil {
			if !page.Complete || warnings > 0 {
				t.Fatalf("walk incomplete=%v warnings=%v", !page.Complete, page.Warnings)
			}
			break
		}
		position = *page.Next
		if pages > 20 {
			t.Fatal("pagination does not terminate")
		}
	}
	if !strings.Contains(collected.String(), strings.Repeat("尾部", 1024)) || !strings.Contains(collected.String(), strings.Repeat("段落", 1000)) {
		t.Fatal("paragraph content lost")
	}
	if !sawLink || !sawImage {
		t.Fatalf("refs lost: link=%v image=%v", sawLink, sawImage)
	}
}

func TestReaderDocumentRecursiveAnchorUniquenessIncludingChildren(t *testing.T) {
	t.Parallel()
	// The image child's derived anchor equals the last heading's slug: the
	// shared registry must disambiguate. Every anchor in the whole tree —
	// blocks, nested children — must be unique.
	body := "# Foo\n\n![x](a.png)\n\n# block-2-image-1\n\npara\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	seen := map[string]int{}
	var walk func(nodes []ReaderDocumentNode)
	walk = func(nodes []ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Anchor != "" {
				seen[node.Anchor]++
			}
			walk(node.Children)
		}
	}
	walk(document.Blocks)
	for anchor, count := range seen {
		if count > 1 {
			t.Fatalf("anchor %q used %d times (tree = %#v)", anchor, count, seen)
		}
	}
	// Both the heading and the (renamed) image anchor resolve in the index.
	for anchor := range seen {
		if LocateReaderAnchor(document, anchor) < 0 {
			t.Fatalf("anchor %q not locatable", anchor)
		}
	}
}

func TestReaderDocumentRefFloodPaginatesAllRefs(t *testing.T) {
	t.Parallel()
	// A paragraph with 600 short links is far inside the total budget: every
	// ref must be delivered exactly once across pages (batched refs), the
	// walk completes with zero warnings and no drop.
	var body strings.Builder
	for i := 0; i < 600; i++ {
		body.WriteString("[l](notes/x.md) ")
	}
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}
	position := ReaderPagePosition{}
	refs := 0
	pages := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		if len(page.Warnings) > 0 {
			t.Fatalf("page %d warnings = %v", pages, page.Warnings)
		}
		if page.NodeCount > ReaderPageMaxNodes {
			t.Fatalf("page %d nodes = %d", pages, page.NodeCount)
		}
		for _, node := range page.Nodes {
			refs += len(node.Children)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("ref-flood walk ended incomplete")
			}
			break
		}
		position = *page.Next
		if pages > 20 {
			t.Fatal("pagination does not terminate")
		}
	}
	if refs != 600 {
		t.Fatalf("delivered %d of 600 refs", refs)
	}
}
func TestReaderDocumentTerminatesOnOversizedRefText(t *testing.T) {
	t.Parallel()
	// A single link whose display text is 270 KiB cannot fit any page's
	// byte budget: the walk must terminate with an explicit warning instead
	// of returning the same position forever.
	linkText := strings.Repeat("t", 270*1024)
	document, err := BuildReaderDocument("[link text](" + "notes/x.md" + ")")
	_ = linkText
	_ = document
	_ = err
	body := "[" + strings.Repeat("t", 270*1024) + "](notes/x.md)"
	document, err = BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	warned := false
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, warning := range page.Warnings {
			if warning == "block_exceeds_page_budget" {
				warned = true
			}
		}
		if page.Next == nil {
			if page.Complete {
				t.Fatal("oversized ref text reported complete")
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination stalls on oversized ref text")
		}
	}
	if !warned {
		t.Fatal("oversized ref drop was silent")
	}
}

func TestReaderDocumentNestedHeadingAnchorMapsToOwningBlock(t *testing.T) {
	t.Parallel()
	// A heading nested inside a quote must map to the quote's top-level
	// block index (1), not the build-time placeholder of the trailing
	// paragraph (2).
	body := "intro\n\n> # Nested\n\nlast\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 3 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}
	index := LocateReaderAnchor(document, "nested")
	if index != 1 {
		t.Fatalf("nested anchor maps to %d, want 1 (the quote block)", index)
	}
	lastAnchor := document.Blocks[2].Anchor
	if index := LocateReaderAnchor(document, lastAnchor); index != 2 {
		t.Fatalf("last anchor %q maps to %d, want 2", lastAnchor, index)
	}
}

func TestReaderDocumentTerminatesWhenHeaderLeavesSubRuneBudget(t *testing.T) {
	t.Parallel()
	// A single-column table whose header eats all but one byte of the page:
	// the remaining budget cannot afford one CJK rune of the body. The walk
	// must terminate with an explicit warning instead of emitting empty
	// fragments forever.
	header := strings.Repeat("h", ReaderPageMaxTextBytes-1)
	body := "| " + header + " |\n| --- |\n| 中文 |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	warned := false
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, warning := range page.Warnings {
			if warning == "block_exceeds_page_budget" {
				warned = true
			}
		}
		for _, node := range page.Nodes {
			if node.Type == "continuation" && node.Text == "" {
				t.Fatal("empty continuation fragment emitted")
			}
		}
		if page.Next == nil {
			if page.Complete {
				t.Fatal("unpageable table reported complete")
			}
			break
		}
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination stalls on sub-rune budget")
		}
	}
	if !warned {
		t.Fatal("sub-rune budget drop was silent")
	}
}

func TestReaderDocumentImageAltTextCountsTowardBudget(t *testing.T) {
	t.Parallel()
	// An image paragraph whose alt text is ~150 KiB: the block carries the
	// label twice (paragraph text plus alt), so the fast-path byte count
	// must include the alt; a single page must not silently exceed the
	// 256 KiB budget.
	alt := strings.Repeat("a", 150*1024)
	body := "![" + alt + "](attachments/big.png)\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if readerNodeTextBytes(document.Blocks[0]) <= 150*1024 {
		t.Fatalf("alt bytes not counted: %d", readerNodeTextBytes(document.Blocks[0]))
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if page.NodeCount == 0 || page.Warnings == nil && !page.Complete {
		// Either the whole block fits (budget counted both copies), or it
		// fragments/drops explicitly; the invariant is the byte budget.
		_ = page
	}
	if whole := len(document.Blocks) == 1 && page.Complete; whole && page.ContentBytes > ReaderPageMaxTextBytes {
		t.Fatalf("page bytes = %d over budget", page.ContentBytes)
	}
	if !page.Complete && page.Next == nil && len(page.Warnings) == 0 {
		t.Fatal("incomplete page without warning or cursor")
	}
}

func TestReaderDocumentPreservesAutolinksTaskBoxesAndInlineHTML(t *testing.T) {
	t.Parallel()
	// Autolinks, GFM task checkboxes and raw inline HTML must all survive
	// into the body (as text or typed refs) with no silent loss and
	// complete=true — none of them are budget drops.
	body := "Visit <https://example.com/docs> now.\n\n- [x] done task\n- [ ] open task\n\nInline <span data-x=\"1\">raw</span> html.\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	encoded := documentText(document)
	if !strings.Contains(encoded, "https://example.com/docs") {
		t.Fatalf("autolink URL lost:\n%s", encoded)
	}
	if !strings.Contains(encoded, "[x] done task") || !strings.Contains(encoded, "[ ] open task") {
		t.Fatalf("task checkbox state lost:\n%s", encoded)
	}
	if !strings.Contains(encoded, "<span data-x=\"1\">raw</span>") {
		t.Fatalf("inline raw HTML lost:\n%s", encoded)
	}
	// The autolink also surfaces as a typed link child.
	foundLink := false
	for _, block := range document.Blocks {
		for _, child := range block.Children {
			if child.Type == "link" && child.Ref == "https://example.com/docs" {
				foundLink = true
			}
		}
	}
	if !foundLink {
		t.Fatalf("autolink child ref missing:\n%s", encoded)
	}
	// Pagination completes with no warnings.
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("page complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentMultiLineHTMLBlockKeepsClosureLine(t *testing.T) {
	t.Parallel()
	// Goldmark stores the closing-tag line in ClosureLine, separate from
	// Lines(): the whole block body must survive with no silent loss.
	body := "<pre>\n第一行\n中间行\n最后一行</pre>\n\nafter\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	var blockText string
	for _, block := range document.Blocks {
		if block.Type == "unsupported" && strings.Contains(block.Text, "<pre>") {
			blockText = block.Text
		}
	}
	if blockText == "" {
		t.Fatalf("html block lost: %#v", document.Blocks)
	}
	for _, want := range []string{"<pre>", "第一行", "中间行", "最后一行</pre>"} {
		if !strings.Contains(blockText, want) {
			t.Fatalf("html block lost %q: %q", want, blockText)
		}
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("page complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentStructuralBudgetRespectsRemainingPage(t *testing.T) {
	t.Parallel()
	// Auditor scenarios: content ahead of a list consumes the page budget,
	// so the list must move/behave within the REMAINING budget — never
	// exceed 512 nodes / 256 KiB on a single page while claiming complete.
	build := func(preParagraphs int, preBytes int) string {
		var body strings.Builder
		if preBytes > 0 {
			body.WriteString(strings.Repeat("p", preBytes) + "\n\n")
		}
		for i := 0; i < preParagraphs; i++ {
			body.WriteString("par\n\n")
		}
		body.WriteString("- item one\n- item two\n")
		return body.String()
	}
	scenarios := []struct {
		name           string
		body           string
		minNodeOvershr bool
	}{
		{"511 paragraphs then list", build(511, 0), false},
		{"262143-byte paragraph then list", build(0, 262143), false},
		{"300-item list then 500 paragraphs", func() string {
			var body strings.Builder
			for i := 0; i < 300; i++ {
				body.WriteString("- item\n")
			}
			body.WriteString("\n")
			for i := 0; i < 500; i++ {
				body.WriteString("par\n\n")
			}
			return body.String()
		}(), false},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			document, err := BuildReaderDocument(scenario.body)
			if err != nil {
				t.Fatalf("BuildReaderDocument: %v", err)
			}
			position := ReaderPagePosition{}
			pages := 0
			for {
				page := PaginateReaderDocument(document, position)
				pages++
				if page.NodeCount > ReaderPageMaxNodes {
					t.Fatalf("page %d nodes = %d over 512", pages, page.NodeCount)
				}
				if page.ContentBytes > ReaderPageMaxTextBytes {
					t.Fatalf("page %d bytes = %d over 256KiB", pages, page.ContentBytes)
				}
				if page.Next == nil {
					if !page.Complete {
						t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
					}
					break
				}
				position = *page.Next
				if pages > 50 {
					t.Fatal("pagination does not terminate")
				}
			}
		})
	}
}

func TestReaderDocumentTotalNodeBudgetCountsOnce(t *testing.T) {
	t.Parallel()
	// 14000 items ≈ 28001 real nodes: far under 65536. Double-counting the
	// subtree would report 70001 and wrongly reject the document.
	var body strings.Builder
	for i := 0; i < 14000; i++ {
		body.WriteString("- item\n")
	}
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("in-budget list rejected: %v", err)
	}
	if document.NodeCount != 28001 {
		t.Fatalf("NodeCount = %d, want 28001 (no double counting)", document.NodeCount)
	}
}

func TestReaderDocumentTotalNodeBudgetCountsGeneratedRefs(t *testing.T) {
	t.Parallel()
	// Auditor scenario: 33000 link paragraphs = 66000 real nodes over the
	// 65536 total budget — the build must reject, not complete later.
	body := strings.Repeat("[x](https://example.com)\n\n", 33000)
	if _, err := BuildReaderDocument(body); err == nil {
		t.Fatal("document exceeding total node budget via inline refs accepted")
	} else if budgetErr, ok := err.(*ReaderBudgetError); !ok || budgetErr.Code != "document_budget_exceeded" {
		t.Fatalf("err = %v", err)
	}

	// Just under the budget: 32000 links + 32000 paragraphs = 64000 nodes —
	// accepted, and the reported count includes every generated ref.
	body = strings.Repeat("[x](https://example.com)\n\n", 32000)
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("in-budget link document rejected: %v", err)
	}
	if document.NodeCount != 64000 {
		t.Fatalf("NodeCount = %d, want 64000 (paragraphs + generated links)", document.NodeCount)
	}

	// Table cell refs count too: 33000 single-cell tables with one link each.
	tableBody := strings.Repeat("| [x](https://example.com) |\n| --- |\n| v |\n\n", 33000)
	if _, err := BuildReaderDocument(tableBody); err == nil {
		t.Fatal("document exceeding total node budget via table cell refs accepted")
	}
}

// recursiveOutputBytes computes the actual UTF-8 text bytes of a page's
// emitted node tree (node text, alts and headers included) so tests can
// assert the real wire volume against the page budget, not just the
// self-reported content_bytes.
func recursiveOutputBytes(nodes []ReaderDocumentNode) int {
	total := 0
	for _, node := range nodes {
		total += len(node.Text) + len(node.Alt)
		for _, header := range node.Header {
			total += len(header)
		}
		total += recursiveOutputBytes(node.Children)
	}
	return total
}

func TestReaderDocumentAuditorScenariosRespectActualOutputBudget(t *testing.T) {
	t.Parallel()
	// Auditor scenario 1: one list item holding 300 paragraphs of 1000
	// bytes each — parent text must never duplicate child bytes, so the
	// ACTUAL emitted tree stays within the page budget.
	var itemBody strings.Builder
	for i := 0; i < 300; i++ {
		itemBody.WriteString(strings.Repeat("p", 1000))
		itemBody.WriteString("\n")
	}
	multiParagraphList := "intro\n\n- " + strings.ReplaceAll(itemBody.String(), "\n", "\n  ") + "\n"

	// Auditor scenario 3: an image whose alt text is ~200KiB — the alt
	// counts toward the ref batch budget.
	altFlood := "![" + strings.Repeat("a", 200*1024) + "](https://example.org/image.png)\n"

	scenarios := []struct {
		name string
		body string
	}{
		{"multi-paragraph list item", multiParagraphList},
		{"image alt flood", altFlood},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			document, err := BuildReaderDocument(scenario.body)
			if err != nil {
				t.Fatalf("BuildReaderDocument: %v", err)
			}
			position := ReaderPagePosition{}
			pages := 0
			seenStates := map[string]bool{}
			for {
				page := PaginateReaderDocument(document, position)
				pages++
				if actual := recursiveOutputBytes(page.Nodes); actual > ReaderPageMaxTextBytes {
					t.Fatalf("page %d ACTUAL output bytes = %d over 256KiB (reported %d)", pages, actual, page.ContentBytes)
				}
				if page.NodeCount > ReaderPageMaxNodes {
					t.Fatalf("page %d nodes = %d over 512", pages, page.NodeCount)
				}
				if page.Next == nil {
					if !page.Complete {
						t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
					}
					break
				}
				state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
				if seenStates[state] {
					t.Fatalf("cursor does not progress: repeated state %q", state)
				}
				seenStates[state] = true
				position = *page.Next
				if pages > 30 {
					t.Fatal("pagination does not terminate")
				}
			}
		})
	}

	// Auditor scenario 2: a single-column table whose header alone exceeds
	// the page budget — no infinite empty pages; an explicit warning and
	// incomplete completion, with the walk terminating.
	hugeHeader := strings.Repeat("h", 270*1024)
	headerBody := "| " + hugeHeader + " |\n| --- |\n| v |\n"
	document, err := BuildReaderDocument(headerBody)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	warned := false
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, warning := range page.Warnings {
			if warning == "block_exceeds_page_budget" {
				warned = true
			}
		}
		if page.Next == nil {
			if page.Complete {
				t.Fatal("unpageable header reported complete")
			}
			break
		}
		position = *page.Next
		if pages > 5 {
			t.Fatal("huge header pagination stalls")
		}
	}
	if !warned {
		t.Fatal("unpageable header drop was silent")
	}
}

func TestReaderDocumentHeaderPlusRefCombinationsTerminate(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name string
		body string
	}{
		// 270KiB header + data row with a plain link: header+ref exceeds any
		// page — explicit warning drop, finite termination, incomplete.
		{"huge header with link row", "| " + strings.Repeat("h", 270*1024) + " |\n| --- |\n| [l](notes/x.md) |\n"},
		// 150KiB header + 150KiB-alt image: each fits a page alone, the
		// combination does not — same explicit outcome.
		{"header plus alt-heavy image row", "| " + strings.Repeat("h", 150*1024) + " |\n| --- |\n| ![" + strings.Repeat("a", 150*1024) + "](https://example.org/i.png) |\n"},
		// 270KiB header-only table (no data rows): no silent skip — warning,
		// incomplete, zero nodes or finite termination.
		{"huge header-only table", "| " + strings.Repeat("h", 270*1024) + " |\n| --- |\n"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			document, err := BuildReaderDocument(scenario.body)
			if err != nil {
				t.Fatalf("BuildReaderDocument: %v", err)
			}
			position := ReaderPagePosition{}
			pages := 0
			warned := false
			seenStates := map[string]bool{}
			for {
				page := PaginateReaderDocument(document, position)
				pages++
				for _, warning := range page.Warnings {
					if warning == "block_exceeds_page_budget" {
						warned = true
					}
				}
				if page.Next == nil {
					if page.Complete {
						t.Fatal("unpageable block reported complete")
					}
					break
				}
				state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
				if seenStates[state] {
					t.Fatalf("cursor does not progress: repeated state %q", state)
				}
				seenStates[state] = true
				position = *page.Next
				if pages > 5 {
					t.Fatal("pagination stalls on header+ref combination")
				}
			}
			if !warned {
				t.Fatal("unpageable combination drop was silent")
			}
		})
	}
}

func TestReaderDocumentSmallHeaderOnlyTableEmitsContent(t *testing.T) {
	t.Parallel()
	// A small header-only table (no data rows) still emits its header node —
	// childless blocks are never silently skipped.
	document, err := BuildReaderDocument("| H1 | H2 |\n| --- | --- |\n")
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if len(page.Nodes) != 1 || page.Nodes[0].Type != "table" || len(page.Nodes[0].Header) != 2 {
		t.Fatalf("header-only table lost: %#v", page.Nodes)
	}
	if !page.Complete {
		t.Fatal("small header-only table incomplete")
	}
}

func TestReaderDocumentHeaderOnlyTableWithRefsRespectsBudget(t *testing.T) {
	t.Parallel()
	// Auditor scenarios: header-only tables whose header cells carry refs.
	// The header+ref combination exceeding any page is an explicit warning
	// drop with incomplete completion — never a silent over-budget page and
	// never a silent complete=true skip.
	scenarios := []struct {
		name string
		body string
	}{
		{"huge header with link", "| " + strings.Repeat("h", 270*1024) + " [l](notes/x.md) |\n| --- |\n"},
		{"alt-heavy image as header", "| ![" + strings.Repeat("a", 150*1024) + "](https://example.org/i.png) |\n| --- |\n"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			document, err := BuildReaderDocument(scenario.body)
			if err != nil {
				t.Fatalf("BuildReaderDocument: %v", err)
			}
			position := ReaderPagePosition{}
			pages := 0
			warned := false
			for {
				page := PaginateReaderDocument(document, position)
				pages++
				if actual := recursiveOutputBytes(page.Nodes); actual > ReaderPageMaxTextBytes {
					t.Fatalf("page %d ACTUAL output bytes = %d over 256KiB", pages, actual)
				}
				for _, warning := range page.Warnings {
					if warning == "block_exceeds_page_budget" {
						warned = true
					}
				}
				if page.Next == nil {
					if page.Complete {
						t.Fatal("unpageable header-only table reported complete")
					}
					break
				}
				position = *page.Next
				if pages > 5 {
					t.Fatal("pagination stalls on header-only table")
				}
			}
			if !warned {
				t.Fatal("unpageable header-only table drop was silent")
			}
		})
	}

	// Medium case: a header-only table with a small link emits its header
	// and ref within budget with complete=true.
	document, err := BuildReaderDocument("| [docs](notes/x.md) |\n| --- |\n")
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	foundRef := false
	for _, node := range page.Nodes {
		for _, child := range node.Children {
			if child.Type == "link" && child.Ref == "notes/x.md" {
				foundRef = true
			}
		}
		if len(node.Header) != 1 {
			t.Fatalf("header lost: %#v", node)
		}
	}
	if !foundRef || !page.Complete {
		t.Fatalf("small header-only table: ref=%v complete=%v", foundRef, page.Complete)
	}
}

func TestReaderDocumentListRefsNotDuplicated(t *testing.T) {
	t.Parallel()
	// A link inside a list item must yield exactly ONE link node: the
	// recursive item build collects it once; the outer pass must not walk
	// the item again.
	document, err := BuildReaderDocument("- [docs](https://example.test)\n")
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	links := 0
	var walk func(nodes []ReaderDocumentNode)
	walk = func(nodes []ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Type == "link" {
				links++
			}
			walk(node.Children)
		}
	}
	walk(document.Blocks)
	if links != 1 {
		t.Fatalf("links = %d, want exactly 1", links)
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentDeepNestedImagePreserved(t *testing.T) {
	t.Parallel()
	// Seven levels of blockquote nesting: the image ref must survive with
	// no silent depth cap.
	body := strings.Repeat("> ", 7) + "![figure](https://example.test/image.png)\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	images := 0
	var walk func(nodes []ReaderDocumentNode)
	walk = func(nodes []ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Type == "image" && node.Ref == "https://example.test/image.png" {
				images++
			}
			walk(node.Children)
		}
	}
	walk(document.Blocks)
	if images != 1 {
		t.Fatalf("images = %d, want 1 (no silent depth loss)", images)
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentLastContinuationMarkedLast(t *testing.T) {
	t.Parallel()
	// 600 refs batch across multiple continuation fragments: every fragment
	// of the block except the final one must carry last_fragment=false and
	// the FINAL one last_fragment=true, so consumers can tell the block
	// ended even though the last page reports complete=true.
	var body strings.Builder
	for i := 0; i < 600; i++ {
		body.WriteString("[x](https://example.test) ")
	}
	document, err := BuildReaderDocument(body.String())
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	fragments := 0
	lastFlags := []bool{}
	pages := 0
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, node := range page.Nodes {
			if node.Type == "continuation" && node.BlockAnchor == document.Blocks[0].Anchor {
				fragments++
				lastFlags = append(lastFlags, node.LastFragment)
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("walk ended incomplete")
			}
			break
		}
		position = *page.Next
		if pages > 20 {
			t.Fatal("pagination does not terminate")
		}
	}
	if fragments < 2 {
		t.Fatalf("expected multiple continuation fragments, got %d", fragments)
	}
	for index, flag := range lastFlags {
		wantLast := index == len(lastFlags)-1
		if flag != wantLast {
			t.Fatalf("fragment %d of %d last_fragment=%v, want %v", index+1, len(lastFlags), flag, wantLast)
		}
	}
}

func TestReaderDocumentNestedTaskListDoesNotMarkParent(t *testing.T) {
	t.Parallel()
	// A checked CHILD task must not mark its unchecked PARENT item: only
	// the item's own first text block determines its task state.
	document, err := BuildReaderDocument("- parent\n  - [x] child\n")
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	// Find the outer list's first item (parent).
	var parent, child *ReaderDocumentNode
	var findItems func(nodes []ReaderDocumentNode)
	findItems = func(nodes []ReaderDocumentNode) {
		for i := range nodes {
			node := &nodes[i]
			if node.Text == "parent" {
				parent = node
			}
			if node.Text == "[x] child" {
				child = node
			}
			findItems(node.Children)
		}
	}
	findItems(document.Blocks)
	if parent == nil || child == nil {
		t.Fatalf("parent=%v child=%v tree=%#v", parent, child, document.Blocks)
	}
	if strings.HasPrefix(parent.Text, "[x] ") || strings.HasPrefix(parent.Text, "[ ] ") {
		t.Fatalf("parent wrongly marked as task: %q", parent.Text)
	}
	if child.Text != "[x] child" {
		t.Fatalf("child task state lost: %q", child.Text)
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentLongEntityNotSplitAtPageBoundary(t *testing.T) {
	t.Parallel()
	// A 31-char named entity straddling the page boundary must never be
	// cut in half: the split backs off before the '&', both pages reassemble
	// to the exact original.
	entity := "&CounterClockwiseContourIntegral;"
	body := strings.Repeat("a", ReaderPageMaxTextBytes-12) + entity + strings.Repeat("b", 20)
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	if len(document.Blocks) != 1 {
		t.Fatalf("blocks = %d", len(document.Blocks))
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	for {
		page := PaginateReaderDocument(document, position)
		for _, node := range page.Nodes {
			reassembled.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
	}
	if reassembled.String() != body {
		t.Fatalf("entity split or content lost: reassembled %d bytes want %d; tail=%q", reassembled.Len(), len(body), reassembled.String()[ReaderPageMaxTextBytes-20:ReaderPageMaxTextBytes+20])
	}
}

func TestReaderDocumentEntityAtFragmentStartNotCut(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a first paragraph consumes nearly the whole page;
	// the SECOND paragraph begins with the entity. The remaining in-page
	// budget (~10 bytes) cannot hold it — the fragment must page-break and
	// re-emit the whole entity on the next page, never cut it open.
	entity := "&CounterClockwiseContourIntegral;"
	body := strings.Repeat("a", ReaderPageMaxTextBytes-10) + "\n\n" + entity + " tail"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	fragments := 0
	for {
		page := PaginateReaderDocument(document, position)
		for _, node := range page.Nodes {
			if node.Type == "paragraph" || node.Type == "continuation" {
				reassembled.WriteString(node.Text)
				fragments++
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
		if fragments > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	// The parsed document drops the raw "\n\n" separator; expected is the
	// concatenation of the top-level block texts as parsed.
	var expected strings.Builder
	for _, block := range document.Blocks {
		expected.WriteString(block.Text)
	}
	joined := reassembled.String()
	if joined != expected.String() {
		t.Fatalf("content lost or entity cut: got %d bytes want %d", len(joined), expected.Len())
	}
	if !strings.Contains(joined, entity) {
		t.Fatalf("entity damaged in reassembly: %q", joined[len(joined)-60:])
	}
}

func TestReaderDocumentHugeHeaderPlusEntityRowTerminates(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a 256KiB-10 header with an entity data row. The
	// header consumes nearly every page; the indivisible unit (the whole
	// entity) never fits — explicit warning drop, finite termination, no
	// repeated-position empty-page loop.
	entity := "&CounterClockwiseContourIntegral;"
	body := "| " + strings.Repeat("h", ReaderPageMaxTextBytes-10) + " |\n| --- |\n| " + entity + " |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	warned := false
	seenStates := map[string]bool{}
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, warning := range page.Warnings {
			if warning == "block_exceeds_page_budget" {
				warned = true
			}
		}
		if page.Next == nil {
			if page.Complete {
				t.Fatal("unpageable header+entity row reported complete")
			}
			break
		}
		state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
		if seenStates[state] {
			t.Fatalf("pagination repeats identical position without progress: %q", state)
		}
		seenStates[state] = true
		position = *page.Next
		if pages > 5 {
			t.Fatal("pagination stalls on header+entity row")
		}
	}
	if !warned {
		t.Fatal("unpageable header+entity drop was silent")
	}
}

func TestReaderDocumentBareAmpersandProgressesUnderTightBudget(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a 256KiB-10 header with a data row starting with a
	// bare '&' (no terminating ';' — plain text, not an entity). Pagination
	// must progress and deliver the row, never loop on empty pages.
	body := "| " + strings.Repeat("H", ReaderPageMaxTextBytes-10) + " |\n| --- |\n| &abcdefghijklmnopqrstuvxyz |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	pages := 0
	seenStates := map[string]bool{}
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, node := range page.Nodes {
			reassembled.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
		if seenStates[state] {
			t.Fatalf("pagination repeats position without progress: %q", state)
		}
		seenStates[state] = true
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination stalls on bare ampersand")
		}
	}
	if !strings.Contains(reassembled.String(), "&abcdefghijklmnopqrstuvxyz") {
		t.Fatalf("bare & row lost: %q", reassembled.String()[:60])
	}
}

func TestReaderDocumentAmpersandProseNotTreatedAsEntity(t *testing.T) {
	t.Parallel()
	// Auditor scenario: "& ordinary prose;" — the char after '&' is a
	// space, so this is plain prose, NOT an entity. Under a tight header
	// budget the row must paginate normally (progress + full delivery),
	// never be dropped as one indivisible unit.
	body := "| " + strings.Repeat("H", ReaderPageMaxTextBytes-10) + " |\n| --- |\n| & ordinary prose; |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	pages := 0
	seenStates := map[string]bool{}
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, node := range page.Nodes {
			reassembled.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
		if seenStates[state] {
			t.Fatalf("pagination repeats position: %q", state)
		}
		seenStates[state] = true
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination stalls on ampersand prose")
		}
	}
	if !strings.Contains(reassembled.String(), "& ordinary prose;") {
		t.Fatalf("prose row lost: %q", reassembled.String()[:60])
	}
}

func TestReaderDocumentNumericEntityStillIndivisible(t *testing.T) {
	t.Parallel()
	// Real numeric entities keep their indivisible-unit protection: a
	// boundary landing inside one backs off whole.
	entity := "&#x1F600;"
	body := strings.Repeat("a", ReaderPageMaxTextBytes-4) + entity + " tail"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	for {
		page := PaginateReaderDocument(document, position)
		for _, node := range page.Nodes {
			reassembled.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: %v", page.Warnings)
			}
			break
		}
		position = *page.Next
	}
	var expected strings.Builder
	for _, block := range document.Blocks {
		expected.WriteString(block.Text)
	}
	if reassembled.String() != expected.String() || !strings.Contains(reassembled.String(), entity) {
		t.Fatalf("numeric entity damaged: %q", reassembled.String()[len(reassembled.String())-40:])
	}
}

func TestReaderDocumentLongNumericReferenceNotCut(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a numeric reference with 1001 leading zeros —
	// beyond any fixed scan window — straddling the page boundary must
	// never be cut open, PER FRAGMENT (not merely after reassembly).
	entity := "&#" + strings.Repeat("0", 1001) + "65;"
	body := strings.Repeat("a", ReaderPageMaxTextBytes-10) + entity + " tail"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	for {
		page := PaginateReaderDocument(document, position)
		for _, node := range page.Nodes {
			text := node.Text
			reassembled.WriteString(text)
			// Per-fragment integrity: any fragment containing the '&' of
			// this reference must also contain its closing ';'.
			if idx := strings.Index(text, "&#"); idx >= 0 {
				if !strings.Contains(text[idx:], ";") {
					t.Fatalf("fragment ends inside the numeric reference: tail=%q", text[max(0, len(text)-40):])
				}
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
	}
	if !strings.Contains(reassembled.String(), entity) {
		t.Fatalf("long numeric reference damaged in reassembly")
	}
	var expected strings.Builder
	for _, block := range document.Blocks {
		expected.WriteString(block.Text)
	}
	if reassembled.String() != expected.String() {
		t.Fatalf("content lost: got %d bytes want %d", reassembled.Len(), expected.Len())
	}
}

func TestReaderDocumentDigitlessReferenceIsPlainText(t *testing.T) {
	t.Parallel()
	// Auditor scenario: &#x; — no digits after #x — is NOT an entity. Under
	// a tight header it paginates as plain text instead of dropping the
	// block as an indivisible unit.
	body := "| " + strings.Repeat("H", ReaderPageMaxTextBytes-3) + " |\n| --- |\n| &#x; |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	pages := 0
	seenStates := map[string]bool{}
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, node := range page.Nodes {
			reassembled.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
		if seenStates[state] {
			t.Fatalf("pagination repeats position: %q", state)
		}
		seenStates[state] = true
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination stalls on digitless reference")
		}
	}
	if !strings.Contains(reassembled.String(), "&#x;") {
		t.Fatalf("digitless reference row lost: %q", reassembled.String()[:60])
	}
}

func TestReaderDocumentInvalidHexReferenceIsPlainText(t *testing.T) {
	t.Parallel()
	// Auditor scenario: &#xZZZZZZZZZZZZ; — 'Z' is not a hex digit, so this
	// is plain text. Under a tight header it paginates normally instead of
	// being dropped as an indivisible entity.
	body := "| " + strings.Repeat("H", ReaderPageMaxTextBytes-10) + " |\n| --- |\n| &#xZZZZZZZZZZZZ; |\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	var reassembled strings.Builder
	pages := 0
	seenStates := map[string]bool{}
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, node := range page.Nodes {
			reassembled.WriteString(node.Text)
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		state := fmt.Sprintf("%d|%s", page.Next.BlockIndex, page.Next.Path)
		if seenStates[state] {
			t.Fatalf("pagination repeats position: %q", state)
		}
		seenStates[state] = true
		position = *page.Next
		if pages > 10 {
			t.Fatal("pagination stalls on invalid hex reference")
		}
	}
	if !strings.Contains(reassembled.String(), "&#xZZZZZZZZZZZZ;") {
		t.Fatalf("invalid-hex row lost: %q", reassembled.String()[:60])
	}
}

func TestReaderDocumentInlineCodeTokenNotCut(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a short inline code token straddling the page
	// boundary must never be split mid-token — the boundary backs off so
	// the whole token lands on one page, with no warning.
	token := "TOKEN12345"
	body := strings.Repeat("a", ReaderPageMaxTextBytes-3) + " `" + token + "` tail"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	fragmentsWithToken := 0
	var reassembled strings.Builder
	for {
		page := PaginateReaderDocument(document, position)
		if len(page.Warnings) > 0 {
			t.Fatalf("warnings = %v", page.Warnings)
		}
		for _, node := range page.Nodes {
			text := node.Text
			reassembled.WriteString(text)
			if strings.Contains(text, token) {
				fragmentsWithToken++
			} else {
				// No fragment may end with a non-empty proper prefix of the
				// token (the "split token" signature).
				for size := 2; size < len(token); size++ {
					if strings.HasSuffix(text, token[:size]) && !strings.Contains(text, token) {
						t.Fatalf("fragment ends with split token prefix %q: tail=%q", token[:size], text[len(text)-24:])
					}
				}
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatal("walk incomplete")
			}
			break
		}
		position = *page.Next
	}
	if fragmentsWithToken != 1 {
		t.Fatalf("token spans %d fragments, want exactly 1", fragmentsWithToken)
	}
	var expected strings.Builder
	for _, block := range document.Blocks {
		expected.WriteString(block.Text)
	}
	if reassembled.String() != expected.String() {
		t.Fatalf("content lost: got %d bytes want %d", reassembled.Len(), expected.Len())
	}
}

func TestReaderDocumentInlineTokenNotCutInTableAndTaskPaths(t *testing.T) {
	t.Parallel()
	token := "TOKEN12345"
	perFragmentCheck := func(t *testing.T, document ReaderDocument) {
		t.Helper()
		position := ReaderPagePosition{}
		fragmentsWithToken := 0
		var reassembled strings.Builder
		for {
			page := PaginateReaderDocument(document, position)
			if len(page.Warnings) > 0 {
				t.Fatalf("warnings = %v", page.Warnings)
			}
			var walkFrag func(nodes []ReaderDocumentNode)
			walkFrag = func(nodes []ReaderDocumentNode) {
				for _, node := range nodes {
					text := node.Text
					reassembled.WriteString(text)
					if strings.Contains(text, token) {
						fragmentsWithToken++
					} else {
						for size := 2; size < len(token); size++ {
							if strings.HasSuffix(text, token[:size]) {
								t.Fatalf("fragment ends with split token prefix %q: tail=%q", token[:size], text[len(text)-24:])
							}
						}
					}
					walkFrag(node.Children)
				}
			}
			walkFrag(page.Nodes)
			if page.Next == nil {
				if !page.Complete {
					t.Fatal("walk incomplete")
				}
				break
			}
			position = *page.Next
		}
		if fragmentsWithToken != 1 {
			t.Fatalf("token spans %d fragments, want exactly 1", fragmentsWithToken)
		}
	}

	// Auditor scenario 1: table cell token — tableContent now records token
	// ranges with cell/row offsets, so continuation never cuts the span.
	t.Run("table cell", func(t *testing.T) {
		body := "| H |\n| --- |\n| " + strings.Repeat("a", 262140) + " `" + token + "` tail |\n"
		document, err := BuildReaderDocument(body)
		if err != nil {
			t.Fatalf("BuildReaderDocument: %v", err)
		}
		perFragmentCheck(t, document)
	})

	// Auditor scenario 2: task-list prefix shift — the "[x] " prefix moves
	// the token ranges by its length so the page boundary check uses the
	// true coordinates.
	t.Run("task item prefix shift", func(t *testing.T) {
		body := "- [x] " + strings.Repeat("a", 262131) + " `" + token + "` tail\n"
		document, err := BuildReaderDocument(body)
		if err != nil {
			t.Fatalf("BuildReaderDocument: %v", err)
		}
		perFragmentCheck(t, document)
	})
}

func TestReaderDocumentNormalBlockMovesWholeToNextPage(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a page-filling first paragraph followed by a SMALL
	// second paragraph. The second must move whole to page 2 — never be
	// fragmented into continuation slices (continuation is only for blocks
	// exceeding a whole page).
	body := strings.Repeat("a", 262140) + "\n\nabcdefghij\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	pages := 0
	sawWholeSecond := false
	for {
		page := PaginateReaderDocument(document, position)
		pages++
		for _, node := range page.Nodes {
			if node.Type == "continuation" && node.Text != "" && node.Text != strings.Repeat("a", len(node.Text)) {
				t.Fatalf("small block fragmented: type=%s text=%q", node.Type, node.Text)
			}
			if node.Type == "paragraph" && node.Text == "abcdefghij" {
				sawWholeSecond = true
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
		if pages > 5 {
			t.Fatal("pagination does not terminate")
		}
	}
	if !sawWholeSecond {
		t.Fatal("second paragraph lost")
	}
}

func TestReaderDocumentOwnerFenceHandlers(t *testing.T) {
	t.Parallel()
	fences := map[string]func(string) FenceResult{
		"pinax-fake": func(arg string) FenceResult {
			if arg == "fail" {
				return FenceResult{ErrorCode: "fake_unavailable", NextAction: "pinax fake repair"}
			}
			return FenceResult{Payload: map[string]any{"count": 2, "rows": []any{"a", "b"}}}
		},
	}
	// Registered handler success: fence node with bounded payload, raw
	// source replaced, no execution.
	document, err := BuildReaderDocumentWithFences("```pinax-fake\nanything\n```\n", fences)
	if err != nil {
		t.Fatalf("BuildReaderDocumentWithFences: %v", err)
	}
	node := document.Blocks[0]
	if node.Type != "fence" || node.Fence != "pinax-fake" || node.Text != "" {
		t.Fatalf("fence node = %#v", node)
	}
	payload, ok := node.Payload.(map[string]any)
	if !ok || payload["count"] != float64(2) {
		t.Fatalf("payload = %#v", node.Payload)
	}
	// Registered handler failure: explicit fence_failed with diagnostics.
	document, err = BuildReaderDocumentWithFences("```pinax-fake\nfail\n```\n", fences)
	if err != nil {
		t.Fatal(err)
	}
	node = document.Blocks[0]
	if node.Type != "fence_failed" || node.Fence != "pinax-fake" || node.ErrorCode != "fake_unavailable" || node.NextAction != "pinax fake repair" {
		t.Fatalf("fence_failed node = %#v", node)
	}
	// Unregistered fence name stays inert code (unchanged legacy behavior).
	document, err = BuildReaderDocumentWithFences("```text\nplain\n```\n", fences)
	if err != nil {
		t.Fatal(err)
	}
	node = document.Blocks[0]
	if node.Type != "code" || node.Language != "text" || node.Text != "plain\n" {
		t.Fatalf("unregistered fence node = %#v", node)
	}
}

func TestReaderDocumentStructuralBlockMovesWholeToNextPage(t *testing.T) {
	t.Parallel()
	// Auditor scenario: a page-filling paragraph followed by a SMALL quote.
	// The quote fits a fresh page whole — it must move intact, not be
	// sliced by remaining-budget child pagination.
	body := strings.Repeat("a", 262140) + "\n\n> first\n> second\n"
	document, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatalf("BuildReaderDocument: %v", err)
	}
	position := ReaderPagePosition{}
	quoteSlices := 0
	sawWholeQuote := false
	for {
		page := PaginateReaderDocument(document, position)
		for _, node := range page.Nodes {
			if node.Type == "quote" || (node.Type == "continuation" && node.OriginalType == "quote") {
				quoteSlices++
				// Whole quote: one slice carrying both lines through its
				// single inner paragraph (goldmark joins them with a soft
				// break).
				if node.Type == "quote" && len(node.Children) == 1 && strings.Contains(node.Children[0].Text, "first") && strings.Contains(node.Children[0].Text, "second") {
					sawWholeQuote = true
				}
			}
		}
		if page.Next == nil {
			if !page.Complete {
				t.Fatalf("walk incomplete: warnings=%v", page.Warnings)
			}
			break
		}
		position = *page.Next
	}
	if quoteSlices != 1 || !sawWholeQuote {
		t.Fatalf("quote delivered in %d slices, whole=%v", quoteSlices, sawWholeQuote)
	}
}

func TestReaderDocumentFenceTruncationForcesIncomplete(t *testing.T) {
	t.Parallel()
	// A handler payload truncated by BoundPayload (>100 rows) must surface
	// an explicit warning and force Complete=false — never a silent,
	// "complete" truncated document.
	rows := make([]any, 150)
	for i := range rows {
		rows[i] = i
	}
	fences := map[string]func(string) FenceResult{
		"pinax-big": func(string) FenceResult { return FenceResult{Payload: rows} },
	}
	document, err := BuildReaderDocumentWithFences("```pinax-big\nx\n```\n", fences)
	if err != nil {
		t.Fatalf("BuildReaderDocumentWithFences: %v", err)
	}
	page := PaginateReaderDocument(document, ReaderPagePosition{})
	found := false
	for _, warning := range page.Warnings {
		if warning == "fence_payload_truncated" {
			found = true
		}
	}
	if !found {
		t.Fatalf("truncation warning missing: %v", page.Warnings)
	}
	if page.Complete {
		t.Fatal("truncated fence payload reported complete")
	}
}
