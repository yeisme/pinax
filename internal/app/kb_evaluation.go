package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	KBEvaluationSuiteSchema         = "pinax.kb.evaluation-suite.v1"
	KBEvaluationReceiptSchema       = "pinax.kb.evaluation-receipt.v1"
	KBEvaluationDatasetReady        = "ready"
	KBEvaluationDatasetInsufficient = "insufficient_dataset"
	KBEvaluationDatasetInvalid      = "invalid"
	KBEvaluationMinimumQuestions    = 20
)

type KBEvaluationStatus string

const (
	KBEvaluationStatusPassed          KBEvaluationStatus = "passed"
	KBEvaluationStatusPartial         KBEvaluationStatus = "partial"
	KBEvaluationStatusNoHit           KBEvaluationStatus = "no_hit"
	KBEvaluationStatusPermissionEmpty KBEvaluationStatus = "permission_empty"
	KBEvaluationStatusError           KBEvaluationStatus = "error"
	KBEvaluationStatusTimeout         KBEvaluationStatus = "timeout"
	KBEvaluationStatusModelMismatch   KBEvaluationStatus = "model_mismatch"
	KBEvaluationStatusSourceDrift     KBEvaluationStatus = "source_drift"
)

type KBEvaluationQuestion struct {
	QuestionID        string   `json:"question_id"`
	Query             string   `json:"query"`
	ExpectedCitations []string `json:"expected_citations"`
}

type KBEvaluationSuite struct {
	SchemaVersion string                 `json:"schema_version"`
	SuiteID       string                 `json:"suite_id"`
	Version       string                 `json:"version"`
	Questions     []KBEvaluationQuestion `json:"questions"`
}

type KBEvaluationQuestionResult struct {
	QuestionID         string             `json:"question_id"`
	Status             KBEvaluationStatus `json:"status"`
	RetrievedCitations []string           `json:"retrieved_citations,omitempty"`
}

type KBEvaluationMetrics struct {
	DatasetStatus        string         `json:"dataset_status"`
	TotalQuestions       int            `json:"total_questions"`
	K                    int            `json:"k"`
	StatusCounts         map[string]int `json:"status_counts"`
	RecallAtK            float64        `json:"recall_at_k"`
	MRRAtK               float64        `json:"mrr_at_k"`
	RecallAt5            float64        `json:"recall_at_5,omitempty"`
	MRRAt10              float64        `json:"mrr_at_10,omitempty"`
	CitationCoverage     float64        `json:"citation_coverage"`
	NoHitCount           int            `json:"no_hit_count"`
	PermissionEmptyCount int            `json:"permission_empty_count"`
	FailureCounts        map[string]int `json:"failure_counts,omitempty"`
}

type KBEvaluationReceipt struct {
	SchemaVersion          string              `json:"schema_version"`
	RunID                  string              `json:"run_id"`
	Status                 string              `json:"status"`
	GenerationID           string              `json:"generation_id"`
	SourceSnapshot         string              `json:"source_snapshot"`
	SourceDigest           string              `json:"source_digest"`
	Protocol               string              `json:"protocol,omitempty"`
	Provider               string              `json:"provider"`
	Model                  string              `json:"model"`
	BaseModelDigest        string              `json:"base_model_digest,omitempty"`
	ModelManifestDigest    string              `json:"model_manifest_digest"`
	ProfileHash            string              `json:"profile_hash"`
	DaemonVersion          string              `json:"daemon_version,omitempty"`
	EmbeddingDim           int                 `json:"embedding_dim"`
	SuiteID                string              `json:"suite_id"`
	SuiteVersion           string              `json:"suite_version"`
	GateConfigHash         string              `json:"gate_config_hash"`
	GenerationManifestHash string              `json:"generation_manifest_hash"`
	Metrics                KBEvaluationMetrics `json:"metrics"`
	StartedAt              string              `json:"started_at,omitempty"`
	FinishedAt             string              `json:"finished_at,omitempty"`
	DurationMS             int64               `json:"duration_ms,omitempty"`
	CreatedAt              string              `json:"created_at"`
}

