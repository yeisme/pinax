package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/app/publishops"
	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/domain"
)

// pipeline.go 实现统一管道交互面（pinax-pipeline-unified-ux-v1）：
//
//   - pinax.plan.v1 读模型：四管道（organize/metadata/repair/restore）reader
//     adapter 从既有存储反序列化并归一 plan 头字段；存储零迁移。
//   - pinax pipeline status：pending saved plans（含 freshness 派生）+ 最近 N 条
//     apply 型 receipt（apply_receipt.v1 / receipt.v1 / sync run / publish run）
//     的单一聚合视图。严格只读：不写 vault、.pinax/** 或远端。
//   - pinax pipeline show <plan-id|receipt-id>：统一详情（plan 风险分组 / receipt
//     applied 事实），changed paths 走既有 path redaction。

const (
	pipelineStatusDefaultReceiptLimit = 10
	pipelineStatusMaxReceiptLimit     = 50
	pipelineDigestHexLen              = 12
)

// PipelineStatusRequest drives pinax pipeline status.
type PipelineStatusRequest struct {
	VaultPath string
	Limit     int
	Kind      string
}

// PipelineShowRequest drives pinax pipeline show.
type PipelineShowRequest struct {
	VaultPath string
	ID        string
}

// PipelineStatus 聚合 pending saved plans 与最近 apply 型 receipts。只读。
func (s *Service) PipelineStatus(ctx context.Context, req PipelineStatusRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("pipeline.status", err), err
	}
	limit := pipelineReceiptLimit(req.Limit)
	kinds := pipelineKindSet(req.Kind)

	plans, unreadable := pipelinePlanViews(ctx, root, kinds)
	receipts := pipelineReceiptViews(root, kinds)
	if len(receipts) > limit {
		receipts = receipts[:limit]
	}

	projection := domain.NewProjection("pipeline.status", "Pipeline status read.")
	projection.Facts["vault"] = root
	projection.Facts["plans"] = fmt.Sprint(len(plans))
	projection.Facts["unreadable_plans"] = fmt.Sprint(len(unreadable))
	projection.Facts["receipts"] = fmt.Sprint(len(receipts))
	projection.Facts["limit"] = fmt.Sprint(limit)
	projection.Facts["schema_version"] = domain.PipelinePlanSchemaVersion
	if len(kinds) > 0 {
		projection.Facts["filter.kind"] = strings.Join(sortedSet(kinds), ",")
	}
	if len(unreadable) > 0 {
		projection.Status = "partial"
	}
	projection.Evidence = []string{
		filepath.ToSlash(filepath.Join(".pinax", "organize-plans")),
		filepath.ToSlash(filepath.Join(".pinax", "metadata-plans")),
		filepath.ToSlash(filepath.Join(".pinax", "repair-plans")),
		filepath.ToSlash(filepath.Join(".pinax", "restore-plans")),
		filepath.ToSlash(filepath.Join(".pinax", "receipts")),
		filepath.ToSlash(filepath.Join(".pinax", "sync-runs")),
		filepath.ToSlash(filepath.Join(".pinax", "publish", "runs")),
	}
	projection.Data = map[string]any{
		"schema_version": domain.PipelinePlanSchemaVersion,
		"readonly":       true,
		"plans":          plans,
		"unreadable":     unreadable,
		"receipts":       receipts,
		"next_commands":  pipelineNextCommands(root, plans),
	}
	if len(plans) > 0 {
		projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax pipeline show %s --vault %s --json", shellQuote(plans[0].PlanID), shellQuote(root))}}
	} else if len(receipts) > 0 {
		projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax pipeline show %s --vault %s --json", shellQuote(receipts[0].ReceiptID), shellQuote(root))}}
	} else {
		projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax repair plan --vault %s --save --json", shellQuote(root))}}
	}
	return projection, nil
}

// PipelineShow 展示单条 plan 或 receipt 的统一详情。只读。
func (s *Service) PipelineShow(ctx context.Context, req PipelineShowRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("pipeline.show", err), err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		err := &domain.CommandError{Code: "pipeline_id_required", Message: "pipeline show requires a plan or receipt id", Hint: "Run pinax pipeline status --vault <vault> --json to list ids"}
		return domain.NewErrorProjection("pipeline.show", err), err
	}
	if view, ok := pipelineShowPlan(ctx, root, id); ok {
		return view, nil
	}
	if view, ok := pipelineShowReceipt(root, id); ok {
		return view, nil
	}
	// 未知 id：稳定错误，不猜测就近匹配；提示用 status 列出可用 id。
	notFound := &domain.CommandError{Code: "pipeline_id_not_found", Message: "id does not match any saved plan or receipt", Hint: fmt.Sprintf("Run pinax pipeline status --vault %s --json to list available plan and receipt ids", shellQuote(root))}
	return domain.NewErrorProjection("pipeline.show", notFound), notFound
}

// ---------------------------------------------------------------------------
// Reader adapters：plan 读模型（pinax.plan.v1）
// ---------------------------------------------------------------------------

