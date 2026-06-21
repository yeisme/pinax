package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/templateengine"
)

func TestBuiltInTemplateLegacyAndRecommendedInspect(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	legacy, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "daily"})
	if err != nil {
		t.Fatalf("inspect legacy daily: %v", err)
	}
	if legacy.Facts["template"] != "daily" || legacy.Facts["engine"] != "simple" || legacy.Facts["issues"] != "0" {
		t.Fatalf("legacy daily inspect facts = %#v", legacy.Facts)
	}

	journal, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "journal.daily"})
	if err != nil {
		t.Fatalf("inspect journal daily: %v", err)
	}
	if journal.Facts["kind"] != "journal_template" || journal.Facts["path_pattern"] != "daily/{{ .Date }}.md" || journal.Facts["managed_blocks"] != "1" {
		t.Fatalf("journal daily inspect facts = %#v", journal.Facts)
	}

	index, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "index.home"})
	if err != nil {
		t.Fatalf("inspect index home: %v", err)
	}
	if index.Facts["kind"] != "index_template" || index.Facts["path_pattern"] != "index/home.md" || index.Facts["managed_blocks"] != "1" {
		t.Fatalf("index home inspect facts = %#v", index.Facts)
	}
}

func TestBuiltInNoteTemplatesCatalogMetadata(t *testing.T) {
	required := []string{"note.quick", "inbox.capture", "meeting.notes", "decision.record", "project.brief", "learning.video", "learning.book", "research.topic", "source.github", "person.profile"}
	for _, name := range required {
		body, ok := builtInTemplates()[name]
		if !ok {
			t.Fatalf("missing built-in note template %s", name)
		}
		doc, err := templateengine.ParseDocument(name, body)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if doc.Metadata.Name != name || doc.Metadata.Kind != "note_template" || doc.Metadata.Output.PathPattern == "" {
			t.Fatalf("identity/output metadata for %s = %#v", name, doc.Metadata)
		}
		if len(doc.Metadata.UseCases) == 0 || len(doc.Metadata.Aliases) == 0 || doc.Metadata.Difficulty == "" || doc.Metadata.Starter == nil || len(doc.Metadata.Defaults) == 0 {
			t.Fatalf("catalog metadata for %s = %#v", name, doc.Metadata)
		}
	}
}

func TestBuiltInNoteTemplateMetadataAppliesToCreateNote(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	checks := []struct {
		template   string
		title      string
		vars       map[string]string
		pathPrefix string
		kind       string
		status     string
		tags       []string
	}{
		{template: "inbox.capture", title: "Later idea", pathPrefix: "inbox/", kind: "inbox", status: "inbox"},
		{template: "meeting.notes", title: "客户同步", pathPrefix: "meetings/", kind: "meeting", status: "active"},
		{template: "decision.record", title: "选择同步策略", pathPrefix: "decisions/", kind: "decision", status: "active"},
		{template: "source.github", title: "iptv-org/iptv", vars: map[string]string{"url": "https://github.com/iptv-org/iptv"}, pathPrefix: "sources/github/", kind: "source", status: "active", tags: []string{"source/github", "reference/source"}},
	}
	for _, check := range checks {
		projection, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: check.title, Template: check.template, Vars: check.vars})
		if err != nil {
			t.Fatalf("create %s: %v", check.template, err)
		}
		if !strings.HasPrefix(projection.Facts["path"], check.pathPrefix) || projection.Facts["kind"] != check.kind || projection.Facts["status"] != check.status || projection.Facts["template"] != check.template {
			t.Fatalf("template %s facts = %#v", check.template, projection.Facts)
		}
		if len(check.tags) > 0 && projection.Facts["tags"] != strings.Join(check.tags, ",") {
			t.Fatalf("template %s tags = %#v", check.template, projection.Facts)
		}
	}

	override, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Later idea", Template: "inbox.capture", Dir: "custom", Kind: "reference", Status: "active"})
	if err != nil {
		t.Fatalf("create override: %v", err)
	}
	if !strings.HasPrefix(override.Facts["path"], "notes/custom/") || override.Facts["kind"] != "reference" || override.Facts["status"] != "active" || override.Facts["template.defaults_source"] != "inbox.capture" || override.Facts["template.overrides"] == "" {
		t.Fatalf("override facts = %#v", override.Facts)
	}
}

