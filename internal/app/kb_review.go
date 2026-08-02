package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
)

type KBReviewRequest struct {
	VaultPath string
	Limit     int
	SuiteID   string
	RunID     string
}

// KBReviewOverview is a bounded, read-only projection for the Workbench KB
// review page. It reports truth states without reading LanceDB rows or
// returning note bodies to the API layer.
func (s *Service) KBReviewOverview(_ context.Context, req KBReviewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.review.overview", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("kb.review.overview", err), err
	}
	indexStatus := "missing"
	providerStatus := "not_checked"
	sidecarStatus := "not_checked"
	activeGeneration := "none"
	generationStatus := "none"
	protocol := ""
	provider := ""
	model := ""
	embeddingDim := 0
	sourceSnapshot := ""
	sourceDigest := ""
	profile := ""
	compatibilityStatus := ""
	nextAction := ""
	rollbackAvailable := false
	if activation, readErr := ReadKBActivationDescriptor(root); readErr == nil && activation.Active != nil {
		activeGeneration = activation.Active.GenerationID
		rollbackAvailable = activation.Previous != nil
		manifest, manifestErr := ReadKBGenerationManifest(root, activation.Active.GenerationID)
		if manifestErr != nil {
			indexStatus = "failed"
			generationStatus = "invalid"
		} else {
			generationStatus = string(manifest.Status)
			protocol = manifest.Protocol
			provider = manifest.Provider
			model = manifest.Model
			embeddingDim = manifest.EmbeddingDim
			sourceSnapshot = manifest.SourceSnapshot
			sourceDigest = manifest.SourceDigest
			storePath := filepath.Join(root, ".pinax", "kb", "generations", manifest.GenerationID, "lancedb")
			if manifest.Status == KBGenerationStatusFailed || manifest.Status == KBGenerationStatusRejected {
				indexStatus = "failed"
			} else if kbSourceDigest(notes) != manifest.SourceDigest {
				indexStatus = "stale"
			} else if _, statErr := os.Stat(storePath); statErr != nil {
				indexStatus = "missing"
			} else {
				indexStatus = "fresh"
			}
			if _, statErr := os.Stat(storePath); statErr == nil {
				// A generation directory proves only that a projection artifact is
				// present. It does not prove the sidecar process or retrieval path is
				// healthy; those remain separate doctor/retrieval states.
				sidecarStatus = "projection_present"
			} else {
				sidecarStatus = "unavailable"
			}
		}
	} else if readErr != nil {
		activeGeneration = "invalid"
		indexStatus = "failed"
	} else if legacy, legacyErr := semantic.DetectLegacyV1(root); legacyErr != nil {
		indexStatus = "failed"
		generationStatus = "invalid"
	} else if legacy.Present {
		indexStatus = "legacy_v1_readonly"
		generationStatus = "legacy_v1_readonly"
		protocol = legacy.Protocol
		provider = legacy.Provider
		model = legacy.Model
		embeddingDim = legacy.EmbeddingDim
		profile = "legacy_v1_readonly"
		compatibilityStatus = "active"
		nextAction = "rebuild_inferrum_v1"
	} else if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "generations")); statErr == nil {
		indexStatus = "candidate_only"
	} else if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "lancedb")); statErr == nil {
		indexStatus = "present_unversioned"
	}
	evaluationCount, latestReceipt, evaluationErr := latestKBReviewReceipt(root)
	if evaluationErr != nil {
		return errorProjection("kb.review.overview", evaluationErr), evaluationErr
	}
	evaluationStatus := "not_generated"
	lastRunID := ""
	lastRunAt := ""
	if latestReceipt != nil {
		evaluationStatus = kbReviewEvaluationStatus(*latestReceipt)
		lastRunID = latestReceipt.RunID
		lastRunAt = latestReceipt.CreatedAt
	}
	sourceStatus := "ready"
	if len(notes) == 0 {
		sourceStatus = "empty_sources"
	}
	projection := domain.NewProjection("kb.review.overview", "KB review overview is ready.")
	projection.Facts["scope"] = "personal_local"
	projection.Facts["sources"] = fmt.Sprint(len(notes))
	projection.Facts["source_status"] = sourceStatus
	projection.Facts["index_status"] = indexStatus
	projection.Facts["active_generation"] = activeGeneration
	projection.Facts["provider_status"] = providerStatus
	projection.Facts["sidecar_status"] = sidecarStatus
	projection.Facts["answer_mode"] = "not_generated"
	projection.Facts["evaluation_count"] = fmt.Sprint(evaluationCount)
	projection.Facts["evaluation_status"] = evaluationStatus
	if lastRunID != "" {
		projection.Facts["last_run_id"] = lastRunID
		projection.Facts["last_run_at"] = lastRunAt
	}
	projection.Facts["rerank"] = "passthrough"
	projection.Facts["generation_status"] = generationStatus
	projection.Facts["rollback_available"] = fmt.Sprint(rollbackAvailable)
	if protocol != "" {
		projection.Facts["protocol"] = protocol
		projection.Facts["provider"] = provider
		projection.Facts["model"] = model
		projection.Facts["embedding_dim"] = fmt.Sprint(embeddingDim)
		projection.Facts["source_snapshot"] = sourceSnapshot
		projection.Facts["source_digest"] = sourceDigest
	}
	if profile != "" {
		projection.Facts["profile"] = profile
		projection.Facts["compatibility_status"] = compatibilityStatus
		projection.Facts["next_action"] = nextAction
	}
	projection.Actions = []domain.Action{
		{Name: "check_sidecar", Command: "pinax kb doctor --vault <vault> --json"},
		{Name: "check_ollama", Command: "pinax kb provider doctor ollama --model pinax-qwen3-embedding:lowmem --vault <vault> --json"},
		{Name: "rebuild_candidate", Command: "pinax kb rebuild --backend lancedb --provider ollama --model pinax-qwen3-embedding:lowmem --vault <vault> --json"},
	}
	projection.Evidence = []string{".pinax/kb", ".pinax/events.jsonl"}
	projection.Data = map[string]any{
		"scope":                "personal_local",
		"source_status":        sourceStatus,
		"source_count":         len(notes),
		"index_status":         indexStatus,
		"active_generation":    activeGeneration,
		"generation_status":    generationStatus,
		"rollback_available":   rollbackAvailable,
		"evaluation_count":     evaluationCount,
		"last_run_id":          lastRunID,
		"last_run_at":          lastRunAt,
		"protocol":             protocol,
		"provider":             provider,
		"model":                model,
		"embedding_dim":        embeddingDim,
		"source_snapshot":      sourceSnapshot,
		"source_digest":        sourceDigest,
		"profile":              profile,
		"compatibility_status": compatibilityStatus,
		"next_action":          nextAction,
		"health": map[string]string{
			"ollama":  providerStatus,
			"lancedb": sidecarStatus,
		},
		"answer_mode": "not_generated",
		"truth_states": map[string]string{
			"source_status":     sourceStatus,
			"provider_status":   providerStatus,
			"sidecar_status":    sidecarStatus,
			"generation_status": indexStatus,
			"evaluation_status": evaluationStatus,
			"answer_mode":       "not_generated",
			"rerank":            "passthrough",
		},
		"supported_sources": kbReviewSourceCapabilities(),
	}
	return projection, nil
}