// pipelinePlanViews 依次运行四管道 reader adapter。单个 plan 文件损坏时以
// unreadable 条目 fail-closed 呈现，不影响其余条目。
func pipelinePlanViews(ctx context.Context, root string, kinds map[string]bool) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
	views := make([]domain.PipelinePlanView, 0)
	unreadable := make([]domain.PipelinePlanUnreadable, 0)
	adapters := []struct {
		kind string
		dir  string
		run  func(rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable)
	}{
		{domain.PipelineKindOrganize, ".pinax/organize-plans", func(rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
			return pipelineOrganizePlanViews(ctx, root, rel)
		}},
		{domain.PipelineKindMetadata, ".pinax/metadata-plans", func(rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
			return pipelineMetadataPlanViews(root, rel)
		}},
		{domain.PipelineKindRepair, ".pinax/repair-plans", func(rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
			return pipelineRepairPlanViews(ctx, root, rel)
		}},
		{domain.PipelineKindRestore, ".pinax/restore-plans", func(rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
			return pipelineRestorePlanViews(root, rel)
		}},
	}
	for _, adapter := range adapters {
		if len(kinds) > 0 && !kinds[adapter.kind] {
			continue
		}
		for _, rel := range pipelineJSONFiles(root, adapter.dir) {
			planViews, bad := adapter.run(rel)
			views = append(views, planViews...)
			unreadable = append(unreadable, bad...)
		}
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].CreatedAt == views[j].CreatedAt {
			return views[i].PlanID > views[j].PlanID
		}
		return views[i].CreatedAt > views[j].CreatedAt
	})
	return views, unreadable
}

func pipelineOrganizePlanViews(ctx context.Context, root, rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
	plan, err := loadOrganizePlan(root, strings.TrimSuffix(filepath.Base(rel), ".json"))
	if err != nil {
		return nil, []domain.PipelinePlanUnreadable{pipelineUnreadable(domain.PipelineKindOrganize, rel, err)}
	}
	view := domain.PipelinePlanView{
		SchemaVersion: domain.PipelinePlanSchemaVersion,
		PlanID:        plan.PlanID,
		Kind:          domain.PipelineKindOrganize,
		SourceSchema:  plan.SchemaVersion,
		Status:        plan.Status,
		CreatedAt:     plan.CreatedAt,
		ExpiresAt:     plan.ExpiresAt,
		FactsDigest:   pipelineFactsDigest(plan.SourceFacts),
		SavedPath:     pipelineSavedPath(plan.SavedPath, rel),
	}
	counts := map[string]int{}
	for _, op := range plan.Operations {
		counts[op.Kind]++
	}
	view.OperationsTotal = len(plan.Operations)
	view.OpCounts = counts
	fresh, reason := pipelineFreshness(func() error { return ensureOrganizePlanFresh(ctx, root, &plan) })
	view.Fresh, view.FreshReason = fresh, reason
	return []domain.PipelinePlanView{view}, nil
}

func pipelineMetadataPlanViews(root, rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
	plan, err := loadMetadataPlan(root, strings.TrimSuffix(filepath.Base(rel), ".json"))
	if err != nil {
		return nil, []domain.PipelinePlanUnreadable{pipelineUnreadable(domain.PipelineKindMetadata, rel, err)}
	}
	counts := map[string]int{}
	for _, op := range plan.Operations {
		counts[op.Kind]++
	}
	view := domain.PipelinePlanView{
		SchemaVersion:   domain.PipelinePlanSchemaVersion,
		PlanID:          plan.PlanID,
		Kind:            domain.PipelineKindMetadata,
		SourceSchema:    plan.SchemaVersion,
		Status:          plan.Status,
		CreatedAt:       plan.CreatedAt,
		ExpiresAt:       plan.ExpiresAt,
		OperationsTotal: len(plan.Operations),
		OpCounts:        counts,
		FactsDigest:     plan.SourceFacts["vault_hash"],
		SavedPath:       pipelineSavedPath(plan.SavedPath, rel),
	}
	fresh, reason := pipelineMetadataPlanFreshness(root, plan)
	view.Fresh, view.FreshReason = fresh, reason
	return []domain.PipelinePlanView{view}, nil
}

func pipelineRepairPlanViews(ctx context.Context, root, rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
	plan, err := loadRepairPlan(root, strings.TrimSuffix(filepath.Base(rel), ".json"))
	if err != nil {
		return nil, []domain.PipelinePlanUnreadable{pipelineUnreadable(domain.PipelineKindRepair, rel, err)}
	}
	counts := map[string]int{}
	for _, op := range plan.Operations {
		counts[op.Kind]++
	}
	view := domain.PipelinePlanView{
		SchemaVersion:   domain.PipelinePlanSchemaVersion,
		PlanID:          plan.PlanID,
		Kind:            domain.PipelineKindRepair,
		SourceSchema:    plan.SchemaVersion,
		Status:          plan.Status,
		CreatedAt:       plan.CreatedAt,
		ExpiresAt:       plan.ExpiresAt,
		OperationsTotal: len(plan.Operations),
		OpCounts:        counts,
		FactsDigest:     pipelineFactsDigest(plan.SourceFacts),
		SavedPath:       pipelineSavedPath(plan.SavedPath, rel),
	}
	fresh, reason := pipelineFreshness(func() error { return ensureRepairPlanFresh(ctx, root, &plan) })
	view.Fresh, view.FreshReason = fresh, reason
	return []domain.PipelinePlanView{view}, nil
}

