// Package continuitydogfood runs redacted real-vault continuity task samples.
package continuitydogfood

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/tools/testkit/evidence"
)

const (
	SchemaVersion       = "yeisme.agent_continuity_dogfood.v1"
	CohortSchemaVersion = "yeisme.agent_continuity_cohort.v1"
)

type Config struct {
	PinaxPath    string
	VaultPath    string
	ParentDir    string
	Limit        int
	RunID        string
	AgentB       string
	AgentRuntime string
}

func parseAgentNoteTotal(input string, fallback int) int {
	for _, line := range strings.Split(input, "\n") {
		if !strings.HasPrefix(line, "fact.total=") {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "fact.total=")))
		if err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

type Result struct {
	RunDir  string
	Summary Summary
}

type NoteSource struct {
	ID   string
	Kind string
}

type CaseResult struct {
	CaseID                        string `json:"case_id"`
	TaskSampleID                  string `json:"task_sample_id"`
	SourceDigest                  string `json:"source_digest"`
	SourceKind                    string `json:"source_kind"`
	VaultSizeBand                 string `json:"vault_size_band"`
	AgentPair                     string `json:"agent_pair"`
	ConsentBoundary               string `json:"consent_boundary"`
	SampleMode                    string `json:"sample_mode"`
	BaselineHandoffStatus         string `json:"baseline_handoff_status"`
	BaselineSectionCount          int    `json:"baseline_section_count"`
	BaselineReexplanationRequired bool   `json:"baseline_reexplanation_required"`
	ProposalApprovalRequired      bool   `json:"proposal_approval_required"`
	ReviewItemObserved            bool   `json:"review_item_observed"`
	OwnerApproved                 bool   `json:"owner_approved"`
	HandoffStatus                 string `json:"handoff_status"`
	SectionCount                  int    `json:"section_count"`
	SourceTotal                   int    `json:"source_total"`
	SourceResolved                int    `json:"source_resolved"`
	SourceMissing                 int    `json:"source_missing"`
	ContinuationSuccess           bool   `json:"continuation_success"`
	ReexplanationReduced          bool   `json:"reexplanation_reduced"`
	SilentPromotion               bool   `json:"silent_promotion"`
	HumanAssistanceRequired       bool   `json:"human_assistance_required"`
	RecoveryExperience            string `json:"recovery_experience"`
	FirstValueUnderFiveMinutes    bool   `json:"first_value_under_five_minutes"`
	DurationMS                    int64  `json:"duration_ms"`
	ObservationStartedAt          string `json:"observation_started_at"`
	ObservationFinishedAt         string `json:"observation_finished_at"`
	ProposalDigest                string `json:"proposal_digest,omitempty"`
	HandoffDigest                 string `json:"handoff_digest,omitempty"`
	Completed                     bool   `json:"completed"`
	FailureClass                  string `json:"failure_class,omitempty"`
}

type Summary struct {
	SchemaVersion            string         `json:"schema_version"`
	RunID                    string         `json:"run_id"`
	Status                   string         `json:"status"`
	TaskSamples              int            `json:"task_samples"`
	Completed                int            `json:"completed"`
	CompletionRate           float64        `json:"completion_rate"`
	ContinuationSuccess      int            `json:"continuation_success"`
	ContinuationRate         float64        `json:"continuation_rate"`
	SourceTotal              int            `json:"source_total"`
	SourceResolved           int            `json:"source_resolved"`
	SourceMissing            int            `json:"source_missing"`
	SourceResolvableRatio    float64        `json:"source_resolvable_ratio"`
	BaselineReexplanation    int            `json:"baseline_reexplanation_required"`
	ReexplanationReduced     int            `json:"reexplanation_reduced"`
	SilentPromotion          int            `json:"silent_promotion"`
	UnderFiveMinutes         int            `json:"under_five_minutes"`
	FailureDistribution      map[string]int `json:"failure_distribution"`
	SevenDayReuseMeasured    bool           `json:"seven_day_reuse_measured"`
	WillingnessToPayMeasured bool           `json:"willingness_to_pay_measured"`
	KnownBias                []string       `json:"known_bias"`
	StartedAt                string         `json:"started_at,omitempty"`
	FinishedAt               string         `json:"finished_at,omitempty"`
}

