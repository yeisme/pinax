package publishops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const PublishThemeSchemaVersion = "pinax.publish_theme.v1"

type HugoStagingRequest struct {
	VaultRoot string
	StageRoot string
	Profile   domain.PublishProfile
	Plan      domain.PublishPlan
	Notes     map[string]domain.Note
}

type HugoStagingResult struct {
	StageRoot    string `json:"stage_root"`
	Theme        string `json:"theme"`
	FilesWritten int    `json:"files_written"`
}

func BuildHugoStagingProject(req HugoStagingRequest) (HugoStagingResult, error) {
	if strings.TrimSpace(req.StageRoot) == "" {
		return HugoStagingResult{}, fmt.Errorf("hugo staging root required")
	}
	result := HugoStagingResult{StageRoot: req.StageRoot, Theme: BuiltinThemeName}
	if err := os.MkdirAll(req.StageRoot, 0o755); err != nil {
		return HugoStagingResult{}, err
	}
	write := func(rel string, body []byte) error {
		result.FilesWritten++
		return writeStagingFile(req.StageRoot, rel, body)
	}
	if err := write("hugo.yaml", []byte(hugoConfig(req.Profile))); err != nil {
		return HugoStagingResult{}, err
	}
	for _, note := range stagingSelectedNotes(req.Plan, req.Notes) {
		if err := write(filepath.ToSlash(filepath.Join("content", "entries", slugForPath(note.Path), "index.md")), []byte(hugoEntryMarkdown(note))); err != nil {
			return HugoStagingResult{}, err
		}
	}
	for tag, notes := range stagingTags(req.Plan, req.Notes) {
		if err := write(filepath.ToSlash(filepath.Join("content", "indexes", "tags", slugString(tag)+".md")), []byte(hugoIndexMarkdown("Tag: "+tag, notes))); err != nil {
			return HugoStagingResult{}, err
		}
	}
	for kind, notes := range stagingKinds(req.Plan, req.Notes) {
		if err := write(filepath.ToSlash(filepath.Join("content", "indexes", "types", slugString(kind)+".md")), []byte(hugoIndexMarkdown("Type: "+kind, notes))); err != nil {
			return HugoStagingResult{}, err
		}
	}
	for _, item := range req.Plan.Selected {
		if item.Kind != "asset" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(req.VaultRoot, filepath.FromSlash(item.SourcePath)))
		if err != nil {
			return HugoStagingResult{}, err
		}
		if err := write(filepath.ToSlash(filepath.Join("static", filepath.FromSlash(item.SourcePath))), body); err != nil {
			return HugoStagingResult{}, err
		}
	}
	dataFiles, err := stagingDataFiles(req)
	if err != nil {
		return HugoStagingResult{}, err
	}
	for rel, body := range dataFiles {
		if err := write(filepath.ToSlash(filepath.Join("data", "pinax", rel)), body); err != nil {
			return HugoStagingResult{}, err
		}
	}
	themeFiles, err := resolveThemeFiles(req.VaultRoot, req.Profile.Site.Theme.Value)
	if err != nil {
		return HugoStagingResult{}, err
	}
	for rel, body := range themeFiles {
		if err := write(filepath.ToSlash(filepath.Join("themes", BuiltinThemeName, rel)), body); err != nil {
			return HugoStagingResult{}, err
		}
	}
	return result, nil
}

func writeStagingFile(root, rel string, body []byte) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func hugoConfig(profile domain.PublishProfile) string {
	baseURL := strings.TrimSpace(profile.Site.BaseURL)
	if baseURL == "" {
		baseURL = "/"
	}
	title := strings.TrimSpace(profile.Site.Title)
	if title == "" {
		title = profile.Name
	}
	return fmt.Sprintf("baseURL: %s\ntitle: %s\ntheme: pinax-encyclopedia\nmarkup:\n  goldmark:\n    renderer:\n      unsafe: false\nparams:\n  pinax_theme_contract: %s\ndisableKinds:\n  - RSS\n  - sitemap\n", baseURL, title, PublishThemeSchemaVersion)
}

func hugoEntryMarkdown(note domain.Note) string {
	return fmt.Sprintf("---\nschema_version: pinax.publish_entry.v1\ntitle: %s\nnote_id: %s\ntype: %s\ntags: [%s]\n---\n\n%s\n", note.Title, note.ID, defaultString(note.Kind, "note"), quotedCSV(note.Tags), strings.TrimSpace(note.Body))
}

func hugoIndexMarkdown(title string, notes []domain.Note) string {
	var b strings.Builder
	b.WriteString("---\nschema_version: pinax.publish_index.v1\ntitle: ")
	b.WriteString(title)
	b.WriteString("\n---\n\n# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, note := range notes {
		b.WriteString("- ")
		b.WriteString(note.Title)
		b.WriteString("\n")
	}
	return b.String()
}

