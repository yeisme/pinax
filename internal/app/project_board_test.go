package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestProjectBoardShowBuildsColumnsWarningsAndCards(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Board Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	writeBoardNote(t, root, "next.md", "note_next", "下一步", "research", "active", "next", "先做 projection。\n\n正文不应进入 card。")
	writeBoardNote(t, root, "blocked.md", "note_blocked", "阻塞项", "research", "blocked", "", "等待接口确认。")
	writeBoardNote(t, root, "done.md", "note_done", "完成项", "research", "done", "done", "已经完成。")
	writeBoardNote(t, root, "legacy.md", "note_legacy", "未知列", "research", "active", "later", "未知列应该警告。")
	writeBoardNote(t, root, "other.md", "note_other", "其它项目", "personal", "active", "next", "不属于 research。")

	projection, err := svc.ProjectBoardShow(ctx, ProjectBoardRequest{VaultPath: root, Project: "research", NoteDisplay: string(domain.NoteDisplayCard)})
	if err != nil {
		t.Fatalf("project board show: %v", err)
	}
	if projection.Command != "project.board.show" || projection.Facts["project"] != "research" || projection.Facts["next"] != "2" || projection.Facts["blocked"] != "1" || projection.Facts["done"] != "1" || projection.Facts["warnings"] != "1" {
		t.Fatalf("projection facts = %#v", projection.Facts)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", projection.Data)
	}
	board, ok := data["board"].(domain.ProjectBoard)
	if !ok {
		t.Fatalf("board data = %#v", data["board"])
	}
	if len(board.Items) != 4 || board.Facts.Next != 2 || board.Facts.Blocked != 1 || board.Facts.Done != 1 || len(board.Warnings) != 1 {
		t.Fatalf("board = %#v", board)
	}
	for _, item := range board.Items {
		if item.Project != "research" {
			t.Fatalf("unexpected project item = %#v", item)
		}
		if item.Note == nil || item.Note.Display != domain.NoteDisplayCard || item.Note.Exposure != domain.NoteExposureAgent {
			t.Fatalf("note card missing or wrong exposure = %#v", item)
		}
		if item.Note.Body != "" || item.Note.Excerpt == "" {
			t.Fatalf("card body/excerpt contract failed = %#v", item.Note)
		}
	}
}

func TestShowNoteProjectionDisplayProfilesGateBody(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Display Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Display Note", Slug: "display", Project: "research", Kind: "task", Status: "active", Body: "secret body should only appear for body display"}); err != nil {
		t.Fatalf("create note: %v", err)
	}

	card, err := svc.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "display.md", Display: string(domain.NoteDisplayCard)})
	if err != nil {
		t.Fatalf("show card: %v", err)
	}
	cardData := card.Data.(map[string]any)
	cardNote := cardData["note"].(domain.NoteDisplay)
	if card.Facts["display"] != "card" || cardNote.Body != "" || cardData["body"] != nil || cardNote.Excerpt == "" {
		t.Fatalf("card projection leaked body or missed facts: facts=%#v data=%#v", card.Facts, card.Data)
	}
	for _, display := range []string{string(domain.NoteDisplayDetail), string(domain.NoteDisplayContext)} {
		projection, err := svc.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "display.md", Display: display})
		if err != nil {
			t.Fatalf("show %s: %v", display, err)
		}
		note := projection.Data.(map[string]any)["note"].(domain.NoteDisplay)
		if projection.Facts["display"] != display || note.Display != domain.NoteDisplayKind(display) || note.Body != "" || note.Excerpt == "" {
			t.Fatalf("%s projection leaked body or missed facts: facts=%#v note=%#v", display, projection.Facts, note)
		}
	}

	body, err := svc.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "display.md", Display: string(domain.NoteDisplayBody)})
	if err != nil {
		t.Fatalf("show body: %v", err)
	}
	bodyNote := body.Data.(map[string]any)["note"].(domain.NoteDisplay)
	if body.Facts["display"] != "body" || bodyNote.Body == "" || bodyNote.Exposure != domain.NoteExposureLocalBody {
		t.Fatalf("body projection = facts %#v note %#v", body.Facts, bodyNote)
	}
}

