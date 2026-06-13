package index

import (
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestLookupAPIMatchesRegisteredAdoptableAssetsAliasAndContent(t *testing.T) {
	root := t.TempDir()
	writeIndexFixture(t, filepath.Join(root, "notes", "alpha-note.md"), "# Alpha")
	writeIndexFixture(t, filepath.Join(root, "notes", "draft-idea.md"), "# Draft")
	writeIndexFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	notes := []domain.Note{{
		ID:    "note_alpha",
		Title: "Alpha Title",
		Path:  "notes/alpha-note.md",
		Body:  "alias:: hidden-alpha\nThe body contains deep lookup phrase.\n![Diagram](../assets/diagram.png)\n",
	}}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	registered, err := Lookup(root, LookupRequest{Query: "Alpha Title", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("lookup registered: %v", err)
	}
	if registered.Status.Status != "fresh" || len(registered.Candidates) != 1 || registered.Candidates[0].Path != "notes/alpha-note.md" || registered.Candidates[0].ManagedStatus != domain.ManagedStatusRegistered || !candidateHasField(registered.Candidates[0], domain.MatchFieldTitle) {
		t.Fatalf("registered lookup = %#v", registered)
	}

	alias, err := Lookup(root, LookupRequest{Query: "hidden-alpha", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("lookup alias: %v", err)
	}
	if len(alias.Candidates) != 1 || !candidateHasField(alias.Candidates[0], domain.MatchFieldAlias) {
		t.Fatalf("alias lookup = %#v", alias)
	}

	content, err := Lookup(root, LookupRequest{Query: "deep lookup phrase", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("lookup content: %v", err)
	}
	if len(content.Candidates) != 1 || !candidateHasField(content.Candidates[0], domain.MatchFieldContent) {
		t.Fatalf("content lookup = %#v", content)
	}

	adoptable, err := Lookup(root, LookupRequest{Query: "draft-idea", Scope: "adoptable", Kind: "file"})
	if err != nil {
		t.Fatalf("lookup adoptable: %v", err)
	}
	if len(adoptable.Candidates) != 1 || adoptable.Candidates[0].ObjectKind != domain.VaultObjectKindFile || adoptable.Candidates[0].ManagedStatus != domain.ManagedStatusAdoptable || adoptable.Candidates[0].Path != "notes/draft-idea.md" {
		t.Fatalf("adoptable lookup = %#v", adoptable)
	}

	all, err := Lookup(root, LookupRequest{Query: "draft-idea", Scope: "all", Kind: "all"})
	if err != nil {
		t.Fatalf("lookup all: %v", err)
	}
	if len(all.Candidates) != 1 || all.Candidates[0].ManagedStatus != domain.ManagedStatusAdoptable {
		t.Fatalf("all lookup = %#v", all)
	}

	asset, err := Lookup(root, LookupRequest{Query: "diagram", Scope: "assets", Kind: "asset"})
	if err != nil {
		t.Fatalf("lookup asset: %v", err)
	}
	if len(asset.Candidates) != 1 || asset.Candidates[0].ObjectKind != domain.VaultObjectKindAsset || asset.Candidates[0].Path != "assets/diagram.png" || asset.Candidates[0].MediaType == "" {
		t.Fatalf("asset lookup = %#v", asset)
	}
}

func TestLookupRankingPrefersExactIdentityFieldsOverContent(t *testing.T) {
	root := t.TempDir()
	writeIndexFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha")
	writeIndexFixture(t, filepath.Join(root, "notes", "content.md"), "# Content")
	writeIndexFixture(t, filepath.Join(root, "assets", "alpha.png"), "png")
	notes := []domain.Note{
		{ID: "note_alpha", Title: "Other", Path: "notes/alpha.md", Body: "body"},
		{ID: "note_content", Title: "Content", Path: "notes/content.md", Body: "alpha appears only in content"},
	}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	all, err := Lookup(root, LookupRequest{Query: "alpha", Scope: "all", Kind: "all"})
	if err != nil {
		t.Fatalf("lookup ranking: %v", err)
	}
	if len(all.Candidates) < 3 {
		t.Fatalf("ranking candidates = %#v", all)
	}
	for _, candidate := range all.Candidates[:2] {
		if !candidateHasField(candidate, domain.MatchFieldStem) || candidate.Score <= all.Candidates[len(all.Candidates)-1].Score {
			t.Fatalf("identity matches should rank before content matches: %#v", all.Candidates)
		}
	}
	last := all.Candidates[len(all.Candidates)-1]
	if last.Path != "notes/content.md" || !candidateHasField(last, domain.MatchFieldContent) {
		t.Fatalf("content match should rank behind identity matches: %#v", all.Candidates)
	}
}

func candidateHasField(candidate domain.VaultObjectCandidate, field domain.MatchField) bool {
	for _, got := range candidate.MatchFields {
		if got == field {
			return true
		}
	}
	return false
}
