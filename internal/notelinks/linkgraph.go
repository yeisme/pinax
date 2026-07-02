package notelinks

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

var (
	wikiLinkPattern     = regexp.MustCompile(`(!?)\[\[([^\]]+)\]\]`)
	markdownLinkPattern = regexp.MustCompile(`!?\[([^\]]*)\]\(([^)]+)\)`)
)

// RawLink describes one note-to-note reference parsed from Markdown.
type RawLink struct {
	Kind    string
	Raw     string
	Target  string
	Alias   string
	Heading string
	Line    int
}

// ResolveResult describes one resolved, broken, or ambiguous link edge.
type ResolveResult struct {
	Link       domain.NoteLink
	Candidates []domain.NoteLinkCandidate
}

// ResolverSnapshot stores deterministic note lookup indexes.
type ResolverSnapshot struct {
	byNoteID      map[string]domain.Note
	byPath        map[string]domain.Note
	byTitle       map[string]domain.Note
	byTitleCounts map[string]int
	byAlias       map[string][]domain.Note
	notes         []domain.Note
}

// ParseNoteLinks parses wiki links and relative Markdown links from a note body.
func ParseNoteLinks(body string) []RawLink {
	links := make([]RawLink, 0)
	seen := map[string]bool{}
	line := 0

	for _, bodyLine := range strings.Split(body, "\n") {
		line++
		for _, match := range wikiLinkPattern.FindAllStringSubmatch(bodyLine, -1) {
			if len(match) < 3 {
				continue
			}
			embed := match[1] == "!"
			raw := strings.TrimSpace(match[2])
			if raw == "" {
				continue
			}
			target, alias, heading := SplitWikiLinkParts(raw)
			if target == "" || (embed && isLikelyNonNoteAssetTarget(target)) {
				continue
			}
			key := "wiki\x00" + raw
			if !seen[key] {
				links = append(links, RawLink{Kind: "wiki", Raw: raw, Target: target, Alias: alias, Heading: heading, Line: line})
				seen[key] = true
			}
		}

		for _, match := range markdownLinkPattern.FindAllStringSubmatch(bodyLine, -1) {
			if len(match) < 3 {
				continue
			}
			alias := strings.TrimSpace(match[1])
			rawTarget := strings.TrimSpace(match[2])
			if rawTarget == "" || IsExternalOrHeadingLink(rawTarget) {
				continue
			}
			target, heading := NormalizeMarkdownLinkTarget(rawTarget)
			if target == "" || !strings.EqualFold(filepath.Ext(target), ".md") {
				continue
			}
			key := "markdown\x00" + rawTarget
			if !seen[key] {
				links = append(links, RawLink{Kind: "markdown", Raw: rawTarget, Target: target, Alias: alias, Heading: heading, Line: line})
				seen[key] = true
			}
		}
	}
	return links
}

// SplitWikiLinkParts splits Obsidian wiki link content into target, alias, and heading.
func SplitWikiLinkParts(raw string) (target, alias, heading string) {
	raw = strings.TrimSpace(raw)
	if before, after, ok := strings.Cut(raw, "|"); ok {
		target = strings.TrimSpace(before)
		alias = strings.TrimSpace(after)
	} else {
		target = raw
	}
	if before, after, ok := strings.Cut(target, "#"); ok {
		target = strings.TrimSpace(before)
		heading = strings.TrimSpace(after)
	}
	return target, alias, heading
}

// NormalizeMarkdownLinkTarget removes query and heading fragments from a Markdown target.
func NormalizeMarkdownLinkTarget(target string) (cleanTarget, heading string) {
	target = strings.TrimSpace(target)
	if IsExternalOrHeadingLink(target) {
		return "", ""
	}
	if before, after, ok := strings.Cut(target, "#"); ok {
		target = before
		heading = strings.TrimSpace(after)
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target), heading
}

// IsExternalOrHeadingLink reports links that should not become note graph edges.
func IsExternalOrHeadingLink(target string) bool {
	t := strings.TrimSpace(strings.ToLower(target))
	return strings.HasPrefix(t, "http://") ||
		strings.HasPrefix(t, "https://") ||
		strings.HasPrefix(t, "mailto:") ||
		strings.HasPrefix(t, "#") ||
		strings.HasPrefix(t, "ftp://")
}

// BuildResolverSnapshot builds note lookup indexes for deterministic resolution.
func BuildResolverSnapshot(notes []domain.Note) ResolverSnapshot {
	s := ResolverSnapshot{
		byNoteID:      map[string]domain.Note{},
		byPath:        map[string]domain.Note{},
		byTitle:       map[string]domain.Note{},
		byTitleCounts: map[string]int{},
		byAlias:       map[string][]domain.Note{},
		notes:         notes,
	}
	for _, note := range notes {
		if note.ID != "" {
			s.byNoteID[strings.ToLower(note.ID)] = note
		}
		if note.Path != "" {
			s.byPath[note.Path] = note
			s.byPath[strings.TrimPrefix(note.Path, "notes/")] = note
		}
		lowerTitle := strings.ToLower(note.Title)
		if lowerTitle != "" {
			s.byTitleCounts[lowerTitle]++
			s.byTitle[lowerTitle] = note
		}
		stem := strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path))
		for _, alias := range append([]string{stem}, frontmatterAliases(note.Frontmatter)...) {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key != "" {
				s.byAlias[key] = append(s.byAlias[key], note)
			}
		}
	}
	return s
}

