package app

import (
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/notelinks"
)

type publishDocCrossDocResult struct {
	Body    string
	Summary domain.PublishDocCrossDocSummary
}

// publishDocResolveCrossDocLinks 把 note 正文里对其他 note 的引用改写为目标 note 的飞书文档 URL。
// 仅改写已发布到同一 target 且 mapping 有 URL 的引用；未发布/未解析/歧义引用保留原样。
func publishDocResolveCrossDocLinks(root string, source domain.Note, body string, target domain.PublishDocTarget) (string, int) {
	result := publishDocAnalyzeCrossDocLinks(root, source, body, target)
	return result.Body, result.Summary.Rewritten
}

func publishDocAnalyzeCrossDocLinks(root string, source domain.Note, body string, target domain.PublishDocTarget) publishDocCrossDocResult {
	result := publishDocCrossDocResult{Body: body}
	if strings.TrimSpace(body) == "" {
		return result
	}
	notes, err := scanNotes(root)
	if err != nil || len(notes) == 0 {
		return result
	}
	snap := notelinks.BuildResolverSnapshot(notes)
	rawLinks := notelinks.ParseNoteLinks(body)
	if len(rawLinks) == 0 {
		return result
	}
	rewritten := body
	for _, raw := range rawLinks {
		link := publishDocResolveOneCrossDocLink(root, source, raw, snap, target, rewritten)
		result.Summary.Total++
		result.Summary.Links = append(result.Summary.Links, link)
		switch link.Status {
		case "rewritten":
			result.Summary.Rewritten += link.Occurrences
			if raw.Kind == "wiki" {
				rewritten = strings.ReplaceAll(rewritten, "[["+raw.Raw+"]]", "["+link.Label+"]("+link.URL+")")
			} else {
				rewritten = strings.ReplaceAll(rewritten, "]("+raw.Raw+")", "]("+link.URL+")")
			}
		case "unpublished":
			result.Summary.Unpublished++
		case "ambiguous":
			result.Summary.Ambiguous++
		case "broken":
			result.Summary.Broken++
		case "self":
			result.Summary.Self++
		}
	}
	result.Body = rewritten
	return result
}

func publishDocResolveOneCrossDocLink(root string, source domain.Note, raw notelinks.RawLink, snap notelinks.ResolverSnapshot, target domain.PublishDocTarget, body string) domain.PublishDocCrossDocLink {
	resolved := notelinks.ResolveLinkTarget(source, raw, snap)
	link := domain.PublishDocCrossDocLink{Kind: raw.Kind, Raw: raw.Raw, Label: strings.TrimSpace(raw.Alias), SourceNoteID: source.ID}
	if len(resolved.Candidates) > 0 {
		link.Candidates = make([]string, 0, len(resolved.Candidates))
		for _, candidate := range resolved.Candidates {
			value := candidate.NoteID
			if candidate.Path != "" {
				value = fmt.Sprintf("%s (%s)", candidate.NoteID, candidate.Path)
			}
			link.Candidates = append(link.Candidates, value)
		}
	}
	if resolved.Link.Status != string(domain.LinkStatusResolved) || resolved.Link.TargetNoteID == "" {
		if len(resolved.Candidates) > 1 || resolved.Link.Status == string(domain.LinkStatusAmbiguous) {
			link.Status = "ambiguous"
		} else {
			link.Status = "broken"
		}
		return link
	}
	link.TargetNoteID = resolved.Link.TargetNoteID
	link.TargetTitle = resolved.Link.TargetTitle
	link.Label = crossDocLinkLabel(raw, resolved)
	if resolved.Link.TargetNoteID == source.ID {
		link.Status = "self"
		return link
	}
	mapping, err := readPublishDocMapping(root, resolved.Link.TargetNoteID, target)
	if err != nil || !publishDocMappingCanRewriteCrossDoc(mapping) {
		link.Status = "unpublished"
		return link
	}
	link.URL = mapping.ExternalObject.URL
	link.Occurrences = publishDocCrossDocOccurrenceCount(raw, body)
	if link.Occurrences == 0 {
		link.Status = "broken"
		return link
	}
	link.Status = "rewritten"
	return link
}

func publishDocMappingCanRewriteCrossDoc(mapping domain.PublishDocMapping) bool {
	if strings.TrimSpace(mapping.ExternalObject.URL) == "" {
		return false
	}
	switch mapping.PublishStatus {
	case domain.PublishDocStatusPublished, domain.PublishDocStatusLinked:
		return true
	default:
		return false
	}
}

func publishDocCrossDocOccurrenceCount(raw notelinks.RawLink, body string) int {
	if raw.Kind == "wiki" {
		return strings.Count(body, "[["+raw.Raw+"]]")
	}
	return strings.Count(body, "]("+raw.Raw+")")
}

// crossDocLinkLabel 选择引用的显示文字：优先 alias，其次目标标题，最后 target/ID。
func crossDocLinkLabel(raw notelinks.RawLink, result notelinks.ResolveResult) string {
	if label := strings.TrimSpace(raw.Alias); label != "" {
		return label
	}
	if title := strings.TrimSpace(result.Link.TargetTitle); title != "" {
		return title
	}
	if target := strings.TrimSpace(raw.Target); target != "" {
		return target
	}
	return result.Link.TargetNoteID
}
