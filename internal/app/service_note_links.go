package app

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func buildNoteLinkGraph(root string) (noteLinkGraph, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return noteLinkGraph{}, err
	}
	byTitle := map[string]domain.Note{}
	byPath := map[string]domain.Note{}
	for _, note := range notes {
		byTitle[strings.ToLower(note.Title)] = note
		byPath[note.Path] = note
	}
	graph := noteLinkGraph{notes: notes, outgoing: map[string][]domain.NoteLink{}, incoming: map[string][]domain.NoteLink{}}
	for _, note := range notes {
		for _, link := range noteGraphLinks(note, byTitle, byPath) {
			graph.outgoing[note.Path] = append(graph.outgoing[note.Path], link)
			if link.TargetPath != "" && !link.Broken {
				graph.incoming[link.TargetPath] = append(graph.incoming[link.TargetPath], link)
			}
		}
	}
	for path := range graph.outgoing {
		sortNoteLinks(graph.outgoing[path])
	}
	for path := range graph.incoming {
		sortNoteLinks(graph.incoming[path])
	}
	return graph, nil
}

func noteGraphLinks(note domain.Note, byTitle map[string]domain.Note, byPath map[string]domain.Note) []domain.NoteLink {
	links := make([]domain.NoteLink, 0)
	seen := map[string]bool{}
	for _, rawTarget := range wikiLinksInBody(note.Body) {
		target := normalizeWikiLinkTarget(rawTarget)
		if target == "" {
			continue
		}
		resolved := byTitle[strings.ToLower(target)]
		link := domain.NoteLink{SourcePath: note.Path, SourceTitle: note.Title, Target: target, Kind: "wiki", Broken: resolved.Path == ""}
		if resolved.Path != "" {
			link.TargetPath = resolved.Path
			link.TargetTitle = resolved.Title
		}
		key := link.Kind + "\x00" + link.Target
		if !seen[key] {
			links = append(links, link)
			seen[key] = true
		}
	}
	for _, rawTarget := range markdownLinksInBody(note.Body) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || !strings.EqualFold(filepath.Ext(target), ".md") {
			continue
		}
		targetPath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(note.Path), target)))
		resolved := byPath[targetPath]
		link := domain.NoteLink{SourcePath: note.Path, SourceTitle: note.Title, Target: target, TargetPath: targetPath, Kind: "markdown", Broken: resolved.Path == ""}
		if resolved.Path != "" {
			link.TargetTitle = resolved.Title
		}
		key := link.Kind + "\x00" + link.TargetPath
		if !seen[key] {
			links = append(links, link)
			seen[key] = true
		}
	}
	return links
}

func markdownLinksInBody(body string) []string {
	links := make([]string, 0)
	seen := map[string]bool{}
	for _, match := range vaultMarkdownLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		links = append(links, target)
	}
	sort.Strings(links)
	return links
}

func normalizeWikiLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if before, _, ok := strings.Cut(target, "|"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func normalizeMarkdownLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
		return ""
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func sortNoteLinks(links []domain.NoteLink) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].SourcePath == links[j].SourcePath {
			return links[i].Target < links[j].Target
		}
		return links[i].SourcePath < links[j].SourcePath
	})
}

func countResolvedLinks(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if !link.Broken {
			count++
		}
	}
	return count
}

func countBrokenLinks(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if link.Broken {
			count++
		}
	}
	return count
}