func latestKBReviewReceipt(root string) (int, *KBEvaluationReceipt, error) {
	dir := filepath.Join(kbRoot(root), "evaluations")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	type receiptEntry struct {
		receipt KBEvaluationReceipt
		name    string
	}
	receipts := make([]receiptEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validKBToken(entry.Name()) {
			continue
		}
		receipt, readErr := ReadKBEvaluationReceipt(root, entry.Name())
		if readErr != nil {
			continue
		}
		receipts = append(receipts, receiptEntry{receipt: receipt, name: entry.Name()})
	}
	if len(receipts) == 0 {
		return 0, nil, nil
	}
	sort.SliceStable(receipts, func(i, j int) bool {
		if receipts[i].receipt.CreatedAt == receipts[j].receipt.CreatedAt {
			return receipts[i].name > receipts[j].name
		}
		return receipts[i].receipt.CreatedAt > receipts[j].receipt.CreatedAt
	})
	latest := receipts[0].receipt
	return len(receipts), &latest, nil
}

// KBReviewSources returns bounded source inventory metadata. It never exposes
// Note.Body or an absolute vault path; source_ref is relative to the vault.
func (s *Service) KBReviewSources(_ context.Context, req KBReviewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.review.sources", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("kb.review.sources", err), err
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	rows := make([]map[string]any, 0, minInt(len(notes), limit))
	for _, note := range notes[:minInt(len(notes), limit)] {
		row := map[string]any{
			"source_id":   note.ID,
			"source_ref":  filepath.ToSlash(note.Path),
			"source_type": "markdown",
			"title":       note.Title,
			"kind":        note.Kind,
			"status":      note.Status,
			"tags":        append([]string(nil), note.Tags...),
		}
		if value := strings.TrimSpace(note.Frontmatter["source_ref"]); value != "" && !filepath.IsAbs(value) && !strings.Contains(filepath.ToSlash(value), "../") {
			row["source_ref"] = filepath.ToSlash(value)
		}
		for _, key := range []string{"source_type", "source_digest", "source_version", "acquired_at", "importer_version"} {
			if value := strings.TrimSpace(note.Frontmatter[key]); value != "" && validKBToken(value) {
				row[key] = value
			}
		}
		rows = append(rows, row)
	}
	projection := domain.NewProjection("kb.review.sources", "KB review sources are ready.")
	projection.Facts["sources"] = fmt.Sprint(len(rows))
	projection.Facts["total"] = fmt.Sprint(len(notes))
	projection.Facts["truncated"] = fmt.Sprint(len(rows) < len(notes))
	projection.Facts["unsupported_formats"] = "pdf,web,code_repository"
	projection.Data = map[string]any{
		"sources":     rows,
		"unsupported": kbReviewUnsupportedSourceCapabilities(),
	}
	return projection, nil
}