func TestDurableSourceCandidateDetection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	note, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "iptv-org/iptv", Body: "# iptv-org/iptv\n\nSource: https://github.com/iptv-org/iptv\n", Kind: "reference", Tags: []string{"github"}, Dir: "research"})
	if err != nil {
		t.Fatalf("create source candidate note: %v", err)
	}
	plan, err := svc.PlanOrganize(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("organize plan: %v", err)
	}
	ops := projectionPlanOperations(t, plan)
	sourceMove := findPlanOperation(ops, "source_move")
	if sourceMove == nil {
		t.Fatalf("missing source_move operation: %#v", ops)
	}
	if sourceMove.Path != note.Facts["path"] || sourceMove.Target != "sources/github/iptv-org-iptv.md" || sourceMove.Status != "manual_review" {
		t.Fatalf("source_move = %#v, note facts=%#v", sourceMove, note.Facts)
	}
	if !strings.Contains(strings.Join(sourceMove.Evidence, ","), "source_url=https://github.com/iptv-org/iptv") {
		t.Fatalf("source_move evidence = %#v", sourceMove.Evidence)
	}
}

func TestMetadataPlanSuggestsDurableSourceFields(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "iptv-org/iptv", Body: "# iptv-org/iptv\n\nSource: https://github.com/iptv-org/iptv\n", Kind: "reference", Tags: []string{"github"}, Dir: "research"}); err != nil {
		t.Fatalf("create source candidate note: %v", err)
	}

	plan, err := svc.PlanMetadata(ctx, VaultRequest{VaultPath: root, Query: "iptv-org/iptv"})
	if err != nil {
		t.Fatalf("metadata plan: %v", err)
	}
	if plan.Facts["writes"] != "false" {
		t.Fatalf("metadata plan writes fact = %#v", plan.Facts)
	}
	sourceMetadata := findPlanOperation(projectionPlanOperations(t, plan), "source_metadata")
	if sourceMetadata == nil {
		t.Fatalf("missing source_metadata operation: %#v", plan.Data)
	}
	for _, want := range []string{"source_url=https://github.com/iptv-org/iptv", "kind=source", "tags=source/github,reference/source", "last_checked_at=<review>", "source_license=<review>", "review_after=<review>"} {
		if !strings.Contains(sourceMetadata.Target, want) {
			t.Fatalf("source_metadata target missing %q: %#v", want, sourceMetadata)
		}
	}
	if sourceMetadata.Status != "manual_review" {
		t.Fatalf("source_metadata must remain manual review: %#v", sourceMetadata)
	}
}

func TestOrganizePlanSuggestsDurableSourceLayout(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "iptv-org/iptv", Body: "# iptv-org/iptv\n\nSource: https://github.com/iptv-org/iptv\n", Kind: "reference", Tags: []string{"github"}, Dir: "research"}); err != nil {
		t.Fatalf("create source candidate note: %v", err)
	}

	plan, err := svc.PlanOrganize(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("organize plan: %v", err)
	}
	ops := projectionPlanOperations(t, plan)
	sourceMove := findPlanOperation(ops, "source_move")
	sourceReview := findPlanOperation(ops, "source_review")
	if sourceMove == nil || sourceReview == nil {
		t.Fatalf("durable source organize operations missing: %#v", ops)
	}
	if sourceMove.Target != "sources/github/iptv-org-iptv.md" || sourceMove.Status != "manual_review" {
		t.Fatalf("source_move = %#v", sourceMove)
	}
	for _, want := range []string{"Use decision", "Risk and boundary", "Verification", "Related notes"} {
		if !strings.Contains(sourceReview.Target, want) {
			t.Fatalf("source_review target missing %q: %#v", want, sourceReview)
		}
	}
	if sourceReview.Status != "manual_review" || !strings.Contains(sourceReview.Reason, "Missing durable source sections") {
		t.Fatalf("source_review = %#v", sourceReview)
	}
}