// ResolveLinkTarget resolves a parsed link against a snapshot.
func ResolveLinkTarget(source domain.Note, rawLink RawLink, snap ResolverSnapshot) ResolveResult {
	result := ResolveResult{}
	link := domain.NoteLink{
		SourcePath:    source.Path,
		SourceTitle:   source.Title,
		SourceNoteID:  source.ID,
		Target:        rawLink.Target,
		TargetRaw:     rawLink.Raw,
		TargetAlias:   rawLink.Alias,
		TargetHeading: rawLink.Heading,
		Kind:          rawLink.Kind,
		Line:          rawLink.Line,
	}
	target := rawLink.Target

	if note, ok := snap.byNoteID[strings.ToLower(target)]; ok {
		return resolved(link, note, "resolved by note_id")
	}
	if note, ok := snap.byPath[target]; ok {
		return resolved(link, note, "resolved by path")
	}
	if note, ok := snap.byPath[filepath.ToSlash(filepath.Join("notes", target))]; ok {
		return resolved(link, note, "resolved by path (notes/ prefix)")
	}
	if rawLink.Kind == "markdown" {
		cleanTarget := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(source.Path), target)))
		if note, ok := snap.byPath[cleanTarget]; ok {
			return resolved(link, note, "resolved by relative path")
		}
	}

	lowerTarget := strings.ToLower(target)
	if note, ok := snap.byTitle[lowerTarget]; ok {
		count := snap.byTitleCounts[lowerTarget]
		if count == 1 {
			return resolved(link, note, "resolved by exact title")
		}
		link.Status = string(domain.LinkStatusAmbiguous)
		link.Broken = false
		link.Evidence = fmt.Sprintf("ambiguous: %d notes with same title", count)
		for _, n := range snap.notes {
			if strings.ToLower(n.Title) == lowerTarget {
				result.Candidates = append(result.Candidates, domain.NoteLinkCandidate{Path: n.Path, Title: n.Title, NoteID: n.ID})
			}
		}
		link.Candidates = result.Candidates
		result.Link = link
		return result
	}

	if candidates, ok := snap.byAlias[lowerTarget]; ok && len(candidates) > 0 {
		if len(candidates) == 1 {
			return resolved(link, candidates[0], "resolved by alias/stem")
		}
		link.Status = string(domain.LinkStatusAmbiguous)
		link.Broken = false
		link.Evidence = fmt.Sprintf("ambiguous: %d candidates by alias", len(candidates))
		for _, n := range candidates {
			result.Candidates = append(result.Candidates, domain.NoteLinkCandidate{Path: n.Path, Title: n.Title, NoteID: n.ID})
		}
		link.Candidates = result.Candidates
		result.Link = link
		return result
	}

	link.Status = string(domain.LinkStatusBroken)
	link.Broken = true
	link.Evidence = "target not found"
	result.Link = link
	return result
}

// BuildGraph builds outgoing and incoming note graph edges.
func BuildGraph(notes []domain.Note) (map[string][]domain.NoteLink, map[string][]domain.NoteLink) {
	snap := BuildResolverSnapshot(notes)
	outgoing := map[string][]domain.NoteLink{}
	incoming := map[string][]domain.NoteLink{}
	for _, note := range notes {
		for _, raw := range ParseNoteLinks(note.Body) {
			link := ResolveLinkTarget(note, raw, snap).Link
			outgoing[note.Path] = append(outgoing[note.Path], link)
			if link.Status == string(domain.LinkStatusResolved) && link.TargetPath != "" {
				incoming[link.TargetPath] = append(incoming[link.TargetPath], link)
			}
		}
		SortNoteLinks(outgoing[note.Path])
	}
	for path := range incoming {
		SortNoteLinks(incoming[path])
	}
	return outgoing, incoming
}

// SortNoteLinks sorts graph edges deterministically.
func SortNoteLinks(links []domain.NoteLink) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].SourcePath == links[j].SourcePath {
			if links[i].Target == links[j].Target {
				return links[i].TargetRaw < links[j].TargetRaw
			}
			return links[i].Target < links[j].Target
		}
		return links[i].SourcePath < links[j].SourcePath
	})
}

func resolved(link domain.NoteLink, note domain.Note, evidence string) ResolveResult {
	link.TargetPath = note.Path
	link.TargetTitle = note.Title
	link.TargetNoteID = note.ID
	link.Status = string(domain.LinkStatusResolved)
	link.Broken = false
	link.Evidence = evidence
	return ResolveResult{Link: link}
}

func frontmatterAliases(fm map[string]string) []string {
	aliases := make([]string, 0)
	for _, key := range []string{"alias", "aliases"} {
		aliases = append(aliases, splitAliasValue(fm[key])...)
	}
	return aliases
}

func splitAliasValue(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.Trim(raw, "[]")
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
	aliases := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		if part != "" {
			aliases = append(aliases, part)
		}
	}
	return aliases
}

func isLikelyNonNoteAssetTarget(target string) bool {
	ext := strings.ToLower(filepath.Ext(target))
	return ext != "" && ext != ".md"
}
