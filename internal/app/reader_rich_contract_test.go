package app

import (
	"strings"
	"testing"
)

func TestReaderDocumentRichContractSemanticChecks(t *testing.T) {
	svc, req := richFixture(t, "```math\nx+y\n```\n\n> [!note]\n> Text\n")
	projection, err := svc.ReaderRichDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	original := projection.Data.(ReaderRichDocumentPage)
	if err := validateRichDocumentPage(original); err != nil {
		t.Fatal("real service output rejected", err)
	}
	for _, mode := range []string{"foreign-anchor", "duplicate", "utf8", "byte-budget", "node-duplicate"} {
		t.Run(mode, func(t *testing.T) {
			page := original
			page.Annotations = append([]ReaderRichAnnotation(nil), original.Annotations...)
			switch mode {
			case "foreign-anchor":
				page.Annotations[0].PageAnchors = []string{"not-delivered"}
			case "duplicate":
				page.Annotations = append(page.Annotations, page.Annotations[0])
			case "utf8":
				page.Annotations[0].Source = string([]byte{0xff})
			case "byte-budget":
				page.Annotations[0].Source = strings.Repeat("字", 23000)
			case "node-duplicate":
				page.Document.Nodes = append(append(page.Document.Nodes[:0:0], page.Document.Nodes...), page.Document.Nodes[0])
			}
			if validateRichDocumentPage(page) == nil {
				t.Fatal("invalid semantic relationship admitted")
			}
		})
	}
	// Empty math fences remain representable even when omitempty omits source.
	svc, req = richFixture(t, "```math\n```\n")
	if _, err := svc.ReaderRichDocument(t.Context(), req); err != nil {
		t.Fatal("empty fence rejected", err)
	}
}
