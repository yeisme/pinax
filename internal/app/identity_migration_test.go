package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
	"github.com/yeisme/pinax/internal/records"
)

func TestRecordIdentityAuditIsReadOnlyAndReportsMigrationIssues(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	canonical := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	writeAppFixture(t, filepath.Join(root, "notes", "canonical.md"), noteFixture(canonical, "Canonical"))
	writeAppFixture(t, filepath.Join(root, "notes", "legacy.md"), noteFixture("note_legacy", "Legacy"))
	writeAppFixture(t, filepath.Join(root, "notes", "missing.md"), noteFixture("", "Missing"))
	writeAppFixture(t, filepath.Join(root, "notes", "duplicate-a.md"), noteFixture("note_duplicate", "Duplicate A"))
	writeAppFixture(t, filepath.Join(root, "notes", "duplicate-b.md"), noteFixture("note_duplicate", "Duplicate B"))

	projection, err := NewService().RecordIdentityAudit(context.Background(), IdentityMigrationRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Command != "record.identity.audit" || projection.Facts["writes"] != "false" {
		t.Fatalf("projection = %#v", projection)
	}
	if projection.Facts["canonical"] != "1" || projection.Facts["legacy"] != "3" || projection.Facts["missing"] != "1" || projection.Facts["duplicates"] != "1" {
		t.Fatalf("facts = %#v", projection.Facts)
	}
	report, ok := projection.Data.(domain.IdentityAuditReport)
	if !ok || len(report.Issues) != 4 {
		t.Fatalf("report = %#v", projection.Data)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
		t.Fatalf("audit wrote .pinax state: %v", err)
	}
}

func TestIdentityMigrationPlanSavesCLIAuthoredMigrationAsset(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "legacy.md"), noteFixture("note_legacy", "Legacy"))
	writeAppFixture(t, filepath.Join(root, "notes", "missing.md"), noteFixture("", "Missing"))

	projection, err := NewService().RecordIdentityPlan(context.Background(), IdentityMigrationRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Command != "record.identity.plan" || projection.Facts["writes"] != "true" || projection.Facts["operations"] != "2" {
		t.Fatalf("projection = %#v", projection)
	}
	plan, ok := projection.Data.(domain.IdentityMigrationPlan)
	if !ok {
		t.Fatalf("plan type = %T", projection.Data)
	}
	if plan.PlanID == "" || plan.SavedPath == "" || len(plan.Operations) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
	for _, operation := range plan.Operations {
		if identity.Classify(operation.ToObjectID) != identity.IDClassCanonical {
			t.Fatalf("operation target is not canonical: %#v", operation)
		}
	}
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(plan.SavedPath)))
	if err != nil {
		t.Fatal(err)
	}
	var saved domain.IdentityMigrationPlan
	if err := json.Unmarshal(payload, &saved); err != nil {
		t.Fatalf("saved plan invalid: %v", err)
	}
	if saved.PlanID != plan.PlanID || saved.SchemaVersion != domain.IdentityMigrationPlanSchemaVersion {
		t.Fatalf("saved plan = %#v", saved)
	}
}

func TestRecordIdentityAuditDetectsLedgerFrontmatterMismatchWithoutWritingState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	frontmatterID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	ledgerID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102"
	writeAppFixture(t, filepath.Join(root, "notes", "mismatch.md"), noteFixture(frontmatterID, "Mismatch"))
	_, err := records.NewService(root).AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "fixture:mismatch", ObjectID: ledgerID, ObjectKind: "note", CurrentPath: "notes/mismatch.md", Title: "Mismatch"})
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, ".pinax", "records", "notes.json")
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}

	projection, err := NewService().RecordIdentityAudit(context.Background(), IdentityMigrationRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Facts["conflicts"] != "1" {
		t.Fatalf("facts = %#v", projection.Facts)
	}
	after, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("read-only audit rewrote ledger registry")
	}
}

func noteFixture(noteID, title string) string {
	idLine := ""
	if noteID != "" {
		idLine = "note_id: " + noteID + "\n"
	}
	return "---\nschema_version: pinax.note.v1\n" + idLine + "title: " + title + "\nkind: reference\n---\n\n# " + title + "\n"
}

func TestRecordIdentityAuditReportsCanonicalDuplicateAndLedgerPathCollision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	duplicateID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	writeAppFixture(t, filepath.Join(root, "notes", "duplicate-a.md"), noteFixture(duplicateID, "Duplicate A"))
	writeAppFixture(t, filepath.Join(root, "notes", "duplicate-b.md"), noteFixture(duplicateID, "Duplicate B"))

	events := []domain.RecordEvent{
		{SchemaVersion: records.EventSchemaVersion, EventID: "record_evt_000001", Seq: 1, IdempotencyKey: "fixture:one", Kind: domain.RecordEventNoteCreated, ObjectID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102", ObjectKind: "note", CurrentPath: "notes/collision.md", NoteID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102", Path: "notes/collision.md"},
		{SchemaVersion: records.EventSchemaVersion, EventID: "record_evt_000002", Seq: 2, IdempotencyKey: "fixture:two", Kind: domain.RecordEventNoteCreated, ObjectID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0103", ObjectKind: "note", CurrentPath: "notes/collision.md", NoteID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0103", Path: "notes/collision.md"},
	}
	var payload []byte
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		payload = append(payload, line...)
		payload = append(payload, '\n')
	}
	writeAppFixture(t, filepath.Join(root, ".pinax", "records", "events.jsonl"), string(payload))

	projection, err := NewService().RecordIdentityAudit(context.Background(), IdentityMigrationRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Facts["duplicates"] != "1" || projection.Facts["path_collisions"] != "1" {
		t.Fatalf("facts = %#v", projection.Facts)
	}
	report := projection.Data.(domain.IdentityAuditReport)
	codes := map[string]bool{}
	for _, issue := range report.Issues {
		codes[issue.Code] = true
	}
	if !codes["identity_duplicate"] || !codes["record_path_collision"] {
		t.Fatalf("issues = %#v", report.Issues)
	}
}

