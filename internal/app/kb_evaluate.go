package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
)

const (
	defaultKBEvaluationK       = 5
	kbEvaluationRecallAt5Gate  = 0.80
	kbEvaluationMRRAt10Gate    = 0.65
	kbEvaluationGateConfigText = "recall_at_5>=0.80;mrr_at_10>=0.65;failure_counts=0"
)

// KBEvaluateRequest runs retrieval/citation evaluation against one immutable
// active or candidate generation. It never mutates the activation descriptor.
type KBEvaluateRequest struct {
	VaultPath         string
	Suite             string
	GenerationID      string
	Backend           string
	Provider          string
	Model             string
	K                 int
	SidecarExecutable string
	SidecarTimeout    time.Duration
}

// KBEvaluate executes the Pinax-owned evaluation suite through the same
// permission, embedding, and Inferrum search path as ordinary KB queries. The
// receipt is immutable and stores identity/metrics only; question text stays
// in memory and is returned only as bounded question-id/citation projection
// data.
func (s *Service) KBEvaluate(ctx context.Context, req KBEvaluateRequest) (domain.Projection, error) {
	start := time.Now().UTC()
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.evaluate", err), err
	}
	suite, suiteRef, err := readKBEvaluationSuite(root, req.Suite)
	if err != nil {
		return commandErrorProjection("kb.evaluate", err)
	}
	k := req.K
	if k <= 0 {
		k = defaultKBEvaluationK
	}
	manifest, storeURI, err := resolveKBGenerationForSearch(root, KBIndexRequest{GenerationID: req.GenerationID})
	if err != nil {
		return commandErrorProjection("kb.evaluate", err)
	}
	if manifest == nil {
		return commandErrorProjection("kb.evaluate", &domain.CommandError{Code: "kb_generation_unavailable", Message: "KB evaluation target is unavailable", Hint: "Pin a ready candidate or activate a generation before evaluating"})
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("kb.evaluate", err), err
	}
	if sourceDigest := kbSourceDigest(notes); sourceDigest != manifest.SourceDigest {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, KBEvaluationStatusSourceDrift, "KB source changed before evaluation")
	}

	backend := manifest.Backend
	if strings.TrimSpace(req.Backend) != "" && strings.TrimSpace(req.Backend) != backend {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, KBEvaluationStatusModelMismatch, "KB evaluation backend does not match the candidate")
	}
	providerName := manifest.Provider
	if strings.TrimSpace(req.Provider) != "" {
		providerName = strings.TrimSpace(req.Provider)
	}
	modelName := manifest.Model
	if strings.TrimSpace(req.Model) != "" {
		modelName = strings.TrimSpace(req.Model)
	}
	if providerName != manifest.Provider || modelName != manifest.Model {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, KBEvaluationStatusModelMismatch, "KB evaluation provider or model does not match the candidate")
	}
	provider, err := semantic.NewProviderForBackend(backend, providerName, modelName)
	if err != nil {
		return commandErrorProjection("kb.evaluate", err)
	}
	identity, identityErr := semantic.InspectProviderIdentity(ctx, providerName, modelName)
	if identityErr != nil {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, classifyKBEvaluationError(identityErr), "KB provider identity could not be verified")
	}
	if identity.ModelManifestDigest != manifest.ModelManifestDigest || identity.ProfileHash != manifest.ProfileHash || (manifest.BaseModelDigest != "" && identity.BaseModelDigest != manifest.BaseModelDigest) {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, KBEvaluationStatusModelMismatch, "KB provider identity does not match the candidate")
	}
	canary, canaryErr := provider.Embed(ctx, "pinax evaluation dimension canary")
	if canaryErr != nil {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, classifyKBEvaluationError(canaryErr), "KB provider embedding canary failed")
	}
	if len(canary) != manifest.EmbeddingDim {
		return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, nil, KBEvaluationStatusModelMismatch, "KB provider dimension does not match the candidate")
	}

	allowedIDs := semantic.ChunkIDsForNotes(notes)
	results := make([]KBEvaluationQuestionResult, 0, len(suite.Questions))
	for _, question := range suite.Questions {
		result := KBEvaluationQuestionResult{QuestionID: question.QuestionID}
		if len(allowedIDs) == 0 {
			result.Status = KBEvaluationStatusPermissionEmpty
			results = append(results, result)
			continue
		}
		limit := k
		if limit < 10 {
			limit = 10
		}
		hits, _, searchErr := semantic.SearchWithAllowedIDs(ctx, root, question.Query, provider, backend, limit, allowedIDs, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout, StoreURI: storeURI})
		if searchErr != nil {
			result.Status = classifyKBEvaluationError(searchErr)
			results = append(results, result)
			continue
		}
		result.RetrievedCitations = evaluationCitations(hits)
		result.Status = classifyKBEvaluationHits(question.ExpectedCitations, result.RetrievedCitations)
		results = append(results, result)
	}
	if latestNotes, scanErr := scanNotes(root); scanErr != nil {
		return errorProjection("kb.evaluate", scanErr), scanErr
	} else if kbSourceDigest(latestNotes) != manifest.SourceDigest {
		for i := range results {
			results[i].Status = KBEvaluationStatusSourceDrift
			results[i].RetrievedCitations = nil
		}
	}
	return s.finishKBEvaluation(ctx, root, suite, suiteRef, *manifest, k, start, results, "", "")
}

