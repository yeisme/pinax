package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

// pipeline_test.go 覆盖 pinax.plan.v1 读模型（reader adapter 归一 + freshness 派生）
// 与 pipeline status/show 聚合（只读、unreadable fail-closed、receipt 归一）。

func pipelineTestVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000a1\ntitle: Alpha\ncreated: 2026-01-01\nupdated: 2026-01-01\nstatus: active\ntags:\n  - research\n---\n\n# Alpha\n\nalpha body\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000b2\ntitle: Beta\ncreated: 2026-01-02\nupdated: 2026-01-02\nstatus: active\n---\n\n# Beta\n\nbeta body\n")
	writeAppFixture(t, filepath.Join(root, "notes", "orphan-note.md"), "plain markdown without pinax frontmatter\n")
	return root
}

func pipelineStatusData(t *testing.T, projection domain.Projection) map[string]any {
	t.Helper()
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("pipeline status data = %#v", projection.Data)
	}
	return data
}

func pipelinePlansOf(t *testing.T, data map[string]any) []domain.PipelinePlanView {
	t.Helper()
	plans, ok := data["plans"].([]domain.PipelinePlanView)
	if !ok {
		t.Fatalf("plans = %#v", data["plans"])
	}
	return plans
}

func TestPipelinePlanReaderAdaptersNormalizeSavedPlans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()

	if _, err := svc.PlanRepair(ctx, RepairPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatalf("repair plan: %v", err)
	}
	if _, err := svc.PlanMetadata(ctx, MetadataPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatalf("metadata plan: %v", err)
	}
	writePipelineRestorePlanFixture(t, root, "restore_fixture01", "2026-01-01T00:00:00Z")

	projection, err := svc.PipelineStatus(ctx, PipelineStatusRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	data := pipelineStatusData(t, projection)
	plans := pipelinePlansOf(t, data)
	if len(plans) != 3 {
		t.Fatalf("plans = %#v", plans)
	}
	byKind := map[string]domain.PipelinePlanView{}
	for _, plan := range plans {
		byKind[plan.Kind] = plan
	}
	for kind, sourceSchema := range map[string]string{
		domain.PipelineKindRepair:   "pinax.repair_plan.v1",
		domain.PipelineKindMetadata: "pinax.metadata_plan.v1",
		domain.PipelineKindRestore:  "pinax.restore_plan.v1",
	} {
		plan, ok := byKind[kind]
		if !ok {
			t.Fatalf("missing %s plan: %#v", kind, plans)
		}
		if plan.SchemaVersion != domain.PipelinePlanSchemaVersion {
			t.Fatalf("%s plan schema = %q", kind, plan.SchemaVersion)
		}
		if plan.SourceSchema != sourceSchema {
			t.Fatalf("%s plan source schema = %q, want %q", kind, plan.SourceSchema, sourceSchema)
		}
		if plan.PlanID == "" || plan.CreatedAt == "" || plan.SavedPath == "" {
			t.Fatalf("%s plan header incomplete: %#v", kind, plan)
		}
		if !plan.Fresh {
			t.Fatalf("%s plan should be fresh on an unchanged vault: %#v", kind, plan)
		}
		if plan.FactsDigest == "" {
			t.Fatalf("%s plan facts digest missing: %#v", kind, plan)
		}
	}
	if _, ok := byKind[domain.PipelineKindOrganize]; ok {
		t.Fatalf("unexpected organize plan: %#v", plans)
	}
	if projection.Facts["plans"] != "3" {
		t.Fatalf("plans fact = %q", projection.Facts["plans"])
	}
	if projection.Facts["unreadable_plans"] != "0" {
		t.Fatalf("unreadable fact = %q", projection.Facts["unreadable_plans"])
	}
}

