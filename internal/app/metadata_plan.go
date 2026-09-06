package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
)

// metadata_plan.go 实现可保存的 metadata backfill 计划（pinax.metadata_plan.v1）。
//
// metadata plan 默认仍是内存态 preview；--save 后落入 .pinax/metadata-plans，
// apply --plan 消费该计划。apply 前接受统一 freshness 守卫：vault hash 漂移 ⇒
// plan_stale 拒绝（--allow-stale 逃生门）。无 --plan 的单命令流程不受影响。

// pipelineMetadataPlanSchemaVersion 是可保存 metadata plan 的 schema。
const pipelineMetadataPlanSchemaVersion = "pinax.metadata_plan.v1"

// MetadataPlanRequest drives pinax metadata plan.
type MetadataPlanRequest struct {
	VaultPath string
	Query     string
	Save      bool
}

// pipelinePlanExpired 判断 plan 的 ExpiresAt 是否已过（空值或解析失败视为未过期）。
func pipelinePlanExpired(expiresAt string) bool {
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return false
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(expires)
}

func metadataPlanID(root string, ops []domain.PlanOperation, created time.Time) string {
	parts := []string{root, created.Format(time.RFC3339Nano)}
	for _, op := range ops {
		parts = append(parts, op.Kind, op.Path, op.Status)
	}
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "metadata-" + hex.EncodeToString(h[:])[:12]
}

func saveMetadataPlan(root string, plan *domain.MetadataPlan) error {
	dir, err := safeJoin(root, ".pinax/metadata-plans")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "metadata-plans", plan.PlanID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	plan.SavedPath = rel
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func loadMetadataPlan(root, planRef string) (domain.MetadataPlan, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return domain.MetadataPlan{}, &domain.CommandError{Code: "plan_required", Message: "metadata plan id cannot be empty", Hint: "Run pinax metadata plan --save to generate a plan"}
	}
	rel := planRef
	if !strings.Contains(planRef, "/") && !strings.HasSuffix(planRef, ".json") {
		rel = filepath.ToSlash(filepath.Join(".pinax", "metadata-plans", planRef+".json"))
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.MetadataPlan{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.MetadataPlan{}, &domain.CommandError{Code: "metadata_plan_not_found", Message: "metadata plan could not be loaded", Hint: "Run pinax metadata plan --vault <vault> --save to generate a fresh plan"}
	}
	var plan domain.MetadataPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return domain.MetadataPlan{}, err
	}
	if plan.SchemaVersion != pipelineMetadataPlanSchemaVersion {
		return domain.MetadataPlan{}, &domain.CommandError{Code: "metadata_plan_schema_invalid", Message: "metadata plan schema is not supported", Hint: "Rerun pinax metadata plan --save"}
	}
	if plan.SavedPath == "" {
		plan.SavedPath = rel
	}
	return plan, nil
}

// ensureMetadataPlanFresh 是 metadata 已保存 plan 的 freshness 守卫，与
// organize/repair 的 ensure*Fresh 同型：过期或 vault hash 漂移 ⇒ plan_stale。
func ensureMetadataPlanFresh(root string, plan *domain.MetadataPlan) error {
	if plan == nil {
		return &domain.CommandError{Code: "plan_required", Message: "metadata plan is required", Hint: "Rerun pinax metadata plan --save"}
	}
	if plan.Status != "planned" {
		return &domain.CommandError{Code: "metadata_plan_not_planned", Message: "metadata plan status is not applicable", Hint: "Rerun pinax metadata plan --save"}
	}
	if pipelinePlanExpired(plan.ExpiresAt) {
		return &domain.CommandError{Code: "plan_stale", Message: "metadata plan has expired", Hint: "pinax metadata plan --vault <vault> --save"}
	}
	recorded := strings.TrimSpace(plan.SourceFacts["vault_hash"])
	if recorded == "" {
		return nil
	}
	current, err := versionVaultHash(root)
	if err != nil {
		return err
	}
	if current != recorded {
		return &domain.CommandError{Code: "plan_stale", Message: "metadata plan does not match current vault facts", Hint: fmt.Sprintf("pinax metadata plan --vault %s --save", shellQuote(root))}
	}
	return nil
}

// buildMetadataPlanSourceFacts 记录 plan 生成时的 vault 指纹。
func buildMetadataPlanSourceFacts(root string) (map[string]string, error) {
	facts, err := scanNoteFacts(root)
	if err != nil {
		return nil, err
	}
	vaultHash, err := versionVaultHash(root)
	if err != nil {
		return nil, err
	}
	return map[string]string{"notes": fmt.Sprint(len(facts)), "vault_hash": vaultHash}, nil
}

// applyMetadataPlanOperation 应用单条 metadata_update 操作；文件已具备元数据时
// 幂等跳过。manual_review 操作由调用方统计为 skipped。
func (s *Service) applyMetadataPlanOperation(ctx context.Context, root string, op domain.PlanOperation) (bool, error) {
	if op.Kind != "metadata_update" {
		return false, nil
	}
	path, err := safeJoin(root, op.Path)
	if err != nil {
		return false, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	note := parseNote(op.Path, string(content))
	if !noteNeedsMetadata(note) {
		return false, nil
	}
	if strings.TrimSpace(note.ID) == "" {
		objectID, allocateErr := s.allocateObjectID(identity.KindNote, root, op.Path)
		if allocateErr != nil {
			return false, allocateErr
		}
		note.ID = objectID
	}
	updated := ensureFrontmatter(note, string(content))
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return false, err
	}
	parsed := parseNote(op.Path, updated)
	if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "metadata.apply:"+parsed.ID+":"+op.Path, parsed, ""); err != nil {
		return false, err
	}
	return true, nil
}