func stagingDataFiles(req HugoStagingRequest) (map[string][]byte, error) {
	manifest := domain.PublishManifest{SchemaVersion: "pinax.publish_manifest.v1", ProfileName: req.Profile.Name, Target: req.Profile.Target, Renderer: string(req.Profile.Renderer), Items: req.Plan.Selected}
	graph := map[string]any{"schema_version": PublishThemeSchemaVersion, "links": req.Plan.LinkGraph}
	search := map[string]any{"schema_version": PublishThemeSchemaVersion, "entries": stagingSearchEntries(req.Plan, req.Notes)}
	taxonomies := map[string]any{"schema_version": PublishThemeSchemaVersion, "tags": stagingTagCounts(req.Plan, req.Notes), "types": stagingKindCounts(req.Plan, req.Notes)}
	sources := map[string]any{"schema_version": PublishThemeSchemaVersion, "sources": req.Plan.Sources}
	build := map[string]any{"schema_version": PublishThemeSchemaVersion, "profile": req.Profile.Name, "target": req.Profile.Target, "renderer": req.Profile.Renderer, "generated_at": time.Now().UTC().Format(time.RFC3339)}
	objects := map[string]any{"manifest.json": manifest, "graph.json": graph, "search-index.json": search, "taxonomies.json": taxonomies, "sources.json": sources, "build.json": build}
	out := make(map[string][]byte, len(objects))
	for rel, value := range objects {
		body, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		out[rel] = append(body, '\n')
	}
	return out, nil
}

func stagingSelectedNotes(plan domain.PublishPlan, notes map[string]domain.Note) []domain.Note {
	selected := make([]domain.Note, 0)
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		if note, ok := notes[item.SourcePath]; ok {
			selected = append(selected, note)
		}
	}
	return selected
}

func stagingTags(plan domain.PublishPlan, notes map[string]domain.Note) map[string][]domain.Note {
	groups := map[string][]domain.Note{}
	for _, note := range stagingSelectedNotes(plan, notes) {
		for _, tag := range note.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				groups[tag] = append(groups[tag], note)
			}
		}
	}
	return groups
}

func stagingKinds(plan domain.PublishPlan, notes map[string]domain.Note) map[string][]domain.Note {
	groups := map[string][]domain.Note{}
	for _, note := range stagingSelectedNotes(plan, notes) {
		kind := defaultString(note.Kind, "note")
		groups[kind] = append(groups[kind], note)
	}
	return groups
}

func stagingTagCounts(plan domain.PublishPlan, notes map[string]domain.Note) map[string]int {
	counts := map[string]int{}
	for tag, group := range stagingTags(plan, notes) {
		counts[tag] = len(group)
	}
	return counts
}

func stagingKindCounts(plan domain.PublishPlan, notes map[string]domain.Note) map[string]int {
	counts := map[string]int{}
	for kind, group := range stagingKinds(plan, notes) {
		counts[kind] = len(group)
	}
	return counts
}

func stagingSearchEntries(plan domain.PublishPlan, notes map[string]domain.Note) []map[string]string {
	entries := make([]map[string]string, 0)
	for _, note := range stagingSelectedNotes(plan, notes) {
		entries = append(entries, map[string]string{"id": note.ID, "title": note.Title, "path": "entries/" + slugForPath(note.Path) + "/"})
	}
	return entries
}

const BuiltinThemeName = "pinax-encyclopedia"

func resolveThemeFiles(vaultRoot, source string) (map[string][]byte, error) {
	source = strings.TrimSpace(source)
	if source == "" || source == "builtin:"+BuiltinThemeName {
		files := builtinThemeFiles()
		out := make(map[string][]byte, len(files))
		for rel, body := range files {
			out[rel] = []byte(body)
		}
		return out, nil
	}
	if !strings.HasPrefix(source, "local:") {
		return nil, fmt.Errorf("unsupported publish theme source %q", source)
	}
	themeRoot, err := cleanLocalThemeRoot(vaultRoot, strings.TrimPrefix(source, "local:"))
	if err != nil {
		return nil, err
	}
	return collectThemeFiles(themeRoot)
}

func cleanLocalThemeRoot(vaultRoot, rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if !safeRelativePath(rel) {
		return "", fmt.Errorf("local publish theme path is unsafe")
	}
	rootAbs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", err
	}
	themeAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(rootAbs, themeAbs)
	if err != nil {
		return "", err
	}
	inside = filepath.ToSlash(inside)
	if strings.HasPrefix(inside, "../") || inside == ".." || strings.HasPrefix(inside, ".pinax/") || inside == ".pinax" {
		return "", fmt.Errorf("local publish theme path escapes the vault")
	}
	info, err := os.Stat(themeAbs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("local publish theme path must be a directory")
	}
	return themeAbs, nil
}

func collectThemeFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !safeRelativePath(rel) {
			return fmt.Errorf("local publish theme file path is unsafe")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = body
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("local publish theme has no files")
	}
	return files, nil
}

func BuiltinThemeInfos() []domain.PublishThemeInfo {
	return []domain.PublishThemeInfo{{
		Name:            BuiltinThemeName,
		Source:          "builtin:" + BuiltinThemeName,
		ContractVersion: PublishThemeSchemaVersion,
		RequiredLayouts: []string{"layouts/_default/baseof.html", "layouts/_default/single.html", "layouts/_default/list.html", "layouts/index.html", "layouts/404.html"},
	}}
}

func WriteBuiltinTheme(name, outDir string) ([]string, error) {
	if strings.TrimSpace(name) != BuiltinThemeName {
		return nil, fmt.Errorf("unknown built-in publish theme %q", name)
	}
	if strings.TrimSpace(outDir) == "" {
		return nil, fmt.Errorf("theme output directory required")
	}
	files := builtinThemeFiles()
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if err := writeStagingFile(outDir, rel, []byte(files[rel])); err != nil {
			return nil, err
		}
	}
	return rels, nil
}

func builtinThemeFiles() map[string]string {
	return map[string]string{
		"theme.toml": `name = "pinax-encyclopedia"
[params]
contract = "pinax.publish_theme.v1"
`,
		"layouts/_default/baseof.html": `<!doctype html>
<html class="no-js" lang="{{ site.LanguageCode | default "en" }}">
<head>
  {{ partial "head.html" . }}
  {{ with resources.Get "css/pinax.css" }}<link rel="stylesheet" href="{{ .RelPermalink }}">{{ end }}
</head>
<body>
  <a class="skip-link" href="#content">Skip to content</a>
  {{ partial "nav.html" . }}
  <main id="content" class="pinax-shell">
    {{ block "main" . }}{{ end }}
  </main>
  {{ partial "sources.html" . }}
  <script id="pinax-search-data" type="application/json" data-source="data/pinax/search-index.json">{{ index site.Data.pinax "search-index" | jsonify }}</script>
  <script id="pinax-graph-data" type="application/json" data-source="data/pinax/graph.json">{{ site.Data.pinax.graph | jsonify }}</script>
  {{ with resources.Get "js/pinax-search.js" }}<script defer src="{{ .RelPermalink }}"></script>{{ end }}
  <noscript><section class="pinax-no-js"><h2>Search</h2><p>JavaScript is disabled. Use the entries, tags, types and sources indexes below.</p></section></noscript>
</body>
</html>
`,
		"layouts/index.html": `{{ define "main" }}
<section class="pinax-home">
  <h1>{{ site.Title }}</h1>
  <form class="pinax-search" role="search" data-source="data/pinax/search-index.json">
    <label for="pinax-search-input">Search</label>
    <input id="pinax-search-input" type="search" autocomplete="off">
    <output id="pinax-search-results"></output>
  </form>
  <section class="pinax-entry-list">
    <h2>Entries</h2>
    {{ $manifest := site.Data.pinax.manifest }}
    <ul data-source="data/pinax/manifest.json">
      {{ range $manifest.items }}{{ if eq .kind "note" }}<li><a href="{{ relURL .output_path }}">{{ .title }}</a></li>{{ end }}{{ end }}
    </ul>
  </section>
  <section class="pinax-taxonomies" data-source="data/pinax/taxonomies.json">
    <h2>Tags</h2>
    <ul>{{ range $name, $count := site.Data.pinax.taxonomies.tags }}<li><a href="{{ relURL (printf "indexes/tags/%s/" ($name | urlize)) }}">{{ $name }}</a> <span>{{ $count }}</span></li>{{ end }}</ul>
    <h2>Types</h2>
    <ul>{{ range $name, $count := site.Data.pinax.taxonomies.types }}<li><a href="{{ relURL (printf "indexes/types/%s/" ($name | urlize)) }}">{{ $name }}</a> <span>{{ $count }}</span></li>{{ end }}</ul>
  </section>
  {{ partial "sources.html" . }}
</section>
{{ end }}
`,
		"layouts/_default/single.html": `{{ define "main" }}
<article class="pinax-entry">
  <header><p class="pinax-kind">{{ .Type | default "entry" }}</p><h1>{{ .Title }}</h1></header>
  <section class="pinax-entry-body">{{ .Content }}</section>
  <section class="pinax-relations" data-source="data/pinax/graph.json">
    <h2>Relations</h2>
    <ul>
      {{ $current := .File.Path }}
      {{ range site.Data.pinax.graph.links }}{{ if or (eq .source_path $current) (eq .source_title $.Title) }}<li>{{ .target_title | default .target }} <span>{{ .status }}</span></li>{{ end }}{{ end }}
    </ul>
    <p class="pinax-placeholder">Unpublished target links are shown as plain text.</p>
  </section>
</article>
{{ end }}
`,
		"layouts/_default/list.html": `{{ define "main" }}
<section class="pinax-index">
  <h1>{{ .Title }}</h1>
  <div>{{ .Content }}</div>
  <h2>Pages</h2>
  <ul>{{ range .Pages }}<li><a href="{{ .RelPermalink }}">{{ .Title }}</a></li>{{ end }}</ul>
</section>
{{ end }}
`,
		"layouts/404.html": `{{ define "main" }}
<section class="pinax-not-found">
  <h1>Not found</h1>
  <p>The requested published page may be unpublished or absent from this build.</p>
  <p><a href="{{ relURL "" }}">Return to index</a></p>
</section>
{{ end }}
`,
		"layouts/partials/head.html": `<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="pinax-theme-contract" content="pinax.publish_theme.v1">
<title>{{ if .Title }}{{ .Title }} · {{ end }}{{ site.Title }}</title>
`,
		"layouts/partials/nav.html": `<nav class="pinax-nav" aria-label="Primary">
  <a href="{{ relURL "" }}">Home</a>
  <a href="{{ relURL "indexes/tags/" }}">Tags</a>
  <a href="{{ relURL "indexes/types/" }}">Types</a>
</nav>
`,
		"layouts/partials/sources.html": `<section class="pinax-sources" data-source="data/pinax/sources.json">
  <h2>Sources</h2>
  <ul>{{ range site.Data.pinax.sources.sources }}<li>{{ .title }} <span>{{ .kind }}</span></li>{{ end }}</ul>
</section>
`,
		"assets/css/pinax.css": `:root{--pinax-bg:#f8fafc;--pinax-surface:#ffffff;--pinax-text:#111827;--pinax-muted:#64748b;--pinax-accent:#2563eb;--pinax-border:#cbd5e1;--pinax-focus:#0f766e;}*{box-sizing:border-box;}body{margin:0;background:var(--pinax-bg);color:var(--pinax-text);font-family:system-ui,sans-serif;line-height:1.55;}a{color:var(--pinax-accent);}a:focus,input:focus{outline:2px solid var(--pinax-focus);outline-offset:2px;}.pinax-shell{max-width:72rem;margin:0 auto;padding:1.5rem;}.pinax-nav{display:flex;gap:1rem;border-bottom:1px solid var(--pinax-border);padding:1rem 1.5rem;background:var(--pinax-surface);}.pinax-nav a{text-decoration:none;}.pinax-search input{width:min(36rem,100%);padding:.55rem;border:1px solid var(--pinax-border);background:var(--pinax-surface);color:var(--pinax-text);}.pinax-entry,.pinax-index,.pinax-home,.pinax-not-found,.pinax-sources{background:var(--pinax-surface);border:1px solid var(--pinax-border);border-radius:6px;padding:1rem;margin-block:1rem;}.pinax-kind,.pinax-placeholder,.pinax-sources span{color:var(--pinax-muted);}.pinax-no-js{border:1px dashed var(--pinax-border);padding:1rem;margin:1rem 0;}.skip-link{position:absolute;left:-999px;}.skip-link:focus{left:1rem;top:1rem;background:var(--pinax-surface);padding:.5rem;}
`,
		"assets/js/pinax-search.js": `(function(){
  var root = document.documentElement;
  root.className = root.className.replace(/\bno-js\b/, 'js');
  window.PinaxSearch = { ready: true };
  var form = document.querySelector('.pinax-search');
  if (!form) return;
  var input = form.querySelector('input[type="search"]');
  var output = document.getElementById('pinax-search-results');
  var payload = document.getElementById('pinax-search-data');
  var data = [];
  try { data = JSON.parse(payload ? payload.textContent : '{}').entries || []; } catch (err) { data = []; }
  form.setAttribute('data-loaded-from', 'search-index.json');
  input.addEventListener('input', function(){
    var q = input.value.toLowerCase();
    var matches = data.filter(function(item){ return item.title && item.title.toLowerCase().indexOf(q) >= 0; }).slice(0, 8);
    output.innerHTML = matches.map(function(item){ return '<a href="' + item.path + '">' + item.title + '</a>'; }).join('');
  });
})();
`,
	}
}

func slugForPath(path string) string {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return slugString(stem)
}

func slugString(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "item"
	}
	return slug
}

func quotedCSV(values []string) string {
	if len(values) == 0 {
		return ""
	}
	items := append([]string(nil), values...)
	sort.Strings(items)
	for i, item := range items {
		items[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(items, ", ")
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
