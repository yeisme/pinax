package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

func scopedFixture(t *testing.T) (*Service, string) {
	t.Helper()
	s := NewService()
	root := t.TempDir()
	if _, err := s.InitVault(context.Background(), InitVaultRequest{VaultPath: root, Title: "Scoped fixture"}); err != nil {
		t.Fatal(err)
	}
	writeAppFixture(t, filepath.Join(root, "Alpha.md"), boundNoteFixture(boundObjectID, "Alpha")+"\n[[Beta]] #topic\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), boundNoteFixture(otherObjectID, "Beta"))
	return s, root
}
func scopedPlan(t *testing.T, s *Service, root string) domain.OrganizePlan {
	t.Helper()
	p, err := s.PlanScopedOrganize(context.Background(), ScopedOrganizeRequest{VaultPath: root, NoteRefs: []string{boundObjectID, "Alpha.md"}, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	return p.Data.(domain.OrganizePlan)
}
func TestScopedOrganizeSelectionContextAndApply(t *testing.T) {
	s, root := scopedFixture(t)
	ctx := context.Background()
	p, err := s.PlanScopedOrganize(ctx, ScopedOrganizeRequest{VaultPath: root, NoteRefs: []string{boundObjectID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "organize-plans")); !os.IsNotExist(err) {
		t.Fatal("preview wrote plans")
	}
	for _, op := range p.Data.(domain.OrganizePlan).Operations {
		if op.ObjectID != boundObjectID || op.Kind == "link_resolution" || op.Kind == "orphan_review" {
			t.Fatalf("incorrect context/selection: %#v", op)
		}
	}
	plan := scopedPlan(t, s, root)
	if len(plan.Selection) != 1 {
		t.Fatal("not deduplicated")
	}
	beta := filepath.Join(root, "notes", "beta.md")
	writeAppFixture(t, beta, boundNoteFixture(otherObjectID, "Beta")+"unrelated change")
	before, _ := os.ReadFile(beta)
	if _, err = s.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, PlanID: plan.PlanID}); domain.ErrorCode(err) != "approval_required" {
		t.Fatal(err)
	}
	result, err := s.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Facts["snapshot_id"] == "" || result.Facts["evidence_complete"] != "true" {
		t.Fatal("missing evidence")
	}
	if len(result.PipelineStages) != 2 || result.PipelineStages[1].Type != domain.PipelineStageCompleted {
		t.Fatal("scoped apply omitted pipeline stages")
	}
	after, _ := os.ReadFile(beta)
	if string(before) != string(after) {
		t.Fatal("unselected file changed")
	}
	if _, err = os.Stat(filepath.Join(root, "notes", "alpha.md")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); domain.ErrorCode(err) != "organize_plan_not_planned" {
		t.Fatal("consumed plan reapplied", err)
	}
	saved, err := loadOrganizePlan(root, plan.PlanID)
	if err != nil || saved.Status != "applied" {
		t.Fatal(err)
	}
}
func TestScopedOrganizeRejectsDriftConflictAndOverride(t *testing.T) {
	for _, kind := range []string{"drift", "conflict", "override", "symlink", "duplicate", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			s, root := scopedFixture(t)
			plan := scopedPlan(t, s, root)
			req := ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}
			switch kind {
			case "drift":
				writeAppFixture(t, filepath.Join(root, "Alpha.md"), boundNoteFixture(boundObjectID, "Changed"))
			case "conflict":
				writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "existing target")
			case "override":
				req.AllowStale = true
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "notes", "alpha.md")); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				writeAppFixture(t, filepath.Join(root, "duplicate.md"), boundNoteFixture(boundObjectID, "Duplicate"))
			case "invalid":
				if _, err := s.PlanScopedOrganize(context.Background(), ScopedOrganizeRequest{VaultPath: root, NoteRefs: []string{"Alpha"}}); err == nil {
					t.Fatal("title accepted")
				}
				return
			}
			before, _ := os.ReadFile(filepath.Join(root, "Alpha.md"))
			if _, err := s.ApplyOrganize(context.Background(), req); err == nil {
				t.Fatal("unsafe apply succeeded")
			}
			after, _ := os.ReadFile(filepath.Join(root, "Alpha.md"))
			if string(before) != string(after) {
				t.Fatal("preflight wrote source")
			}
			saved, _ := loadOrganizePlan(root, plan.PlanID)
			if saved.Status != "planned" || saved.SnapshotID != "" {
				t.Fatal("preflight mutated plan")
			}
		})
	}
}
func TestScopedOrganizeReceiptFailureStopsReplay(t *testing.T) {
	s, root := scopedFixture(t)
	plan := scopedPlan(t, s, root)
	writeAppFixture(t, filepath.Join(root, ".pinax", "receipts"), "blocked")
	p, err := s.ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
	if err == nil || p.Facts["evidence_complete"] != "false" || p.Facts["snapshot_id"] == "" {
		t.Fatalf("unreported failure: %+v %v", p.Facts, err)
	}
	if _, err = s.ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); err == nil {
		t.Fatal("reapplied after evidence failure")
	}
}