func pipelineRestorePlanViews(root, rel string) ([]domain.PipelinePlanView, []domain.PipelinePlanUnreadable) {
	plan, err := loadRestorePlan(root, strings.TrimSuffix(filepath.Base(rel), ".json"))
	if err != nil {
		return nil, []domain.PipelinePlanUnreadable{pipelineUnreadable(domain.PipelineKindRestore, rel, err)}
	}
	view := domain.PipelinePlanView{
		SchemaVersion: domain.PipelinePlanSchemaVersion,
		PlanID:        plan.PlanID,
		Kind:          domain.PipelineKindRestore,
		SourceSchema:  plan.SchemaVersion,
		Status:        "planned",
		CreatedAt:     plan.CreatedAt,
		ExpiresAt:     plan.ExpiresAt,
		OpCounts:      map[string]int{plan.Operation.Kind: 1},
		FactsDigest:   plan.VaultHash,
		SavedPath:     pipelineSavedPath(plan.SavedPath, rel),
	}
	view.OperationsTotal = 1
	fresh, reason := pipelineRestorePlanFreshness(root, plan)
	view.Fresh, view.FreshReason = fresh, reason
	return []domain.PipelinePlanView{view}, nil
}

func pipelineUnreadable(kind, rel string, err error) domain.PipelinePlanUnreadable {
	message := "unreadable plan file"
	if err != nil {
		message = syncops.SanitizeString(err.Error())
	}
	return domain.PipelinePlanUnreadable{Kind: kind, SavedPath: rel, Error: message}
}

func pipelineSavedPath(savedPath, rel string) string {
	if strings.TrimSpace(savedPath) != "" {
		return savedPath
	}
	return rel
}

// pipelineJSONFiles 列出 vault 下某 plan 存储目录的全部 json 文件（排序）。
// 目录不存在时返回空——readers 对缺失存储保持静默。
func pipelineJSONFiles(root, dir string) []string {
	abs, err := safeJoin(root, dir)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Join(dir, entry.Name())))
	}
	sort.Strings(files)
	return files
}

// ---------------------------------------------------------------------------
// freshness：与各管道 apply 守卫同一套判定，读取端零漂移
// ---------------------------------------------------------------------------

// pipelineFreshness 把既有 ensure*Fresh 守卫包成只读 freshness 评估。
// reason 使用稳定错误码，plan_stale 细分 expired / vault_changed。
func pipelineFreshness(guard func() error) (bool, string) {
	err := guard()
	if err == nil {
		return true, ""
	}
	code := domain.ErrorCode(err)
	if code == "" {
		return false, "unreadable"
	}
	if code == "plan_stale" {
		if strings.Contains(err.Error(), "expired") {
			return false, code + ":expired"
		}
		return false, code + ":vault_changed"
	}
	return false, code
}

// pipelineMetadataPlanFreshness 与 metadata apply 守卫同一套判定：
// 过期 ⇒ stale；vault hash 漂移 ⇒ stale。
func pipelineMetadataPlanFreshness(root string, plan domain.MetadataPlan) (bool, string) {
	if pipelinePlanExpired(plan.ExpiresAt) {
		return false, "plan_stale:expired"
	}
	recorded := strings.TrimSpace(plan.SourceFacts["vault_hash"])
	if recorded == "" {
		return true, ""
	}
	current, err := versionVaultHash(root)
	if err != nil {
		return false, "unreadable"
	}
	if current != recorded {
		return false, "plan_stale:vault_changed"
	}
	return true, ""
}

// pipelineRestorePlanFreshness 与 version restore apply 守卫同一套判定。
func pipelineRestorePlanFreshness(root string, plan domain.RestorePlan) (bool, string) {
	if pipelinePlanExpired(plan.ExpiresAt) {
		return false, "plan_stale:expired"
	}
	if plan.VaultHash == "" {
		return true, ""
	}
	current, err := versionVaultHash(root)
	if err != nil {
		return false, "unreadable"
	}
	if current != plan.VaultHash {
		return false, "plan_stale:vault_changed"
	}
	return true, ""
}

// pipelineFactsDigest 把 plan 记录的 facts 摘要压缩成稳定短指纹（仅展示用）。
func pipelineFactsDigest(facts map[string]string) string {
	if len(facts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+facts[key])
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:pipelineDigestHexLen]
}

// ---------------------------------------------------------------------------
// Receipt readers：apply 型记录聚合（只读）
// ---------------------------------------------------------------------------

func pipelineReceiptViews(root string, kinds map[string]bool) []domain.PipelineReceiptView {
	receipts := make([]domain.PipelineReceiptView, 0)
	receipts = append(receipts, pipelineApplyReceiptViews(root, kinds)...)
	receipts = append(receipts, pipelineGenericReceiptViews(root, kinds)...)
	receipts = append(receipts, pipelineSyncRunViews(root, kinds)...)
	receipts = append(receipts, pipelinePublishRunViews(root, kinds)...)
	sort.SliceStable(receipts, func(i, j int) bool {
		if receipts[i].CreatedAt == receipts[j].CreatedAt {
			return receipts[i].ReceiptID > receipts[j].ReceiptID
		}
		return receipts[i].CreatedAt > receipts[j].CreatedAt
	})
	return receipts
}

