package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func TestVaultInitValidateSearchAndShow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "我的知识库"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "project.md"), "# Pinax 推进\n\n整理本地知识库。\n")

	validation, err := svc.ValidateVault(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("validate vault: %v", err)
	}
	if validation.Facts["notes"] != "1" {
		t.Fatalf("notes fact = %#v", validation.Facts)
	}
	data, ok := validation.Data.(map[string]any)
	if !ok {
		t.Fatalf("validation data = %#v", validation.Data)
	}
	issues, ok := data["issues"].([]domain.Issue)
	if !ok {
		t.Fatalf("validation issues = %#v", data["issues"])
	}
	if len(issues) == 0 {
		t.Fatalf("expected missing metadata issue")
	}

	search, err := svc.SearchNotes(ctx, SearchRequest{VaultPath: root, Query: "知识库"})
	if err != nil {
		t.Fatalf("search notes: %v", err)
	}
	if len(search.Notes) != 1 || search.Notes[0].Title != "Pinax 推进" {
		t.Fatalf("search notes = %#v", search.Notes)
	}

	shown, err := svc.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "project.md"})
	if err != nil {
		t.Fatalf("show note: %v", err)
	}
	if !strings.Contains(shown.Body, "整理本地知识库") {
		t.Fatalf("show body = %q", shown.Body)
	}
}

func TestProjectRegistryCreateListAndSwitch(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	created, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", Description: "长期研究", NotesPrefix: "notes/research"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Command != "project.create" || created.Facts["project"] != "research" {
		t.Fatalf("created projection = %#v", created)
	}

	listed, err := svc.ListProjects(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if listed.Facts["projects"] != "1" || listed.Facts["current_project"] != "research" {
		t.Fatalf("list facts = %#v", listed.Facts)
	}

	if _, err := svc.SwitchProject(ctx, ProjectRequest{VaultPath: root, Slug: "research"}); err != nil {
		t.Fatalf("switch project: %v", err)
	}
	asset := readFile(t, filepath.Join(root, ".pinax", "projects.json"))
	var registry map[string]any
	if err := json.Unmarshal([]byte(asset), &registry); err != nil {
		t.Fatalf("projects asset invalid: %v\n%s", err, asset)
	}
	if registry["schema_version"] != "pinax.projects.v1" || registry["current_project"] != "research" {
		t.Fatalf("projects registry = %#v", registry)
	}
}

func TestProjectCreateRejectsUnsafePrefixAndConflicts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "bad", Name: "Bad", NotesPrefix: "../outside"}); err == nil {
		t.Fatalf("unsafe project prefix succeeded")
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "notes/research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "冲突", NotesPrefix: "notes/other"}); err == nil {
		t.Fatalf("conflicting project create succeeded")
	}
}

func TestStorageS3ConfigurationAndDoctor(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	configured, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/", Profile: "work"})
	if err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if configured.Facts["backend"] != "s3" || configured.Facts["bucket"] != "notes" {
		t.Fatalf("configured facts = %#v", configured.Facts)
	}
	asset := readFile(t, filepath.Join(root, ".pinax", "storage.json"))
	if strings.Contains(strings.ToLower(asset), "secret") || strings.Contains(strings.ToLower(asset), "access_key") {
		t.Fatalf("storage asset appears to contain secret material:\n%s", asset)
	}

	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("storage doctor: %v", err)
	}
	if doctor.Facts["backend"] != "s3" || doctor.Facts["credential_source"] != "profile:work" {
		t.Fatalf("doctor facts = %#v", doctor.Facts)
	}
	data, ok := doctor.Data.(map[string]any)
	if !ok {
		t.Fatalf("doctor data = %#v", doctor.Data)
	}
	profile, ok := data["storage"].(domain.StorageProfile)
	if !ok {
		t.Fatalf("doctor storage data = %#v", data["storage"])
	}
	if profile.Local != nil {
		t.Fatalf("s3 storage doctor retained local profile: %#v", profile.Local)
	}
}

