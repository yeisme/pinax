package app

import (
	"strconv"

	"github.com/yeisme/pinax/internal/domain"
)

// pipeline_stage.go 实现共享阶段事件 helper（pinax.pipeline.stage.v1）。
//
// apply 型命令（organize/metadata/repair/restore apply、sync push/pull、
// publish build/deploy、proof loop run）通过 tracker 在 projection 上按序累积
// stage.started / stage.completed / stage.failed；--events 渲染时输出为 NDJSON。
// 既有事件一律保留原名原义，本合同是 additive 新增。

// newPipelineStageTracker 构造一次 apply/run 的阶段事件累积器。
// pipeline 取 domain.PipelineKind*；planID/runID 二选一（可空）；stage 是
// 单阶段命令（apply 型）的默认阶段名，多阶段命令（proof loop）用 start/complete
// 显式命名。
func newPipelineStageTracker(pipeline, planID, runID, stage string) *pipelineStageTracker {
	return &pipelineStageTracker{pipeline: pipeline, planID: planID, runID: runID, defaultStage: stage}
}

type pipelineStageTracker struct {
	pipeline     string
	planID       string
	runID        string
	defaultStage string
	stages       []domain.PipelineStage
	open         string
}

// setRunID 回填 run id（底层 run id 常在执行后才可知，如 sync run / publish run）。
func (t *pipelineStageTracker) setRunID(runID string) {
	if t == nil {
		return
	}
	t.runID = runID
	for index := range t.stages {
		t.stages[index].RunID = runID
	}
}

// start 记录某阶段的 stage.started（单阶段命令用 begin）。
func (t *pipelineStageTracker) start(stage string) {
	if t == nil || stage == "" {
		return
	}
	t.open = stage
	t.stages = append(t.stages, domain.PipelineStage{
		SchemaVersion: domain.PipelineStageSchemaVersion,
		Type:          domain.PipelineStageStarted,
		Pipeline:      t.pipeline,
		Stage:         stage,
		PlanID:        t.planID,
		RunID:         t.runID,
	})
}

// begin 记录默认阶段（apply）的 stage.started。只有 begin 之后的结果才会出现
// 在事件流里，因此 approval_required 等前置拒绝不产生阶段事件。
func (t *pipelineStageTracker) begin() {
	if t == nil {
		return
	}
	t.start(t.defaultStage)
}

// complete 记录某阶段的 stage.completed（携带计数）。
func (t *pipelineStageTracker) complete(stage string, counts map[string]int) {
	if t == nil || stage == "" {
		return
	}
	if t.open == stage {
		t.open = ""
	}
	t.stages = append(t.stages, domain.PipelineStage{
		SchemaVersion: domain.PipelineStageSchemaVersion,
		Type:          domain.PipelineStageCompleted,
		Pipeline:      t.pipeline,
		Stage:         stage,
		PlanID:        t.planID,
		RunID:         t.runID,
		Counts:        counts,
	})
}

// fail 记录当前开放阶段的 stage.failed（携带稳定 reason）。
func (t *pipelineStageTracker) fail(reason string) {
	if t == nil || t.open == "" {
		return
	}
	stage := t.open
	t.open = ""
	if reason == "" {
		reason = "error"
	}
	t.stages = append(t.stages, domain.PipelineStage{
		SchemaVersion: domain.PipelineStageSchemaVersion,
		Type:          domain.PipelineStageFailed,
		Pipeline:      t.pipeline,
		Stage:         stage,
		PlanID:        t.planID,
		RunID:         t.runID,
		Reason:        reason,
	})
}

// Stages 返回已累积的阶段事件副本。
func (t *pipelineStageTracker) Stages() []domain.PipelineStage {
	if t == nil {
		return nil
	}
	return append([]domain.PipelineStage(nil), t.stages...)
}

// finish 在单阶段命令结束时补齐 stage.completed 或 stage.failed（reason 取
// 稳定错误码）并把全部阶段事件附加到 projection。
func (t *pipelineStageTracker) finish(projection *domain.Projection, err error, counts map[string]int) {
	if t == nil || t.open == "" {
		return
	}
	if err == nil {
		t.complete(t.open, counts)
	} else {
		t.fail(domain.ErrorCode(err))
	}
	t.attach(projection)
}

// attach 把当前已累积的阶段事件附加到 projection。
func (t *pipelineStageTracker) attach(projection *domain.Projection) {
	if t == nil || projection == nil || len(t.stages) == 0 {
		return
	}
	projection.PipelineStages = append(projection.PipelineStages, t.stages...)
}

// pipelineStageCounts 从 projection facts 里提取既有数值事实为阶段计数，
// 只保留 keys 中点名的事实。
func pipelineStageCounts(projection domain.Projection, keys ...string) map[string]int {
	counts := map[string]int{}
	for _, key := range keys {
		if value := projection.Facts[key]; value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				counts[key] = parsed
			}
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}
