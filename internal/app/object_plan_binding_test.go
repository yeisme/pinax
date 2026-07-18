package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	boundObjectID = "01982d84-2b48-7000-8000-000000000041"
	otherObjectID = "01982d84-2b48-7000-8000-000000000042"
)

func TestOrganizePlanObjectBindingSafelyRebasesMovedObject(t *testing.T) {
	root := t.TempDir()
	oldPath := "notes/alpha.md"
	newPath := "projects/alpha.md"
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(oldPath)), boundNoteFixture(boundObjectID, "Alpha"))
	plan := domain.OrganizePlan{Operations: []domain.OrganizeOperation{{OperationID: "op-1", Kind: "tag_patch", Path: oldPath}}}
	if err := bindOrganizePlanObjects(context.Background(), root, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Operations[0].ObjectID != boundObjectID || plan.Operations[0].ExpectedContentRevision.Hash == "" {
		t.Fatalf("operation = %#v", plan.Operations[0])
	}
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, filepath.FromSlash(oldPath)), filepath.Join(root, filepath.FromSlash(newPath))); err != nil {
		t.Fatal(err)
	}
	bound, err := rebaseOrganizePlanObjects(context.Background(), root, &plan)
	if err != nil {
		t.Fatal(err)
	}
	if !bound || plan.Operations[0].Path != newPath {
		t.Fatalf("bound = %v, operation = %#v", bound, plan.Operations[0])
	}
}

func TestOrganizePlanObjectBindingRejectsObservedPathReuse(t *testing.T) {
	root := t.TempDir()
	oldPath := "notes/alpha.md"
	newPath := "projects/alpha.md"
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(oldPath)), boundNoteFixture(boundObjectID, "Alpha"))
	plan := domain.OrganizePlan{Operations: []domain.OrganizeOperation{{OperationID: "op-1", Kind: "tag_patch", Path: oldPath}}}
	if err := bindOrganizePlanObjects(context.Background(), root, &plan); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, filepath.FromSlash(oldPath)), filepath.Join(root, filepath.FromSlash(newPath))); err != nil {
		t.Fatal(err)
	}
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(oldPath)), boundNoteFixture(otherObjectID, "Other"))
	if _, err := rebaseOrganizePlanObjects(context.Background(), root, &plan); domain.ErrorCode(err) != "plan_stale" {
		t.Fatalf("error = %v", err)
	}
}

func TestRepairPlanObjectBindingRejectsRevisionDrift(t *testing.T) {
	root := t.TempDir()
	path := "notes/alpha.md"
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(path)), boundNoteFixture(boundObjectID, "Alpha"))
	plan := domain.RepairPlan{Operations: []domain.RepairOperation{{OperationID: "op-1", Kind: "metadata_patch", Path: path}}}
	if err := bindRepairPlanObjects(context.Background(), root, &plan); err != nil {
		t.Fatal(err)
	}
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(path)), boundNoteFixture(boundObjectID, "Changed"))
	if _, err := rebaseRepairPlanObjects(context.Background(), root, &plan); domain.ErrorCode(err) != "plan_stale" {
		t.Fatalf("error = %v", err)
	}
}

func boundNoteFixture(objectID, title string) string {
	return "---\nschema_version: pinax.note.v1\nnote_id: " + objectID + "\ntitle: " + title + "\nkind: reference\n---\n\n# " + title + "\n"
}