// pipelineApplyReceiptViews 读取 pinax.apply_receipt.v1（organize/metadata/repair apply）。
func pipelineApplyReceiptViews(root string, kinds map[string]bool) []domain.PipelineReceiptView {
	views := make([]domain.PipelineReceiptView, 0)
	for _, rel := range pipelineJSONFiles(root, ".pinax/receipts") {
		payload, err := readPipelineJSON(root, rel)
		if err != nil {
			continue
		}
		var receipt domain.ApplyReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != domain.ApplyReceiptSchemaVersion {
			continue
		}
		pipeline := pipelineKindFromCommand(receipt.Command)
		if pipeline == "" || (len(kinds) > 0 && !kinds[pipeline]) {
			continue
		}
		if strings.TrimSpace(receipt.Status) == "" {
			receipt.Status = "applied"
		}
		views = append(views, domain.PipelineReceiptView{
			SchemaVersion: domain.PipelinePlanSchemaVersion,
			ReceiptID:     receipt.ReceiptID,
			Pipeline:      pipeline,
			Command:       receipt.Command,
			Status:        receipt.Status,
			PlanID:        receipt.PlanID,
			SnapshotID:    receipt.SnapshotID,
			ChangedPaths:  len(receipt.ChangedPaths),
			Counts:        map[string]int{"changed": len(receipt.ChangedPaths), "objects": len(receipt.Objects)},
			SavedPath:     rel,
			CreatedAt:     receipt.CreatedAt,
		})
	}
	return views
}

// pipelineGenericReceiptViews 读取 pinax.receipt.v1 中 apply 型 kind
// （restore、proof_loop）。
func pipelineGenericReceiptViews(root string, kinds map[string]bool) []domain.PipelineReceiptView {
	views := make([]domain.PipelineReceiptView, 0)
	for _, rel := range pipelineJSONFiles(root, ".pinax/receipts") {
		payload, err := readPipelineJSON(root, rel)
		if err != nil {
			continue
		}
		var receipt pipelineGenericReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != "pinax.receipt.v1" {
			continue
		}
		var pipeline string
		switch receipt.Kind {
		case "restore":
			pipeline = domain.PipelineKindRestore
		case "proof_loop":
			pipeline = domain.PipelineKindProofLoop
		default:
			continue
		}
		if len(kinds) > 0 && !kinds[pipeline] {
			continue
		}
		status := strings.TrimSpace(receipt.Status)
		if status == "" {
			status = "applied"
		}
		stem := strings.TrimSuffix(filepath.Base(rel), ".json")
		views = append(views, domain.PipelineReceiptView{
			SchemaVersion: domain.PipelinePlanSchemaVersion,
			ReceiptID:     stem,
			Pipeline:      pipeline,
			Command:       receipt.Kind,
			Status:        status,
			PlanID:        receipt.PlanID,
			RunID:         receipt.RunID,
			ChangedPaths:  0,
			SavedPath:     rel,
			CreatedAt:     receipt.CreatedAt,
		})
	}
	return views
}

// pipelineSyncRunViews 读取 .pinax/sync-runs 下的 sync run receipts。
func pipelineSyncRunViews(root string, kinds map[string]bool) []domain.PipelineReceiptView {
	if len(kinds) > 0 && !kinds[domain.PipelineKindSync] {
		return nil
	}
	views := make([]domain.PipelineReceiptView, 0)
	base, err := safeJoin(root, ".pinax/sync-runs")
	if err != nil {
		return nil
	}
	_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var receipt SyncRunReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != syncRunSchemaVersion {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		counts := map[string]int{}
		for _, key := range []string{"operations", "added", "modified", "deleted", "conflicts"} {
			if value := receipt.Counts[key]; value > 0 {
				counts[key] = value
			}
		}
		views = append(views, domain.PipelineReceiptView{
			SchemaVersion: domain.PipelinePlanSchemaVersion,
			ReceiptID:     receipt.RunID,
			Pipeline:      domain.PipelineKindSync,
			Command:       receipt.Command,
			Status:        receipt.Status,
			RunID:         receipt.RunID,
			ChangedPaths:  receipt.Counts["operations"],
			Counts:        counts,
			SavedPath:     filepath.ToSlash(rel),
			CreatedAt:     receipt.CreatedAt,
		})
		return nil
	})
	return views
}

// pipelinePublishRunViews 读取 .pinax/publish/runs/<run>/receipt.json。
func pipelinePublishRunViews(root string, kinds map[string]bool) []domain.PipelineReceiptView {
	if len(kinds) > 0 && !kinds[domain.PipelineKindPublish] {
		return nil
	}
	views := make([]domain.PipelineReceiptView, 0)
	base, err := safeJoin(root, ".pinax/publish/runs")
	if err != nil {
		return nil
	}
	_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Base(path) != "receipt.json" {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var receipt domain.PublishReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != publishops.PublishReceiptSchemaVersion {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		status := "built"
		if receipt.DeployStatus != "" && receipt.DeployStatus != "not_deployed" {
			status = receipt.DeployStatus
		}
		views = append(views, domain.PipelineReceiptView{
			SchemaVersion: domain.PipelinePlanSchemaVersion,
			ReceiptID:     receipt.RunID,
			Pipeline:      domain.PipelineKindPublish,
			Command:       "publish." + status,
			Status:        status,
			RunID:         receipt.RunID,
			ChangedPaths:  receipt.Counts["selected"] + receipt.Counts["assets"],
			Counts:        receipt.Counts,
			SavedPath:     filepath.ToSlash(rel),
			CreatedAt:     receipt.StartedAt,
		})
		return nil
	})
	return views
}

