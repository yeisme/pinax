package render

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// Only a literal terminal marker on a top-level paragraph is admitted.
// Rendered text alone cannot prove this: escapes, links and code spans may
// look identical while having different source semantics.
func readerParagraphBlockID(node ast.Node, source []byte) string {
	paragraph, ok := node.(*ast.Paragraph)
	if !ok || paragraph.Parent() == nil || paragraph.Parent().Kind() != ast.KindDocument {
		return ""
	}
	tail, ok := paragraph.LastChild().(*ast.Text)
	if !ok {
		return ""
	}
	raw := strings.TrimRight(string(tail.Segment.Value(source)), " \t\r\n")
	caret := strings.LastIndexByte(raw, '^')
	if caret < 1 || (raw[caret-1] != ' ' && raw[caret-1] != '\t' && raw[caret-1] != '\n') {
		return ""
	}
	id := raw[caret+1:]
	if len(id) == 0 || len(id) > 64 {
		return ""
	}
	for _, ch := range id {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' {
			return ""
		}
	}
	return id
}