func ParseKBEvaluationSuite(payload []byte) (KBEvaluationSuite, error) {
	var suite KBEvaluationSuite
	if err := json.Unmarshal(payload, &suite); err != nil {
		return KBEvaluationSuite{}, invalidKBEvaluationSuite("suite JSON is invalid")
	}
	if err := suite.Validate(); err != nil {
		return KBEvaluationSuite{}, err
	}
	return suite, nil
}

func HashKBGenerationManifest(manifest KBGenerationManifest) string {
	payload, _ := json.Marshal(manifest)
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func HashKBEvaluationReceipt(receipt KBEvaluationReceipt) string {
	payload, _ := json.Marshal(receipt)
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func ValidateKBEvaluationReceiptForCandidate(receipt KBEvaluationReceipt, manifest KBGenerationManifest, suite KBEvaluationSuite, gateConfigHash string) error {
	if err := manifest.Validate(); err != nil {
		return invalidKBEvaluationGate("candidate manifest is invalid")
	}
	if err := suite.Validate(); err != nil {
		return invalidKBEvaluationGate("evaluation suite is invalid")
	}
	if err := ValidateKBEvaluationReceipt(receipt); err != nil {
		return err
	}
	if receipt.Status != "passed" || receipt.Metrics.DatasetStatus != KBEvaluationDatasetReady {
		return invalidKBEvaluationGate("only a passed receipt with a ready dataset may activate")
	}
	if receipt.Protocol != "" && receipt.Protocol != manifest.Protocol {
		return invalidKBEvaluationGate("protocol does not match the candidate")
	}
	if receipt.BaseModelDigest != "" && receipt.BaseModelDigest != manifest.BaseModelDigest {
		return invalidKBEvaluationGate("base_model_digest does not match the candidate")
	}
	if receipt.DaemonVersion != "" && manifest.DaemonVersion != "" && receipt.DaemonVersion != manifest.DaemonVersion {
		return invalidKBEvaluationGate("daemon_version does not match the candidate")
	}
	expected := map[string][2]string{
		"generation_id":            {receipt.GenerationID, manifest.GenerationID},
		"source_snapshot":          {receipt.SourceSnapshot, manifest.SourceSnapshot},
		"source_digest":            {receipt.SourceDigest, manifest.SourceDigest},
		"provider":                 {receipt.Provider, manifest.Provider},
		"model":                    {receipt.Model, manifest.Model},
		"model_manifest_digest":    {receipt.ModelManifestDigest, manifest.ModelManifestDigest},
		"profile_hash":             {receipt.ProfileHash, manifest.ProfileHash},
		"embedding_dim":            {fmt.Sprint(receipt.EmbeddingDim), fmt.Sprint(manifest.EmbeddingDim)},
		"suite_id":                 {receipt.SuiteID, suite.SuiteID},
		"suite_version":            {receipt.SuiteVersion, suite.Version},
		"gate_config_hash":         {receipt.GateConfigHash, gateConfigHash},
		"generation_manifest_hash": {receipt.GenerationManifestHash, HashKBGenerationManifest(manifest)},
	}
	for field, pair := range expected {
		if pair[0] != pair[1] {
			return invalidKBEvaluationGate(field + " does not match the candidate")
		}
	}
	return nil
}

func ValidateKBEvaluationReceipt(receipt KBEvaluationReceipt) error {
	if receipt.SchemaVersion != KBEvaluationReceiptSchema || !validKBToken(receipt.RunID) || (receipt.Status != "passed" && receipt.Status != "failed") || !validKBToken(receipt.GenerationID) || !validKBToken(receipt.SourceSnapshot) || !validKBToken(receipt.SourceDigest) || !validKBToken(receipt.Provider) || !validKBToken(receipt.Model) || !validKBToken(receipt.ModelManifestDigest) || !validKBToken(receipt.ProfileHash) || !validKBToken(receipt.SuiteID) || !validKBToken(receipt.SuiteVersion) || !validKBToken(receipt.GateConfigHash) || !validKBToken(receipt.GenerationManifestHash) || receipt.EmbeddingDim <= 0 {
		return invalidKBEvaluationReceipt("receipt identity is incomplete or unsafe")
	}
	if receipt.Metrics.TotalQuestions < 0 || receipt.Metrics.K <= 0 || (receipt.Metrics.DatasetStatus != KBEvaluationDatasetReady && receipt.Metrics.DatasetStatus != KBEvaluationDatasetInsufficient && receipt.Metrics.DatasetStatus != KBEvaluationDatasetInvalid) {
		return invalidKBEvaluationReceipt("receipt metrics are invalid")
	}
	if _, err := time.Parse(time.RFC3339, receipt.CreatedAt); err != nil {
		return invalidKBEvaluationReceipt("created_at must be RFC3339")
	}
	for _, value := range []string{receipt.StartedAt, receipt.FinishedAt} {
		if value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return invalidKBEvaluationReceipt("evaluation timestamps must be RFC3339")
			}
		}
	}
	if receipt.DurationMS < 0 {
		return invalidKBEvaluationReceipt("duration_ms must not be negative")
	}
	for field, value := range map[string]string{"protocol": receipt.Protocol, "base_model_digest": receipt.BaseModelDigest, "daemon_version": receipt.DaemonVersion} {
		if value != "" && !validKBToken(value) {
			return invalidKBEvaluationReceipt(field + " is invalid")
		}
	}
	return nil
}

