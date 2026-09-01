package knowledgeops

import "testing"

func TestKnowledgePathAllowedEmptyAllowlist(t *testing.T) {
	t.Parallel()
	if PathAllowed("notes/public.md", nil) {
		t.Fatal("empty allowlist must reject every path")
	}
}

func TestKnowledgeDualConditionSelect(t *testing.T) {
	t.Parallel()
	candidates := []Candidate{
		{Path: "notes/public.md", NoteID: "n1", Title: "Public", Frontmatter: map[string]string{AllowMarkerKey: AllowMarkerValue}, Digest: "sha256:aaa", ChangedAt: "2026-09-01T00:00:00Z"},
		{Path: "notes/path-only.md", NoteID: "n2", Title: "Path only", Frontmatter: map[string]string{}, Digest: "sha256:bbb", ChangedAt: "2026-09-01T00:00:00Z"},
	}
	entries, omitted := SelectCurrent(candidates)
	if omitted != 1 {
		t.Fatalf("omitted = %d, want 1", omitted)
	}
	if len(entries) != 1 || entries[0].Refs.Path != "notes/public.md" {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Digest == "" || entries[0].Permission.Export != "allowlisted" || entries[0].Citation.Path == "" || entries[0].Freshness.ContentDigest == "" {
		t.Fatalf("missing schema fields: %#v", entries[0])
	}
}

func TestKnowledgeAssemblePackageEmptyAllowlist(t *testing.T) {
	t.Parallel()
	pkg := AssemblePackage("2026-09-01T00:00:00Z", nil, 0, 0, []Entry{{Kind: EntryKindNote}}, 0, nil)
	if pkg.EmptyReason != EmptyReasonAllowlistEmpty {
		t.Fatalf("empty reason = %q", pkg.EmptyReason)
	}
	if len(pkg.Entries) != 0 {
		t.Fatalf("empty allowlist must emit zero entries, got %#v", pkg.Entries)
	}
}

func TestKnowledgeDigestDiffTombstoneAndUnchanged(t *testing.T) {
	t.Parallel()
	prior := &Package{Entries: []Entry{
		BuildNoteEntry(Candidate{Path: "notes/keep.md", NoteID: "keep", Title: "Keep", Digest: "sha256:same", ChangedAt: "2026-09-01T00:00:00Z"}),
		BuildNoteEntry(Candidate{Path: "notes/gone.md", NoteID: "gone", Title: "Gone", Digest: "sha256:old", ChangedAt: "2026-09-01T00:00:00Z"}),
	}}
	current := []Entry{
		BuildNoteEntry(Candidate{Path: "notes/keep.md", NoteID: "keep", Title: "Keep", Digest: "sha256:same", ChangedAt: "2026-09-01T00:00:00Z"}),
	}
	entries, unchanged, tombstones := DigestDiff(current, prior)
	if unchanged != 1 || tombstones != 1 {
		t.Fatalf("unchanged=%d tombstones=%d", unchanged, tombstones)
	}
	if len(entries) != 1 || entries[0].Kind != EntryKindTombstone || entries[0].Refs.Path != "notes/gone.md" {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Revocation == nil || entries[0].Revocation.Status != "tombstone" {
		t.Fatalf("missing tombstone revocation: %#v", entries[0])
	}
}