func TestPlanWeeklyIncludesSavedProjectBoardSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Planning Board Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "实现 board planning", Column: "next"}); err != nil {
		t.Fatalf("add next item: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "等待外部确认", Column: "blocked"}); err != nil {
		t.Fatalf("add blocked item: %v", err)
	}
	boardPlan, err := svc.ProjectBoardPlan(ctx, ProjectBoardRequest{VaultPath: root, Project: "research", Save: true})
	if err != nil {
		t.Fatalf("save board plan: %v", err)
	}
	snapshotID := boardPlan.Facts["snapshot_id"]
	if snapshotID == "" {
		t.Fatalf("board plan missing snapshot_id: %#v", boardPlan.Facts)
	}

	weekly, err := svc.PlanWeekly(ctx, PlanningRequest{VaultPath: root, WithTaskBridge: true, DryRun: true})
	if err != nil {
		t.Fatalf("plan weekly: %v", err)
	}
	if weekly.Facts["board_snapshot_id"] != snapshotID || weekly.Facts["board_next"] != "1" || weekly.Facts["board_blocked"] != "1" {
		t.Fatalf("weekly plan did not include board facts: %#v", weekly.Facts)
	}
	snapshot := weekly.Data.(map[string]any)["snapshot"].(domain.PlanningSnapshot)
	if snapshot.Facts["board_snapshot_id"] != snapshotID || snapshot.Facts["board_project"] != "research" {
		t.Fatalf("planning snapshot board facts = %#v", snapshot.Facts)
	}
}

func TestProjectItemArchiveRequiresVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Snapshot Guard Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "需要归档", Column: "doing"})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	itemID := created.Facts["item_id"]

	failed, err := svc.ProjectItemArchive(ctx, ProjectItemRequest{VaultPath: root, ItemID: itemID, Yes: true})
	if err == nil || failed.Error == nil || failed.Error.Code != "snapshot_required" {
		t.Fatalf("archive without snapshot should fail with snapshot_required: projection=%#v err=%v", failed, err)
	}
	if len(failed.Actions) != 1 || !strings.Contains(failed.Actions[0].Command, "pinax version snapshot") {
		t.Fatalf("snapshot action missing: %#v", failed.Actions)
	}

	if _, err := svc.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "archive 前快照"}); err != nil {
		t.Fatalf("version snapshot: %v", err)
	}
	archived, err := svc.ProjectItemArchive(ctx, ProjectItemRequest{VaultPath: root, ItemID: itemID, Yes: true})
	if err != nil {
		t.Fatalf("archive after snapshot: %v", err)
	}
	if archived.Facts["column"] != "done" || archived.Facts["writes"] != "true" {
		t.Fatalf("archive facts = %#v", archived.Facts)
	}
}

func TestProjectItemMoveDoneRequiresApprovalAndVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Move Guard Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "需要完成", Column: "doing"})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	itemID := created.Facts["item_id"]

	failed, err := svc.ProjectItemMove(ctx, ProjectItemRequest{VaultPath: root, ItemID: itemID, Column: "done"})
	if err == nil || failed.Error == nil || failed.Error.Code != "approval_required" {
		t.Fatalf("move done without approval should fail: projection=%#v err=%v", failed, err)
	}
	failed, err = svc.ProjectItemMove(ctx, ProjectItemRequest{VaultPath: root, ItemID: itemID, Column: "done", Yes: true})
	if err == nil || failed.Error == nil || failed.Error.Code != "snapshot_required" {
		t.Fatalf("move done without snapshot should fail: projection=%#v err=%v", failed, err)
	}
	if len(failed.Actions) != 1 || !strings.Contains(failed.Actions[0].Command, "pinax version snapshot") {
		t.Fatalf("snapshot action missing: %#v", failed.Actions)
	}
	if _, err := svc.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "move done 前快照"}); err != nil {
		t.Fatalf("version snapshot: %v", err)
	}
	moved, err := svc.ProjectItemMove(ctx, ProjectItemRequest{VaultPath: root, ItemID: itemID, Column: "done", Yes: true})
	if err != nil {
		t.Fatalf("move done after snapshot: %v", err)
	}
	if moved.Facts["column"] != "done" || moved.Facts["status"] != "done" {
		t.Fatalf("move done facts = %#v", moved.Facts)
	}
}