// kbReviewSourceCapabilities is a truthful capability projection, not an
// acquire registry. Unsupported or unconfigured formats carry a copyable
// import fallback so the review client can guide the operator without
// rendering a fake upload/sync action.
func kbReviewSourceCapabilities() []map[string]string {
	return []map[string]string{
		{
			"type":        "markdown",
			"status":      "ready",
			"next_action": "pinax kb import <source> --vault <vault> --dry-run",
		},
		{
			"type":        "pdf",
			"status":      "unsupported",
			"next_action": "Export PDF to Markdown or plain text, then run pinax kb import <source> --vault <vault> --dry-run",
		},
		{
			"type":        "web",
			"status":      "unsupported",
			"next_action": "Save the page as Markdown or plain text, then run pinax kb import <source> --vault <vault> --dry-run",
		},
		{
			"type":        "code_repository",
			"status":      "unsupported",
			"next_action": "Export repository docs to Markdown or plain text, then run pinax kb import <source> --vault <vault> --dry-run",
		},
		{
			"type":        "feishu",
			"status":      "not_configured",
			"next_action": "Export the Feishu document to Markdown or plain text, then run pinax kb import <source> --vault <vault> --dry-run",
		},
		{
			"type":        "image_ocr",
			"status":      "not_configured",
			"next_action": "Run an external OCR step and save Markdown or plain text, then run pinax kb import <source> --vault <vault> --dry-run",
		},
	}
}

func kbReviewUnsupportedSourceCapabilities() []map[string]string {
	capabilities := kbReviewSourceCapabilities()
	unsupported := make([]map[string]string, 0, len(capabilities)-1)
	for _, capability := range capabilities {
		if capability["status"] == "ready" {
			continue
		}
		unsupported = append(unsupported, capability)
	}
	return unsupported
}