func pipelineKindFromCommand(command string) string {
	switch strings.TrimSpace(command) {
	case "organize.apply":
		return domain.PipelineKindOrganize
	case "metadata.apply":
		return domain.PipelineKindMetadata
	case "repair.apply":
		return domain.PipelineKindRepair
	default:
		return ""
	}
}

func readPipelineJSON(root, rel string) ([]byte, error) {
	path, err := safeJoin(root, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// ---------------------------------------------------------------------------
// show：plan / receipt 双形态
// ---------------------------------------------------------------------------

func pipelineShowPlan(ctx context.Context, root, id string) (domain.Projection, bool) {
	if projection, ok := pipelineShowOrganizePlan(ctx, root, id); ok {
		return projection, true
	}
	if projection, ok := pipelineShowMetadataPlan(root, id); ok {
		return projection, true
	}
	if projection, ok := pipelineShowRepairPlan(ctx, root, id); ok {
		return projection, true
	}
	if projection, ok := pipelineShowRestorePlan(root, id); ok {
		return projection, true
	}
	return domain.Projection{}, false
}

func pipelineShowOrganizePlan(ctx context.Context, root, id string) (domain.Projection, bool) {
	plan, err := loadOrganizePlan(root, id)
	if err != nil || plan.PlanID != id {
		return domain.Projection{}, false
	}
	counts := map[string]int{}
	ops := make([]domain.PipelinePlanOperationView, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		counts[op.Kind]++
		ops = append(ops, pipelineOperationView(op.Kind, op.Mode, op.Status, op.Path, op.Target, op.Reason, op.Risk))
	}
	view := domain.PipelinePlanView{
		SchemaVersion: domain.PipelinePlanSchemaVersion, PlanID: plan.PlanID, Kind: domain.PipelineKindOrganize,
		SourceSchema: plan.SchemaVersion, Status: plan.Status, CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
		OperationsTotal: len(plan.Operations), OpCounts: counts, FactsDigest: pipelineFactsDigest(plan.SourceFacts),
		SavedPath: pipelineSavedPath(plan.SavedPath, filepath.ToSlash(filepath.Join(".pinax", "organize-plans", id+".json"))),
	}
	view.Fresh, view.FreshReason = pipelineFreshness(func() error { return ensureOrganizePlanFresh(ctx, root, &plan) })
	return pipelinePlanShowProjection(root, view, ops), true
}

func pipelineShowMetadataPlan(root, id string) (domain.Projection, bool) {
	plan, err := loadMetadataPlan(root, id)
	if err != nil || plan.PlanID != id {
		return domain.Projection{}, false
	}
	counts := map[string]int{}
	ops := make([]domain.PipelinePlanOperationView, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		counts[op.Kind]++
		ops = append(ops, pipelineOperationView(op.Kind, "", op.Status, op.Path, op.Target, op.Reason, ""))
	}
	view := domain.PipelinePlanView{
		SchemaVersion: domain.PipelinePlanSchemaVersion, PlanID: plan.PlanID, Kind: domain.PipelineKindMetadata,
		SourceSchema: plan.SchemaVersion, Status: plan.Status, CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
		OperationsTotal: len(plan.Operations), OpCounts: counts, FactsDigest: plan.SourceFacts["vault_hash"],
		SavedPath: pipelineSavedPath(plan.SavedPath, filepath.ToSlash(filepath.Join(".pinax", "metadata-plans", id+".json"))),
	}
	view.Fresh, view.FreshReason = pipelineMetadataPlanFreshness(root, plan)
	return pipelinePlanShowProjection(root, view, ops), true
}

func pipelineShowRepairPlan(ctx context.Context, root, id string) (domain.Projection, bool) {
	plan, err := loadRepairPlan(root, id)
	if err != nil || plan.PlanID != id {
		return domain.Projection{}, false
	}
	counts := map[string]int{}
	ops := make([]domain.PipelinePlanOperationView, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		counts[op.Kind]++
		ops = append(ops, pipelineOperationView(op.Kind, op.Mode, op.Status, op.Path, op.Target, op.Reason, op.Risk))
	}
	view := domain.PipelinePlanView{
		SchemaVersion: domain.PipelinePlanSchemaVersion, PlanID: plan.PlanID, Kind: domain.PipelineKindRepair,
		SourceSchema: plan.SchemaVersion, Status: plan.Status, CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
		OperationsTotal: len(plan.Operations), OpCounts: counts, FactsDigest: pipelineFactsDigest(plan.SourceFacts),
		SavedPath: pipelineSavedPath(plan.SavedPath, filepath.ToSlash(filepath.Join(".pinax", "repair-plans", id+".json"))),
	}
	view.Fresh, view.FreshReason = pipelineFreshness(func() error { return ensureRepairPlanFresh(ctx, root, &plan) })
	return pipelinePlanShowProjection(root, view, ops), true
}

func pipelineShowRestorePlan(root, id string) (domain.Projection, bool) {
	plan, err := loadRestorePlan(root, id)
	if err != nil || plan.PlanID != id {
		return domain.Projection{}, false
	}
	op := plan.Operation
	ops := []domain.PipelinePlanOperationView{pipelineOperationView(op.Kind, "", op.Status, op.Path, plan.Revision, op.Reason, "")}
	view := domain.PipelinePlanView{
		SchemaVersion: domain.PipelinePlanSchemaVersion, PlanID: plan.PlanID, Kind: domain.PipelineKindRestore,
		SourceSchema: plan.SchemaVersion, Status: "planned", CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
		OperationsTotal: 1, OpCounts: map[string]int{op.Kind: 1}, FactsDigest: plan.VaultHash,
		SavedPath: pipelineSavedPath(plan.SavedPath, filepath.ToSlash(filepath.Join(".pinax", "restore-plans", id+".json"))),
	}
	view.Fresh, view.FreshReason = pipelineRestorePlanFreshness(root, plan)
	return pipelinePlanShowProjection(root, view, ops), true
}

// pipelineOperationView 把单条 plan 操作投影为风险分组视图。
// vault 结构写入（move/version_restore）与元数据写入分组；manual_review 单列。
func pipelineOperationView(kind, mode, status, path, target, reason, risk string) domain.PipelinePlanOperationView {
	group := "metadata_write"
	if status == "manual_review" || mode == "manual_review" {
		group = "manual_review"
	} else if status == "skipped" || status == "applied" {
		group = "skipped"
	} else {
		switch kind {
		case "move", "version_restore":
			group = "vault_write"
		}
	}
	return domain.PipelinePlanOperationView{Group: group, Kind: kind, Path: path, Target: target, Reason: reason, Status: status, Risk: risk}
}

func pipelinePlanShowProjection(root string, view domain.PipelinePlanView, ops []domain.PipelinePlanOperationView) domain.Projection {
	projection := domain.NewProjection("pipeline.show", "Pipeline plan read.")
	projection.Facts["plan_id"] = view.PlanID
	projection.Facts["kind"] = view.Kind
	projection.Facts["source_schema"] = view.SourceSchema
	projection.Facts["schema_version"] = domain.PipelinePlanSchemaVersion
	projection.Facts["created_at"] = view.CreatedAt
	if view.ExpiresAt != "" {
		projection.Facts["expires_at"] = view.ExpiresAt
	}
	if view.Fresh {
		projection.Facts["freshness"] = "fresh"
	} else {
		projection.Facts["freshness"] = "stale"
		projection.Facts["fresh_reason"] = view.FreshReason
	}
	if view.FactsDigest != "" {
		projection.Facts["facts_digest"] = view.FactsDigest
	}
	projection.Facts["vault_writes"] = fmt.Sprint(countPipelineOpGroup(ops, "vault_write"))
	projection.Facts["metadata_writes"] = fmt.Sprint(countPipelineOpGroup(ops, "metadata_write"))
	projection.Facts["manual_review"] = fmt.Sprint(countPipelineOpGroup(ops, "manual_review"))
	projection.Evidence = []string{view.SavedPath}
	next := pipelinePlanApplyCommand(root, view.Kind, view.PlanID)
	projection.Actions = []domain.Action{{Name: "apply", Command: next}}
	projection.Data = map[string]any{
		"schema_version": domain.PipelinePlanSchemaVersion,
		"plan":           view,
		"operations":     ops,
		"changed_paths":  redactPipelinePaths(pipelineChangedPaths(ops)),
		"next":           next,
		"readonly":       true,
	}
	return projection
}

func countPipelineOpGroup(ops []domain.PipelinePlanOperationView, group string) int {
	count := 0
	for _, op := range ops {
		if op.Group == group {
			count++
		}
	}
	return count
}

// pipelineChangedPaths 汇总操作涉及的路径（source 与 target），去重排序。
func pipelineChangedPaths(ops []domain.PipelinePlanOperationView) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	for _, op := range ops {
		for _, value := range []string{op.Path, op.Target} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			paths = append(paths, value)
		}
	}
	sort.Strings(paths)
	return paths
}