func projectionPlanOperations(t *testing.T, projection domain.Projection) []domain.PlanOperation {
	t.Helper()
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("projection data has type %T: %#v", projection.Data, projection.Data)
	}
	ops, ok := data["operations"].([]domain.PlanOperation)
	if !ok {
		t.Fatalf("projection operations have type %T: %#v", data["operations"], data["operations"])
	}
	return ops
}

func findPlanOperation(ops []domain.PlanOperation, kind string) *domain.PlanOperation {
	for i := range ops {
		if ops[i].Kind == kind {
			return &ops[i]
		}
	}
	return nil
}

func TestBuiltInIndexTemplatesCatalogMetadata(t *testing.T) {
	required := []string{"index.decisions", "index.learning", "index.meetings", "index.research"}
	for _, name := range required {
		body, ok := builtInTemplates()[name]
		if !ok {
			t.Fatalf("missing built-in index template %s", name)
		}
		doc, err := templateengine.ParseDocument(name, body)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if doc.Metadata.Name != name || doc.Metadata.Kind != "index_template" || doc.Metadata.Output.PathPattern == "" {
			t.Fatalf("index metadata for %s = %#v", name, doc.Metadata)
		}
		blocks, err := templateengine.InspectManagedBlocks(doc.Body)
		if err != nil {
			t.Fatalf("inspect blocks for %s: %v", name, err)
		}
		if len(blocks) == 0 || len(doc.Metadata.Queries) == 0 {
			t.Fatalf("index template %s blocks=%#v queries=%#v", name, blocks, doc.Metadata.Queries)
		}
		for queryName, query := range doc.Metadata.Queries {
			if query.MaxRows <= 0 || query.SQL == "" {
				t.Fatalf("query %s for %s is not bounded: %#v", queryName, name, query)
			}
		}
	}
}

func TestIndexDecisionsLearningMeetingsResearchPreview(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	for _, item := range []struct {
		name string
		path string
	}{
		{name: "decisions", path: "index/decisions.md"},
		{name: "learning", path: "index/learning.md"},
		{name: "meetings", path: "index/meetings.md"},
		{name: "research", path: "index/research.md"},
	} {
		projection, err := svc.PreviewIndexPage(ctx, IndexPageRequest{VaultPath: root, Name: item.name})
		if err != nil {
			t.Fatalf("preview %s: %v", item.name, err)
		}
		if projection.Facts["path"] != item.path || projection.Facts["query_count"] == "0" || projection.Facts["managed_blocks"] == "0" {
			t.Fatalf("preview %s facts = %#v", item.name, projection.Facts)
		}
	}
}

func TestSystemIndexNoteIndexPageExcludedFromOrdinaryResults(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Alpha", Slug: "alpha", Body: "body"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := svc.CreateIndexPage(ctx, IndexPageRequest{VaultPath: root, Name: "home"}); err != nil {
		t.Fatalf("create index page: %v", err)
	}
	lookup, err := svc.IndexLookup(ctx, IndexLookupRequest{VaultPath: root, Query: "home", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("lookup index page: %v", err)
	}
	if lookup.Facts["candidates"] != "0" {
		t.Fatalf("index page should be excluded from registered note lookup: %#v", lookup)
	}
	orphans, err := svc.NoteOrphans(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("note orphans: %v", err)
	}
	if strings.Contains(fmt.Sprint(orphans.Data), "index/home.md") {
		t.Fatalf("index page should be excluded from orphans: %#v", orphans.Data)
	}
}

func TestTemplateInspectUseCasesManagedBlocks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	meeting, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting.notes"})
	if err != nil {
		t.Fatalf("inspect meeting: %v", err)
	}
	if meeting.Facts["use_cases"] == "" || meeting.Facts["aliases"] == "" || meeting.Facts["difficulty"] != "focused" || meeting.Facts["starter"] != "false" || meeting.Facts["after_create_action_count"] == "0" {
		t.Fatalf("meeting inspect facts = %#v", meeting.Facts)
	}
	index, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "index.home"})
	if err != nil {
		t.Fatalf("inspect index: %v", err)
	}
	if index.Facts["managed_blocks"] != "1" || index.Facts["refreshable"] != "true" || index.Facts["after_create_action_count"] == "0" {
		t.Fatalf("index inspect facts = %#v", index.Facts)
	}
}

