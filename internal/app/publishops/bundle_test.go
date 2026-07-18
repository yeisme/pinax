package publishops

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestBuildPublishBundleWritesPublishSafeDataAndAssets(t *testing.T) {
	vaultRoot := t.TempDir()
	bundleRoot := filepath.Join(t.TempDir(), "bundle")
	writePublishOpsFile(t, vaultRoot, "assets/diagram.png", "fake image")
	writePublishOpsFile(t, vaultRoot, ".pinax/private.txt", "PINAX_INTERNAL_SENTINEL")
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetLocal, domain.PublishRendererPinaxWeb)
	plan := domain.PublishPlan{
		ProfileName: profile.Name,
		Target:      profile.Target,
		Renderer:    profile.Renderer,
		Selected: []domain.PublishItem{
			{ID: "note_alpha", Kind: "note", Title: "Alpha", SourcePath: "notes/alpha.md", OutputPath: "notes/alpha/index.html"},
			{Kind: "asset", SourcePath: "assets/diagram.png", OutputPath: "assets/diagram.png"},
		},
		Skipped: []domain.PublishItem{
			{ID: "note_private", Kind: "note", Title: "Private", SourcePath: "notes/private.md", Reason: "privacy_excluded"},
			{ID: "note_draft", Kind: "note", Title: "Draft", SourcePath: "notes/draft.md", Reason: "status_not_allowed"},
			{ID: "note_unpublished", Kind: "note", Title: "Unpublished", SourcePath: "notes/unpublished.md", Reason: "publish_value_not_allowed"},
		},
		Sources:   []domain.PublishSource{{ID: "note_alpha", Title: "Alpha", SourcePath: "notes/alpha.md", Kind: "concept", Status: "active"}},
		LinkGraph: []domain.NoteLink{{SourcePath: "notes/alpha.md", SourceTitle: "Alpha", Target: "Beta", TargetPath: "notes/beta.md", TargetTitle: "Beta", Kind: "wiki", Status: string(domain.LinkStatusResolved)}},
	}
	notes := map[string]domain.Note{
		"notes/alpha.md":       {ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Kind: "concept", Status: "active", Tags: []string{"alpha"}, Body: "# Alpha\n\nPublic body."},
		"notes/private.md":     {ID: "note_private", Title: "Private", Path: "notes/private.md", Kind: "concept", Status: "active", Body: "PRIVATE BODY MUST NOT LEAK"},
		"notes/draft.md":       {ID: "note_draft", Title: "Draft", Path: "notes/draft.md", Kind: "concept", Status: "draft", Body: "DRAFT BODY MUST NOT LEAK"},
		"notes/unpublished.md": {ID: "note_unpublished", Title: "Unpublished", Path: "notes/unpublished.md", Kind: "concept", Status: "active", Body: "UNPUBLISHED BODY MUST NOT LEAK"},
		"notes/secret.md":      {ID: "note_secret", Title: "Secret", Path: "notes/secret.md", Kind: "concept", Status: "active", Body: "SECRET BODY MUST NOT LEAK token=RAW_SECRET"},
	}

	result, err := BuildPublishBundle(PublishBundleRequest{VaultRoot: vaultRoot, BundleRoot: bundleRoot, Profile: profile, Plan: plan, Notes: notes})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	if result.BundleRoot != bundleRoot || result.FilesWritten == 0 || result.SelectedNotes != 1 || result.Assets != 1 {
		t.Fatalf("bundle result = %#v", result)
	}
	for _, rel := range []string{"manifest.json", "notes.json", "graph.json", "taxonomies.json", "search-index.json", "sources.json", "assets/diagram.png"} {
		if !fileExists(filepath.Join(bundleRoot, filepath.FromSlash(rel))) {
			t.Fatalf("missing bundle file %s", rel)
		}
	}
	combined := strings.Join([]string{
		mustReadPublishOpsFile(t, filepath.Join(bundleRoot, "manifest.json")),
		mustReadPublishOpsFile(t, filepath.Join(bundleRoot, "notes.json")),
		mustReadPublishOpsFile(t, filepath.Join(bundleRoot, "graph.json")),
		mustReadPublishOpsFile(t, filepath.Join(bundleRoot, "taxonomies.json")),
		mustReadPublishOpsFile(t, filepath.Join(bundleRoot, "search-index.json")),
		mustReadPublishOpsFile(t, filepath.Join(bundleRoot, "sources.json")),
	}, "\n")
	for _, want := range []string{"note_alpha", "Public body", "privacy_excluded", "status_not_allowed", "publish_value_not_allowed"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("bundle missing %q:\n%s", want, combined)
		}
	}
	for _, forbidden := range []string{"PRIVATE BODY MUST NOT LEAK", "DRAFT BODY MUST NOT LEAK", "UNPUBLISHED BODY MUST NOT LEAK", "SECRET BODY MUST NOT LEAK", "PINAX_INTERNAL_SENTINEL", vaultRoot, ".pinax"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("bundle leaked %q:\n%s", forbidden, combined)
		}
	}
}
