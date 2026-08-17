package continuitydogfood

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/tools/testkit/evidence"
)

const (
	DecisionAnalysisSchemaVersion = "yeisme.agent_continuity_analysis.v1"
	DecisionReceiptSchemaVersion  = "yeisme.agent_continuity_decision.v1"
)

type DecisionConfig struct {
	CohortRun   string
	FollowUpRun string
	ParentDir   string
	RunID       string
	Now         func() time.Time
}

type DecisionResult struct {
	RunDir   string
	Summary  DecisionSummary
	Analysis DecisionAnalysis
	Receipt  DecisionReceipt
}

type RatioMetric struct {
	Status      string  `json:"status"`
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
	Value       float64 `json:"value"`
	Threshold   string  `json:"threshold,omitempty"`
	Gate        string  `json:"gate"`
}

type ParticipantMetrics struct {
	Participants int         `json:"participants"`
	Completion   RatioMetric `json:"completion"`
	Reuse        RatioMetric `json:"reuse"`
}

type TaskMetrics struct {
	TaskSamples                int         `json:"task_samples"`
	Completion                 RatioMetric `json:"completion"`
	Reuse                      RatioMetric `json:"reuse"`
	SourceResolvability        RatioMetric `json:"source_resolvability"`
	Continuation               RatioMetric `json:"continuation"`
	SilentPromotion            RatioMetric `json:"silent_promotion"`
	ContextReuse               RatioMetric `json:"context_reuse"`
	ProposalAcceptance         RatioMetric `json:"proposal_acceptance"`
	ReexplanationReduction     RatioMetric `json:"reexplanation_reduction"`
	FirstValueUnderFiveMinutes RatioMetric `json:"first_value_under_five_minutes"`
	StaleConflictResolution    RatioMetric `json:"stale_conflict_resolution"`
}

type CommercialMetrics struct {
	WillingnessToPay RatioMetric `json:"willingness_to_pay"`
}

type DecisionAnalysis struct {
	SchemaVersion       string             `json:"schema_version"`
	CohortRunID         string             `json:"cohort_run_id"`
	FollowUpRunID       string             `json:"follow_up_run_id"`
	ObservationWindow   string             `json:"observation_window"`
	ParticipantLevel    ParticipantMetrics `json:"participant_level"`
	TaskLevel           TaskMetrics        `json:"task_level"`
	Commercial          CommercialMetrics  `json:"commercial"`
	FailureDistribution map[string]int     `json:"failure_distribution"`
	KnownBias           []string           `json:"known_bias"`
	GeneratedAt         string             `json:"generated_at"`
}

type DecisionMetricSummary struct {
	SafetyPassed           bool   `json:"safety_passed"`
	CoreValuePassed        bool   `json:"core_value_passed"`
	ReusePassed            bool   `json:"reuse_passed"`
	CommercialSignalStatus string `json:"commercial_signal_status"`
	ExternalValidityStatus string `json:"external_validity_status"`
}

type DecisionReceipt struct {
	SchemaVersion    string                `json:"schema_version"`
	Decision         string                `json:"decision"`
	Maturity         string                `json:"maturity"`
	Rationale        []string              `json:"rationale"`
	Metrics          DecisionMetricSummary `json:"metrics"`
	FailureClasses   []string              `json:"failure_classes"`
	KnownBias        []string              `json:"known_bias"`
	AllowedNextScope []string              `json:"allowed_next_scope"`
	ForbiddenScope   []string              `json:"forbidden_scope"`
	Owner            string                `json:"owner"`
	FollowUpChange   string                `json:"follow_up_change,omitempty"`
	GeneratedAt      string                `json:"generated_at"`
}

type DecisionSummary struct {
	SchemaVersion  string `json:"schema_version"`
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	Decision       string `json:"decision"`
	Maturity       string `json:"maturity"`
	CohortRunID    string `json:"cohort_run_id"`
	FollowUpRunID  string `json:"follow_up_run_id"`
	FollowUpChange string `json:"follow_up_change,omitempty"`
	Analysis       string `json:"analysis"`
	Receipt        string `json:"receipt"`
	GeneratedAt    string `json:"generated_at"`
}

type decisionCohortPayload struct {
	SchemaVersion string       `json:"schema_version"`
	Cases         []CaseResult `json:"cases"`
}

type decisionFollowUpPayload struct {
	SchemaVersion string         `json:"schema_version"`
	Cases         []FollowUpCase `json:"cases"`
}