// KBReviewEvaluationSuites returns only suite identity and bounded counts. It
// never returns the suite payload or a vault path, so the Workbench can render
// the evaluation index without becoming a second filesystem reader.
func (s *Service) KBReviewEvaluationSuites(_ context.Context, req KBReviewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.review.evaluation_suites", err), err
	}
	rows, configured, err := readKBReviewSuiteRows(root)
	if err != nil {
		return errorProjection("kb.review.evaluation_suites", err), err
	}
	status := "not_configured"
	if configured {
		status = "ready"
		for _, row := range rows {
			if row["status"] != "ready" {
				status = "partial"
				break
			}
		}
	}
	projection := domain.NewProjection("kb.review.evaluation_suites", "KB evaluation suites are ready.")
	projection.Facts["status"] = status
	projection.Facts["suite_count"] = fmt.Sprint(len(rows))
	projection.Data = map[string]any{"status": status, "suites": rows}
	return projection, nil
}

// KBReviewEvaluationQuestions returns one suite's bounded question summary.
// Query text is intentionally capped for the review surface; receipts and
// integration evidence never persist it.
func (s *Service) KBReviewEvaluationQuestions(_ context.Context, req KBReviewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.review.evaluation_questions", err), err
	}
	suiteID := strings.TrimSpace(req.SuiteID)
	if !validKBToken(suiteID) {
		err := &domain.CommandError{Code: "kb_evaluation_suite_not_found", Message: "KB evaluation suite was not found", Hint: "Choose a configured suite before loading questions"}
		return domain.NewErrorProjection("kb.review.evaluation_questions", err), err
	}
	suite, sourceRef, err := readKBReviewSuite(root, suiteID)
	if err != nil {
		if os.IsNotExist(err) {
			cmdErr := &domain.CommandError{Code: "kb_evaluation_suite_not_found", Message: "KB evaluation suite was not found", Hint: "Choose a configured suite before loading questions"}
			return domain.NewErrorProjection("kb.review.evaluation_questions", cmdErr), cmdErr
		}
		return errorProjection("kb.review.evaluation_questions", err), err
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	questions := make([]map[string]any, 0, minInt(len(suite.Questions), limit))
	for _, question := range suite.Questions[:minInt(len(suite.Questions), limit)] {
		query := strings.TrimSpace(question.Query)
		if len(query) > 512 {
			query = query[:512]
		}
		questions = append(questions, map[string]any{
			"question_id":        question.QuestionID,
			"query":              query,
			"expected_citations": append([]string(nil), question.ExpectedCitations...),
		})
	}
	projection := domain.NewProjection("kb.review.evaluation_questions", "KB evaluation questions are ready.")
	projection.Facts["suite_id"] = suite.SuiteID
	projection.Facts["suite_version"] = suite.Version
	projection.Facts["questions"] = fmt.Sprint(len(questions))
	projection.Facts["total"] = fmt.Sprint(len(suite.Questions))
	projection.Facts["truncated"] = fmt.Sprint(len(questions) < len(suite.Questions))
	projection.Data = map[string]any{
		"suite_id":       suite.SuiteID,
		"suite_version":  suite.Version,
		"dataset_status": suite.DatasetStatus(),
		"source_ref":     sourceRef,
		"questions":      questions,
	}
	return projection, nil
}