func TestMetadataApplyRequiresApprovalAndWritesFrontmatter(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	note := filepath.Join(root, "notes", "raw.md")
	writeFile(t, note, "# Raw Note\n\nbody\n")

	plan, err := svc.PlanMetadata(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("plan metadata: %v", err)
	}
	if plan.Facts["planned_updates"] != "1" {
		t.Fatalf("metadata plan facts = %#v", plan.Facts)
	}

	if _, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root}); err == nil {
		t.Fatalf("metadata apply without approval succeeded")
	}
	if _, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root, Yes: true}); err != nil {
		t.Fatalf("metadata apply: %v", err)
	}
	content := readFile(t, note)
	for _, want := range []string{"schema_version: pinax.note.v1", "note_id:", "title: Raw Note", "tags: []"} {
		if !strings.Contains(content, want) {
			t.Fatalf("metadata content missing %q:\n%s", want, content)
		}
	}
}

func TestOrganizeApplyRequiresSnapshotAndMovesInsideVault(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	source := filepath.Join(root, "Inbox Note.md")
	writeFile(t, source, "# Inbox Note\n\nbody\n")

	plan, err := svc.PlanOrganize(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("plan organize: %v", err)
	}
	if plan.Facts["planned_moves"] != "1" {
		t.Fatalf("organize plan facts = %#v", plan.Facts)
	}

	if _, err := svc.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, Yes: true}); err == nil {
		t.Fatalf("organize apply without snapshot succeeded")
	}
	if _, err := svc.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, Yes: true, SnapshotMessage: "整理前快照"}); err != nil {
		t.Fatalf("organize apply with snapshot: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists or stat error was %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "inbox-note.md")); err != nil {
		t.Fatalf("organized note missing: %v", err)
	}
}

func TestCoreNoteTemplateIndexAndSyncMVP(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "notes/research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.InitTemplates(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("init templates: %v", err)
	}
	rendered, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "mermaid", Title: "架构", Project: "research", Tags: []string{"pinax", "sync"}})
	if err != nil {
		t.Fatalf("render template: %v", err)
	}
	if rendered.Facts["template"] != "mermaid" || !strings.Contains(fmt.Sprint(rendered.Data), "```mermaid") {
		t.Fatalf("rendered mermaid projection = %#v", rendered)
	}
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "研究日志", Project: "research", Tags: []string{"pinax", "sync"}, Template: "mermaid"})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.Facts["project"] != "research" || created.Facts["tags"] != "pinax,sync" {
		t.Fatalf("created facts = %#v", created.Facts)
	}
	path := filepath.Join(root, created.Facts["path"])
	content := readFile(t, path)
	for _, want := range []string{"schema_version: pinax.note.v1", "title: 研究日志", "tags: [pinax, sync]", "project: research", "```mermaid"} {
		if !strings.Contains(content, want) {
			t.Fatalf("note missing %q:\n%s", want, content)
		}
	}
	writeFile(t, filepath.Join(root, "notes", "research", "source.md"), "# Source\n\n链接 [[研究日志]] #pinax\n")
	indexed, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if indexed.Facts["notes"] != "2" || indexed.Facts["links"] != "1" {
		t.Fatalf("index facts = %#v", indexed.Facts)
	}
	search, err := svc.SearchProjection(ctx, SearchRequest{VaultPath: root, Query: "研究日志"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if search.Facts["matches"] == "0" || search.Facts["engine"] == "" {
		t.Fatalf("search facts = %#v", search.Facts)
	}
	syncPlan, err := svc.SyncDiff(ctx, SyncRequest{VaultPath: root, Target: "cloud"})
	if err != nil {
		t.Fatalf("sync diff: %v", err)
	}
	if syncPlan.Facts["backend_required"] != "true" || syncPlan.Facts["target"] != "cloud" {
		t.Fatalf("sync facts = %#v", syncPlan.Facts)
	}
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud"}); err == nil {
		t.Fatalf("sync push without approval succeeded")
	}
}

