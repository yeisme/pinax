package publishops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestBuildHugoStagingProjectWritesSafeThemeContract(t *testing.T) {
	vaultRoot := t.TempDir()
	stageRoot := filepath.Join(t.TempDir(), "stage")
	writePublishOpsFile(t, vaultRoot, "assets/diagram.png", "fake image")
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Site.Title = "Knowledge Base"
	profile.Site.BaseURL = "https://example.github.io/kb/"
	plan := domain.PublishPlan{
		ProfileName: profile.Name,
		Target:      profile.Target,
		Renderer:    profile.Renderer,
		Selected: []domain.PublishItem{
			{ID: "note_alpha", Kind: "note", Title: "Alpha", SourcePath: "notes/alpha.md", OutputPath: "entries/alpha/index.html"},
			{Kind: "asset", SourcePath: "assets/diagram.png", OutputPath: "assets/diagram.png"},
		},
		Sources:   []domain.PublishSource{{ID: "note_alpha", Title: "Alpha", SourcePath: "notes/alpha.md", Kind: "concept", Status: "active"}},
		LinkGraph: []domain.NoteLink{{SourcePath: "notes/alpha.md", SourceTitle: "Alpha", Target: "Beta", TargetPath: "notes/beta.md", TargetTitle: "Beta", Kind: "wiki", Status: string(domain.LinkStatusResolved)}},
	}
	notes := map[string]domain.Note{"notes/alpha.md": {ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Kind: "concept", Status: "active", Tags: []string{"wiki"}, Body: "# Alpha\n\nPublic body with ![Diagram](../assets/diagram.png)."}}

	result, err := BuildHugoStagingProject(HugoStagingRequest{VaultRoot: vaultRoot, StageRoot: stageRoot, Profile: profile, Plan: plan, Notes: notes})
	if err != nil {
		t.Fatalf("build hugo staging: %v", err)
	}
	if result.StageRoot != stageRoot || result.Theme != "pinax-encyclopedia" || result.FilesWritten == 0 {
		t.Fatalf("staging result = %#v", result)
	}
	for _, rel := range []string{"hugo.yaml", "content/entries/alpha/index.md", "content/indexes/tags/wiki.md", "content/indexes/types/concept.md", "data/pinax/manifest.json", "data/pinax/graph.json", "data/pinax/search-index.json", "data/pinax/taxonomies.json", "data/pinax/sources.json", "data/pinax/build.json", "static/assets/diagram.png", "themes/pinax-encyclopedia/theme.toml", "themes/pinax-encyclopedia/layouts/_default/single.html"} {
		if !fileExists(filepath.Join(stageRoot, filepath.FromSlash(rel))) {
			t.Fatalf("missing staging file %s", rel)
		}
	}
	hugoConfig := mustReadPublishOpsFile(t, filepath.Join(stageRoot, "hugo.yaml"))
	for _, want := range []string{"baseURL: https://example.github.io/kb/", "title: Knowledge Base", "theme: pinax-encyclopedia", "unsafe: false", "pinax.publish_theme.v1"} {
		if !strings.Contains(hugoConfig, want) {
			t.Fatalf("hugo config missing %q:\n%s", want, hugoConfig)
		}
	}
	entry := mustReadPublishOpsFile(t, filepath.Join(stageRoot, "content", "entries", "alpha", "index.md"))
	if !strings.Contains(entry, "schema_version: pinax.publish_entry.v1") || strings.Contains(entry, "PRIVATE_SENTINEL") {
		t.Fatalf("entry content invalid:\n%s", entry)
	}
	if !strings.Contains(mustReadPublishOpsFile(t, filepath.Join(stageRoot, "data", "pinax", "manifest.json")), "note_alpha") {
		t.Fatalf("manifest missing note id")
	}
	if scan, err := ScanPublishTree(stageRoot); err != nil || len(scan.Findings) != 0 {
		t.Fatalf("staging scan findings=%#v err=%v", scan.Findings, err)
	}
}

func TestBuildHugoStagingProjectMaterializesSafeLocalTheme(t *testing.T) {
	vaultRoot := t.TempDir()
	stageRoot := filepath.Join(t.TempDir(), "stage")
	writePublishOpsFile(t, vaultRoot, "themes/local/theme.toml", "name = \"local-reviewable\"\n[params]\ncontract = \"pinax.publish_theme.v1\"\n")
	writePublishOpsFile(t, vaultRoot, "themes/local/layouts/_default/single.html", "{{ define \"main\" }}LOCAL_THEME_MARKER{{ end }}\n")
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Site.Theme.Value = "local:themes/local"
	profile.Site.Theme.ContractVersion = PublishThemeSchemaVersion

	if _, err := BuildHugoStagingProject(HugoStagingRequest{VaultRoot: vaultRoot, StageRoot: stageRoot, Profile: profile, Plan: domain.PublishPlan{}, Notes: nil}); err != nil {
		t.Fatalf("build hugo staging with local theme: %v", err)
	}

	materialized := mustReadPublishOpsFile(t, filepath.Join(stageRoot, "themes", "pinax-encyclopedia", "layouts", "_default", "single.html"))
	if !strings.Contains(materialized, "LOCAL_THEME_MARKER") {
		t.Fatalf("local theme was not materialized:\n%s", materialized)
	}
	if scan, err := ScanPublishTree(filepath.Join(stageRoot, "themes", "pinax-encyclopedia")); err != nil || len(scan.Findings) != 0 {
		t.Fatalf("local theme scan findings=%#v err=%v", scan.Findings, err)
	}
}