func TestValidateVaultChecksProjectBoardAssets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Board Asset Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.ProjectBoardConfigure(ctx, ProjectBoardRequest{VaultPath: root, Project: "research", Columns: []string{"inbox", "next", "done"}}); err != nil {
		t.Fatalf("configure board: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "Validate Item", Column: "next"}); err != nil {
		t.Fatalf("add item: %v", err)
	}
	if _, err := svc.ProjectBoardPlan(ctx, ProjectBoardRequest{VaultPath: root, Project: "research", Save: true}); err != nil {
		t.Fatalf("save board plan: %v", err)
	}
	valid, err := svc.ValidateVault(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("validate valid board assets: %v", err)
	}
	if valid.Facts["issues"] != "0" {
		t.Fatalf("valid board assets produced issues: %#v", valid.Data)
	}

	writeAppFixture(t, filepath.Join(root, ".pinax", "project-boards", "research.json"), `{"schema_version":"wrong","project_slug":"research"}`+"\n")
	writeAppFixture(t, filepath.Join(root, ".pinax", "planning", "project-boards", "bad.json"), `{"schema_version":"wrong","project_slug":"research"}`+"\n")
	invalid, err := svc.ValidateVault(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("validate invalid board assets: %v", err)
	}
	issues := invalid.Data.(map[string]any)["issues"].([]domain.Issue)
	if !hasIssueCode(issues, "invalid_project_board_config") || !hasIssueCode(issues, "invalid_project_board_snapshot") {
		t.Fatalf("expected board asset issues, got %#v", issues)
	}
}

func TestConfiguredProjectBoardColumnsDriveItemsAndProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Learning Board Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "investing", Name: "学习炒股", NotesPrefix: "notes/investing"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.ProjectSubprojectCreate(ctx, ProjectWorkspaceRequest{VaultPath: root, Project: "investing", Subproject: "stock-learning", Title: "学习炒股的全部笔记", Template: "long-term-learning"}); err != nil {
		t.Fatalf("create subproject: %v", err)
	}
	columns := []string{"inbox", "planned", "learning", "practice", "review", "retrospective", "done"}
	if _, err := svc.ProjectBoardConfigure(ctx, ProjectBoardRequest{VaultPath: root, Project: "investing", Subproject: "stock-learning", Columns: columns}); err != nil {
		t.Fatalf("configure learning board: %v", err)
	}
	created, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "investing", Subproject: "stock-learning", Title: "学习 K 线基础", Column: "learning", Labels: []string{"stock", "technical-analysis"}})
	if err != nil {
		t.Fatalf("add learning item using configured column: projection=%#v err=%v", created, err)
	}
	if created.Facts["column"] != "learning" || created.Facts["status"] != "active" {
		t.Fatalf("created learning item facts = %#v", created.Facts)
	}

	boardProjection, err := svc.ProjectBoardShow(ctx, ProjectBoardRequest{VaultPath: root, Project: "investing", Subproject: "stock-learning", NoteDisplay: string(domain.NoteDisplayCard)})
	if err != nil {
		t.Fatalf("show learning board: %v", err)
	}
	if boardProjection.Facts["column.learning"] != "1" || boardProjection.Facts["column.practice"] != "0" || boardProjection.Facts["items"] != "1" {
		t.Fatalf("dynamic column facts missing: %#v", boardProjection.Facts)
	}
	board := boardProjection.Data.(map[string]any)["board"].(domain.ProjectBoard)
	if len(board.Columns) != len(columns) {
		t.Fatalf("board columns = %#v", board.Columns)
	}
	for i, want := range columns {
		if board.Columns[i].ID != want {
			t.Fatalf("board column %d = %q, want %q; columns=%#v", i, board.Columns[i].ID, want, board.Columns)
		}
	}
	if board.Facts.ColumnCounts["learning"] != 1 || board.Items[0].Column != "learning" {
		t.Fatalf("board counts/items = facts %#v items %#v", board.Facts, board.Items)
	}
}