func TestTemplatePreviewJournal(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	projection, err := svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "journal.daily"})
	if err != nil {
		t.Fatalf("preview journal: %v", err)
	}
	if projection.Facts["template"] != "journal.daily" || projection.Facts["query_count"] != "0" || !strings.Contains(fmt.Sprint(projection.Data), "daily-captures") {
		t.Fatalf("journal preview projection = %#v", projection)
	}
}

func TestTemplatePreviewIndexQuery(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	projection, err := svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "index.decisions"})
	if err != nil {
		t.Fatalf("preview index query: %v", err)
	}
	if projection.Facts["query_count"] != "1" || !strings.Contains(fmt.Sprint(projection.Data), "recent-decisions") {
		t.Fatalf("index preview projection = %#v", projection)
	}

	bad := strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: index_template", "engine: go-template", "queries:", "  broken:", "    language: sql", "    required: true", "    max_rows: 1", "    sql: SELECT missing FROM nowhere LIMIT 1", "---", "# Bad"}, "\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "bad-query", Body: bad}); err != nil {
		t.Fatalf("create bad query template: %v", err)
	}
	_, err = svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "bad-query"})
	if !hasCommandCode(err, "template_query_execute_failed") || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("bad query err = %v", err)
	}
	cmdErr := err.(*domain.CommandError)
	if !strings.Contains(cmdErr.Hint, "query explain") || !strings.Contains(cmdErr.Hint, "index sync") {
		t.Fatalf("bad query hint = %q", cmdErr.Hint)
	}
}

func TestTemplatePreviewQueryBackedMissingIndexIsReadOnly(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove index: %v", err)
	}
	projection, err := svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "index.decisions"})
	if !hasCommandCode(err, "template_index_required") {
		t.Fatalf("preview missing index should fail with template_index_required: projection=%#v err=%v", projection, err)
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "index.sqlite")) {
		t.Fatalf("template preview created index.sqlite")
	}
	if len(projection.Actions) == 0 || !strings.Contains(projection.Actions[0].Command, "pinax index rebuild --vault") {
		t.Fatalf("preview missing index action = %#v", projection.Actions)
	}
}

func TestTemplateNextAction(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	projection, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "journal.daily"})
	if err != nil {
		t.Fatalf("inspect journal: %v", err)
	}
	if len(projection.Actions) == 0 || !strings.Contains(projection.Actions[0].Command, "pinax journal daily show --template journal.daily") {
		t.Fatalf("inspect actions = %#v", projection.Actions)
	}
	if projection.Facts["after_create_action_count"] != "1" {
		t.Fatalf("inspect action facts = %#v", projection.Facts)
	}
}

func TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	starter, err := svc.ListTemplateCatalog(ctx, TemplateRequest{VaultPath: root, Pack: "starter"})
	if err != nil {
		t.Fatalf("list starter: %v", err)
	}
	if starter.Facts["templates"] == "0" || !strings.Contains(fmt.Sprint(starter.Data), "note.quick") {
		t.Fatalf("starter list = %#v", starter)
	}
	meeting, err := svc.RecommendTemplate(ctx, TemplateRequest{VaultPath: root, Intent: "meeting"})
	if err != nil {
		t.Fatalf("recommend meeting: %v", err)
	}
	if meeting.Facts["primary"] != "meeting.notes" {
		t.Fatalf("meeting recommendation = %#v", meeting)
	}
	fallback, err := svc.RecommendTemplate(ctx, TemplateRequest{VaultPath: root, Intent: "unknown-intent"})
	if err != nil {
		t.Fatalf("recommend fallback: %v", err)
	}
	if fallback.Facts["primary"] != "note.quick" && fallback.Facts["primary"] != "inbox.capture" {
		t.Fatalf("fallback recommendation = %#v", fallback)
	}
}