func TestIdentityMigrationApplyRequiresApprovalAndCreatesSnapshotReceipt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "notes", "legacy.md")
	writeAppFixture(t, path, noteFixture("note_legacy", "Legacy"))
	svc := NewService()
	planned, err := svc.RecordIdentityPlan(context.Background(), IdentityMigrationRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := planned.Data.(domain.IdentityMigrationPlan)

	if _, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID}); domain.ErrorCode(err) != "approval_required" {
		t.Fatalf("approval error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != noteFixture("note_legacy", "Legacy") {
		t.Fatal("unapproved apply changed note")
	}

	projection, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Facts["applied"] != "1" || projection.Facts["snapshot_id"] == "" || projection.Facts["receipt"] == "" {
		t.Fatalf("projection = %#v", projection)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	toID := plan.Operations[0].ToObjectID
	if !strings.Contains(string(after), "note_id: "+toID+"\n") {
		t.Fatalf("frontmatter not migrated:\n%s", after)
	}
	state, err := records.NewService(root).ReplayReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	record := state.Records[toID]
	if record.ObjectID != toID || len(record.LegacyAliases) != 1 || record.LegacyAliases[0] != "note_legacy" {
		t.Fatalf("record = %#v", record)
	}
	receiptPath := filepath.Join(root, filepath.FromSlash(projection.Facts["receipt"]))
	var receipt domain.IdentityMigrationReceipt
	payload, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "completed" || receipt.SnapshotID == "" || len(receipt.RestoreCommands) != 1 {
		t.Fatalf("receipt = %#v", receipt)
	}
}

func TestIdentityMigrationApplyRejectsStalePlanAndCompletedResumeIsIdempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "notes", "legacy.md")
	writeAppFixture(t, path, noteFixture("note_legacy", "Legacy"))
	svc := NewService()
	planned, err := svc.RecordIdentityPlan(context.Background(), IdentityMigrationRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := planned.Data.(domain.IdentityMigrationPlan)
	writeAppFixture(t, path, noteFixture("note_changed", "Legacy"))
	if _, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); domain.ErrorCode(err) != "plan_stale" {
		t.Fatalf("stale error = %v", err)
	}

	writeAppFixture(t, path, noteFixture("note_legacy", "Legacy"))
	first, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true, Resume: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Facts["status"] != "completed" || second.Facts["snapshot_id"] != first.Facts["snapshot_id"] {
		t.Fatalf("resume = %#v", second)
	}
}

func TestIdentityMigrationResumeAfterLedgerFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "notes", "legacy.md")
	writeAppFixture(t, path, noteFixture("note_legacy", "Legacy"))
	svc := NewService()
	planned, err := svc.RecordIdentityPlan(context.Background(), IdentityMigrationRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := planned.Data.(domain.IdentityMigrationPlan)
	writeAppFixture(t, filepath.Join(root, ".pinax", "records", "events.jsonl"), "{broken json\n")

	projection, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
	if domain.ErrorCode(err) != "frontmatter_or_ledger_failed" || projection.Facts["receipt"] == "" {
		t.Fatalf("failure projection = %#v err=%v", projection, err)
	}
	payload, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(payload), "note_id: "+plan.Operations[0].ToObjectID+"\n") {
		t.Fatalf("frontmatter phase did not complete:\n%s", payload)
	}
	writeAppFixture(t, filepath.Join(root, ".pinax", "records", "events.jsonl"), "")
	resumed, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true, Resume: true})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Facts["status"] != "completed" || resumed.Facts["applied"] != "1" {
		t.Fatalf("resumed = %#v", resumed)
	}
}

func TestIdentityMigrationPlanRepairsFrontmatterMirrorFromLedger(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	frontmatterID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	ledgerID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102"
	path := filepath.Join(root, "notes", "mismatch.md")
	writeAppFixture(t, path, noteFixture(frontmatterID, "Mismatch"))
	if _, err := records.NewService(root).AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "fixture:mismatch-repair", ObjectID: ledgerID, ObjectKind: "note", CurrentPath: "notes/mismatch.md", Title: "Mismatch"}); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	planned, err := svc.RecordIdentityPlan(context.Background(), IdentityMigrationRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := planned.Data.(domain.IdentityMigrationPlan)
	if len(plan.Operations) != 1 || plan.Operations[0].Kind != "repair_frontmatter_mirror" || plan.Operations[0].ToObjectID != ledgerID {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := svc.RecordIdentityApply(context.Background(), IdentityMigrationApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "note_id: "+ledgerID+"\n") {
		t.Fatalf("frontmatter mirror not repaired:\n%s", payload)
	}
	state, err := records.NewService(root).ReplayReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Records[ledgerID].ObjectID != ledgerID {
		t.Fatalf("ledger authority changed: %#v", state.Records)
	}
}