func TestVaultStatsAndDoctorServices(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "active.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active\ntitle: Active\ntags: [pinax, cli]\ncreated_at: 2026-01-01T00:00:00Z\nupdated_at: 2026-01-02T00:00:00Z\n---\n\n# Active\n\n链接 [[Missing]] #cli\n")
	writeFile(t, filepath.Join(root, "notes", "duplicate.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_duplicate\ntitle: Active\ntags: []\n---\n\n# Active\n\n")
	writeFile(t, filepath.Join(root, "notes", "raw.md"), "# Raw\n\n")
	writeFile(t, filepath.Join(root, "notes", "empty.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_empty\ntitle: Empty\ntags: [inbox]\n---\n\n")
	old := time.Now().Add(-120 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "notes", "raw.md"), old, old); err != nil {
		t.Fatalf("chtimes raw note: %v", err)
	}

	stats, err := svc.VaultStats(ctx, VaultStatsRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("vault stats: %v", err)
	}
	if stats.Command != "vault.stats" || stats.Facts["notes"] != "4" || stats.Facts["index_status"] != "missing" {
		t.Fatalf("stats projection = %#v", stats)
	}
	statsData, ok := stats.Data.(domain.VaultStats)
	if !ok {
		t.Fatalf("stats data = %#v", stats.Data)
	}
	if statsData.TagCount != 3 || statsData.FrontmatterCoverage != 75 {
		t.Fatalf("stats data = %#v", statsData)
	}

	doctor, err := svc.VaultDoctor(ctx, VaultDoctorRequest{VaultPath: root, StaleAfter: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("vault doctor: %v", err)
	}
	if doctor.Command != "vault.doctor" || doctor.Status != "partial" {
		t.Fatalf("doctor projection = %#v", doctor)
	}
	doctorData, ok := doctor.Data.(domain.VaultDoctorReport)
	if !ok {
		t.Fatalf("doctor data = %#v", doctor.Data)
	}
	for _, want := range []string{"missing_pinax_metadata", "missing_tags", "duplicate_title", "empty_note", "stale_note", "orphan_note", "index_stale"} {
		if !hasVaultIssue(doctorData.Issues, want) {
			t.Fatalf("doctor missing issue %q: %#v", want, doctorData.Issues)
		}
	}
}

func hasVaultIssue(issues []domain.VaultIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestNoteUXServiceResolverListCreateAndMutate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "work", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\ntags: [research]\nproject: work\nstatus: active\n---\n\n# Alpha\n")
	writeFile(t, filepath.Join(root, "notes", "work", "meeting-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_meeting_a\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")
	writeFile(t, filepath.Join(root, "notes", "work", "meeting-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_meeting_b\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")

	resolved, err := svc.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "Alpha"})
	if err != nil || resolved.Path != "notes/work/alpha.md" || resolved.ID != "note_alpha" {
		t.Fatalf("resolve title note=%#v err=%v", resolved, err)
	}
	_, err = svc.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "Meeting"})
	if !hasCommandCode(err, "note_ref_ambiguous") {
		t.Fatalf("ambiguous err = %v", err)
	}

	listed, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Tags: []string{"research"}, Project: "work", Status: "active", Recent: true, Limit: 10})
	if err != nil {
		t.Fatalf("list query: %v", err)
	}
	if listed.Facts["total"] != "1" || !strings.Contains(fmt.Sprint(listed.Data), "notes/work/alpha.md") {
		t.Fatalf("listed projection = %#v", listed)
	}

	dryRun, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Draft", Body: "body", Dir: "work", Slug: "draft", Status: "draft", DryRun: true})
	if err != nil {
		t.Fatalf("dry run create: %v", err)
	}
	if dryRun.Command != "note.new" || dryRun.Facts["planned_path"] != "notes/work/draft.md" || fileExistsApp(filepath.Join(root, "notes", "work", "draft.md")) {
		t.Fatalf("dry run projection = %#v", dryRun)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Bad", Body: "a", SourcePath: filepath.Join(root, "missing.md")}); !hasCommandCode(err, "note_source_conflict") {
		t.Fatalf("source conflict err = %v", err)
	}
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Created", Body: "body", Dir: "work", Slug: "created", Status: "active", Tags: []string{"inbox"}})
	if err != nil || created.Facts["path"] != "notes/work/created.md" {
		t.Fatalf("create projection=%#v err=%v", created, err)
	}

	renamed, err := svc.RenameNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Created", Title: "Created Renamed"})
	if err != nil || renamed.Facts["path"] != "notes/work/created-renamed.md" {
		t.Fatalf("rename projection=%#v err=%v", renamed, err)
	}
	moved, err := svc.MoveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Created Renamed", TargetDir: "archive"})
	if err != nil || moved.Facts["path"] != "notes/archive/created-renamed.md" {
		t.Fatalf("move projection=%#v err=%v", moved, err)
	}
	archived, err := svc.ArchiveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Created Renamed"})
	if err != nil || archived.Facts["status"] != "archived" {
		t.Fatalf("archive projection=%#v err=%v", archived, err)
	}
	tagged, err := svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: "Created Renamed", Operation: "add", Tags: []string{"important"}})
	if err != nil || !strings.Contains(fmt.Sprint(tagged.Data), "important") {
		t.Fatalf("tag projection=%#v err=%v", tagged, err)
	}
	if _, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Created Renamed", Hard: true}); !hasCommandCode(err, "approval_required") {
		t.Fatalf("hard delete without yes err = %v", err)
	}
	deleted, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Created Renamed", Yes: true})
	if err != nil || deleted.Facts["trash_path"] == "" {
		t.Fatalf("delete projection=%#v err=%v", deleted, err)
	}
}

