package render

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
)

// An unresolved syntactic candidate, not permission to read a target. Asset
// versus note identity, fragments and visibility belong to the app resolver.
type ReaderEmbedCandidate struct {
	Anchor string
	Raw    string
}

func readerEmbedCandidate(node ast.Node, source []byte, anchor string) (ReaderEmbedCandidate, bool) {
	paragraph, ok := node.(*ast.Paragraph)
	if !ok || paragraph.Parent() == nil || paragraph.Parent().Kind() != ast.KindDocument || paragraph.Lines().Len() != 1 {
		return ReaderEmbedCandidate{}, false
	}
	for child := paragraph.FirstChild(); child != nil; child = child.NextSibling() {
		if _, ok := child.(*ast.Text); !ok {
			return ReaderEmbedCandidate{}, false
		}
	}
	raw := strings.Trim(string(paragraph.Lines().Value(source)), " \t\r\n")
	if !strings.HasPrefix(raw, "![[") || !strings.HasSuffix(raw, "]]") {
		return ReaderEmbedCandidate{}, false
	}
	inner := raw[3 : len(raw)-2]
	if strings.TrimSpace(inner) == "" || !utf8.ValidString(inner) || strings.ContainsAny(inner, "[]\\") {
		return ReaderEmbedCandidate{}, false
	}
	for _, r := range inner {
		if unicode.IsControl(r) {
			return ReaderEmbedCandidate{}, false
		}
	}
	return ReaderEmbedCandidate{Anchor: anchor, Raw: raw}, true
}
