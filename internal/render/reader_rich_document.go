package render

import (
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
)

// Rich annotations are owner-internal semantics, not legacy reader wire fields.
// They never authorize HTML, network access, script execution or plugin calls.
type ReaderRichAnnotation struct {
	Anchor      string
	Kind        string // mermaid | math | callout
	Source      string
	CalloutKind string
	Title       string
	Fold        string // fixed | expanded | collapsed
}

type ReaderRichDocument struct {
	Document    ReaderDocument
	Annotations []ReaderRichAnnotation
	Embeds      []ReaderEmbedCandidate
}

const readerRichMaxAnnotations = 512
const readerRichMaxPayloadBytes = 64 * 1024
const readerRichTotalPayloadBytes = 256 * 1024

// BuildReaderRichDocument preserves the same text, nodes and anchors as the
// inert base builder. No fence-handler registry is accepted at this boundary.
func BuildReaderRichDocument(body string) (ReaderRichDocument, error) {
	result := ReaderRichDocument{Annotations: []ReaderRichAnnotation{}, Embeds: []ReaderEmbedCandidate{}}
	document, err := buildReaderDocument(body, nil, &result.Annotations, &result.Embeds)
	if err != nil {
		return ReaderRichDocument{}, err
	}
	result.Document = document
	total := 0
	for _, annotation := range result.Annotations {
		size := len(annotation.Source) + len(annotation.Title)
		if size > readerRichMaxPayloadBytes || size > readerRichTotalPayloadBytes-total {
			return ReaderRichDocument{}, &ReaderBudgetError{Code: "document_budget_exceeded"}
		}
		total += size
	}
	return result, nil
}

func readerRichAnnotation(node ast.Node, source []byte, anchor string) (ReaderRichAnnotation, bool) {
	result := ReaderRichAnnotation{Anchor: anchor}
	switch typed := node.(type) {
	case *ast.Paragraph:
		if math, ok := readerDisplayMath(typed, source); ok {
			result.Kind, result.Source = "math", math
			return result, true
		}
	case *ast.FencedCodeBlock:
		if typed.Info == nil {
			return result, false
		}
		language := strings.TrimSpace(string(typed.Info.Value(source)))
		if language != "mermaid" && language != "math" {
			return result, false
		}
		result.Kind = language
		result.Source = string(typed.Lines().Value(source))
		return result, true
	case *ast.Blockquote:
		paragraph, ok := typed.FirstChild().(*ast.Paragraph)
		if !ok {
			return result, false
		}
		if paragraph.Lines().Len() == 0 {
			return result, false
		}
		// Goldmark may split the marker across multiple plain Text nodes.
		// Require an entirely literal first line rather than one text token.
		for child := paragraph.FirstChild(); child != nil; child = child.NextSibling() {
			part, ok := child.(*ast.Text)
			if !ok {
				return result, false
			}
			if part.SoftLineBreak() || part.HardLineBreak() {
				break
			}
		}
		segment := paragraph.Lines().At(0)
		line := strings.TrimRight(string(segment.Value(source)), "\r\n")
		if !strings.HasPrefix(line, "[!") {
			return result, false
		}
		end := strings.IndexByte(line, ']')
		if end < 3 {
			return result, false
		}
		kind := strings.ToLower(line[2:end])
		switch kind {
		case "note", "abstract", "summary", "tldr", "info", "todo", "tip", "hint", "important", "success", "check", "done", "question", "help", "faq", "warning", "caution", "attention", "failure", "fail", "missing", "danger", "error", "bug", "example", "quote", "cite":
		default:
			return result, false
		}
		rest := line[end+1:]
		fold := "fixed"
		if strings.HasPrefix(rest, "+") {
			fold = "expanded"
			rest = rest[1:]
		} else if strings.HasPrefix(rest, "-") {
			fold = "collapsed"
			rest = rest[1:]
		}
		if rest != "" && rest[0] != ' ' && rest[0] != '\t' && rest[0] != '\r' {
			return result, false
		}
		title := strings.TrimSpace(rest)
		if len(title) > 512 || !utf8.ValidString(title) || strings.ContainsRune(title, 0) {
			return result, false
		}
		result.Kind = "callout"
		result.CalloutKind = kind
		result.Title = title
		result.Fold = fold
		return result, true
	}
	return result, false
}