func TestNoteHardeningEditorCommandParser(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		executable string
		args       []string
	}{
		{name: "code wait", input: "code --wait", executable: "code", args: []string{"--wait"}},
		{name: "vim no swap", input: "vim -n", executable: "vim", args: []string{"-n"}},
		{name: "quoted arg", input: `fake-editor --label "work notes"`, executable: "fake-editor", args: []string{"--label", "work notes"}},
		{name: "shell metachar token", input: `fake-editor --literal ';rm -rf /'`, executable: "fake-editor", args: []string{"--literal", ";rm -rf /"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := parseEditorCommand(tt.input)
			if err != nil {
				t.Fatalf("parse editor: %v", err)
			}
			if cmd.Executable != tt.executable || strings.Join(cmd.Args, "\x00") != strings.Join(tt.args, "\x00") {
				t.Fatalf("command = %#v", cmd)
			}
		})
	}
	if _, err := parseEditorCommand("   "); !hasCommandCode(err, "editor_not_configured") {
		t.Fatalf("empty editor err = %v", err)
	}
}

func TestNoteHardeningCommitKeepsOriginalWhenPrepareFails(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "notes", "current.md")
	writeFile(t, current, "original")
	blocker := filepath.Join(root, "blocked")
	writeFile(t, blocker, "not a directory")
	if err := commitNoteContent(current, filepath.Join(blocker, "target.md"), "new content"); err == nil {
		t.Fatalf("commit unexpectedly succeeded")
	}
	if got := readFile(t, current); got != "original" {
		t.Fatalf("original changed after failed prepare: %q", got)
	}
}

