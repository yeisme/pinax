package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
)

// pipeline_stage_test.go 覆盖 pinax.pipeline.stage.v1 阶段事件合同：
// golden NDJSON（--events 渲染）、stale 拒绝的 stage.failed{reason=plan_stale}、
// proof loop run 的多阶段事件。

func TestPipelineStageGoldenNDJSONEventsContract(t *testing.T) {
	t.Parallel()
	projection := domain.NewProjection("organize.apply", "Organize structure applied.")
	projection.Facts["applied"] = "12"
	projection.PipelineStages = []domain.PipelineStage{
		{SchemaVersion: domain.PipelineStageSchemaVersion, Type: domain.PipelineStageStarted, Pipeline: domain.PipelineKindOrganize, Stage: "apply", PlanID: "organize-abc123"},
		{SchemaVersion: domain.PipelineStageSchemaVersion, Type: domain.PipelineStageCompleted, Pipeline: domain.PipelineKindOrganize, Stage: "apply", PlanID: "organize-abc123", Counts: map[string]int{"applied": 12}},
	}
	var buf bytes.Buffer
	if err := output.RenderWithOptions(&buf, output.ModeEvents, projection, output.RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("events lines = %#v", lines)
	}
	if !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[0], `"seq":1`) {
		t.Fatalf("start line = %q", lines[0])
	}
	started := lines[1]
	for _, want := range []string{`"type":"stage.started"`, `"schema_version":"pinax.pipeline.stage.v1"`, `"pipeline":"organize"`, `"stage":"apply"`, `"plan_id":"organize-abc123"`, `"seq":2`} {
		if !strings.Contains(started, want) {
			t.Fatalf("stage.started line missing %s: %q", want, started)
		}
	}
	if strings.Contains(started, `"counts"`) {
		t.Fatalf("stage.started must not carry counts: %q", started)
	}
	completed := lines[2]
	for _, want := range []string{`"type":"stage.completed"`, `"counts":{"applied":12}`, `"seq":3`} {
		if !strings.Contains(completed, want) {
			t.Fatalf("stage.completed line missing %s: %q", want, completed)
		}
	}
	if !strings.Contains(lines[3], `"type":"end"`) || !strings.Contains(lines[3], `"seq":4`) {
		t.Fatalf("end line = %q", lines[3])
	}
}

func TestPipelineStageFailedGoldenNDJSONWithPlanStaleReason(t *testing.T) {
	t.Parallel()
	projection := domain.NewErrorProjection("organize.apply", &domain.CommandError{Code: "plan_stale", Message: "organize plan does not match current vault facts"})
	projection.PipelineStages = []domain.PipelineStage{
		{SchemaVersion: domain.PipelineStageSchemaVersion, Type: domain.PipelineStageStarted, Pipeline: domain.PipelineKindOrganize, Stage: "apply", PlanID: "organize-9c1d"},
		{SchemaVersion: domain.PipelineStageSchemaVersion, Type: domain.PipelineStageFailed, Pipeline: domain.PipelineKindOrganize, Stage: "apply", PlanID: "organize-9c1d", Reason: "plan_stale"},
	}
	var buf bytes.Buffer
	if err := output.RenderWithOptions(&buf, output.ModeEvents, projection, output.RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	stream := buf.String()
	if !strings.Contains(stream, `"type":"stage.failed"`) || !strings.Contains(stream, `"reason":"plan_stale"`) {
		t.Fatalf("stage.failed missing:\n%s", stream)
	}
	if !strings.Contains(stream, `"type":"error"`) {
		t.Fatalf("error end event missing:\n%s", stream)
	}
}

func TestPipelineStageTrackerOnlyEmitsAfterBegin(t *testing.T) {
	t.Parallel()
	tracker := newPipelineStageTracker(domain.PipelineKindRepair, "repair-abc", "", "apply")
	projection := domain.NewProjection("repair.apply", "ok")
	tracker.finish(&projection, nil, map[string]int{"applied": 1})
	if len(projection.PipelineStages) != 0 {
		t.Fatalf("no stages expected before begin: %#v", projection.PipelineStages)
	}
	tracker.begin()
	tracker.finish(&projection, nil, map[string]int{"applied": 1})
	if len(projection.PipelineStages) != 2 {
		t.Fatalf("stages = %#v", projection.PipelineStages)
	}
	if projection.PipelineStages[0].Type != domain.PipelineStageStarted || projection.PipelineStages[1].Type != domain.PipelineStageCompleted {
		t.Fatalf("stage types = %#v", projection.PipelineStages)
	}
	if projection.PipelineStages[1].Counts["applied"] != 1 {
		t.Fatalf("counts = %#v", projection.PipelineStages[1].Counts)
	}
}

func TestPipelineStageEventsFromProofLoopRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := pipelineTestVault(t)
	projection, err := NewService().ProofLoopRun(ctx, ProofLoopRunRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.PipelineStages) != 6 {
		t.Fatalf("stages = %#v", projection.PipelineStages)
	}
	seen := map[string]int{}
	for _, stage := range projection.PipelineStages {
		if stage.Pipeline != domain.PipelineKindProofLoop || stage.RunID == "" {
			t.Fatalf("stage = %#v", stage)
		}
		seen[stage.Type+" "+stage.Stage]++
	}
	for _, key := range []string{
		domain.PipelineStageStarted + " capture",
		domain.PipelineStageCompleted + " capture",
		domain.PipelineStageStarted + " diagnose",
		domain.PipelineStageCompleted + " diagnose",
		domain.PipelineStageStarted + " plan",
		domain.PipelineStageCompleted + " plan",
	} {
		if seen[key] != 1 {
			t.Fatalf("stage %q seen %d times: %#v", key, seen[key], projection.PipelineStages)
		}
	}
}
