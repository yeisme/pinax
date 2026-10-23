package render

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// Recognize a complete top-level paragraph only. Raw source, not flattened
// Markdown text, supplies TeX bytes; emphasis parsing must not erase underscores.
func readerDisplayMath(paragraph *ast.Paragraph, source []byte) (string, bool) {
	if paragraph.Parent() == nil || paragraph.Parent().Kind() != ast.KindDocument || paragraph.Lines().Len() == 0 {
		return "", false
	}
	safe := true
	_ = ast.Walk(paragraph, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch node.(type) {
			case *ast.Link, *ast.Image, *ast.AutoLink, *ast.CodeSpan, *ast.RawHTML:
				safe = false
			}
		}
		return ast.WalkContinue, nil
	})
	if !safe {
		return "", false
	}
	raw := string(paragraph.Lines().Value(source))
	raw = strings.TrimSuffix(raw, "\n")
	raw = strings.TrimSuffix(raw, "\r")
	if len(raw) < 5 || !strings.HasPrefix(raw, "$$") || !strings.HasSuffix(raw, "$$") {
		return "", false
	}
	body := raw[2 : len(raw)-2]
	if strings.TrimSpace(body) == "" {
		return "", false
	}
	// Odd backslashes escape a dollar; an escaped closing delimiter is not a
	// complete formula. Multiple unescaped delimiters remain literal.
	slashes := 0
	for _, ch := range body {
		if ch == '\\' {
			slashes++
			continue
		}
		if ch == '$' && slashes%2 == 0 {
			return "", false
		}
		slashes = 0
	}
	if slashes%2 != 0 {
		return "", false
	}
	return body, true
}