func TestNoteHardeningTrashAndRecentSemantics(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	notePath := filepath.Join(root, "notes", "work", "note.md")
	writeFile(t, notePath, "---\nschema_version: pinax.note.v1\nnote_id: note_work\ntitle: Work Note\ntags: [work]\nstatus: active\nupdated_at: 2026-01-02T00:00:00Z\n---\n\n# Work Note\n")
	writeFile(t, filepath.Join(root, "notes", "old.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_old\ntitle: Old Note\ntags: []\n---\n\n# Old Note\n")

	trashDate := time.Now().UTC().Format("20060102")
	existingTrash := filepath.Join(root, ".pinax", "trash", trashDate, "work", "note.md")
	writeFile(t, existingTrash, "existing trash")
	deleted, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Work Note", Yes: true})
	if err != nil {
		t.Fatalf("delete note: %v", err)
	}
	if deleted.Facts["trash_path"] == ".pinax/trash/"+trashDate+"/work/note.md" {
		t.Fatalf("trash path overwrote existing target: %#v", deleted.Facts)
	}
	if got := readFile(t, existingTrash); got != "existing trash" {
		t.Fatalf("existing trash was overwritten: %q", got)
	}
	if !strings.Contains(deleted.Facts["trash_path"], "note-2.md") {
		t.Fatalf("trash path should use numeric suffix: %#v", deleted.Facts)
	}

	listed, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Recent: true})
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if listed.Facts["recent"] != "true" || listed.Facts["sort"] != "updated" || listed.Facts["total"] != "1" {
		t.Fatalf("recent facts = %#v", listed.Facts)
	}
	hardPath := filepath.Join(root, "notes", "hard.md")
	writeFile(t, hardPath, "---\nschema_version: pinax.note.v1\nnote_id: note_hard\ntitle: Hard\ntags: []\n---\n\n# Hard\n")
	hardDeleted, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Hard", Yes: true, Hard: true})
	if err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	if hardDeleted.Facts["hard"] != "true" || hardDeleted.Facts["trash_path"] != "" || fileExistsApp(hardPath) {
		t.Fatalf("hard delete projection/path = %#v exists=%v", hardDeleted.Facts, fileExistsApp(hardPath))
	}
}

func TestNoteHardeningFrontmatterPatchPreservesUserMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	path := filepath.Join(root, "notes", "annotated.md")
	writeFile(t, path, "---\n# user comment\nschema_version: pinax.note.v1\nnote_id: note_annotated\ntitle: Annotated\nowner: alice\ntags: [inbox]\n---\n\n# Annotated\n\nbody\n")
	if _, err := svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: "Annotated", Operation: "add", Tags: []string{"research"}}); err != nil {
		t.Fatalf("tag note: %v", err)
	}
	content := readFile(t, path)
	for _, want := range []string{"# user comment", "owner: alice", "tags: [inbox, research]", "# Annotated\n\nbody"} {
		if !strings.Contains(content, want) {
			t.Fatalf("patched content missing %q:\n%s", want, content)
		}
	}
	if _, err := svc.ArchiveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Annotated"}); err != nil {
		t.Fatalf("archive note: %v", err)
	}
	archived := readFile(t, path)
	for _, want := range []string{"# user comment", "owner: alice", "status: archived", "# Annotated\n\nbody"} {
		if !strings.Contains(archived, want) {
			t.Fatalf("archived content missing %q:\n%s", want, archived)
		}
	}
}

func TestNoteHardeningPatchFrontmatterFields(t *testing.T) {
	content := "---\n# keep me\ntitle: Old\nowner: alice\ntags: [inbox]\n---\n\n# Old\n"
	patched, normalized := patchFrontmatterFields(content, map[string]string{"title": "New", "tags": "[inbox, research]"})
	if normalized {
		t.Fatalf("ordinary frontmatter patch should not normalize")
	}
	for _, want := range []string{"# keep me", "title: New", "owner: alice", "tags: [inbox, research]", "# Old"} {
		if !strings.Contains(patched, want) {
			t.Fatalf("patched content missing %q:\n%s", want, patched)
		}
	}
}