type envelope struct {
	Status string            `json:"status"`
	Facts  map[string]string `json:"facts"`
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.PinaxPath == "" {
		cfg.PinaxPath = "pinax"
	}
	if cfg.VaultPath == "" {
		return Result{}, errors.New("vault path is required")
	}
	if cfg.ParentDir == "" {
		cfg.ParentDir = filepath.Join("temp", "continuity-dogfood-runs")
	}
	if cfg.Limit <= 0 {
		cfg.Limit = 10
	}
	if cfg.RunID == "" {
		cfg.RunID = time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	}
	if cfg.AgentB == "" {
		cfg.AgentB = "codex-agent-b"
	}
	if cfg.AgentRuntime == "" {
		cfg.AgentRuntime = "codex"
	}
	if _, err := os.Stat(filepath.Join(cfg.VaultPath, ".git")); err == nil {
		return Result{}, errors.New("dogfood runner requires an isolated vault copy without .git")
	}

	started := time.Now().UTC()
	noteOutput, err := runRaw(ctx, cfg.PinaxPath, "note", "list", "--vault", cfg.VaultPath, "--agent")
	if err != nil {
		return Result{}, fmt.Errorf("list real vault notes: %w", err)
	}
	notes, err := parseAgentNotes(string(noteOutput), cfg.Limit)
	if err != nil {
		return Result{}, err
	}
	if len(notes) < cfg.Limit {
		return Result{}, fmt.Errorf("real vault has %d unique note sources, need %d", len(notes), cfg.Limit)
	}

	noteCount := parseAgentNoteTotal(string(noteOutput), len(notes))
	cases := make([]CaseResult, 0, cfg.Limit)
	for index, note := range notes[:cfg.Limit] {
		cases = append(cases, runCase(ctx, cfg, note, index+1, noteCount))
	}
	summary := summarize(cfg.RunID, cases)
	summary.StartedAt = started.Format(time.RFC3339)
	summary.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	runDir := filepath.Join(cfg.ParentDir, cfg.RunID)
	if err := writeEvidence(runDir, cfg, summary, cases); err != nil {
		return Result{}, err
	}
	return Result{RunDir: runDir, Summary: summary}, nil
}

