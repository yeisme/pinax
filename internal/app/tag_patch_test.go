package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagNotePreservesExactSource(t *testing.T) {
	s, root, path := collaborationFixture(t)
	full := filepath.Join(root, path)
	original, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Replace(string(original), "---\n", "---\n# keep\ncustom: {nested: [one, two]}\nquoted: 'untouched' # comment\n", 1)
	end := strings.Index(source[4:], "\n---") + 4
	source = source[:end+5] + "\n\n\n  exact body\r\n\n"
	if err := os.WriteFile(full, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	_, beforeBody := splitFrontmatter(source)
	if _, err := s.TagNote(context.Background(), NoteTagRequest{VaultPath: root, NoteRef: path, Operation: "add", Tags: []string{"reviewed"}}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	_, afterBody := splitFrontmatter(string(after))
	if beforeBody != afterBody {
		t.Fatalf("body changed: before=%q after=%q", beforeBody, afterBody)
	}
	for _, line := range []string{"# keep\n", "custom: {nested: [one, two]}\n", "quoted: 'untouched' # comment\n"} {
		if !strings.Contains(string(after), line) {
			t.Fatalf("untouched metadata changed: %q", line)
		}
	}
}

func TestTagNoteRejectsUnsafeRepresentation(t *testing.T) {
	for _, metadata := range []string{"tags:\n  - one\n", "tags: [one]\ntags: [two]\n", "tags: &t [one]\ncopy: *t\n", "tags: [one]\ncustom:\n  tags: nested\nupdated_at: |\n  yesterday\n", "tags: |\n", "tags: [one]\n---suffix: invalid\n", "tags: [one]\ncustom: first\u2028second\n"} {
		t.Run(metadata, func(t *testing.T) {
			s, root, path := collaborationFixture(t)
			full := filepath.Join(root, path)
			raw, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			kept := lines[:0]
			for _, line := range lines {
				if !strings.HasPrefix(line, "tags:") && !strings.HasPrefix(line, "updated_at:") {
					kept = append(kept, line)
				}
			}
			source := strings.Replace(strings.Join(kept, "\n"), "---\n", "---\n"+metadata, 1)
			if err := os.WriteFile(full, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.WriterPropose(context.Background(), root, CollaborationChange{Action: "tags", NoteRef: path, TagOperation: "set", Tags: []string{"reviewed"}}); err == nil {
				t.Fatal("unsafe proposal accepted")
			}
			if _, err := s.TagNote(context.Background(), NoteTagRequest{VaultPath: root, NoteRef: path, Operation: "set", Tags: []string{"reviewed"}}); err == nil {
				t.Fatal("unsafe representation accepted")
			}
			after, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != source {
				t.Fatal("rejected source changed")
			}
		})
	}
}
