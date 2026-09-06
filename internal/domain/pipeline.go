package domain

// 统一管道读模型与阶段事件合同（pinax-pipeline-unified-ux-v1）。
//
// PipelinePlanView 是跨 organize/metadata/repair/restore 四条 plan/apply 管道的
// 只读归一投影：各管道存储原地不动，读取端归一（pinax.plan.v1）。
// PipelineStage 是 apply 型命令共享的阶段事件（pinax.pipeline.stage.v1），
// additive 叠加在既有事件合同之上。

const (
	// PipelinePlanSchemaVersion 是统一 plan 读模型 schema。
	PipelinePlanSchemaVersion = "pinax.plan.v1"
	// PipelineStageSchemaVersion 是统一阶段事件 schema。
	PipelineStageSchemaVersion = "pinax.pipeline.stage.v1"
)

// Pipeline kinds.
const (
	PipelineKindOrganize  = "organize"
	PipelineKindMetadata  = "metadata"
	PipelineKindRepair    = "repair"
	PipelineKindRestore   = "restore"
	PipelineKindSync      = "sync"
	PipelineKindPublish   = "publish"
	PipelineKindProofLoop = "proof_loop"
)

// PipelineStageType values.
const (
	PipelineStageStarted   = "stage.started"
	PipelineStageCompleted = "stage.completed"
	PipelineStageFailed    = "stage.failed"
)

// PipelinePlanView 归一化单条已保存 plan 的头字段。SourceSchema 透传原管道
// plan 的 schema_version，主 schema 变更不丢信息。
type PipelinePlanView struct {
	SchemaVersion   string         `json:"schema_version"`
	PlanID          string         `json:"plan_id"`
	Kind            string         `json:"kind"`
	SourceSchema    string         `json:"source_schema"`
	Status          string         `json:"status"`
	CreatedAt       string         `json:"created_at"`
	ExpiresAt       string         `json:"expires_at,omitempty"`
	OperationsTotal int            `json:"operations_total"`
	OpCounts        map[string]int `json:"op_counts,omitempty"`
	Fresh           bool           `json:"fresh"`
	FreshReason     string         `json:"fresh_reason,omitempty"`
	FactsDigest     string         `json:"facts_digest,omitempty"`
	SavedPath       string         `json:"saved_path"`
}

// PipelinePlanUnreadable 是损坏 plan 文件的 fail-closed 条目：呈现为 unreadable
// 并附修复提示，不中断其余条目。
type PipelinePlanUnreadable struct {
	Kind      string `json:"kind"`
	SavedPath string `json:"saved_path"`
	Error     string `json:"error"`
}

// PipelineReceiptView 归一化单条 apply 型 receipt（apply_receipt.v1、
// receipt.v1、sync run、publish run 四类存储只读读取）。
type PipelineReceiptView struct {
	SchemaVersion string         `json:"schema_version"`
	ReceiptID     string         `json:"receipt_id"`
	Pipeline      string         `json:"pipeline"`
	Command       string         `json:"command"`
	Status        string         `json:"status"`
	PlanID        string         `json:"plan_id,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	SnapshotID    string         `json:"snapshot_id,omitempty"`
	ChangedPaths  int            `json:"changed_paths"`
	Counts        map[string]int `json:"counts,omitempty"`
	SavedPath     string         `json:"saved_path"`
	CreatedAt     string         `json:"created_at"`
}

// PipelinePlanOperationView 是 show 形态下单条 plan 操作的风险分组投影。
// Group 取 vault_write | metadata_write | manual_review | skipped。
type PipelinePlanOperationView struct {
	Group  string `json:"group"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
	Status string `json:"status"`
	Risk   string `json:"risk,omitempty"`
}

// PipelineStage 是 apply 型命令通过共享 helper 发出的统一阶段事件。
type PipelineStage struct {
	SchemaVersion string         `json:"schema_version"`
	Type          string         `json:"type"`
	Pipeline      string         `json:"pipeline"`
	Stage         string         `json:"stage"`
	PlanID        string         `json:"plan_id,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	Counts        map[string]int `json:"counts,omitempty"`
	Reason        string         `json:"reason,omitempty"`
}
