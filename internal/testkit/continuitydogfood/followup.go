package continuitydogfood

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/internal/testkit/evidence"
)

const FollowUpSchemaVersion = "yeisme.agent_continuity_followup.v1"

type FollowUpConfig struct {
	PinaxPath string
	VaultPath string
	CohortRun string
	ParentDir string
	RunID     string
	Now       func() time.Time
}

type FollowUpResult struct {
	RunDir  string
	Summary FollowUpSummary
}

type FollowUpCase struct {
	CaseID                     string  `json:"case_id"`
	TaskSampleID               string  `json:"task_sample_id"`
	SourceDigest               string  `json:"source_digest"`
	HandoffStatus              string  `json:"handoff_status"`
	SectionCount               int     `json:"section_count"`
	SourceTotal                int     `json:"source_total"`
	SourceResolved             int     `json:"source_resolved"`
	SourceMissing              int     `json:"source_missing"`
	ReviewItems                int     `json:"review_items"`
	ReuseVerified              bool    `json:"reuse_verified"`
	ReexplanationStillRequired bool    `json:"reexplanation_still_required"`
	ObservationAgeDays         float64 `json:"observation_age_days"`
	ObservedAt                 string  `json:"observed_at"`
	FailureClass               string  `json:"failure_class,omitempty"`
}

type FollowUpSummary struct {
	SchemaVersion              string         `json:"schema_version"`
	RunID                      string         `json:"run_id"`
	PriorCohortRunID           string         `json:"prior_cohort_run_id"`
	Status                     string         `json:"status"`
	TaskSamples                int            `json:"task_samples"`
	ReuseVerified              int            `json:"reuse_verified"`
	ReuseRate                  float64        `json:"reuse_rate"`
	SourceTotal                int            `json:"source_total"`
	SourceResolved             int            `json:"source_resolved"`
	SourceMissing              int            `json:"source_missing"`
	SourceResolvableRatio      float64        `json:"source_resolvable_ratio"`
	ReexplanationStillRequired int            `json:"reexplanation_still_required"`
	FailureDistribution        map[string]int `json:"failure_distribution"`
	ObservedAt                 string         `json:"observed_at"`
	KnownBias                  []string       `json:"known_bias"`
}

type cohortPayload struct {
	SchemaVersion string       `json:"schema_version"`
	Cases         []CaseResult `json:"cases"`
}

