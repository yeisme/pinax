package search

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type Result struct {
	Engine string        `json:"engine"`
	Notes  []domain.Note `json:"notes"`
}

func Notes(ctx context.Context, root, query string, fallback []domain.Note) Result {
	if strings.TrimSpace(query) == "" {
		return Result{Engine: "scan", Notes: fallback}
	}
	matches, ok := rgNotes(ctx, root, query, fallback)
	if ok {
		return Result{Engine: "rg", Notes: matches}
	}
	return Result{Engine: "scan", Notes: scanNotes(query, fallback)}
}

func rgNotes(ctx context.Context, root, query string, notes []domain.Note) ([]domain.Note, bool) {
	if _, err := exec.LookPath("rg"); err != nil {
		return nil, false
	}
	cmd := exec.CommandContext(ctx, "rg", "--line-number", "--with-filename", "--glob", "*.md", "--glob", "!.pinax/**", query, root)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return []domain.Note{}, true
		}
		return nil, false
	}
	byPath := map[string]domain.Note{}
	for _, note := range notes {
		byPath[filepath.ToSlash(note.Path)] = note
	}
	seen := map[string]domain.Note{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		file, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if note, ok := byPath[rel]; ok {
			seen[rel] = note
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	matched := make([]domain.Note, 0, len(paths))
	for _, path := range paths {
		matched = append(matched, seen[path])
	}
	return matched, true
}

func scanNotes(query string, notes []domain.Note) []domain.Note {
	query = strings.ToLower(query)
	matched := make([]domain.Note, 0)
	for _, note := range notes {
		haystack := strings.ToLower(note.Title + "\n" + strings.Join(note.Tags, " ") + "\n" + note.Body)
		if strings.Contains(haystack, query) {
			matched = append(matched, note)
		}
	}
	return matched
}
