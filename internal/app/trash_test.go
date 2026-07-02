package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrashServiceRestorePurgeDryRunAndPathBoundary(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "history", Name: "History", NotesPrefix: "notes/history"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "history", "index.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_history\ntitle: History\n---\n\n# History\n")

	deleted, err := svc.ProjectDelete(ctx, ProjectDeleteRequest{VaultPath: root, Project: "history", Yes: true})
	if err != nil {
		t.Fatalf("delete project: %v", err)
	}
	trashPath := filepath.Join(root, filepath.FromSlash(deleted.Facts["trash_path"]))
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("trash path missing after delete: %v", err)
	}

	if _, err := svc.TrashRestore(ctx, TrashRequest{VaultPath: root, ObjectRef: "../history"}); !hasCommandCode(err, "trash_object_required") {
		t.Fatalf("path escape restore err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "history")); !os.IsNotExist(err) {
		t.Fatalf("path escape restore modified content path: %v", err)
	}

	if _, err := svc.TrashRestore(ctx, TrashRequest{VaultPath: root, ObjectRef: "project/history"}); err != nil {
		t.Fatalf("restore project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "history", "index.md")); err != nil {
		t.Fatalf("restored content missing: %v", err)
	}

	deletedAgain, err := svc.ProjectDelete(ctx, ProjectDeleteRequest{VaultPath: root, Project: "history", Yes: true})
	if err != nil {
		t.Fatalf("delete project again: %v", err)
	}
	if deletedAgain.Facts["trash_path"] == deleted.Facts["trash_path"] || !strings.Contains(deletedAgain.Facts["trash_path"], "-2") {
		t.Fatalf("second trash path did not avoid collision: first=%q second=%q", deleted.Facts["trash_path"], deletedAgain.Facts["trash_path"])
	}

	dryRun, err := svc.TrashPurge(ctx, TrashRequest{VaultPath: root, ObjectRef: "project/history", DryRun: true})
	if err != nil {
		t.Fatalf("purge dry-run: %v", err)
	}
	if dryRun.Facts["local_write"] != "false" || dryRun.Facts["dry_run"] != "true" {
		t.Fatalf("dry-run facts = %#v", dryRun.Facts)
	}
	secondTrashPath := filepath.Join(root, filepath.FromSlash(deletedAgain.Facts["trash_path"]))
	if _, err := os.Stat(secondTrashPath); err != nil {
		t.Fatalf("purge dry-run removed trash path: %v", err)
	}
	if _, err := svc.TrashPurge(ctx, TrashRequest{VaultPath: root, ObjectRef: "project/history", Hard: true, Yes: true}); err != nil {
		t.Fatalf("purge hard: %v", err)
	}
	if _, err := os.Stat(secondTrashPath); !os.IsNotExist(err) {
		t.Fatalf("purge hard left trash path: %v", err)
	}
}