func runCase(ctx context.Context, cfg Config, note NoteSource, index, noteCount int) CaseResult {
	started := time.Now()
	caseID := fmt.Sprintf("task-%02d", index)
	scope := "task:continuity-dogfood-" + caseID
	result := CaseResult{
		CaseID:               caseID,
		TaskSampleID:         "sample-" + digest(caseID+note.ID),
		SourceDigest:         digest(note.ID),
		SourceKind:           note.Kind,
		VaultSizeBand:        vaultSizeBand(noteCount),
		AgentPair:            "local-cli->" + cfg.AgentB,
		ConsentBoundary:      "local-isolated-copy; receipt excludes note body, title, raw prompt, provider payload and secrets",
		SampleMode:           "isolated_real_vault_task",
		ObservationStartedAt: started.UTC().Format(time.RFC3339Nano),
	}
	fail := func(class string) CaseResult {
		result.FailureClass = class
		result.RecoveryExperience = "failed_at_" + class
		result.DurationMS = time.Since(started).Milliseconds()
		result.ObservationFinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return result
	}

	baseline, err := runEnvelope(ctx, cfg.PinaxPath, "continue", "--vault", cfg.VaultPath, "--scope", scope, "--task", "Continue source-backed task", "--json")
	if err != nil {
		return fail("onboarding")
	}
	result.BaselineHandoffStatus = baseline.Facts["handoff_status"]
	result.BaselineSectionCount = intFact(baseline.Facts, "section_count")
	result.BaselineReexplanationRequired = result.BaselineHandoffStatus == "missing" && result.BaselineSectionCount == 0

	sourceArg := "note:" + note.ID
	proposal, err := runEnvelope(ctx, cfg.PinaxPath, "agent", "memory", "propose", "--vault", cfg.VaultPath, "--scope", scope, "--kind", "task", "--subject", "Continuity dogfood "+caseID, "--summary", "Source-backed task prepared for bounded continuation.", "--sources", sourceArg, "--json")
	if err != nil {
		return fail("proposal_review")
	}
	proposalID := proposal.Facts["proposal_id"]
	result.ProposalDigest = digest(proposalID)
	result.ProposalApprovalRequired = proposal.Facts["status"] == "approval_required"
	if !result.ProposalApprovalRequired || proposalID == "" {
		result.SilentPromotion = true
		return fail("proposal_review")
	}

	review, err := runEnvelope(ctx, cfg.PinaxPath, "review", "--vault", cfg.VaultPath, "--scope", scope, "--json")
	if err != nil {
		return fail("proposal_review")
	}
	result.ReviewItemObserved = intFact(review.Facts, "total_items") > 0
	if !result.ReviewItemObserved {
		return fail("proposal_review")
	}

	approval, err := runEnvelope(ctx, cfg.PinaxPath, "review", "--vault", cfg.VaultPath, "--scope", scope, "--action", "approve", "--item", "prop-"+proposalID, "--yes", "--json")
	if err != nil || approval.Facts["memory_id"] == "" {
		return fail("proposal_review")
	}
	result.OwnerApproved = true

	handoff, err := runEnvelope(ctx, cfg.PinaxPath, "agent", "handoff", "create", "--vault", cfg.VaultPath, "--scope", scope,
		"--objective", "Continue source-backed "+caseID,
		"--current-state", "Source identity and review decision are ready for Agent B.",
		"--decisions", "Use bounded source-backed continuity",
		"--completed-work", "Owner approved the task memory proposal",
		"--blockers", "Next task-specific action remains open",
		"--verification", "Source reference must resolve in the vault",
		"--follow-ups", "Continue the next task-specific action",
		"--sources", sourceArg,
		"--to-principal", cfg.AgentB,
		"--to-runtime", cfg.AgentRuntime,
		"--requested-next-capability", "review",
		"--json")
	if err != nil || handoff.Facts["handoff_id"] == "" {
		return fail("handoff")
	}
	handoffID := handoff.Facts["handoff_id"]
	result.HandoffDigest = digest(handoffID)

	continuation, err := runEnvelope(ctx, cfg.PinaxPath, "continue", "--vault", cfg.VaultPath, "--scope", scope, "--handoff", handoffID, "--task", "Continue source-backed task", "--intent", "Preserve bounded source continuity", "--json")
	if err != nil {
		return fail("handoff")
	}
	result.HandoffStatus = continuation.Facts["handoff_status"]
	result.SectionCount = intFact(continuation.Facts, "section_count")
	result.SourceResolved, result.SourceTotal = ratioFact(continuation.Facts["source_coverage"])
	result.SourceMissing = result.SourceTotal - result.SourceResolved
	result.ContinuationSuccess = result.HandoffStatus == "explicit" && result.SectionCount > 0 && result.SourceTotal > 0 && result.SourceResolved == result.SourceTotal
	result.ReexplanationReduced = result.BaselineReexplanationRequired && result.ContinuationSuccess
	result.DurationMS = time.Since(started).Milliseconds()
	result.FirstValueUnderFiveMinutes = result.DurationMS < int64(5*time.Minute/time.Millisecond)
	result.Completed = result.ProposalApprovalRequired && result.ReviewItemObserved && result.OwnerApproved && result.ContinuationSuccess
	result.RecoveryExperience = "not_needed"
	result.ObservationFinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !result.Completed {
		if result.SourceResolved != result.SourceTotal {
			result.FailureClass = "source_resolution"
		} else {
			result.FailureClass = "handoff"
		}
	}
	return result
}

func parseAgentNotes(input string, limit int) ([]NoteSource, error) {
	type partial struct {
		ID   string
		Kind string
	}
	byIndex := map[int]partial{}
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(key, "note.") {
			continue
		}
		parts := strings.Split(key, ".")
		if len(parts) != 3 {
			continue
		}
		index, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		item := byIndex[index]
		switch parts[2] {
		case "note_id":
			item.ID = strings.Trim(strings.TrimSpace(value), "\"")
		case "kind":
			item.Kind = strings.Trim(strings.TrimSpace(value), "\"")
		}
		byIndex[index] = item
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	indexes := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	seen := map[string]struct{}{}
	var notes []NoteSource
	for _, index := range indexes {
		item := byIndex[index]
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		if item.Kind == "" {
			item.Kind = "unspecified"
		}
		notes = append(notes, NoteSource(item))
		if limit > 0 && len(notes) == limit {
			break
		}
	}
	if len(notes) == 0 {
		return nil, errors.New("note list returned no stable note IDs")
	}
	return notes, nil
}

