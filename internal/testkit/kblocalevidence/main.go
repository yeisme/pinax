// Package main runs a small real-provider local-KB canary and delegates
// evidence persistence to the repository-standard integration evidence writer.
// The canary is intentionally synthetic and is never a quality claim for a
// user's real corpus.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
	"github.com/yeisme/pinax/internal/testkit/evidence"
)

type canaryOptions struct {
	Provider           string
	Model              string
	AllowDiskHighWater bool
	RunID              string
}

type pinaxCommandMeasurement struct {
	Envelope    map[string]any
	DurationMS  int64
	MaxRSSBytes int64
	CPUUserMS   int64
	CPUSystemMS int64
}

type ollamaProcessStats struct {
	ModelCount      int
	TotalModelBytes int64
	TotalVRAMBytes  int64
}

type ollamaResourceStats struct {
	PeakModelBytes int64
	PeakVRAMBytes  int64
	Samples        int
}

type ollamaResourceSampler struct {
	done    chan struct{}
	closed  sync.Once
	wg      sync.WaitGroup
	mu      sync.Mutex
	peak    ollamaProcessStats
	samples int
}

func main() {
	child := flag.Bool("child", false, "run the canary child under the evidence wrapper")
	provider := flag.String("provider", "ollama", "embedding provider")
	model := flag.String("model", "pinax-qwen3-embedding:lowmem", "exact embedding model tag")
	allowDisk := flag.Bool("allow-disk-high-water", false, "explicitly allow the current disk high-water mark")
	runID := flag.String("run-id", "", "evidence run id used by the child")
	flag.Parse()
	if *child {
		if err := runChild(canaryOptions{Provider: *provider, Model: *model, AllowDiskHighWater: *allowDisk, RunID: *runID}); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "kb canary failed:", safeFailure(err))
			os.Exit(1)
		}
		return
	}

	runIDValue := time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	args := []string{"run", "./internal/testkit/kblocalevidence", "--child", "--run-id", runIDValue, "--provider", *provider, "--model", *model}
	if *allowDisk {
		args = append(args, "--allow-disk-high-water")
	}
	result, err := evidence.Run(evidence.Config{
		RunID:             runIDValue,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           append([]string{"go"}, args...),
		PassThroughStdout: os.Stdout,
		PassThroughStderr: os.Stderr,
		PassStatus:        "passed",
		Layer:             "component",
		ExtraChecks: map[string]any{
			"synthetic_canary":           true,
			"real_ollama_embed":          *provider == "ollama",
			"inferrum_sidecar_v1":        true,
			"candidate_evaluation":       true,
			"activation_decision":        true,
			"answer_mode":                "not_generated",
			"real_corpus_quality_proven": false,
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "integration evidence error: %v\n", err)
		if result.ExitCode == 0 {
			os.Exit(1)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "integration evidence: %s\n", result.RunDir)
	os.Exit(result.ExitCode)
}

func runChild(opts canaryOptions) error {
	if strings.TrimSpace(opts.RunID) == "" {
		return errors.New("missing_run_id")
	}
	artifactDir := filepath.Join("temp", "integration-test-runs", opts.RunID, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return errors.New("artifact_directory_failed")
	}
	vault, err := os.MkdirTemp("", "pinax-kb-canary-")
	if err != nil {
		return errors.New("canary_vault_failed")
	}
	defer func() { _ = os.RemoveAll(vault) }()
	if _, err := runPinaxMeasured("init", vault, "--title", "KB Canary", "--json"); err != nil {
		return writeCanaryFailure(artifactDir, "init", err)
	}
	if err := writeCanaryNotes(vault); err != nil {
		return writeCanaryFailure(artifactDir, "fixture", errors.New("fixture_write_failed"))
	}
	suiteRef := ".pinax/kb/evaluation-suites/local-canary.json"
	if err := writeCanarySuite(vault); err != nil {
		return writeCanaryFailure(artifactDir, "fixture", errors.New("suite_write_failed"))
	}
	rebuildArgs := []string{"kb", "rebuild", "--vault", vault, "--backend", "lancedb", "--provider", opts.Provider, "--model", opts.Model, "--json"}
	if opts.AllowDiskHighWater {
		rebuildArgs = append(rebuildArgs, "--allow-disk-high-water")
	}
	sampler := startOllamaResourceSampler()
	defer sampler.Close()
	rebuildMeasurement, err := runPinaxMeasured(rebuildArgs...)
	if err != nil {
		return writeCanaryFailure(artifactDir, "rebuild", err)
	}
	rebuild := rebuildMeasurement.Envelope
	generationID, err := nestedString(rebuild, "facts", "generation_id")
	if err != nil {
		return writeCanaryFailure(artifactDir, "rebuild", errors.New("generation_id_missing"))
	}
	evaluationMeasurement, err := runPinaxMeasured("kb", "evaluate", "--suite", suiteRef, "--generation", generationID, "--vault", vault, "--json")
	if err != nil {
		return writeCanaryFailure(artifactDir, "evaluate", err)
	}
	evaluation := evaluationMeasurement.Envelope
	runID, err := nestedString(evaluation, "facts", "run_id")
	if err != nil {
		return writeCanaryFailure(artifactDir, "evaluate", errors.New("evaluation_run_id_missing"))
	}
	activationMeasurement, err := runPinaxMeasured("kb", "activate", "--generation", generationID, "--suite", suiteRef, "--run-id", runID, "--vault", vault, "--json")
	if err != nil {
		return writeCanaryFailure(artifactDir, "activate", err)
	}
	activation := activationMeasurement.Envelope
	searchMeasurement, err := runPinaxMeasured("kb", "search", "Inferrum local retrieval", "--generation", generationID, "--vault", vault, "--provider", opts.Provider, "--model", opts.Model, "--json")
	if err != nil {
		return writeCanaryFailure(artifactDir, "search", err)
	}
	search := searchMeasurement.Envelope
	shadowCompare, err := runShadowCompare(artifactDir, strings.TrimSpace(os.Getenv("PINAX_KB_SIDECAR")))
	if err != nil {
		return writeCanaryFailure(artifactDir, "shadow_compare", err)
	}
	resourceStats := sampler.Close()
	afterOllama, afterOllamaErr := fetchOllamaProcessSnapshot()
	resourceObservation := "observed"
	if afterOllamaErr != nil {
		resourceObservation = "unavailable"
	}
	maxRSSBytes := maxInt64(rebuildMeasurement.MaxRSSBytes, evaluationMeasurement.MaxRSSBytes, activationMeasurement.MaxRSSBytes, searchMeasurement.MaxRSSBytes)
	resources := map[string]any{
		"measurement_scope":           "go_run_command_process",
		"cold_rebuild_duration_ms":    rebuildMeasurement.DurationMS,
		"warm_evaluation_duration_ms": evaluationMeasurement.DurationMS,
		"warm_activation_duration_ms": activationMeasurement.DurationMS,
		"warm_search_duration_ms":     searchMeasurement.DurationMS,
		"max_rss_bytes":               maxRSSBytes,
		"cpu_user_ms":                 rebuildMeasurement.CPUUserMS + evaluationMeasurement.CPUUserMS + activationMeasurement.CPUUserMS + searchMeasurement.CPUUserMS,
		"cpu_system_ms":               rebuildMeasurement.CPUSystemMS + evaluationMeasurement.CPUSystemMS + activationMeasurement.CPUSystemMS + searchMeasurement.CPUSystemMS,
		"ollama_ps_observation":       resourceObservation,
		"ollama_ps_samples":           resourceStats.Samples,
		"ollama_peak_model_bytes":     resourceStats.PeakModelBytes,
		"ollama_peak_vram_bytes":      resourceStats.PeakVRAMBytes,
		"ollama_unload_status":        ollamaUnloadStatus(afterOllama),
		"ollama_models_after":         afterOllama.ModelCount,
	}
	result := map[string]any{
		"schema_version":        "pinax.kb.canary-evidence.v1",
		"scope":                 "synthetic_component_canary",
		"provider":              opts.Provider,
		"model":                 opts.Model,
		"generation_id":         generationID,
		"evaluation_run_id":     runID,
		"evaluation_status":     nestedStringOrEmpty(evaluation, "facts", "status"),
		"activation_status":     nestedStringOrEmpty(activation, "facts", "status"),
		"search_matches":        nestedStringOrEmpty(search, "facts", "matches"),
		"documents":             nestedStringOrEmpty(rebuild, "facts", "documents"),
		"chunks":                nestedStringOrEmpty(rebuild, "facts", "chunks"),
		"embedding_dim":         nestedStringOrEmpty(rebuild, "facts", "embedding_dim"),
		"protocol":              nestedStringOrEmpty(rebuild, "facts", "protocol"),
		"model_manifest_digest": nestedStringOrEmpty(rebuild, "facts", "model_manifest_digest"),
		"profile_hash":          nestedStringOrEmpty(rebuild, "facts", "profile_hash"),
		"base_model_digest":     nestedStringOrEmpty(rebuild, "facts", "base_model_digest"),
		"daemon_version":        nestedStringOrEmpty(rebuild, "facts", "daemon_version"),
		"source_snapshot":       nestedStringOrEmpty(rebuild, "facts", "source_snapshot"),
		"source_digest":         nestedStringOrEmpty(rebuild, "facts", "source_digest"),
		"answer_mode":           "not_generated",
		"quality_verdict":       "not_proven_without_real_corpus",
		"disk_override":         opts.AllowDiskHighWater,
		"sidecar_configured":    strings.TrimSpace(os.Getenv("PINAX_KB_SIDECAR")) != "",
		"ollama_host_scope":     "loopback_or_configured_local",
		"recall_at_5":           nestedStringOrEmpty(evaluation, "facts", "recall_at_5"),
		"mrr_at_10":             nestedStringOrEmpty(evaluation, "facts", "mrr_at_10"),
		"citation_coverage":     nestedStringOrEmpty(evaluation, "facts", "citation_coverage"),
		"shadow_compare":        shadowCompare,
		"resources":             resources,
	}
	if err := writeArtifact(artifactDir, "canary.json", result); err != nil {
		return errors.New("artifact_write_failed")
	}
	return nil
}

func runShadowCompare(artifactDir, sidecarExecutable string) (map[string]any, error) {
	started := time.Now()
	result, err := buildShadowCompare(sidecarExecutable)
	if err != nil {
		return nil, err
	}
	result["duration_ms"] = time.Since(started).Milliseconds()
	if err := writeArtifact(artifactDir, "shadow-compare.json", result); err != nil {
		return nil, errors.New("shadow_artifact_write_failed")
	}
	return result, nil
}

func buildShadowCompare(sidecarExecutable string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	notes := []domain.Note{
		{ID: "shadow-alpha", Path: "notes/shadow-alpha.md", Title: "Alpha", Body: "# Alpha\n\nInferrum local retrieval uses a bounded citation."},
		{ID: "shadow-beta", Path: "notes/shadow-beta.md", Title: "Beta", Body: "# Beta\n\nCandidate evaluation precedes activation."},
	}
	provider := semantic.FakeProvider{ModelName: semantic.FakeProviderModel}
	chunks, err := semantic.BuildChunks(ctx, notes, provider, semantic.DefaultBackend)
	if err != nil {
		return nil, errors.New("shadow_build_failed")
	}
	if len(chunks) != 2 {
		return nil, errors.New("shadow_chunk_count_invalid")
	}
	root, err := os.MkdirTemp("", "pinax-kb-shadow-")
	if err != nil {
		return nil, errors.New("shadow_temp_failed")
	}
	defer func() { _ = os.RemoveAll(root) }()
	legacyRoot := filepath.Join(root, "v1")
	inferrumRoot := filepath.Join(root, "inferrum-v1")
	if err := semantic.NewFileStore(legacyRoot, semantic.DefaultBackend).Save(chunks); err != nil {
		return nil, errors.New("shadow_v1_save_failed")
	}
	if err := writeLegacyShadowMetadata(legacyRoot, provider, chunks); err != nil {
		return nil, errors.New("shadow_v1_metadata_failed")
	}
	storeURI := filepath.Join(inferrumRoot, "vector-store")
	if _, err := semantic.Save(ctx, inferrumRoot, chunks, semantic.DefaultBackend, semantic.SidecarConfig{Executable: sidecarExecutable, Timeout: 30 * time.Second, StoreURI: storeURI}, len(notes)); err != nil {
		return nil, errors.New("shadow_inferrum_v1_save_failed")
	}
	allowedIDs := semantic.ChunkIDsForNotes(notes)
	query := "Inferrum local retrieval"
	v1Hits, v1Total, err := semantic.SearchLegacyV1(ctx, legacyRoot, query, provider, 5, allowedIDs)
	if err != nil {
		return nil, errors.New("shadow_v1_search_failed")
	}
	inferrumHits, inferrumTotal, err := semantic.SearchWithAllowedIDs(ctx, inferrumRoot, query, provider, semantic.DefaultBackend, 5, allowedIDs, semantic.SidecarConfig{Executable: sidecarExecutable, Timeout: 30 * time.Second, StoreURI: storeURI})
	if err != nil {
		return nil, errors.New("shadow_inferrum_v1_search_failed")
	}
	v1Citations := shadowCitationSet(v1Hits)
	inferrumCitations := shadowCitationSet(inferrumHits)
	if v1Total != len(chunks) || inferrumTotal != len(chunks) || len(v1Citations) != len(inferrumCitations) || !sameStrings(v1Citations, inferrumCitations) {
		return nil, errors.New("shadow_citation_mismatch")
	}
	if !shadowHitsSafe(v1Hits) || !shadowHitsSafe(inferrumHits) {
		return nil, errors.New("shadow_unsafe_citation")
	}
	_, v1EmptyTotal, v1EmptyErr := semantic.SearchLegacyV1(ctx, legacyRoot, query, provider, 5, []string{})
	_, inferrumEmptyTotal, inferrumEmptyErr := semantic.SearchWithAllowedIDs(ctx, inferrumRoot, query, provider, semantic.DefaultBackend, 5, []string{}, semantic.SidecarConfig{Executable: sidecarExecutable, Timeout: 30 * time.Second, StoreURI: storeURI})
	if v1EmptyErr != nil || inferrumEmptyErr != nil || v1EmptyTotal != 0 || inferrumEmptyTotal != 0 {
		return nil, errors.New("shadow_permission_empty_mismatch")
	}
	missingV1Root := filepath.Join(root, "missing-v1")
	missingInferrumRoot := filepath.Join(root, "missing-inferrum-v1")
	_, _, v1MissingErr := semantic.SearchLegacyV1(ctx, missingV1Root, query, provider, 5, allowedIDs)
	_, _, inferrumMissingErr := semantic.SearchWithAllowedIDs(ctx, missingInferrumRoot, query, provider, semantic.DefaultBackend, 5, allowedIDs, semantic.SidecarConfig{Executable: sidecarExecutable, Timeout: 30 * time.Second, StoreURI: filepath.Join(missingInferrumRoot, "vector-store")})
	v1MissingCode := shadowCommandCode(v1MissingErr)
	inferrumMissingCode := shadowCommandCode(inferrumMissingErr)
	if shadowErrorClass(v1MissingCode) != "unavailable" || shadowErrorClass(inferrumMissingCode) != "unavailable" {
		return nil, errors.New("shadow_missing_projection_mismatch")
	}
	return map[string]any{
		"schema_version":                   "pinax.kb.shadow-compare.v1",
		"scope":                            "synthetic_deterministic_corpus",
		"provider":                         provider.Name(),
		"model":                            provider.Model(),
		"legacy_protocol":                  semantic.LegacySidecarSchema,
		"inferrum_protocol":                semantic.SidecarSchema,
		"inferrum_sidecar":                 "real_configured_sidecar",
		"documents_legacy":                 len(notes),
		"documents_inferrum":               len(notes),
		"chunks_legacy":                    len(chunks),
		"chunks_inferrum":                  len(chunks),
		"top_k":                            5,
		"legacy_total":                     v1Total,
		"inferrum_total":                   inferrumTotal,
		"legacy_citations":                 v1Citations,
		"inferrum_citations":               inferrumCitations,
		"citation_identity_match":          true,
		"safe_citations":                   true,
		"permission_empty_semantics_match": true,
		"score_bytes_compared":             false,
		"query_in_evidence":                false,
		"raw_vectors_in_evidence":          false,
		"forbidden_fields_absent":          true,
		"error_semantics_compared":         []string{"permission_empty", "missing_projection"},
		"missing_projection_legacy_code":   v1MissingCode,
		"missing_projection_inferrum_code": inferrumMissingCode,
		"missing_projection_class_match":   true,
	}, nil
}

func writeLegacyShadowMetadata(root string, provider semantic.Provider, chunks []semantic.Chunk) error {
	metadata := map[string]any{
		"schema_version": semantic.LegacySidecarSchema,
		"backend":        semantic.DefaultBackend,
		"provider":       provider.Name(),
		"model":          provider.Model(),
		"embedding_dim":  len(chunks[0].Vector),
		"indexed_at":     "2026-08-02T00:00:00Z",
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	path := filepath.Join(root, ".pinax", "kb", "lancedb", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o600)
}

func shadowCitationSet(hits []semantic.SearchHit) []string {
	items := make([]string, 0, len(hits))
	for _, hit := range hits {
		items = append(items, filepath.ToSlash(hit.Path)+"#"+hit.HeadingPath)
	}
	sort.Strings(items)
	return items
}

func shadowHitsSafe(hits []semantic.SearchHit) bool {
	for _, hit := range hits {
		if strings.TrimSpace(hit.Path) == "" || filepath.IsAbs(hit.Path) || strings.Contains(filepath.ToSlash(hit.Path), "../") || strings.TrimSpace(hit.Preview) == "" {
			return false
		}
	}
	return true
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func shadowCommandCode(err error) string {
	if err == nil {
		return "success"
	}
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) && commandErr.Code != "" {
		return commandErr.Code
	}
	return "error"
}

func shadowErrorClass(code string) string {
	switch code {
	case "kb_legacy_v1_unavailable", "kb_index_missing", "kb_sidecar_failed", "kb_sidecar_unavailable", "kb_sidecar_timeout", "vector_index_missing":
		return "unavailable"
	default:
		return code
	}
}

func runPinaxMeasured(args ...string) (pinaxCommandMeasurement, error) {
	command := append([]string{"run", "./cmd/pinax"}, args...)
	cmd := exec.Command("go", command...)
	started := time.Now()
	output, err := cmd.CombinedOutput()
	measurement := pinaxCommandMeasurement{DurationMS: time.Since(started).Milliseconds()}
	measurement.MaxRSSBytes, measurement.CPUUserMS, measurement.CPUSystemMS = processResourceUsage(cmd.ProcessState)
	var envelope map[string]any
	_ = json.Unmarshal(output, &envelope)
	measurement.Envelope = envelope
	if err != nil {
		if code := nestedStringOrEmpty(envelope, "error", "code"); code != "" {
			return measurement, errors.New(code)
		}
		return measurement, errors.New("pinax_command_failed")
	}
	if status := nestedStringOrEmpty(envelope, "status"); status == "failed" {
		if code := nestedStringOrEmpty(envelope, "error", "code"); code != "" {
			return measurement, errors.New(code)
		}
		return measurement, errors.New("pinax_command_failed")
	}
	return measurement, nil
}

func processResourceUsage(state *os.ProcessState) (maxRSSBytes, cpuUserMS, cpuSystemMS int64) {
	if state == nil {
		return 0, 0, 0
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage == nil {
		return 0, 0, 0
	}
	// Linux reports ru_maxrss in KiB. This canary runs on the current Linux
	// server; keeping the conversion here avoids persisting platform-specific
	// raw structs in evidence.
	maxRSSBytes = int64(usage.Maxrss) * 1024
	cpuUserMS = int64(usage.Utime.Sec)*1000 + int64(usage.Utime.Usec)/1000
	cpuSystemMS = int64(usage.Stime.Sec)*1000 + int64(usage.Stime.Usec)/1000
	return maxRSSBytes, cpuUserMS, cpuSystemMS
}

func startOllamaResourceSampler() *ollamaResourceSampler {
	sampler := &ollamaResourceSampler{done: make(chan struct{})}
	if stats, err := fetchOllamaProcessSnapshot(); err == nil {
		sampler.record(stats)
	}
	sampler.wg.Add(1)
	go func() {
		defer sampler.wg.Done()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if stats, err := fetchOllamaProcessSnapshot(); err == nil {
					sampler.record(stats)
				}
			case <-sampler.done:
				return
			}
		}
	}()
	return sampler
}

func (s *ollamaResourceSampler) record(stats ollamaProcessStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples++
	if stats.TotalModelBytes > s.peak.TotalModelBytes {
		s.peak.TotalModelBytes = stats.TotalModelBytes
	}
	if stats.TotalVRAMBytes > s.peak.TotalVRAMBytes {
		s.peak.TotalVRAMBytes = stats.TotalVRAMBytes
	}
	if stats.ModelCount > s.peak.ModelCount {
		s.peak.ModelCount = stats.ModelCount
	}
}

func (s *ollamaResourceSampler) Close() ollamaResourceStats {
	s.closed.Do(func() {
		close(s.done)
		s.wg.Wait()
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	return ollamaResourceStats{PeakModelBytes: s.peak.TotalModelBytes, PeakVRAMBytes: s.peak.TotalVRAMBytes, Samples: s.samples}
}

func parseOllamaProcessSnapshot(payload []byte) (ollamaProcessStats, error) {
	var response struct {
		Models []struct {
			Size     int64 `json:"size"`
			SizeVRAM int64 `json:"size_vram"`
		} `json:"models"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return ollamaProcessStats{}, err
	}
	stats := ollamaProcessStats{ModelCount: len(response.Models)}
	for _, model := range response.Models {
		stats.TotalModelBytes += model.Size
		stats.TotalVRAMBytes += model.SizeVRAM
	}
	return stats, nil
}

func fetchOllamaProcessSnapshot() (ollamaProcessStats, error) {
	request, err := http.NewRequest(http.MethodGet, ollamaBaseURL()+"/api/ps", nil)
	if err != nil {
		return ollamaProcessStats{}, err
	}
	client := &http.Client{Timeout: 750 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return ollamaProcessStats{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return ollamaProcessStats{}, fmt.Errorf("ollama_ps_status_%d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return ollamaProcessStats{}, err
	}
	return parseOllamaProcessSnapshot(payload)
}

func ollamaBaseURL() string {
	base := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	return strings.TrimRight(base, "/")
}

func ollamaUnloadStatus(stats ollamaProcessStats) string {
	if stats.ModelCount == 0 {
		return "unloaded"
	}
	return "still_loaded"
}

func maxInt64(values ...int64) int64 {
	var max int64
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func writeCanaryNotes(vault string) error {
	notes := map[string]string{
		"canary.md":   "canary-inferrum|Inferrum local retrieval|Inferrum local retrieval uses a bounded v1 sidecar projection and safe citations.",
		"protocol.md": "canary-protocol|Inferrum protocol|The inferrum.sidecar.v1 protocol stores opaque metadata and bounded records.",
		"operator.md": "canary-operator|Operator recovery|Candidate evaluation precedes activation and rollback remains explicit.",
	}
	for name, value := range notes {
		parts := strings.SplitN(value, "|", 3)
		if len(parts) != 3 {
			return errors.New("fixture_format_failed")
		}
		path := filepath.Join(vault, "notes", name)
		body := fmt.Sprintf("---\nschema_version: pinax.note.v1\nnote_id: %s\ntitle: %s\nkind: reference\nstatus: active\n---\n\n# %s\n\n%s\n", parts[0], parts[1], parts[1], parts[2])
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeCanarySuite(vault string) error {
	questions := make([]map[string]any, 20)
	for i := range questions {
		questions[i] = map[string]any{"question_id": fmt.Sprintf("canary-%02d", i+1), "query": "Inferrum local retrieval", "expected_citations": []string{"notes/canary.md#Inferrum local retrieval"}}
	}
	payload := map[string]any{"schema_version": "pinax.kb.evaluation-suite.v1", "suite_id": "local-canary", "version": "synthetic-2026-08-02", "questions": questions}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path := filepath.Join(vault, ".pinax", "kb", "evaluation-suites", "local-canary.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func nestedString(value map[string]any, keys ...string) (string, error) {
	result := nestedStringOrEmpty(value, keys...)
	if result == "" {
		return "", errors.New("missing_fact")
	}
	return result, nil
}

func nestedStringOrEmpty(value map[string]any, keys ...string) string {
	var current any = value
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	return strings.TrimSpace(fmt.Sprint(current))
}

func writeCanaryFailure(dir, stage string, err error) error {
	code := safeFailure(err)
	payload := map[string]any{"schema_version": "pinax.kb.canary-failure.v1", "stage": stage, "code": code, "status": "failed"}
	if writeErr := writeArtifact(dir, "failure.json", payload); writeErr != nil {
		return errors.New("artifact_write_failed")
	}
	return errors.New(stage + ":" + code)
}

func safeFailure(err error) string {
	if err == nil {
		return "unknown"
	}
	value := strings.TrimSpace(err.Error())
	if value == "" {
		return "unknown"
	}
	if strings.ContainsAny(value, " /\\\n\t") {
		return "command_failed"
	}
	return value
}

func writeArtifact(dir, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0o644)
}