func fileExistsApp(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestTemplateAuthoringCreateRenderValidateDelete(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	design, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "视频学习"})
	if err != nil {
		t.Fatalf("create template design: %v", err)
	}
	if design.Facts["template"] != "视频学习" || design.Facts["kind"] != "template_design" {
		t.Fatalf("design projection = %#v", design)
	}
	designContent := readFile(t, filepath.Join(root, ".pinax", "templates", "视频学习.md"))
	for _, want := range []string{"---\n", "schema_version: pinax.template_design.v1", "kind: template_design", "title: 视频学习", "## 模板正文"} {
		if !strings.Contains(designContent, want) {
			t.Fatalf("template design missing %q:\n%s", want, designContent)
		}
	}

	created, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Body: "# {{title}}\n客户: {{client}}\n负责人: {{owner}}\n"})
	if err != nil {
		t.Fatalf("create template body: %v", err)
	}
	if created.Command != "template.create" || created.Facts["template"] != "meeting" {
		t.Fatalf("created projection = %#v", created)
	}
	content := readFile(t, filepath.Join(root, ".pinax", "templates", "meeting.md"))
	if !strings.Contains(content, "客户: {{client}}") {
		t.Fatalf("template content = %q", content)
	}

	source := filepath.Join(root, "source-template.md")
	writeFile(t, source, "# {{title}}\n\n```mermaid\nflowchart TD\n  A --> B\n```\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "diagram", SourcePath: source}); err != nil {
		t.Fatalf("create template file: %v", err)
	}

	rendered, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Title: "客户会议", Vars: map[string]string{"client": "Acme", "owner": "张三"}})
	if err != nil {
		t.Fatalf("render template vars: %v", err)
	}
	if body := fmt.Sprint(rendered.Data); !strings.Contains(body, "客户: Acme") || !strings.Contains(body, "负责人: 张三") {
		t.Fatalf("rendered body = %#v", rendered.Data)
	}

	if _, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Title: "客户会议"}); !hasCommandCode(err, "template_variable_missing") {
		t.Fatalf("missing var err = %v", err)
	}

	valid, err := svc.ValidateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "diagram"})
	if err != nil {
		t.Fatalf("validate diagram: %v", err)
	}
	if valid.Facts["issues"] != "0" || valid.Facts["template"] != "diagram" {
		t.Fatalf("valid facts = %#v", valid.Facts)
	}

	writeFile(t, filepath.Join(root, ".pinax", "templates", "bad.md"), "# Bad\n\n```mermaid\nflowchart TD\n")
	bad, err := svc.ValidateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "bad"})
	if err != nil {
		t.Fatalf("validate bad should report partial projection, got err: %v", err)
	}
	if bad.Status != "partial" || bad.Facts["issues"] == "0" || !strings.Contains(fmt.Sprint(bad.Data), "template_fence_unclosed") {
		t.Fatalf("bad validation = %#v", bad)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "坏模板", Template: "bad"}); !hasCommandCode(err, "template_invalid") {
		t.Fatalf("bad template note err = %v", err)
	}

	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "客户会议", Template: "meeting", Tags: []string{"meeting"}, Vars: map[string]string{"client": "Acme", "owner": "张三"}}); err != nil {
		t.Fatalf("create note from custom template: %v", err)
	}
	note := readFile(t, filepath.Join(root, "notes", "客户会议.md"))
	if !strings.Contains(note, "客户: Acme") || !strings.Contains(note, "负责人: 张三") {
		t.Fatalf("note from template = %s", note)
	}

	if _, err := svc.DeleteTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting"}); !hasCommandCode(err, "approval_required") {
		t.Fatalf("delete without approval err = %v", err)
	}
	if _, err := svc.DeleteTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Yes: true}); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "templates", "meeting.md")); !os.IsNotExist(err) {
		t.Fatalf("template still exists or stat error = %v", err)
	}
	if _, err := svc.DeleteTemplate(ctx, TemplateRequest{VaultPath: root, Name: "mermaid", Yes: true}); !hasCommandCode(err, "builtin_template_protected") {
		t.Fatalf("delete builtin err = %v", err)
	}
}

func TestTemplateAuthoringRejectsUnsafeInput(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "../bad", Body: "x"}); !hasCommandCode(err, "invalid_template_name") {
		t.Fatalf("unsafe name err = %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "conflict", Body: "x", SourcePath: filepath.Join(root, "missing.md")}); !hasCommandCode(err, "template_source_conflict") {
		t.Fatalf("source conflict err = %v", err)
	}
	if _, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "note", Vars: map[string]string{"bad key": "x"}}); !hasCommandCode(err, "template_variable_invalid") {
		t.Fatalf("invalid var err = %v", err)
	}
}

