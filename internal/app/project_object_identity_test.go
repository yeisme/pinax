package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
)

func TestProjectSubprojectBoardAndTaskAdoptUseStableObjectIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	project := created.Data.(map[string]any)["project"].(domain.Project)
	if identity.Classify(project.ObjectID) != identity.IDClassCanonical {
		t.Fatalf("project = %#v", project)
	}
	repeated, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Data.(map[string]any)["project"].(domain.Project).ObjectID != project.ObjectID {
		t.Fatal("project identity changed on idempotent create")
	}
	workspaceProjection, err := svc.ProjectSubprojectCreate(ctx, ProjectWorkspaceRequest{VaultPath: root, Project: "alpha", Subproject: "research", Title: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaceProjection.Data.(map[string]any)["workspace"].(domain.ProjectWorkspace)
	if identity.Classify(workspace.ObjectID) != identity.IDClassCanonical || workspace.ProjectObjectID != project.ObjectID {
		t.Fatalf("workspace = %#v", workspace)
	}

	noteID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	body := "---\nschema_version: pinax.note.v1\nnote_id: " + noteID + "\ntitle: Work\nproject: alpha\nsubproject: research\nkind: task\nstatus: active\n---\n\n# Work\n\n- [ ] Investigate ^investigate\n"
	writeAppFixture(t, filepath.Join(root, "notes", "alpha", "work.md"), body)
	board, err := svc.ProjectBoardShow(ctx, ProjectBoardRequest{VaultPath: root, Project: "alpha", Subproject: "research"})
	if err != nil {
		t.Fatal(err)
	}
	boardData := board.Data.(map[string]any)["board"].(domain.ProjectBoard)
	if boardData.ProjectObjectID != project.ObjectID || boardData.SubprojectObjectID != workspace.ObjectID {
		t.Fatalf("board identity = %#v", boardData)
	}
	var noteItem, taskItem domain.BoardItem
	for _, item := range boardData.Items {
		if item.SourceKind == domain.BoardItemSourceNote {
			noteItem = item
		}
		if item.SourceKind == domain.BoardItemSourceInlineTask {
			taskItem = item
		}
	}
	if noteItem.ObjectID != noteID || taskItem.SourceObjectID != noteID || taskItem.SourceAnchor != "block:investigate" {
		t.Fatalf("items note=%#v task=%#v", noteItem, taskItem)
	}
	planned, err := svc.TaskAdopt(ctx, TaskAdoptRequest{VaultPath: root, ItemID: taskItem.ItemID})
	if err != nil {
		t.Fatal(err)
	}
	adoption := planned.Data.(map[string]any)["adoption"].(domain.TaskAdoption)
	if identity.Classify(adoption.ObjectID) != identity.IDClassCanonical || adoption.SourceObjectID != noteID || adoption.SourceAnchor != "block:investigate" {
		t.Fatalf("adoption = %#v", adoption)
	}
	applied, err := svc.TaskAdopt(ctx, TaskAdoptRequest{VaultPath: root, ItemID: taskItem.ItemID, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Data.(map[string]any)["adoption"].(domain.TaskAdoption).ObjectID != adoption.ObjectID {
		t.Fatal("task identity changed between plan and apply")
	}
}

func TestInlineTaskIDSurvivesSourceNoteMove(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101", Path: "notes/a.md", Body: "- [ ] Stable task ^stable"}
	first := checklistBoardItems(note, nil, defaultBoardColumns, nil)[0]
	note.Path = "notes/archive/a.md"
	second := checklistBoardItems(note, nil, defaultBoardColumns, nil)[0]
	if first.ItemID != second.ItemID || first.SourceAnchor != second.SourceAnchor {
		t.Fatalf("before=%#v after=%#v", first, second)
	}
}