func (s *Service) finishKBEvaluation(_ context.Context, root string, suite KBEvaluationSuite, suiteRef string, manifest KBGenerationManifest, k int, start time.Time, results []KBEvaluationQuestionResult, forcedStatus KBEvaluationStatus, failureHint string) (domain.Projection, error) {
	if results == nil {
		results = make([]KBEvaluationQuestionResult, len(suite.Questions))
		for i := range results {
			results[i] = KBEvaluationQuestionResult{QuestionID: suite.Questions[i].QuestionID, Status: forcedStatus}
		}
	}
	metrics, err := ComputeKBEvaluationMetrics(suite, results, k)
	if err != nil {
		return commandErrorProjection("kb.evaluate", err)
	}
	if forcedStatus != "" {
		metrics.StatusCounts[string(forcedStatus)] = len(results)
		metrics.FailureCounts[string(forcedStatus)] = len(results)
	}
	status := "failed"
	if forcedStatus == "" && suite.DatasetStatus() == KBEvaluationDatasetReady && metrics.RecallAt5 >= kbEvaluationRecallAt5Gate && metrics.MRRAt10 >= kbEvaluationMRRAt10Gate && len(metrics.FailureCounts) == 0 {
		status = "passed"
	}
	finished := time.Now().UTC()
	receipt := KBEvaluationReceipt{
		SchemaVersion:          KBEvaluationReceiptSchema,
		RunID:                  newKBEvaluationRunID(manifest.GenerationID, suite, start),
		Status:                 status,
		GenerationID:           manifest.GenerationID,
		SourceSnapshot:         manifest.SourceSnapshot,
		SourceDigest:           manifest.SourceDigest,
		Protocol:               manifest.Protocol,
		Provider:               manifest.Provider,
		Model:                  manifest.Model,
		BaseModelDigest:        manifest.BaseModelDigest,
		ModelManifestDigest:    manifest.ModelManifestDigest,
		ProfileHash:            manifest.ProfileHash,
		DaemonVersion:          manifest.DaemonVersion,
		EmbeddingDim:           manifest.EmbeddingDim,
		SuiteID:                suite.SuiteID,
		SuiteVersion:           suite.Version,
		GateConfigHash:         kbEvaluationGateConfigHash(),
		GenerationManifestHash: HashKBGenerationManifest(manifest),
		Metrics:                metrics,
		StartedAt:              start.Format(time.RFC3339Nano),
		FinishedAt:             finished.Format(time.RFC3339Nano),
		DurationMS:             finished.Sub(start).Milliseconds(),
		CreatedAt:              finished.Format(time.RFC3339),
	}
	_, err = WriteKBEvaluationReceipt(root, receipt)
	if err != nil {
		return commandErrorProjection("kb.evaluate", err)
	}
	projection := domain.NewProjection("kb.evaluate", "KB retrieval evaluation completed.")
	if status != "passed" {
		projection.Summary = "KB retrieval evaluation completed; candidate did not pass the configured gate."
	}
	projection.Facts["run_id"] = receipt.RunID
	projection.Facts["status"] = receipt.Status
	projection.Facts["generation_id"] = receipt.GenerationID
	projection.Facts["generation_status"] = string(manifest.Status)
	projection.Facts["protocol"] = manifest.Protocol
	projection.Facts["provider"] = manifest.Provider
	projection.Facts["model"] = manifest.Model
	projection.Facts["embedding_dim"] = fmt.Sprint(manifest.EmbeddingDim)
	projection.Facts["suite_id"] = suite.SuiteID
	projection.Facts["suite_version"] = suite.Version
	projection.Facts["dataset_status"] = metrics.DatasetStatus
	projection.Facts["recall_at_5"] = fmt.Sprintf("%.6f", metrics.RecallAt5)
	projection.Facts["mrr_at_10"] = fmt.Sprintf("%.6f", metrics.MRRAt10)
	projection.Facts["citation_coverage"] = fmt.Sprintf("%.6f", metrics.CitationCoverage)
	projection.Facts["failure_count"] = fmt.Sprint(len(metrics.FailureCounts))
	projection.Facts["activation_unchanged"] = "true"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "kb", "evaluations", receipt.RunID, "receipt.json")), suiteRef}
	projection.Data = map[string]any{
		"run_id":               receipt.RunID,
		"status":               receipt.Status,
		"generation_id":        receipt.GenerationID,
		"source_snapshot":      receipt.SourceSnapshot,
		"source_digest":        receipt.SourceDigest,
		"provider":             receipt.Provider,
		"model":                receipt.Model,
		"embedding_dim":        receipt.EmbeddingDim,
		"suite_id":             receipt.SuiteID,
		"suite_version":        receipt.SuiteVersion,
		"metrics":              receipt.Metrics,
		"results":              results,
		"receipt_path":         filepath.ToSlash(filepath.Join(".pinax", "kb", "evaluations", receipt.RunID, "receipt.json")),
		"answer_mode":          "not_generated",
		"activation_unchanged": true,
	}
	if failureHint != "" {
		projection.Warnings = []domain.ProjectionWarning{{Code: string(forcedStatus), Message: failureHint}}
	}
	return projection, nil
}