func TestProjectLearningInitCreatesReusableStockLearningPack(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Stock Learning Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	created, err := svc.ProjectLearningInit(ctx, ProjectLearningRequest{VaultPath: root, Project: "investing", Subproject: "stock-learning", Title: "学习炒股的全部笔记", ProjectName: "学习炒股", NotesPrefix: "notes/investing", Preset: "stock-learning"})
	if err != nil {
		t.Fatalf("learning init: projection=%#v err=%v", created, err)
	}
	if created.Command != "project.learning.init" || created.Facts["project"] != "investing" || created.Facts["subproject"] != "stock-learning" || created.Facts["preset"] != "stock-learning" || created.Facts["writes"] != "true" {
		t.Fatalf("learning init facts = %#v command=%s", created.Facts, created.Command)
	}
	for _, rel := range []string{
		"notes/projects/investing/stock-learning/charter/learning-charter.md",
		"notes/projects/investing/stock-learning/sources/source-index.md",
		"notes/projects/investing/stock-learning/retros/weekly-review.md",
	} {
		if !fileExistsApp(filepath.Join(root, filepath.FromSlash(rel))) {
			t.Fatalf("learning starter note missing: %s", rel)
		}
	}
	content := readAppFixture(t, filepath.Join(root, "notes", "projects", "investing", "stock-learning", "charter", "learning-charter.md"))
	for _, forbidden := range []string{"buy recommendation", "sell recommendation", "guaranteed returns", "automated trading decision"} {
		if strings.Contains(strings.ToLower(content), forbidden) {
			t.Fatalf("stock learning charter must not include forbidden advice phrase %q:\n%s", forbidden, content)
		}
	}

	second, err := svc.ProjectLearningInit(ctx, ProjectLearningRequest{VaultPath: root, Project: "investing", Subproject: "stock-learning", Title: "学习炒股的全部笔记", ProjectName: "学习炒股", NotesPrefix: "notes/investing", Preset: "stock-learning"})
	if err != nil {
		t.Fatalf("learning init second run: projection=%#v err=%v", second, err)
	}
	if second.Facts["notes.created"] != "0" || second.Facts["items.created"] != "0" {
		t.Fatalf("second run should be idempotent, facts=%#v", second.Facts)
	}
}

func TestProjectItemMoveRefusesUnmanagedNote(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Unmanaged Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "reference.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_reference\ntitle: Reference\nproject: research\nkind: reference\nstatus: active\nboard_column: next\n---\n\n不是 task，不能由 project item move 改写。\n")
	projection, err := svc.ProjectItemMove(ctx, ProjectItemRequest{VaultPath: root, ItemID: "note_reference", Column: "done", Yes: true})
	if err == nil || projection.Error == nil || projection.Error.Code != "project_item_unmanaged" {
		t.Fatalf("expected unmanaged item refusal: projection=%#v err=%v", projection, err)
	}
}