func TestPlanningActionDraftBuildsTaskBridgeActionsSchema(t *testing.T) {
	now := time.Date(2026, 6, 7, 8, 30, 0, 0, time.UTC)
	snapshot := domain.PlanningSnapshot{SnapshotID: "plan_snap_test"}
	decision := domain.PlanningDecision{
		DecisionID:  "plan_dec_test",
		Period:      domain.PlanningDaily,
		Deferred:    []string{"task_123"},
		Reasons:     []domain.PlanningReason{{Kind: "capacity", Summary: "今日容量不足，推迟低优先级任务"}},
		CreatedAt:   now.Format(time.RFC3339),
		NextActions: []domain.Action{{Name: "open", Command: "pinax daily open --vault ./notes"}},
	}

	draft := buildPlanningActionDraft("daily", snapshot, decision, now)

	if draft.SchemaVersion != "taskbridge.actions.v1" {
		t.Fatalf("schema = %q", draft.SchemaVersion)
	}
	if draft.SourceDecision != "plan_dec_test" || draft.SourceSnapshot != "plan_snap_test" {
		t.Fatalf("source refs = %#v", draft)
	}
	if !draft.RequiresConfirmation {
		t.Fatalf("draft should require confirmation: %#v", draft)
	}
	if len(draft.Tasks) != 1 {
		t.Fatalf("tasks = %#v", draft.Tasks)
	}
	task := draft.Tasks[0]
	if task.ActionID == "" || task.TaskID != "task_123" || task.Kind != "defer" || !task.RequiresConfirmation {
		t.Fatalf("task draft = %#v", task)
	}
	if task.Reason == "" || len(draft.EvidenceRefs) == 0 {
		t.Fatalf("draft missing reason/evidence: %#v", draft)
	}
}

func TestActionDraftDryRunDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanActions(ctx, PlanningRequest{VaultPath: root, FromPeriod: "daily"})
	if err != nil {
		t.Fatalf("plan actions dry-run: %v", err)
	}
	if projection.Facts["dry_run"] != "true" || projection.Facts["saved_path"] != "" {
		t.Fatalf("dry-run facts = %#v", projection.Facts)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", projection.Data)
	}
	draft, ok := data["draft"].(domain.PlanningActionDraft)
	if !ok {
		t.Fatalf("draft data = %#v", data["draft"])
	}
	if draft.SchemaVersion != "taskbridge.actions.v1" || draft.SourceDecision == "" || draft.SourceSnapshot == "" {
		t.Fatalf("draft refs = %#v", draft)
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "planning", "actions")) {
		t.Fatalf("dry-run created actions directory")
	}
}

func TestActionDraftSaveWritesAssetAndReceipt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanActions(ctx, PlanningRequest{VaultPath: root, FromPeriod: "weekly", Save: true})
	if err != nil {
		t.Fatalf("plan actions save: %v", err)
	}
	rel := projection.Facts["saved_path"]
	if rel == "" || !strings.HasPrefix(rel, ".pinax/planning/actions/") {
		t.Fatalf("saved_path = %#v", projection.Facts)
	}
	asset := readFile(t, filepath.Join(root, rel))
	var draft domain.PlanningActionDraft
	if err := json.Unmarshal([]byte(asset), &draft); err != nil {
		t.Fatalf("draft json invalid: %v\n%s", err, asset)
	}
	if draft.SchemaVersion != "taskbridge.actions.v1" || draft.SourcePeriod != "weekly" || draft.SourceDecision == "" || draft.SourceSnapshot == "" {
		t.Fatalf("draft asset = %#v", draft)
	}
	if !strings.Contains(readFile(t, filepath.Join(root, ".pinax", "events.jsonl")), "plan.actions") {
		t.Fatalf("plan action save did not append receipt event")
	}
	if len(projection.Actions) != 1 || !strings.Contains(projection.Actions[0].Command, "taskbridge agent execute --action-file") || !strings.Contains(projection.Actions[0].Command, "--dry-run") {
		t.Fatalf("next actions = %#v", projection.Actions)
	}
}

func hasCommandCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr.Code == code
	}
	return false
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