func TestBuiltinThemeProvidesEncyclopediaLayouts(t *testing.T) {
	vaultRoot := t.TempDir()
	stageRoot := filepath.Join(t.TempDir(), "stage")
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	plan := domain.PublishPlan{
		ProfileName: profile.Name,
		Target:      profile.Target,
		Renderer:    profile.Renderer,
		Selected: []domain.PublishItem{
			{ID: "note_alpha", Kind: "note", Title: "Alpha", SourcePath: "notes/alpha.md", OutputPath: "entries/alpha/index.html"},
			{ID: "note_beta", Kind: "note", Title: "Beta", SourcePath: "notes/beta.md", OutputPath: "entries/beta/index.html"},
		},
		Sources:   []domain.PublishSource{{ID: "source_one", Title: "Source One", SourcePath: "notes/source.md", Kind: "source", Status: "active"}},
		LinkGraph: []domain.NoteLink{{SourcePath: "notes/alpha.md", SourceTitle: "Alpha", TargetPath: "notes/beta.md", TargetTitle: "Beta", Kind: "wiki", Status: string(domain.LinkStatusResolved)}},
	}
	notes := map[string]domain.Note{
		"notes/alpha.md": {ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Kind: "concept", Status: "active", Tags: []string{"wiki"}, Body: "# Alpha\n\nSee [[Beta]]."},
		"notes/beta.md":  {ID: "note_beta", Title: "Beta", Path: "notes/beta.md", Kind: "project", Status: "active", Tags: []string{"wiki"}, Body: "# Beta"},
	}

	if _, err := BuildHugoStagingProject(HugoStagingRequest{VaultRoot: vaultRoot, StageRoot: stageRoot, Profile: profile, Plan: plan, Notes: notes}); err != nil {
		t.Fatalf("build hugo staging: %v", err)
	}

	themeRoot := filepath.Join(stageRoot, "themes", "pinax-encyclopedia")
	required := map[string][]string{
		"layouts/_default/baseof.html":  {"data/pinax/search-index.json", "data/pinax/graph.json", "noscript", "pinax-search"},
		"layouts/index.html":            {"data/pinax/manifest.json", "data/pinax/taxonomies.json", "Entries", "Search"},
		"layouts/_default/single.html":  {"pinax-entry", "Relations", "Unpublished target"},
		"layouts/_default/list.html":    {"pinax-index", "Pages"},
		"layouts/404.html":              {"Not found", "unpublished"},
		"layouts/partials/sources.html": {"data/pinax/sources.json", "Sources"},
		"assets/js/pinax-search.js":     {"window.PinaxSearch", "search-index.json", "no-js"},
	}
	for rel, wants := range required {
		body := mustReadPublishOpsFile(t, filepath.Join(themeRoot, filepath.FromSlash(rel)))
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Fatalf("theme file %s missing %q:\n%s", rel, want, body)
			}
		}
	}
}

func TestBuiltinThemeUsesLocalAssetsAndStableHTMLStructure(t *testing.T) {
	stageRoot := filepath.Join(t.TempDir(), "stage")
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	if _, err := BuildHugoStagingProject(HugoStagingRequest{VaultRoot: t.TempDir(), StageRoot: stageRoot, Profile: profile, Plan: domain.PublishPlan{}, Notes: nil}); err != nil {
		t.Fatalf("build hugo staging: %v", err)
	}

	themeRoot := filepath.Join(stageRoot, "themes", "pinax-encyclopedia")
	for _, rel := range []string{"layouts/_default/baseof.html", "layouts/index.html", "layouts/_default/single.html", "layouts/partials/head.html", "assets/css/pinax.css", "assets/js/pinax-search.js"} {
		body := mustReadPublishOpsFile(t, filepath.Join(themeRoot, filepath.FromSlash(rel)))
		for _, forbidden := range []string{"https://", "http://", "//cdn", "analytics", "fonts.googleapis", "@font-face", "remote image"} {
			if strings.Contains(strings.ToLower(body), forbidden) {
				t.Fatalf("theme file %s contains forbidden external resource marker %q:\n%s", rel, forbidden, body)
			}
		}
	}

	css := mustReadPublishOpsFile(t, filepath.Join(themeRoot, "assets", "css", "pinax.css"))
	for _, want := range []string{"--pinax-surface", "--pinax-text", "--pinax-border", "--pinax-accent", "--pinax-focus"} {
		if !strings.Contains(css, want) {
			t.Fatalf("theme css missing semantic variable %q:\n%s", want, css)
		}
	}
	base := mustReadPublishOpsFile(t, filepath.Join(themeRoot, "layouts", "_default", "baseof.html"))
	for _, want := range []string{"<main id=\"content\"", "{{ partial \"nav.html\" . }}", "pinax-search-data", "pinax-graph-data"} {
		if !strings.Contains(base, want) {
			t.Fatalf("base layout missing stable structure %q:\n%s", want, base)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
