package render

import (
	"reflect"
	"strings"
	"testing"
)

func TestReaderDocumentRichInertAnnotations(t *testing.T) {
	body := "# Heading\n\n```mermaid\ngraph TD\n A-->B\n click A \"https://example.invalid\"\n```\n\n```math\n\\frac{a}{b} + \\href{javascript:alert(1)}{x}\n```\n\n> [!WARNING]- Plain title\n> Body <script>bad()</script>\n\n> [!note]\n> Second body\n"
	legacy, err := BuildReaderDocument(body)
	if err != nil {
		t.Fatal(err)
	}
	rich, err := BuildReaderRichDocument(body)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy, rich.Document) {
		t.Fatal("rich parser changed legacy text/nodes/anchors")
	}
	if len(rich.Annotations) != 4 {
		t.Fatalf("annotations=%#v", rich.Annotations)
	}
	for _, annotation := range rich.Annotations {
		if LocateReaderAnchor(rich.Document, annotation.Anchor) < 0 {
			t.Fatal("annotation lost original anchor")
		}
	}
	if rich.Annotations[0].Kind != "mermaid" || !strings.Contains(rich.Annotations[0].Source, "click A") {
		t.Fatal("mermaid source changed")
	}
	if rich.Annotations[1].Kind != "math" || !strings.Contains(rich.Annotations[1].Source, "javascript:") {
		t.Fatal("math source was executed or rewritten")
	}
	if a := rich.Annotations[2]; a.Kind != "callout" || a.CalloutKind != "warning" || a.Fold != "collapsed" || a.Title != "Plain title" {
		t.Fatalf("wrong callout: %#v", a)
	}
	if a := rich.Annotations[3]; a.CalloutKind != "note" || a.Title != "" || a.Fold != "fixed" {
		t.Fatalf("wrong untitled callout: %#v", a)
	}
}

func TestReaderDocumentRichLiteralAndComplexRemainInert(t *testing.T) {
	for _, body := range []string{
		"> \\[!note] escaped\n", "> `[!note]` code\n", "> [!unknown] title\n", "> [!note]joined\n", "> [!note] **bold title**\n",
		"[!note] outside quote\n", "```mermaid extra\ngraph TD\n```\n", "```Mermaid\ngraph TD\n```\n", "```dataview\nTABLE password\n```\n", "Price $5 and $10\n", "> [!note] " + strings.Repeat("x", 513) + "\n",
	} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			doc, err := BuildReaderRichDocument(body)
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Annotations) != 0 {
				t.Fatalf("unsupported literal gained semantics: %#v", doc.Annotations)
			}
		})
	}
}

func TestReaderDocumentRichNestedCalloutAndAnchor(t *testing.T) {
	body := "> [!tip]+ Title\n>\n> > [!warning] Nested\n> > Body\n\n```dataview\nTABLE secret\n```\n"
	doc, err := BuildReaderRichDocument(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Annotations) != 2 {
		t.Fatalf("missing nested callouts: %#v", doc.Annotations)
	}
	seen := map[string]bool{}
	for _, annotation := range doc.Annotations {
		if annotation.Kind != "callout" || seen[annotation.Anchor] || LocateReaderAnchor(doc.Document, annotation.Anchor) != 0 {
			t.Fatal("nested annotation identity drift")
		}
		seen[annotation.Anchor] = true
	}
	if doc.Document.Blocks[len(doc.Document.Blocks)-1].Type != "code" {
		t.Fatal("rich builder invoked a fence handler")
	}
	// The old opt-in fence API still works, but is not shared by rich mode.
	calls := 0
	_, err = BuildReaderDocumentWithFences(body, map[string]func(string) FenceResult{"dataview": func(string) FenceResult { calls++; return FenceResult{} }})
	if err != nil || calls != 1 {
		t.Fatal("legacy explicit fence behavior changed", err)
	}
}

func TestReaderDocumentRichBudgets(t *testing.T) {
	for _, body := range []string{
		"```mermaid\n" + strings.Repeat("x", readerRichMaxPayloadBytes+1) + "\n```",
		strings.Repeat("```math\nx\n```\n\n", readerRichMaxAnnotations+1),
		strings.Repeat("```math\n"+strings.Repeat("x", 60000)+"\n```\n\n", 5),
	} {
		if _, err := BuildReaderRichDocument(body); err == nil {
			t.Fatal("rich annotation budget ignored")
		}
	}
	doc, err := BuildReaderRichDocument(strings.Repeat("```math\nx\n```\n\n", readerRichMaxAnnotations))
	if err != nil || len(doc.Annotations) != readerRichMaxAnnotations {
		t.Fatal("annotation boundary refused", err)
	}
}