func (s KBEvaluationSuite) Validate() error {
	if s.SchemaVersion != KBEvaluationSuiteSchema || !validKBToken(s.SuiteID) || !validKBToken(s.Version) || len(s.Questions) == 0 {
		return invalidKBEvaluationSuite("schema, suite identity, or question count is invalid")
	}
	seen := make(map[string]struct{}, len(s.Questions))
	for _, question := range s.Questions {
		if !validKBToken(question.QuestionID) || strings.TrimSpace(question.Query) == "" || len(question.ExpectedCitations) == 0 {
			return invalidKBEvaluationSuite("each question needs a stable id, query, and expected citation")
		}
		if _, ok := seen[question.QuestionID]; ok {
			return invalidKBEvaluationSuite("question ids must be unique")
		}
		seen[question.QuestionID] = struct{}{}
		for _, citation := range question.ExpectedCitations {
			if !validKBCitationRef(citation) {
				return invalidKBEvaluationSuite("expected citation ref is unsafe")
			}
		}
	}
	return nil
}

func (s KBEvaluationSuite) DatasetStatus() string {
	if len(s.Questions) < KBEvaluationMinimumQuestions {
		return KBEvaluationDatasetInsufficient
	}
	return KBEvaluationDatasetReady
}

func ComputeKBEvaluationMetrics(suite KBEvaluationSuite, results []KBEvaluationQuestionResult, k int) (KBEvaluationMetrics, error) {
	if err := suite.Validate(); err != nil {
		return KBEvaluationMetrics{}, err
	}
	if k <= 0 {
		return KBEvaluationMetrics{}, invalidKBEvaluationSuite("k must be positive")
	}
	byID := make(map[string]KBEvaluationQuestionResult, len(results))
	for _, result := range results {
		if !validKBToken(result.QuestionID) {
			return KBEvaluationMetrics{}, invalidKBEvaluationResults("result question id is invalid")
		}
		if _, exists := byID[result.QuestionID]; exists {
			return KBEvaluationMetrics{}, invalidKBEvaluationResults("result question ids must be unique")
		}
		byID[result.QuestionID] = result
	}
	metrics := KBEvaluationMetrics{DatasetStatus: suite.DatasetStatus(), TotalQuestions: len(suite.Questions), K: k, StatusCounts: map[string]int{}, FailureCounts: map[string]int{}}
	var recallNumerator, recallDenominator float64
	var mrrNumerator float64
	var coverageNumerator, coverageDenominator float64
	for _, question := range suite.Questions {
		result, ok := byID[question.QuestionID]
		if !ok {
			return KBEvaluationMetrics{}, invalidKBEvaluationResults("missing result for question " + question.QuestionID)
		}
		metrics.StatusCounts[string(result.Status)]++
		if result.Status != KBEvaluationStatusPassed {
			metrics.FailureCounts[string(result.Status)]++
		}
		if result.Status == KBEvaluationStatusNoHit {
			metrics.NoHitCount++
		}
		if result.Status == KBEvaluationStatusPermissionEmpty {
			metrics.PermissionEmptyCount++
		}
		expected := uniqueStrings(question.ExpectedCitations)
		retrieved := uniqueStrings(result.RetrievedCitations)
		if len(retrieved) > k {
			retrieved = retrieved[:k]
		}
		retrievedSet := make(map[string]int, len(retrieved))
		for rank, citation := range retrieved {
			retrievedSet[citation] = rank + 1
		}
		hits := 0
		firstRank := 0
		for _, citation := range expected {
			if rank, ok := retrievedSet[citation]; ok {
				hits++
				if firstRank == 0 || rank < firstRank {
					firstRank = rank
				}
			}
		}
		recallNumerator += float64(hits)
		recallDenominator += float64(len(expected))
		coverageDenominator++
		if hits > 0 {
			coverageNumerator++
		}
		if firstRank > 0 {
			mrrNumerator += 1 / float64(firstRank)
		}
	}
	if recallDenominator > 0 {
		metrics.RecallAtK = recallNumerator / recallDenominator
	}
	if coverageDenominator > 0 {
		metrics.MRRAtK = mrrNumerator / coverageDenominator
		metrics.CitationCoverage = coverageNumerator / coverageDenominator
	}
	metrics.RecallAt5, _, _ = citationMetricsAtK(suite, byID, 5)
	_, metrics.MRRAt10, _ = citationMetricsAtK(suite, byID, 10)
	return metrics, nil
}