func TestPipelineKindFilterLimitsStatusToRequestedPipelines(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()
	if _, err := svc.PlanRepair(ctx, RepairPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PlanMetadata(ctx, MetadataPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatal(err)
	}
	projection, err := svc.PipelineStatus(ctx, PipelineStatusRequest{VaultPath: root, Kind: "repair"})
	if err != nil {
		t.Fatal(err)
	}
	plans := pipelinePlansOf(t, pipelineStatusData(t, projection))
	if len(plans) != 1 || plans[0].Kind != domain.PipelineKindRepair {
		t.Fatalf("filtered plans = %#v", plans)
	}
	if projection.Facts["filter.kind"] != "repair" {
		t.Fatalf("filter fact = %q", projection.Facts["filter.kind"])
	}
}

func TestPipelineFreshnessMarksPlanStaleAfterNoteChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()

	if _, err := svc.PlanMetadata(ctx, MetadataPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatal(err)
	}
	// 改一个 note ⇒ plan 的 vault 指纹漂移 ⇒ stale。
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000a1\ntitle: Alpha\ncreated: 2026-01-01\nupdated: 2026-01-03\nstatus: active\n---\n\n# Alpha\n\nchanged body\n")

	projection, err := svc.PipelineStatus(ctx, PipelineStatusRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	plans := pipelinePlansOf(t, pipelineStatusData(t, projection))
	if len(plans) != 1 {
		t.Fatalf("plans = %#v", plans)
	}
	if plans[0].Fresh {
		t.Fatalf("plan should be stale after note change: %#v", plans[0])
	}
	if !strings.Contains(plans[0].FreshReason, "vault_changed") {
		t.Fatalf("fresh reason = %q", plans[0].FreshReason)
	}
	// show 同一判定。
	show, err := svc.PipelineShow(ctx, PipelineShowRequest{VaultPath: root, ID: plans[0].PlanID})
	if err != nil {
		t.Fatal(err)
	}
	if show.Facts["freshness"] != "stale" || show.Facts["fresh_reason"] != plans[0].FreshReason {
		t.Fatalf("show freshness = %#v", show.Facts)
	}
}

func TestPipelineStalePlanApplyRejectedThenAllowedWithEscapeHatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()

	planProjection, err := svc.PlanMetadata(ctx, MetadataPlanRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	planID := planProjection.Facts["plan_id"]
	// plan 之后 vault 漂移（orphan note 被改写）。
	writeAppFixture(t, filepath.Join(root, "notes", "orphan-note.md"), "changed plain markdown\n")

	rejected, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root, PlanID: planID, Yes: true})
	if err == nil {
		t.Fatalf("stale apply should be rejected: %#v", rejected)
	}
	if domain.ErrorCode(err) != "plan_stale" {
		t.Fatalf("error code = %q", domain.ErrorCode(err))
	}
	if len(rejected.PipelineStages) != 2 ||
		rejected.PipelineStages[0].Type != domain.PipelineStageStarted ||
		rejected.PipelineStages[1].Type != domain.PipelineStageFailed ||
		rejected.PipelineStages[1].Reason != "plan_stale" {
		t.Fatalf("stale rejection stages = %#v", rejected.PipelineStages)
	}

	applied, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root, PlanID: planID, Yes: true, AllowStale: true})
	if err != nil {
		t.Fatalf("allow-stale apply: %v", err)
	}
	if applied.Facts["allow_stale"] != "true" {
		t.Fatalf("allow_stale fact = %#v", applied.Facts)
	}
	if len(applied.Warnings) == 0 || applied.Warnings[0].Code != "plan_stale_overridden" {
		t.Fatalf("warnings = %#v", applied.Warnings)
	}
	// 逃生门路径照常写 receipt，receipt 进入 status 聚合。
	status, err := svc.PipelineStatus(ctx, PipelineStatusRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	data := pipelineStatusData(t, status)
	receipts, ok := data["receipts"].([]domain.PipelineReceiptView)
	if !ok || len(receipts) == 0 {
		t.Fatalf("receipts = %#v", data["receipts"])
	}
	found := false
	for _, receipt := range receipts {
		if receipt.Pipeline == domain.PipelineKindMetadata && receipt.PlanID == planID && receipt.Status == "applied" {
			found = true
		}
	}
	if !found {
		t.Fatalf("metadata receipt missing from status: %#v", receipts)
	}
}