func RunFollowUp(ctx context.Context, cfg FollowUpConfig) (FollowUpResult, error) {
	if cfg.PinaxPath == "" {
		cfg.PinaxPath = "pinax"
	}
	if cfg.VaultPath == "" || cfg.CohortRun == "" {
		return FollowUpResult{}, errors.New("vault path and cohort run are required")
	}
	if cfg.ParentDir == "" {
		cfg.ParentDir = filepath.Join("temp", "continuity-dogfood-followups")
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	now := cfg.Now().UTC()
	if cfg.RunID == "" {
		cfg.RunID = now.Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	}
	if _, err := os.Stat(filepath.Join(cfg.VaultPath, ".git")); err == nil {
		return FollowUpResult{}, errors.New("follow-up runner requires the preserved isolated vault copy without .git")
	}

	cohortPath := filepath.Join(cfg.CohortRun, "artifacts", "cohort.json")
	payload, err := os.ReadFile(cohortPath)
	if err != nil {
		return FollowUpResult{}, fmt.Errorf("read cohort evidence: %w", err)
	}
	var cohort cohortPayload
	if err := json.Unmarshal(payload, &cohort); err != nil {
		return FollowUpResult{}, fmt.Errorf("decode cohort evidence: %w", err)
	}
	if len(cohort.Cases) == 0 {
		return FollowUpResult{}, errors.New("cohort evidence contains no task samples")
	}
	startedAt, err := time.Parse(time.RFC3339Nano, cohort.Cases[0].ObservationFinishedAt)
	if err != nil {
		return FollowUpResult{}, fmt.Errorf("parse cohort observation time: %w", err)
	}
	if err := validateFollowUpWindow(startedAt, now); err != nil {
		return FollowUpResult{}, err
	}

	cases := make([]FollowUpCase, 0, len(cohort.Cases))
	for _, prior := range cohort.Cases {
		cases = append(cases, runFollowUpCase(ctx, cfg, prior, startedAt, now))
	}
	priorRunID := filepath.Base(filepath.Clean(cfg.CohortRun))
	summary := summarizeFollowUp(cfg.RunID, priorRunID, cases)
	summary.ObservedAt = now.Format(time.RFC3339)
	runDir := filepath.Join(cfg.ParentDir, cfg.RunID)
	if err := writeFollowUpEvidence(runDir, summary, cases); err != nil {
		return FollowUpResult{}, err
	}
	return FollowUpResult{RunDir: runDir, Summary: summary}, nil
}

func validateFollowUpWindow(startedAt, observedAt time.Time) error {
	age := observedAt.Sub(startedAt)
	if age < 7*24*time.Hour {
		return fmt.Errorf("follow-up is too early: %.1f days elapsed, need at least 7", age.Hours()/24)
	}
	if age > 14*24*time.Hour {
		return fmt.Errorf("follow-up is too late: %.1f days elapsed, maximum is 14", age.Hours()/24)
	}
	return nil
}

func runFollowUpCase(ctx context.Context, cfg FollowUpConfig, prior CaseResult, startedAt, now time.Time) FollowUpCase {
	result := FollowUpCase{
		CaseID:             prior.CaseID,
		TaskSampleID:       prior.TaskSampleID,
		SourceDigest:       prior.SourceDigest,
		ObservationAgeDays: now.Sub(startedAt).Hours() / 24,
		ObservedAt:         now.Format(time.RFC3339),
	}
	scope := "task:continuity-dogfood-" + prior.CaseID
	continuation, err := runEnvelope(ctx, cfg.PinaxPath, "continue", "--vault", cfg.VaultPath, "--scope", scope, "--task", "Reuse source-backed task in a new agent switch", "--json")
	if err != nil {
		result.FailureClass = "handoff"
		result.ReexplanationStillRequired = true
		return result
	}
	result.HandoffStatus = continuation.Facts["handoff_status"]
	result.SectionCount = intFact(continuation.Facts, "section_count")
	result.SourceResolved, result.SourceTotal = ratioFact(continuation.Facts["source_coverage"])
	result.SourceMissing = result.SourceTotal - result.SourceResolved
	review, err := runEnvelope(ctx, cfg.PinaxPath, "review", "--vault", cfg.VaultPath, "--scope", scope, "--json")
	if err == nil {
		result.ReviewItems = intFact(review.Facts, "total_items")
	}
	result.ReuseVerified = result.HandoffStatus == "consumed" && result.SectionCount > 0 && result.SourceTotal > 0 && result.SourceResolved == result.SourceTotal
	result.ReexplanationStillRequired = !result.ReuseVerified
	if !result.ReuseVerified {
		if result.SourceResolved != result.SourceTotal {
			result.FailureClass = "source_resolution"
		} else {
			result.FailureClass = "handoff"
		}
	}
	return result
}

func summarizeFollowUp(runID, priorRunID string, cases []FollowUpCase) FollowUpSummary {
	summary := FollowUpSummary{
		SchemaVersion:       FollowUpSchemaVersion,
		RunID:               runID,
		PriorCohortRunID:    priorRunID,
		Status:              "success",
		TaskSamples:         len(cases),
		FailureDistribution: map[string]int{},
		KnownBias: []string{
			"same single local operator and preserved isolated vault state",
			"verified task reuse does not measure willingness-to-pay",
		},
	}
	for _, item := range cases {
		if item.ReuseVerified {
			summary.ReuseVerified++
		}
		summary.SourceTotal += item.SourceTotal
		summary.SourceResolved += item.SourceResolved
		summary.SourceMissing += item.SourceMissing
		if item.ReexplanationStillRequired {
			summary.ReexplanationStillRequired++
		}
		if item.FailureClass != "" {
			summary.FailureDistribution[item.FailureClass]++
		}
	}
	if summary.TaskSamples > 0 {
		summary.ReuseRate = float64(summary.ReuseVerified) / float64(summary.TaskSamples)
	}
	if summary.SourceTotal > 0 {
		summary.SourceResolvableRatio = float64(summary.SourceResolved) / float64(summary.SourceTotal)
	}
	if summary.ReuseVerified != summary.TaskSamples {
		summary.Status = "partial"
	}
	return summary
}

func writeFollowUpEvidence(runDir string, summary FollowUpSummary, cases []FollowUpCase) error {
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return err
	}
	payload := struct {
		SchemaVersion string         `json:"schema_version"`
		Cases         []FollowUpCase `json:"cases"`
	}{SchemaVersion: FollowUpSchemaVersion, Cases: cases}
	files := map[string][]byte{
		"summary.json": mustJSON(summary),
		"command.txt":  []byte("go run ./internal/testkit/continuitydogfoodfollowup --vault <preserved-isolated-vault> --cohort-run <prior-run>\n"),
		"stdout.log":   []byte(evidence.Redact(fmt.Sprintf("continuity follow-up run=%s status=%s tasks=%d reuse=%d sources=%d/%d\n", summary.RunID, summary.Status, summary.TaskSamples, summary.ReuseVerified, summary.SourceResolved, summary.SourceTotal))),
		"stderr.log":   nil,
		"env.json": mustJSON(map[string]any{
			"schema_version":        "yeisme.agent_continuity_followup_env.v1",
			"network_used":          false,
			"live_vault_modified":   false,
			"follow_up_window_days": "7-14",
		}),
		filepath.Join("artifacts", "followup.json"): mustJSON(payload),
		filepath.Join("artifacts", "README.txt"):    []byte("Redacted seven-day continuity reuse evidence.\n"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(runDir, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