func pipelinePlanApplyCommand(root, kind, planID string) string {
	switch kind {
	case domain.PipelineKindOrganize:
		return fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(planID))
	case domain.PipelineKindMetadata:
		return fmt.Sprintf("pinax metadata apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(planID))
	case domain.PipelineKindRepair:
		return fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(planID))
	case domain.PipelineKindRestore:
		return fmt.Sprintf("pinax version restore apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(planID))
	default:
		return fmt.Sprintf("pinax pipeline status --vault %s --json", shellQuote(root))
	}
}

func pipelineNextCommands(root string, plans []domain.PipelinePlanView) []string {
	commands := make([]string, 0, len(plans))
	for _, plan := range plans {
		commands = append(commands, pipelinePlanApplyCommand(root, plan.Kind, plan.PlanID))
	}
	return commands
}

func pipelineShowReceipt(root, id string) (domain.Projection, bool) {
	// 1) apply_receipt.v1（receipt id 即 apply-<digest>）。
	for _, rel := range pipelineJSONFiles(root, ".pinax/receipts") {
		payload, err := readPipelineJSON(root, rel)
		if err != nil {
			continue
		}
		var receipt domain.ApplyReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != domain.ApplyReceiptSchemaVersion {
			continue
		}
		if receipt.ReceiptID == id {
			return pipelineApplyReceiptShowProjection(root, receipt, rel), true
		}
	}
	// 2) receipt.v1 中的聚合 kind（restore、proof_loop）：id 命中文件名 stem 或 run_id。
	if projection, ok := pipelineGenericReceiptShowProjection(root, id); ok {
		return projection, true
	}
	// 3) sync run receipts（id 即 run_id）。
	if projection, ok := pipelineSyncRunShowProjection(root, id); ok {
		return projection, true
	}
	// 4) publish run receipts（id 即 run id）。
	if projection, ok := pipelinePublishRunShowProjection(root, id); ok {
		return projection, true
	}
	return domain.Projection{}, false
}