func TestPipelineUnreadablePlanFailsClosedWithoutInterruptingOthers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()
	if _, err := svc.PlanRepair(ctx, RepairPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PlanMetadata(ctx, MetadataPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatal(err)
	}
	// 损坏 metadata plan 文件：unreadable 条目呈现，repair plan 正常列出。
	corrupt := filepath.Join(root, ".pinax", "metadata-plans")
	entries, err := os.ReadDir(corrupt)
	if err != nil || len(entries) == 0 {
		t.Fatalf("metadata plans dir: %v %#v", err, entries)
	}
	if err := os.WriteFile(filepath.Join(corrupt, entries[0].Name()), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	projection, err := svc.PipelineStatus(ctx, PipelineStatusRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	data := pipelineStatusData(t, projection)
	plans := pipelinePlansOf(t, data)
	if len(plans) != 1 || plans[0].Kind != domain.PipelineKindRepair {
		t.Fatalf("plans after corruption = %#v", plans)
	}
	unreadable, ok := data["unreadable"].([]domain.PipelinePlanUnreadable)
	if !ok || len(unreadable) != 1 {
		t.Fatalf("unreadable = %#v", data["unreadable"])
	}
	if unreadable[0].Kind != domain.PipelineKindMetadata || unreadable[0].SavedPath == "" || unreadable[0].Error == "" {
		t.Fatalf("unreadable entry = %#v", unreadable[0])
	}
	if projection.Status != "partial" {
		t.Fatalf("status = %q, want partial", projection.Status)
	}
	if projection.Facts["unreadable_plans"] != "1" {
		t.Fatalf("unreadable fact = %q", projection.Facts["unreadable_plans"])
	}
}

func TestPipelineShowUnknownIDReturnsStableError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	projection, err := NewService().PipelineShow(ctx, PipelineShowRequest{VaultPath: root, ID: "plan-does-not-exist"})
	if err == nil {
		t.Fatalf("unknown id should fail: %#v", projection)
	}
	if domain.ErrorCode(err) != "pipeline_id_not_found" {
		t.Fatalf("error code = %q", domain.ErrorCode(err))
	}
	if !strings.Contains(projection.Error.Hint, "pinax pipeline status") {
		t.Fatalf("hint = %q", projection.Error.Hint)
	}
}

func TestPipelineShowPlanGroupsVaultWritesAndMetadataWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()
	// metadata plan：orphan note 产生 metadata_update（元数据写入分组）。
	planProjection, err := svc.PlanMetadata(ctx, MetadataPlanRequest{VaultPath: root, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	planID := planProjection.Facts["plan_id"]
	show, err := svc.PipelineShow(ctx, PipelineShowRequest{VaultPath: root, ID: planID})
	if err != nil {
		t.Fatal(err)
	}
	if show.Facts["kind"] != domain.PipelineKindMetadata || show.Facts["plan_id"] != planID {
		t.Fatalf("show facts = %#v", show.Facts)
	}
	data, ok := show.Data.(map[string]any)
	if !ok {
		t.Fatalf("show data = %#v", show.Data)
	}
	operations, ok := data["operations"].([]domain.PipelinePlanOperationView)
	if !ok || len(operations) == 0 {
		t.Fatalf("operations = %#v", data["operations"])
	}
	for _, op := range operations {
		if op.Group != "metadata_write" {
			t.Fatalf("metadata op group = %q", op.Group)
		}
	}
	if show.Facts["metadata_writes"] != strconv.Itoa(len(operations)) {
		t.Fatalf("metadata_writes fact = %q", show.Facts["metadata_writes"])
	}
	changed, ok := data["changed_paths"].([]string)
	if !ok || len(changed) == 0 {
		t.Fatalf("changed paths = %#v", data["changed_paths"])
	}
	for _, path := range changed {
		if !strings.HasPrefix(path, "notes/") {
			t.Fatalf("changed path outside notes: %#v", changed)
		}
	}
}

func TestPipelineShowReceiptAppliesRedactedChangedPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()
	applied, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	receiptID := applied.Facts["receipt_id"]
	show, err := svc.PipelineShow(ctx, PipelineShowRequest{VaultPath: root, ID: receiptID})
	if err != nil {
		t.Fatal(err)
	}
	if show.Facts["pipeline"] != domain.PipelineKindMetadata || show.Facts["status"] != "applied" {
		t.Fatalf("show facts = %#v", show.Facts)
	}
	if show.Facts["ledger_seq"] == "" {
		t.Fatalf("ledger seq missing: %#v", show.Facts)
	}
	data := show.Data.(map[string]any)
	if _, ok := data["changed_paths"].([]string); !ok {
		t.Fatalf("changed paths = %#v", data["changed_paths"])
	}
	if _, ok := data["receipt"].(domain.ApplyReceipt); !ok {
		t.Fatalf("receipt = %#v", data["receipt"])
	}
}

func TestPipelineStatusIsReadonly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	svc := NewService()
	if _, err := svc.PlanRepair(ctx, RepairPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatal(err)
	}
	before := pipelineTreeSnapshot(t, root)
	if _, err := svc.PipelineStatus(ctx, PipelineStatusRequest{VaultPath: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PipelineShow(ctx, PipelineShowRequest{VaultPath: root, ID: "any-id"}); err == nil {
		t.Fatal("unknown id should fail")
	}
	after := pipelineTreeSnapshot(t, root)
	if before != after {
		t.Fatalf("pipeline status/show mutated vault:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestPipelineGenericAndPublishReceiptViews(t *testing.T) {
	t.Parallel()
	root := pipelineTestVault(t)
	// proof_loop receipt（pinax.receipt.v1）进入聚合。
	if _, err := writeReceipt(root, "proof_loop", map[string]any{"run_id": "proof_loop_20260101T000000Z", "status": "applied"}); err != nil {
		t.Fatal(err)
	}
	views := pipelineReceiptViews(root, nil)
	if len(views) != 1 {
		t.Fatalf("views = %#v", views)
	}
	if views[0].Pipeline != domain.PipelineKindProofLoop || views[0].RunID != "proof_loop_20260101T000000Z" {
		t.Fatalf("view = %#v", views[0])
	}
	if !strings.HasPrefix(views[0].ReceiptID, "proof_loop-") || !strings.HasSuffix(views[0].ReceiptID, "Z") {
		t.Fatalf("receipt id = %q", views[0].ReceiptID)
	}
	// show 双形态：receipt id（文件名 stem）与 run_id 都能命中。
	svc := NewService()
	show, showErr := svc.PipelineShow(context.Background(), PipelineShowRequest{VaultPath: root, ID: views[0].ReceiptID})
	if showErr != nil {
		t.Fatalf("show by receipt id: %v", showErr)
	}
	if show.Facts["pipeline"] != domain.PipelineKindProofLoop {
		t.Fatalf("show facts = %#v", show.Facts)
	}
	showByRun, showByRunErr := svc.PipelineShow(context.Background(), PipelineShowRequest{VaultPath: root, ID: "proof_loop_20260101T000000Z"})
	if showByRunErr != nil {
		t.Fatalf("show by run id: %v", showByRunErr)
	}
	if showByRun.Facts["pipeline"] != domain.PipelineKindProofLoop {
		t.Fatalf("show by run id facts = %#v", showByRun.Facts)
	}
	// 非聚合 kind（import receipt）不进入聚合。
	if _, err := writeReceipt(root, "import", map[string]any{"status": "applied"}); err != nil {
		t.Fatal(err)
	}
	views = pipelineReceiptViews(root, nil)
	if len(views) != 1 {
		t.Fatalf("views after import receipt = %#v", views)
	}
}

func TestPipelineFactsDigestIsStableAndOrderInsensitive(t *testing.T) {
	t.Parallel()
	a := pipelineFactsDigest(map[string]string{"notes": "2", "note.x.sha1": "aa"})
	b := pipelineFactsDigest(map[string]string{"note.x.sha1": "aa", "notes": "2"})
	if a == "" || a != b {
		t.Fatalf("digest = %q vs %q", a, b)
	}
	if pipelineFactsDigest(map[string]string{"notes": "3", "note.x.sha1": "aa"}) == a {
		t.Fatalf("digest should change with facts")
	}
}

func writePipelineRestorePlanFixture(t *testing.T, root, planID, createdAt string) {
	t.Helper()
	vaultHash, err := versionVaultHash(root)
	if err != nil {
		t.Fatal(err)
	}
	plan := domain.RestorePlan{
		SchemaVersion:  "pinax.restore_plan.v1",
		PlanID:         planID,
		CreatedAt:      createdAt,
		ExpiresAt:      time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		VaultRoot:      root,
		VaultHash:      vaultHash,
		Path:           "notes/alpha.md",
		Revision:       "rev_1",
		VersionBackend: "local",
		Operation:      domain.PlanOperation{Kind: "version_restore", Path: "notes/alpha.md", Reason: "fixture", Status: "planned"},
	}
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".pinax", "restore-plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, planID+".json"), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pipelineTreeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		lines = append(lines, rel+" "+strconv.Itoa(int(info.Size()))+" "+strconv.Itoa(int(info.Mode().Perm())))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