type scopedSnapshotHook struct {
	pinaxversion.VersionBackend
	after func()
}

func (h scopedSnapshotHook) Snapshot(ctx context.Context, req pinaxversion.SnapshotRequest) (pinaxversion.Snapshot, error) {
	snapshot, err := h.VersionBackend.Snapshot(ctx, req)
	if err == nil {
		h.after()
	}
	return snapshot, err
}

func TestScopedOrganizeInterruptedExecutionIsQueryable(t *testing.T) {
	s, root := scopedFixture(t)
	plan := scopedPlan(t, s, root)
	s = NewServiceWithVersionBackend(scopedSnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), after: func() {
		writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "external target")
	}})
	p, err := s.ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
	if err == nil || p.Facts["snapshot_id"] == "" || p.Facts["evidence_complete"] != "true" {
		t.Fatal("failure evidence missing", err)
	}
	saved, err := loadOrganizePlan(root, plan.PlanID)
	if err != nil || saved.OperationResults[0].Status != "failed" || saved.OperationResults[1].Status != "pending" {
		t.Fatal("operation outcomes lost", err)
	}
	if _, err := s.ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); err == nil {
		t.Fatal("failed plan replayed")
	}
	view, ok := pipelineShowOrganizePlan(context.Background(), root, plan.PlanID)
	if !ok {
		t.Fatal("original plan is not queryable")
	}
	for _, a := range view.Actions {
		if strings.Contains(a.Command, "organize apply") {
			t.Fatal("unsafe terminal action")
		}
	}
}

func TestScopedOrganizeFullContextAndMovedSelection(t *testing.T) {
	s, root := scopedFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "notes", "beta.md"), filepath.Join(root, "projects", "beta.md")); err != nil {
		t.Fatal(err)
	}
	plan := scopedPlan(t, s, root)
	for _, op := range plan.Operations {
		if op.Kind == "link_resolution" || op.Kind == "orphan_review" {
			t.Fatal("batch-external graph lost")
		}
	}
	if err := os.Rename(filepath.Join(root, "Alpha.md"), filepath.Join(root, "notes", "renamed.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); err != nil {
		t.Fatal("safe object rebase failed", err)
	}
}

func TestScopedOrganizeNoOperationSelectionStillPinsRevision(t *testing.T) {
	s, root := scopedFixture(t)
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), strings.Replace(boundNoteFixture(otherObjectID, "Beta"), "kind: reference", "kind: reference\nstatus: active", 1))
	p, err := s.PlanScopedOrganize(context.Background(), ScopedOrganizeRequest{VaultPath: root, NoteRefs: []string{otherObjectID}, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := p.Data.(domain.OrganizePlan)
	if len(plan.Operations) != 0 {
		t.Fatal("fixture must select an object without operations")
	}
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), boundNoteFixture(otherObjectID, "Changed"))
	if _, err := s.ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true}); domain.ErrorCode(err) != "plan_stale" {
		t.Fatal(err)
	}
}

func TestScopedOrganizeSelectionValidationAndConcurrentConsume(t *testing.T) {
	s, root := scopedFixture(t)
	for _, refs := range [][]string{{}, {""}, {"../Alpha.md"}, {"missing"}, {boundObjectID, "missing"}} {
		if _, err := s.PlanScopedOrganize(context.Background(), ScopedOrganizeRequest{VaultPath: root, NoteRefs: refs}); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	p, err := s.PlanScopedOrganize(context.Background(), ScopedOrganizeRequest{VaultPath: root, NoteRefs: []string{"beta.md"}})
	if err != nil || p.Facts["selected"] != "1" {
		t.Fatal("canonical display path rejected", err)
	}
	plan := scopedPlan(t, s, root)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := NewService().ApplyOrganize(context.Background(), ApplyRequest{VaultPath: root, PlanID: plan.PlanID, Yes: true})
			results <- err
		}()
	}
	count := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			count++
		}
	}
	if count != 1 {
		t.Fatal("plan did not apply exactly once")
	}
}

func TestScopedOrganizeMoveNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "a.md"), "source")
	writeAppFixture(t, filepath.Join(root, "b.md"), "target")
	if err := moveScopedNote(root, "a.md", "b.md"); err == nil {
		t.Fatal("overwrote destination")
	}
	b, _ := os.ReadFile(filepath.Join(root, "b.md"))
	if strings.TrimSpace(string(b)) != "target" {
		t.Fatal("target changed")
	}
}