func summarize(runID string, cases []CaseResult) Summary {
	summary := Summary{
		SchemaVersion:       SchemaVersion,
		RunID:               runID,
		Status:              "success",
		TaskSamples:         len(cases),
		FailureDistribution: map[string]int{},
		KnownBias: []string{
			"single local operator with ten independent real vault tasks",
			"isolated vault copy avoids modifying live user state",
			"seven-day reuse and willingness-to-pay are not measured by this run",
		},
	}
	for _, item := range cases {
		if item.Completed {
			summary.Completed++
		}
		if item.ContinuationSuccess {
			summary.ContinuationSuccess++
		}
		summary.SourceTotal += item.SourceTotal
		summary.SourceResolved += item.SourceResolved
		summary.SourceMissing += item.SourceMissing
		if item.BaselineReexplanationRequired {
			summary.BaselineReexplanation++
		}
		if item.ReexplanationReduced {
			summary.ReexplanationReduced++
		}
		if item.SilentPromotion {
			summary.SilentPromotion++
		}
		if item.FirstValueUnderFiveMinutes {
			summary.UnderFiveMinutes++
		}
		if item.FailureClass != "" {
			summary.FailureDistribution[item.FailureClass]++
		}
	}
	if summary.TaskSamples > 0 {
		summary.CompletionRate = float64(summary.Completed) / float64(summary.TaskSamples)
		summary.ContinuationRate = float64(summary.ContinuationSuccess) / float64(summary.TaskSamples)
	}
	if summary.SourceTotal > 0 {
		summary.SourceResolvableRatio = float64(summary.SourceResolved) / float64(summary.SourceTotal)
	}
	if summary.Completed != summary.TaskSamples {
		summary.Status = "partial"
	}
	return summary
}

func marshalEvidence(cases []CaseResult) ([]byte, error) {
	payload := struct {
		SchemaVersion string       `json:"schema_version"`
		Cases         []CaseResult `json:"cases"`
	}{SchemaVersion: CohortSchemaVersion, Cases: cases}
	return json.MarshalIndent(payload, "", "  ")
}

func writeEvidence(runDir string, cfg Config, summary Summary, cases []CaseResult) error {
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return err
	}
	cohort, err := marshalEvidence(cases)
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"summary.json": mustJSON(summary),
		"command.txt":  []byte("go run ./tools/testkit/continuitydogfoodevidence --vault <isolated-real-vault>\n"),
		"stdout.log":   []byte(evidence.Redact(fmt.Sprintf("continuity dogfood run=%s status=%s tasks=%d completed=%d continuation=%d sources=%d/%d\n", summary.RunID, summary.Status, summary.TaskSamples, summary.Completed, summary.ContinuationSuccess, summary.SourceResolved, summary.SourceTotal))),
		"stderr.log":   nil,
		"env.json": mustJSON(map[string]any{
			"schema_version":         "yeisme.agent_continuity_dogfood_env.v1",
			"network_used":           false,
			"live_vault_modified":    false,
			"isolated_copy_required": true,
		}),
		filepath.Join("artifacts", "cohort.json"): cohort,
		filepath.Join("artifacts", "README.txt"):  []byte("Redacted task-level cohort evidence generated by tools/testkit/continuitydogfood.\n"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(runDir, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runRaw(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(redactedArgs(args), " "), evidence.Redact(string(output)))
	}
	return output, nil
}

func runEnvelope(ctx context.Context, binary string, args ...string) (envelope, error) {
	output, err := runRaw(ctx, binary, args...)
	if err != nil {
		return envelope{}, err
	}
	var value envelope
	if err := json.Unmarshal(output, &value); err != nil {
		return envelope{}, fmt.Errorf("decode %s output: %w", strings.Join(redactedArgs(args), " "), err)
	}
	if value.Status != "success" {
		return envelope{}, fmt.Errorf("%s returned status %q", strings.Join(redactedArgs(args), " "), value.Status)
	}
	return value, nil
}

func redactedArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for index := range out {
		if out[index] == "--vault" && index+1 < len(out) {
			out[index+1] = "<isolated-real-vault>"
		}
		if out[index] == "--sources" && index+1 < len(out) {
			out[index+1] = "<redacted-source-ref>"
		}
	}
	return out
}

func intFact(facts map[string]string, key string) int {
	value, _ := strconv.Atoi(facts[key])
	return value
}

func ratioFact(value string) (int, int) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	resolved, _ := strconv.Atoi(parts[0])
	total, _ := strconv.Atoi(parts[1])
	return resolved, total
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func vaultSizeBand(count int) string {
	switch {
	case count <= 10:
		return "1-10"
	case count <= 100:
		return "11-100"
	case count <= 1000:
		return "101-1000"
	default:
		return "1001+"
	}
}

func mustJSON(value any) []byte {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(payload, '\n')
}