func TestProjectBoardInferredChecklistTasksRequireAdoption(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Checklist Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "checklist.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_checklist\ntitle: Checklist Source\nproject: research\nkind: reference\nstatus: active\n---\n\n## Tasks\n\n- [ ] Review source material\n- [x] Already handled\n")

	boardProjection, err := svc.ProjectBoardShow(ctx, ProjectBoardRequest{VaultPath: root, Project: "research"})
	if err != nil {
		t.Fatalf("project board show: %v", err)
	}
	board := boardProjection.Data.(map[string]any)["board"].(domain.ProjectBoard)
	var inferred domain.BoardItem
	for _, item := range board.Items {
		if item.Title == "Review source material" {
			inferred = item
		}
	}
	if inferred.ItemID == "" || inferred.SourceKind != domain.BoardItemSourceInlineTask || inferred.SourceStatus != "inferred" || inferred.Writable {
		t.Fatalf("inferred checklist item not read-only: %#v", inferred)
	}

	moveProjection, err := svc.ProjectItemMove(ctx, ProjectItemRequest{VaultPath: root, ItemID: inferred.ItemID, Column: "doing"})
	if err == nil || moveProjection.Error == nil || moveProjection.Error.Code != "task_unmanaged" {
		t.Fatalf("move inferred task should fail with task_unmanaged: projection=%#v err=%v", moveProjection, err)
	}

	plan, err := svc.TaskAdopt(ctx, TaskAdoptRequest{VaultPath: root, ItemID: inferred.ItemID})
	if err != nil {
		t.Fatalf("task adopt plan: %v", err)
	}
	if plan.Command != "task.adopt" || plan.Facts["writes"] != "false" || plan.Facts["source_status"] != "inferred" || plan.Facts["adopted"] != "0" {
		t.Fatalf("task adopt plan facts = %#v", plan.Facts)
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "task-adoptions")) {
		t.Fatalf("task adopt plan wrote ledger")
	}

	adopted, err := svc.TaskAdopt(ctx, TaskAdoptRequest{VaultPath: root, ItemID: inferred.ItemID, Yes: true})
	if err != nil {
		t.Fatalf("task adopt apply: projection=%#v err=%v", adopted, err)
	}
	if adopted.Facts["writes"] != "true" || adopted.Facts["adopted"] != "1" || adopted.Facts["ledger_path"] == "" {
		t.Fatalf("task adopt apply facts = %#v", adopted.Facts)
	}
	ledgerPath := filepath.Join(root, filepath.FromSlash(adopted.Facts["ledger_path"]))
	ledger := readAppFixture(t, ledgerPath)
	if !strings.Contains(ledger, `"schema_version": "pinax.task_adoption.v1"`) || !strings.Contains(ledger, `"source_status": "adopted"`) || strings.Contains(ledger, "Already handled") {
		t.Fatalf("task adoption ledger invalid or leaked unrelated body:\n%s", ledger)
	}

	afterProjection, err := svc.ProjectBoardShow(ctx, ProjectBoardRequest{VaultPath: root, Project: "research"})
	if err != nil {
		t.Fatalf("project board after adopt: %v", err)
	}
	afterBoard := afterProjection.Data.(map[string]any)["board"].(domain.ProjectBoard)
	var after domain.BoardItem
	for _, item := range afterBoard.Items {
		if item.ItemID == inferred.ItemID {
			after = item
		}
	}
	if after.SourceStatus != "adopted" || !after.Writable {
		t.Fatalf("adopted checklist item not promoted in board projection: %#v", after)
	}
}

func hasIssueCode(issues []domain.Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func readAppFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}

func writeBoardNote(t *testing.T, root, rel, id, title, project, status, column, body string) {
	t.Helper()
	columnLine := ""
	if column != "" {
		columnLine = fmt.Sprintf("board_column: %s\n", column)
	}
	writeAppFixture(t, filepath.Join(root, rel), fmt.Sprintf("---\nschema_version: pinax.note.v1\nnote_id: %s\ntitle: %s\nproject: %s\nkind: task\nstatus: %s\n%supdated_at: 2026-06-08T00:00:00Z\ntags: [pinax, board]\n---\n\n%s\n", id, title, project, status, columnLine, body))
}