func citationMetricsAtK(suite KBEvaluationSuite, results map[string]KBEvaluationQuestionResult, k int) (float64, float64, float64) {
	if k <= 0 {
		return 0, 0, 0
	}
	var recallNumerator, recallDenominator float64
	var mrrNumerator, coverageNumerator, coverageDenominator float64
	for _, question := range suite.Questions {
		result := results[question.QuestionID]
		expected := uniqueStrings(question.ExpectedCitations)
		retrieved := uniqueStrings(result.RetrievedCitations)
		if len(retrieved) > k {
			retrieved = retrieved[:k]
		}
		retrievedSet := make(map[string]int, len(retrieved))
		for rank, citation := range retrieved {
			retrievedSet[citation] = rank + 1
		}
		hits := 0
		firstRank := 0
		for _, citation := range expected {
			if rank, ok := retrievedSet[citation]; ok {
				hits++
				if firstRank == 0 || rank < firstRank {
					firstRank = rank
				}
			}
		}
		recallNumerator += float64(hits)
		recallDenominator += float64(len(expected))
		coverageDenominator++
		if hits > 0 {
			coverageNumerator++
		}
		if firstRank > 0 {
			mrrNumerator += 1 / float64(firstRank)
		}
	}
	var recall, mrr, coverage float64
	if recallDenominator > 0 {
		recall = recallNumerator / recallDenominator
	}
	if coverageDenominator > 0 {
		mrr = mrrNumerator / coverageDenominator
		coverage = coverageNumerator / coverageDenominator
	}
	return recall, mrr, coverage
}

func validKBCitationRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || len(ref) > 512 || strings.HasPrefix(ref, "/") || strings.Contains(ref, `\`) || strings.Contains(ref, "../") || strings.ContainsAny(ref, "\r\n\t") {
		return false
	}
	return true
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func invalidKBEvaluationSuite(hint string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_evaluation_suite_invalid", Message: "KB evaluation suite is invalid", Hint: hint}
}

func invalidKBEvaluationResults(hint string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_evaluation_results_invalid", Message: "KB evaluation results are invalid", Hint: fmt.Sprint(hint)}
}

func invalidKBEvaluationReceipt(hint string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_evaluation_receipt_invalid", Message: "KB evaluation receipt is invalid", Hint: hint}
}

func invalidKBEvaluationGate(hint string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_evaluation_gate_failed", Message: "KB candidate evaluation gate failed", Hint: hint}
}
