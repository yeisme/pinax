package vaultops

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type Fact struct {
	Note           domain.Note
	Meta           map[string]string
	Rel            string
	ModTime        time.Time
	Size           int64
	HasFrontmatter bool
}

var inlineTagPattern = regexp.MustCompile(`(^|\s)#([\pL\pN_/-]+)`)

func Stats(root string, facts []Fact, elapsed time.Duration) domain.VaultStats {
	dirs := map[string]int{}
	facts = OrdinaryFacts(facts)
	tags := map[string]bool{}
	frontmatterReady := 0
	recentUpdates := 0
	notes := make([]domain.NoteStat, 0, len(facts))
	for _, fact := range facts {
		dir := filepath.ToSlash(filepath.Dir(fact.Rel))
		if dir == "." {
			dir = "/"
		}
		dirs[dir]++
		for _, tag := range noteAllTags(fact.Note) {
			tags[tag] = true
		}
		if fact.Meta["schema_version"] == "pinax.note.v1" && fact.Meta["note_id"] != "" {
			frontmatterReady++
		}
		if time.Since(fact.ModTime) <= 7*24*time.Hour {
			recentUpdates++
		}
		notes = append(notes, domain.NoteStat{ID: fact.Note.ID, Title: fact.Note.Title, Path: fact.Rel, Tags: fact.Note.Tags, HasFrontmatter: fact.HasFrontmatter, UpdatedAt: fact.ModTime.UTC().Format(time.RFC3339), SizeBytes: fact.Size})
	}
	coverage := 0
	if len(facts) > 0 {
		coverage = frontmatterReady * 100 / len(facts)
	}
	status, indexPath := indexStatus(root, facts)
	return domain.VaultStats{VaultPath: root, NoteCount: len(facts), TagCount: len(tags), DirectoryCounts: dirs, FrontmatterCoverage: coverage, RecentUpdates: recentUpdates, ScanDurationMillis: elapsed.Milliseconds(), IndexStatus: status, IndexPath: indexPath, Notes: notes}
}

func OrdinaryFacts(facts []Fact) []Fact {
	ordinary := make([]Fact, 0, len(facts))
	for _, fact := range facts {
		if fact.Note.Kind == "asset" || strings.HasPrefix(fact.Rel, ".pinax/") {
			continue
		}
		ordinary = append(ordinary, fact)
	}
	return ordinary
}

func indexStatus(root string, facts []Fact) (string, string) {
	path := filepath.Join(root, ".pinax", "index.sqlite")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	}
	if err != nil {
		return "unreadable", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	}
	for _, fact := range facts {
		if fact.ModTime.After(info.ModTime()) {
			return "stale", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
		}
	}
	return "fresh", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
}

func noteAllTags(note domain.Note) []string {
	seen := map[string]bool{}
	for _, tag := range note.Tags {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
		if tag != "" {
			seen[tag] = true
		}
	}
	for _, match := range inlineTagPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) > 2 && match[2] != "" {
			seen[match[2]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