// pipelineGenericReceipt 是 pinax.receipt.v1 中聚合 kind（restore、proof_loop）
// 的有界读字段。
type pipelineGenericReceipt struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	PlanID        string `json:"plan_id"`
	RunID         string `json:"run_id"`
	Path          string `json:"path"`
	Revision      string `json:"revision"`
	Mode          string `json:"mode"`
	CreatedAt     string `json:"created_at"`
}

// pipelineGenericReceiptShowProjection 展示 pinax.receipt.v1 的 restore/proof_loop
// 收据（apply 事实 + 有界 facts）。
func pipelineGenericReceiptShowProjection(root, id string) (domain.Projection, bool) {
	for _, rel := range pipelineJSONFiles(root, ".pinax/receipts") {
		payload, err := readPipelineJSON(root, rel)
		if err != nil {
			continue
		}
		var receipt pipelineGenericReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != "pinax.receipt.v1" {
			continue
		}
		var pipeline string
		switch receipt.Kind {
		case "restore":
			pipeline = domain.PipelineKindRestore
		case "proof_loop":
			pipeline = domain.PipelineKindProofLoop
		default:
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(rel), ".json")
		if stem != id && receipt.RunID != id {
			continue
		}
		status := defaultString(strings.TrimSpace(receipt.Status), "applied")
		projection := domain.NewProjection("pipeline.show", "Pipeline receipt read.")
		projection.Facts["receipt_id"] = stem
		projection.Facts["pipeline"] = pipeline
		projection.Facts["command"] = receipt.Kind
		projection.Facts["status"] = status
		projection.Facts["changed_paths"] = "0"
		if receipt.PlanID != "" {
			projection.Facts["plan_id"] = receipt.PlanID
		}
		if receipt.RunID != "" {
			projection.Facts["run_id"] = syncops.SanitizeString(receipt.RunID)
		}
		if receipt.Path != "" {
			projection.Facts["changed_paths"] = "1"
			projection.Facts["path"] = syncops.RedactPath(receipt.Path, "default")
		}
		if receipt.Revision != "" {
			projection.Facts["revision"] = syncops.SanitizeString(receipt.Revision)
		}
		if receipt.Mode != "" {
			projection.Facts["mode"] = syncops.SanitizeString(receipt.Mode)
		}
		projection.Evidence = []string{rel}
		projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax pipeline status --vault %s --json", shellQuote(root))}}
		projection.Data = map[string]any{
			"schema_version": domain.PipelinePlanSchemaVersion,
			"receipt":        receipt.sanitized(),
			"readonly":       true,
		}
		return projection, true
	}
	return domain.Projection{}, false
}

// sanitized 只保留可安全展示的字段并走 path redaction。
func (receipt pipelineGenericReceipt) sanitized() map[string]string {
	view := map[string]string{
		"schema_version": receipt.SchemaVersion,
		"kind":           receipt.Kind,
		"status":         receipt.Status,
		"created_at":     receipt.CreatedAt,
	}
	for key, value := range map[string]string{
		"plan_id":  receipt.PlanID,
		"run_id":   syncops.SanitizeString(receipt.RunID),
		"path":     syncops.RedactPath(receipt.Path, "default"),
		"revision": syncops.SanitizeString(receipt.Revision),
		"mode":     syncops.SanitizeString(receipt.Mode),
	} {
		if strings.TrimSpace(value) != "" {
			view[key] = value
		}
	}
	return view
}

func pipelineApplyReceiptShowProjection(root string, receipt domain.ApplyReceipt, rel string) domain.Projection {
	projection := domain.NewProjection("pipeline.show", "Pipeline receipt read.")
	projection.Facts["receipt_id"] = receipt.ReceiptID
	projection.Facts["pipeline"] = pipelineKindFromCommand(receipt.Command)
	projection.Facts["command"] = receipt.Command
	projection.Facts["status"] = defaultString(receipt.Status, "applied")
	projection.Facts["changed_paths"] = fmt.Sprint(len(receipt.ChangedPaths))
	projection.Facts["objects"] = fmt.Sprint(len(receipt.Objects))
	projection.Facts["ledger_seq"] = fmt.Sprint(receipt.LedgerSeq)
	if receipt.PlanID != "" {
		projection.Facts["plan_id"] = receipt.PlanID
	}
	if receipt.SnapshotID != "" {
		projection.Facts["snapshot_id"] = receipt.SnapshotID
	}
	projection.Evidence = []string{receipt.SavedPath, rel}
	projection.Actions = []domain.Action{{Name: "show_plan", Command: fmt.Sprintf("pinax pipeline show %s --vault %s --json", shellQuote(receipt.PlanID), shellQuote(root))}}
	if receipt.PlanID == "" {
		projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax pipeline status --vault %s --json", shellQuote(root))}}
	}
	projection.Data = map[string]any{
		"schema_version": domain.PipelinePlanSchemaVersion,
		"receipt":        receipt,
		"changed_paths":  redactPipelinePaths(receipt.ChangedPaths),
		"readonly":       true,
	}
	return projection
}