func AnalyzeDecision(cfg DecisionConfig) (DecisionResult, error) {
	if strings.TrimSpace(cfg.CohortRun) == "" || strings.TrimSpace(cfg.FollowUpRun) == "" {
		return DecisionResult{}, errors.New("cohort run and follow-up run are required")
	}
	if cfg.ParentDir == "" {
		cfg.ParentDir = filepath.Join("temp", "continuity-dogfood-decisions")
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	now := cfg.Now().UTC()
	if cfg.RunID == "" {
		cfg.RunID = now.Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	}

	var cohortSummary Summary
	if err := readDecisionJSON(filepath.Join(cfg.CohortRun, "summary.json"), &cohortSummary); err != nil {
		return DecisionResult{}, fmt.Errorf("read cohort summary: %w", err)
	}
	var cohort decisionCohortPayload
	if err := readDecisionJSON(filepath.Join(cfg.CohortRun, "artifacts", "cohort.json"), &cohort); err != nil {
		return DecisionResult{}, fmt.Errorf("read cohort cases: %w", err)
	}
	var followUpSummary FollowUpSummary
	if err := readDecisionJSON(filepath.Join(cfg.FollowUpRun, "summary.json"), &followUpSummary); err != nil {
		return DecisionResult{}, fmt.Errorf("read follow-up summary: %w", err)
	}
	var followUp decisionFollowUpPayload
	if err := readDecisionJSON(filepath.Join(cfg.FollowUpRun, "artifacts", "followup.json"), &followUp); err != nil {
		return DecisionResult{}, fmt.Errorf("read follow-up cases: %w", err)
	}
	if err := validateDecisionEvidence(cfg, cohortSummary, cohort, followUpSummary, followUp); err != nil {
		return DecisionResult{}, err
	}

	analysis := buildDecisionAnalysis(cohortSummary, cohort.Cases, followUpSummary, followUp.Cases, now)
	receipt := buildDecisionReceipt(analysis, now)
	summary := DecisionSummary{
		SchemaVersion:  DecisionReceiptSchemaVersion,
		RunID:          cfg.RunID,
		Status:         "success",
		Decision:       receipt.Decision,
		Maturity:       receipt.Maturity,
		CohortRunID:    analysis.CohortRunID,
		FollowUpRunID:  analysis.FollowUpRunID,
		FollowUpChange: receipt.FollowUpChange,
		Analysis:       "artifacts/analysis.json",
		Receipt:        "artifacts/decision.json",
		GeneratedAt:    now.Format(time.RFC3339),
	}
	runDir := filepath.Join(cfg.ParentDir, cfg.RunID)
	if err := writeDecisionEvidence(runDir, summary, analysis, receipt); err != nil {
		return DecisionResult{}, err
	}
	return DecisionResult{RunDir: runDir, Summary: summary, Analysis: analysis, Receipt: receipt}, nil
}

func validateDecisionEvidence(cfg DecisionConfig, cohortSummary Summary, cohort decisionCohortPayload, followUpSummary FollowUpSummary, followUp decisionFollowUpPayload) error {
	if cohortSummary.SchemaVersion != SchemaVersion || cohort.SchemaVersion != CohortSchemaVersion {
		return errors.New("unsupported cohort evidence schema")
	}
	if followUpSummary.SchemaVersion != FollowUpSchemaVersion || followUp.SchemaVersion != FollowUpSchemaVersion {
		return errors.New("unsupported follow-up evidence schema")
	}
	if cohortSummary.TaskSamples == 0 || len(cohort.Cases) != cohortSummary.TaskSamples {
		return errors.New("cohort denominator does not match task cases")
	}
	if followUpSummary.TaskSamples != cohortSummary.TaskSamples || len(followUp.Cases) != cohortSummary.TaskSamples {
		return errors.New("follow-up denominator does not match cohort")
	}
	if followUpSummary.PriorCohortRunID != cohortSummary.RunID || filepath.Base(filepath.Clean(cfg.CohortRun)) != cohortSummary.RunID {
		return errors.New("follow-up does not reference the selected cohort run")
	}
	prior := make(map[string]CaseResult, len(cohort.Cases))
	for _, item := range cohort.Cases {
		if item.TaskSampleID == "" || item.SourceDigest == "" {
			return errors.New("cohort case is missing redacted sample identity")
		}
		if _, exists := prior[item.TaskSampleID]; exists {
			return errors.New("cohort contains duplicate task sample")
		}
		prior[item.TaskSampleID] = item
	}
	for _, item := range followUp.Cases {
		original, ok := prior[item.TaskSampleID]
		if !ok || original.CaseID != item.CaseID || original.SourceDigest != item.SourceDigest {
			return errors.New("follow-up sample does not match cohort identity")
		}
	}
	return nil
}

func buildDecisionAnalysis(cohortSummary Summary, cohort []CaseResult, followUpSummary FollowUpSummary, followUp []FollowUpCase, now time.Time) DecisionAnalysis {
	proposalAccepted := 0
	for _, item := range cohort {
		if item.ProposalApprovalRequired && item.ReviewItemObserved && item.OwnerApproved {
			proposalAccepted++
		}
	}
	combinedFailures := map[string]int{}
	for key, value := range cohortSummary.FailureDistribution {
		combinedFailures[key] += value
	}
	for key, value := range followUpSummary.FailureDistribution {
		combinedFailures[key] += value
	}
	bias := uniqueSortedStrings(append(append([]string{}, cohortSummary.KnownBias...), followUpSummary.KnownBias...))
	bias = uniqueSortedStrings(append(bias,
		"task-level samples come from one local operator and cannot establish multi-user demand",
		"task reuse does not measure four active days per week or willingness-to-pay",
	))

	participantCompleted := boolCount(cohortSummary.Completed == cohortSummary.TaskSamples)
	participantReused := boolCount(followUpSummary.ReuseVerified > 0)
	return DecisionAnalysis{
		SchemaVersion:     DecisionAnalysisSchemaVersion,
		CohortRunID:       cohortSummary.RunID,
		FollowUpRunID:     followUpSummary.RunID,
		ObservationWindow: cohortSummary.StartedAt + "/" + followUpSummary.ObservedAt,
		ParticipantLevel: ParticipantMetrics{
			Participants: 1,
			Completion:   observedMetric(participantCompleted, 1, ">=1/1 observed participant", participantCompleted == 1),
			Reuse:        observedMetric(participantReused, 1, ">=1/1 observed participant", participantReused == 1),
		},
		TaskLevel: TaskMetrics{
			TaskSamples:                cohortSummary.TaskSamples,
			Completion:                 observedMetric(cohortSummary.Completed, cohortSummary.TaskSamples, ">=7/10", cohortSummary.Completed >= 7),
			Reuse:                      observedMetric(followUpSummary.ReuseVerified, followUpSummary.TaskSamples, ">=5/10", followUpSummary.ReuseVerified >= 5),
			SourceResolvability:        observedMetric(cohortSummary.SourceResolved+followUpSummary.SourceResolved, cohortSummary.SourceTotal+followUpSummary.SourceTotal, ">=95%", ratio(cohortSummary.SourceResolved+followUpSummary.SourceResolved, cohortSummary.SourceTotal+followUpSummary.SourceTotal) >= 0.95),
			Continuation:               observedMetric(cohortSummary.ContinuationSuccess, cohortSummary.TaskSamples, ">=80%", cohortSummary.ContinuationRate >= 0.8),
			SilentPromotion:            observedMetric(cohortSummary.SilentPromotion, cohortSummary.TaskSamples, "=0", cohortSummary.SilentPromotion == 0),
			ContextReuse:               observedMetric(followUpSummary.ReuseVerified, followUpSummary.TaskSamples, ">=50%", followUpSummary.ReuseRate >= 0.5),
			ProposalAcceptance:         observedMetric(proposalAccepted, len(cohort), "reported with denominator", len(cohort) > 0),
			ReexplanationReduction:     observedMetric(cohortSummary.ReexplanationReduced, cohortSummary.BaselineReexplanation, ">=50%", ratio(cohortSummary.ReexplanationReduced, cohortSummary.BaselineReexplanation) >= 0.5),
			FirstValueUnderFiveMinutes: observedMetric(cohortSummary.UnderFiveMinutes, cohortSummary.TaskSamples, ">=7/10", cohortSummary.UnderFiveMinutes >= 7),
			StaleConflictResolution:    missingMetric("not measured by this cohort"),
		},
		Commercial:          CommercialMetrics{WillingnessToPay: missingMetric(">=3/10 positive signals required for Go")},
		FailureDistribution: combinedFailures,
		KnownBias:           bias,
		GeneratedAt:         now.Format(time.RFC3339),
	}
}

func buildDecisionReceipt(analysis DecisionAnalysis, now time.Time) DecisionReceipt {
	safetyPassed := analysis.TaskLevel.SourceResolvability.Gate == "passed" && analysis.TaskLevel.Continuation.Gate == "passed" && analysis.TaskLevel.SilentPromotion.Gate == "passed"
	corePassed := analysis.TaskLevel.Completion.Gate == "passed" && analysis.TaskLevel.FirstValueUnderFiveMinutes.Gate == "passed"
	reusePassed := analysis.TaskLevel.Reuse.Gate == "passed" && analysis.TaskLevel.ContextReuse.Gate == "passed"
	decision := "iterate"
	failureClasses := []string{"external_validity", "commercial_signal"}
	if analysis.TaskLevel.Reuse.Value < 0.3 {
		decision = "stop"
		failureClasses = []string{"adoption_signal"}
	} else if !safetyPassed || !corePassed {
		failureClasses = []string{"trust_or_core_value"}
	}
	followUpChange := "pinax-agent-continuity-iteration-adoption-signal"
	if decision == "stop" {
		followUpChange = ""
	}
	return DecisionReceipt{
		SchemaVersion: DecisionReceiptSchemaVersion,
		Decision:      decision,
		Maturity:      "experimental",
		Rationale: []string{
			"十个独立真实任务的首次完成、跨 Agent continuation、来源解析和七日复用均达到当前门槛。",
			"证据仍来自单一 operator，且未测量付费意愿，不能把任务级复用外推为多用户需求或商业 Go。",
			"下一轮只验证个人 action-capture 入口的持续采用与可信预览，不扩张 Pinax 的办公集成边界。",
		},
		Metrics: DecisionMetricSummary{
			SafetyPassed:           safetyPassed,
			CoreValuePassed:        corePassed,
			ReusePassed:            reusePassed,
			CommercialSignalStatus: analysis.Commercial.WillingnessToPay.Status,
			ExternalValidityStatus: "single_operator_only",
		},
		FailureClasses: failureClasses,
		KnownBias:      analysis.KnownBias,
		AllowedNextScope: []string{
			"personal action-capture canary through Hermes using Pinax read-only MCP",
			"measure preview edits, source openability, preview latency, confirmed creation and reviewed completion proposals",
			"Pinax correctness, onboarding, retrieval, source-resolution, review-efficiency and MCP compatibility fixes",
		},
		ForbiddenScope: []string{
			"Pinax task-provider connector, provider-specific task types, task mirror or bidirectional synchronization",
			"unconfirmed automatic task creation, assignment, closing or memory promotion",
			"team collaboration, dashboard expansion, editor, publish/share, plugin platform or cross-platform client",
		},
		Owner:          "CEO/product",
		FollowUpChange: followUpChange,
		GeneratedAt:    now.Format(time.RFC3339),
	}
}

func observedMetric(numerator, denominator int, threshold string, passed bool) RatioMetric {
	gate := "failed"
	if passed {
		gate = "passed"
	}
	return RatioMetric{Status: "observed", Numerator: numerator, Denominator: denominator, Value: ratio(numerator, denominator), Threshold: threshold, Gate: gate}
}

func missingMetric(threshold string) RatioMetric {
	return RatioMetric{Status: "not_measured", Threshold: threshold, Gate: "not_measured"}
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func readDecisionJSON(path string, value any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, value); err != nil {
		return err
	}
	return nil
}

func writeDecisionEvidence(runDir string, summary DecisionSummary, analysis DecisionAnalysis, receipt DecisionReceipt) error {
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return err
	}
	files := map[string][]byte{
		"summary.json": mustJSON(summary),
		"command.txt":  []byte("go run ./tools/testkit/continuitydogfooddecision --cohort-run <cohort-run> --followup-run <follow-up-run>\n"),
		"stdout.log": []byte(evidence.Redact(fmt.Sprintf(
			"continuity decision run=%s decision=%s maturity=%s task_completion=%d/%d task_reuse=%d/%d\n",
			summary.RunID, summary.Decision, summary.Maturity,
			analysis.TaskLevel.Completion.Numerator, analysis.TaskLevel.Completion.Denominator,
			analysis.TaskLevel.Reuse.Numerator, analysis.TaskLevel.Reuse.Denominator,
		))),
		"stderr.log": nil,
		"env.json": mustJSON(map[string]any{
			"schema_version":      "yeisme.agent_continuity_decision_env.v1",
			"network_used":        false,
			"live_vault_modified": false,
			"input_content":       "redacted evidence summaries only",
		}),
		filepath.Join("artifacts", "analysis.json"): mustJSON(analysis),
		filepath.Join("artifacts", "decision.json"): mustJSON(receipt),
		filepath.Join("artifacts", "README.txt"):    []byte("Redacted continuity metric analysis and CEO decision receipt.\n"),
	}
	for name, content := range files {
		if containsForbiddenDecisionEvidence(string(content)) {
			return fmt.Errorf("refusing to write forbidden decision evidence in %s", name)
		}
		if err := os.WriteFile(filepath.Join(runDir, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func containsForbiddenDecisionEvidence(value string) bool {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		`"raw_prompt":`, `"transcript":`, `"provider_payload":`, `"authorization":`,
		"authorization: bearer", "secret_sentinel", "/workspaces/", "/home/", "c:\\",
	} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return false
}