func readKBEvaluationSuite(root, spec string) (KBEvaluationSuite, string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return KBEvaluationSuite{}, "", &domain.CommandError{Code: "kb_evaluation_suite_required", Message: "KB evaluation suite is required", Hint: "Use --suite <relative-path-or-suite-id>"}
	}
	if strings.Contains(spec, "/") || strings.EqualFold(filepath.Ext(spec), ".json") {
		path, err := safeJoin(root, filepath.ToSlash(spec))
		if err != nil {
			return KBEvaluationSuite{}, "", err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return KBEvaluationSuite{}, "", &domain.CommandError{Code: "kb_evaluation_suite_not_found", Message: "KB evaluation suite was not found", Hint: "Use a suite path inside .pinax/kb/evaluation-suites or a configured suite id"}
		}
		suite, err := ParseKBEvaluationSuite(payload)
		if err != nil {
			return KBEvaluationSuite{}, "", err
		}
		rel, _ := filepath.Rel(root, path)
		return suite, filepath.ToSlash(rel), nil
	}
	dir := filepath.Join(root, ".pinax", "kb", "evaluation-suites")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return KBEvaluationSuite{}, "", &domain.CommandError{Code: "kb_evaluation_suite_not_found", Message: "KB evaluation suite was not found", Hint: "Create a versioned suite under .pinax/kb/evaluation-suites"}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return KBEvaluationSuite{}, "", readErr
		}
		suite, parseErr := ParseKBEvaluationSuite(payload)
		if parseErr == nil && suite.SuiteID == spec {
			return suite, filepath.ToSlash(filepath.Join(".pinax", "kb", "evaluation-suites", entry.Name())), nil
		}
	}
	return KBEvaluationSuite{}, "", &domain.CommandError{Code: "kb_evaluation_suite_not_found", Message: "KB evaluation suite was not found", Hint: "Use a configured suite id or a relative suite path"}
}

func classifyKBEvaluationHits(expected, retrieved []string) KBEvaluationStatus {
	if len(retrieved) == 0 {
		return KBEvaluationStatusNoHit
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, citation := range expected {
		expectedSet[citation] = struct{}{}
	}
	hits := 0
	seen := make(map[string]struct{}, len(retrieved))
	for _, citation := range retrieved {
		if _, ok := seen[citation]; ok {
			continue
		}
		seen[citation] = struct{}{}
		if _, ok := expectedSet[citation]; ok {
			hits++
		}
	}
	if hits == len(expectedSet) && hits > 0 {
		return KBEvaluationStatusPassed
	}
	return KBEvaluationStatusPartial
}

func evaluationCitations(hits []semantic.SearchHit) []string {
	seen := make(map[string]struct{}, len(hits)*2)
	out := make([]string, 0, len(hits)*2)
	for _, hit := range hits {
		path := filepath.ToSlash(strings.TrimSpace(hit.Path))
		if path == "" || filepath.IsAbs(path) || strings.Contains(path, "../") || strings.Contains(path, `\`) {
			continue
		}
		refs := []string{path}
		if heading := strings.TrimSpace(hit.HeadingPath); heading != "" {
			refs = append([]string{path + "#" + heading}, refs...)
		}
		for _, ref := range refs {
			if !validKBCitationRef(ref) {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}

func classifyKBEvaluationError(err error) KBEvaluationStatus {
	if err == nil {
		return KBEvaluationStatusError
	}
	code := strings.ToLower(commandErrorCode(err))
	if strings.Contains(code, "timeout") {
		return KBEvaluationStatusTimeout
	}
	if code == "kb_model_mismatch" || strings.Contains(code, "dimension") || code == "provider_model_missing" {
		return KBEvaluationStatusModelMismatch
	}
	return KBEvaluationStatusError
}

func kbEvaluationGateConfigHash() string {
	digest := sha256.Sum256([]byte(kbEvaluationGateConfigText))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func newKBEvaluationRunID(generationID string, suite KBEvaluationSuite, started time.Time) string {
	seed := generationID + "\x00" + suite.SuiteID + "\x00" + suite.Version + "\x00" + started.Format(time.RFC3339Nano)
	digest := sha256.Sum256([]byte(seed))
	return "run-" + started.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(digest[:])[:12]
}