func pipelineSyncRunShowProjection(root, id string) (domain.Projection, bool) {
	base, err := safeJoin(root, ".pinax/sync-runs")
	if err != nil {
		return domain.Projection{}, false
	}
	var found *SyncRunReceipt
	var foundRel string
	_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if found != nil || walkErr != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var receipt SyncRunReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != syncRunSchemaVersion || receipt.RunID != id {
			return nil
		}
		found = &receipt
		foundRel = path
		return nil
	})
	if found == nil {
		return domain.Projection{}, false
	}
	rel, _ := filepath.Rel(root, foundRel)
	projection := domain.NewProjection("pipeline.show", "Pipeline receipt read.")
	projection.Facts["receipt_id"] = found.RunID
	projection.Facts["pipeline"] = domain.PipelineKindSync
	projection.Facts["command"] = found.Command
	projection.Facts["status"] = found.Status
	projection.Facts["direction"] = found.Direction
	projection.Facts["target"] = found.Target
	projection.Facts["operations"] = fmt.Sprint(found.Counts["operations"])
	if found.RevisionID != "" {
		projection.Facts["revision_id"] = syncops.SanitizeString(found.RevisionID)
	}
	projection.Evidence = []string{filepath.ToSlash(rel)}
	projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", shellQuote(found.RunID), shellQuote(root))}}
	projection.Data = map[string]any{
		"schema_version": domain.PipelinePlanSchemaVersion,
		"receipt":        redactSyncRunReceipt(*found),
		"readonly":       true,
	}
	return projection, true
}

func redactSyncRunReceipt(receipt SyncRunReceipt) SyncRunReceipt {
	receipt.WorkspaceID = syncops.SanitizeString(receipt.WorkspaceID)
	receipt.DeviceID = syncops.SanitizeString(receipt.DeviceID)
	receipt.RequestID = syncops.SanitizeString(receipt.RequestID)
	receipt.BaseRevision = syncops.SanitizeString(receipt.BaseRevision)
	receipt.RemoteRevisionBefore = syncops.SanitizeString(receipt.RemoteRevisionBefore)
	receipt.RevisionID = syncops.SanitizeString(receipt.RevisionID)
	receipt.ManifestBlobID = syncops.SanitizeString(receipt.ManifestBlobID)
	receipt.Operations = nil
	return receipt
}

func pipelinePublishRunShowProjection(root, id string) (domain.Projection, bool) {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return domain.Projection{}, false
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "publish", "runs", id, "receipt.json"))
	payload, err := readPipelineJSON(root, rel)
	if err != nil {
		return domain.Projection{}, false
	}
	var receipt domain.PublishReceipt
	if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SchemaVersion != publishops.PublishReceiptSchemaVersion {
		return domain.Projection{}, false
	}
	projection := domain.NewProjection("pipeline.show", "Pipeline receipt read.")
	projection.Facts["receipt_id"] = receipt.RunID
	projection.Facts["pipeline"] = domain.PipelineKindPublish
	projection.Facts["status"] = defaultString(receipt.DeployStatus, "built")
	projection.Facts["profile"] = receipt.ProfileName
	projection.Facts["target"] = string(receipt.Target)
	if receipt.OutputHash != "" {
		projection.Facts["output_hash"] = receipt.OutputHash
	}
	projection.Evidence = []string{rel}
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax pipeline status --vault %s --json", shellQuote(root))}}
	projection.Data = map[string]any{
		"schema_version": domain.PipelinePlanSchemaVersion,
		"receipt":        receipt,
		"readonly":       true,
	}
	return projection, true
}

// redactPipelinePaths 对 changed paths 施加既有 path redaction 策略。
func redactPipelinePaths(paths []string) []string {
	redacted := make([]string, 0, len(paths))
	for _, path := range paths {
		if value := syncops.RedactPath(path, "default"); value != "" {
			redacted = append(redacted, value)
		}
	}
	return redacted
}

func pipelineReceiptLimit(limit int) int {
	if limit <= 0 {
		return pipelineStatusDefaultReceiptLimit
	}
	if limit > pipelineStatusMaxReceiptLimit {
		return pipelineStatusMaxReceiptLimit
	}
	return limit
}

func pipelineKindSet(value string) map[string]bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "all") {
		return nil
	}
	set := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		switch part {
		case domain.PipelineKindOrganize, domain.PipelineKindMetadata, domain.PipelineKindRepair, domain.PipelineKindRestore, domain.PipelineKindSync, domain.PipelineKindPublish, domain.PipelineKindProofLoop, "proof loop":
			if part == "proof loop" {
				part = domain.PipelineKindProofLoop
			}
			set[part] = true
		default:
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func sortedSet(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
