package app

import (
	"net/url"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/render"
)

// Heading and block references use the owner's inert parser/anchor registry,
// never fence handlers or a recursive cross-note resolver. Cache only outline
// and explicit-block indexes, with shared source/node budgets across targets.
type readerHeadingRefs struct {
	outlines    map[string][]render.ReaderOutlineEntry
	blocks      map[string]map[string]string
	sourceBytes int
	nodes       int
}

func (r *readerHeadingRefs) uri(note domain.Note, heading string) (string, bool, error) {
	base := "pinax://note/" + note.ID + "?v=" + readerNoteRevision(note)
	if heading == "" {
		return base, true, nil
	}
	if len(heading) > 512 {
		return "", false, nil
	}
	decoded, err := url.PathUnescape(heading)
	if err != nil {
		return "", false, nil
	}
	if r.outlines == nil {
		r.outlines = map[string][]render.ReaderOutlineEntry{}
		r.blocks = map[string]map[string]string{}
	}
	key := note.ID + "\x00" + readerNoteRevision(note)
	outline, exists := r.outlines[key]
	if !exists {
		if note.OversizedSource || len(r.outlines) >= 32 || len(note.Body) > render.ReaderSourceMaxBytes-r.sourceBytes {
			return "", false, &render.ReaderBudgetError{Code: "document_budget_exceeded"}
		}
		doc, err := render.BuildReaderDocument(note.Body)
		if err != nil {
			return "", false, err
		}
		r.sourceBytes += len(note.Body)
		r.nodes += doc.NodeCount
		if r.nodes >= render.ReaderTotalMaxNodes {
			return "", false, &render.ReaderBudgetError{Code: "document_budget_exceeded"}
		}
		outline = doc.Outline
		r.outlines[key] = outline
		r.blocks[key] = doc.BlockRefs
	}
	if strings.HasPrefix(decoded, "^") {
		anchor := r.blocks[key][strings.TrimPrefix(decoded, "^")]
		if anchor == "" {
			return "", false, nil
		}
		return base + "&anchor=" + url.QueryEscape(anchor), true, nil
	}
	// Exact duplicate titles are ambiguous even if the slug happens to match
	// the first heading. Explicit generated anchors remain selectable.
	anchor := ""
	matches := 0
	for _, entry := range outline {
		if entry.Title == decoded {
			matches++
			anchor = entry.Anchor
		}
	}
	if matches > 1 {
		return "", false, nil
	}
	if matches == 0 {
		for _, entry := range outline {
			if entry.Anchor == decoded {
				anchor = entry.Anchor
				break
			}
		}
	}
	if anchor == "" {
		return "", false, nil
	}
	return base + "&anchor=" + url.QueryEscape(anchor), true, nil
}