// KBReviewRun returns a sanitized immutable receipt projection. It deliberately
// omits raw questions, permission IDs, vectors, and any provider payload.
func (s *Service) KBReviewRun(_ context.Context, req KBReviewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.review.run", err), err
	}
	receipt, err := ReadKBEvaluationReceipt(root, strings.TrimSpace(req.RunID))
	if err != nil {
		if os.IsNotExist(err) {
			cmdErr := &domain.CommandError{Code: "kb_evaluation_run_not_found", Message: "KB evaluation run was not found", Hint: "Refresh the run list or use a stable run id"}
			return domain.NewErrorProjection("kb.review.run", cmdErr), cmdErr
		}
		return errorProjection("kb.review.run", err), err
	}
	projection := domain.NewProjection("kb.review.run", "KB evaluation run is ready.")
	projection.Facts["run_id"] = receipt.RunID
	projection.Facts["status"] = receipt.Status
	projection.Facts["generation_id"] = receipt.GenerationID
	projection.Facts["protocol"] = receipt.Protocol
	projection.Facts["provider"] = receipt.Provider
	projection.Facts["model"] = receipt.Model
	projection.Facts["embedding_dim"] = fmt.Sprint(receipt.EmbeddingDim)
	projection.Facts["suite_id"] = receipt.SuiteID
	projection.Facts["suite_version"] = receipt.SuiteVersion
	evaluationStatus := kbReviewEvaluationStatus(receipt)
	projection.Facts["evaluation_status"] = evaluationStatus
	projection.Facts["rerank"] = "passthrough"
	projection.Facts["answer_mode"] = "not_generated"
	projection.Data = map[string]any{
		"run_id":                   receipt.RunID,
		"status":                   receipt.Status,
		"generation_id":            receipt.GenerationID,
		"source_snapshot":          receipt.SourceSnapshot,
		"source_digest":            receipt.SourceDigest,
		"provider":                 receipt.Provider,
		"model":                    receipt.Model,
		"base_model_digest":        receipt.BaseModelDigest,
		"model_manifest_digest":    receipt.ModelManifestDigest,
		"profile_hash":             receipt.ProfileHash,
		"daemon_version":           receipt.DaemonVersion,
		"embedding_dim":            receipt.EmbeddingDim,
		"suite_id":                 receipt.SuiteID,
		"suite_version":            receipt.SuiteVersion,
		"gate_config_hash":         receipt.GateConfigHash,
		"generation_manifest_hash": receipt.GenerationManifestHash,
		"metrics":                  receipt.Metrics,
		"evaluation_status":        evaluationStatus,
		"rerank":                   "passthrough",
		"created_at":               receipt.CreatedAt,
		"answer_mode":              "not_generated",
	}
	return projection, nil
}

func kbReviewEvaluationStatus(receipt KBEvaluationReceipt) string {
	if receipt.Metrics.DatasetStatus != KBEvaluationDatasetReady {
		return receipt.Metrics.DatasetStatus
	}
	counts := receipt.Metrics.StatusCounts
	if counts["no_hit"] >= receipt.Metrics.TotalQuestions && receipt.Metrics.TotalQuestions > 0 {
		return "no_hits"
	}
	if counts["partial"] > 0 || counts["no_hit"] > 0 {
		return "partial"
	}
	if receipt.Status == "passed" {
		return "passed"
	}
	if receipt.Status == "failed" {
		return "failed"
	}
	return "not_generated"
}

func readKBReviewSuiteRows(root string) ([]map[string]any, bool, error) {
	dir := filepath.Join(root, ".pinax", "kb", "evaluation-suites")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	rows := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".pinax", "kb", "evaluation-suites", entry.Name()))
		payload, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, false, readErr
		}
		suite, parseErr := ParseKBEvaluationSuite(payload)
		if parseErr != nil {
			rows = append(rows, map[string]any{"source_ref": rel, "status": "invalid"})
			continue
		}
		rows = append(rows, map[string]any{
			"suite_id":       suite.SuiteID,
			"version":        suite.Version,
			"question_count": len(suite.Questions),
			"dataset_status": suite.DatasetStatus(),
			"status":         "ready",
			"source_ref":     rel,
		})
	}
	return rows, true, nil
}

func readKBReviewSuite(root, suiteID string) (KBEvaluationSuite, string, error) {
	dir := filepath.Join(root, ".pinax", "kb", "evaluation-suites")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return KBEvaluationSuite{}, "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return KBEvaluationSuite{}, "", readErr
		}
		suite, parseErr := ParseKBEvaluationSuite(payload)
		if parseErr == nil && suite.SuiteID == suiteID {
			return suite, filepath.ToSlash(filepath.Join(".pinax", "kb", "evaluation-suites", entry.Name())), nil
		}
	}
	return KBEvaluationSuite{}, "", os.ErrNotExist
}
