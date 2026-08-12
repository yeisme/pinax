// Package main runs either a synthetic local-KB canary or an explicitly
// confirmed real-corpus candidate evaluation. It delegates evidence persistence
// to the repository-standard integration evidence writer and never activates a
// real-corpus candidate itself.
package main

import (
	"bufio"
	"context"
	"debug/macho"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
	"github.com/yeisme/pinax/internal/testkit/evidence"
)

type canaryOptions struct {
	Provider           string
	Model              string
	AllowDiskHighWater bool
	RealCorpus         bool
	M4Preflight        bool
	M4PreflightOnly    bool
	Vault              string
	Suite              string
	PinaxBinary        string
	InferrumBinary     string
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

type hostProfile struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	CPUCores       int    `json:"cpu_cores"`
	CPUBrand       string `json:"cpu_brand,omitempty"`
	MemoryBytes    int64  `json:"memory_bytes,omitempty"`
	TargetM4Status string `json:"target_m4_status"`
	// TargetM4AirStatus is an additive release-gate fact. TargetM4Status keeps
	// its existing meaning: an Apple M4 chip was observed on macOS arm64.
	TargetM4AirStatus string `json:"target_m4_air_status"`
	// MachineModel is only used to distinguish the supported M4 Air host class.
	// It is deliberately excluded from persisted evidence to avoid turning a
	// hardware identifier into an operator/device inventory record.
	MachineModel string `json:"-"`
}

// inferrumEmbeddedData is the narrow, path-free subset of the existing
// inferrum validate embedded JSON projection that qualifies an M4 run. It is
// intentionally distinct from the raw command response so evidence never
// retains caller paths, sidecar arguments, or sidecar stderr.
type inferrumEmbeddedData struct {
	Backend       string `json:"backend"`
	Dependency    string `json:"dependency"`
	EmbeddingDim  int    `json:"embedding_dim"`
	Rows          int    `json:"rows"`
	Hits          int    `json:"hits"`
	StoreState    string `json:"store_state"`
	StoreURIScope string `json:"store_uri_scope"`
	Table         string `json:"table"`
}

type inferrumEmbeddedValidation struct {
	Command string               `json:"command"`
	Status  string               `json:"status"`
	Data    inferrumEmbeddedData `json:"data"`
}

// m4PreflightArtifact records only bounded compatibility facts. It never
// records a binary path, sidecar path, vault path, raw command output, or
// LanceDB store URI.
type m4PreflightArtifact struct {
	SchemaVersion              string `json:"schema_version"`
	Scope                      string `json:"scope"`
	Status                     string `json:"status"`
	FailureCode                string `json:"failure_code,omitempty"`
	TargetM4Status             string `json:"target_m4_status"`
	TargetM4AirStatus          string `json:"target_m4_air_status"`
	MacOSVersion               string `json:"macos_version,omitempty"`
	PythonVersion              string `json:"python_version,omitempty"`
	SidecarPythonArchitecture  string `json:"sidecar_python_architecture,omitempty"`
	InferrumBinaryArchitecture string `json:"inferrum_binary_architecture,omitempty"`
	PinaxBinaryArchitecture    string `json:"pinax_binary_architecture,omitempty"`
	EmbeddedBackend            string `json:"embedded_backend,omitempty"`
	EmbeddedDependency         string `json:"embedded_dependency,omitempty"`
	EmbeddedEmbeddingDim       int    `json:"embedded_embedding_dim,omitempty"`
	EmbeddedRows               int    `json:"embedded_rows,omitempty"`
	EmbeddedHits               int    `json:"embedded_hits,omitempty"`
	EmbeddedStoreState         string `json:"embedded_store_state,omitempty"`
	EmbeddedStoreURIScope      string `json:"embedded_store_uri_scope,omitempty"`
	EmbeddedTable              string `json:"embedded_table,omitempty"`
}

// providerBenchmarkArtifact is the bounded Pinax projection of the existing
// Inferrum benchmark envelope. It intentionally excludes synthetic input
// details, raw output, binary paths, endpoints, and provider payloads.
type providerBenchmarkArtifact struct {
	SchemaVersion  string   `json:"schema_version"`
	Status         string   `json:"status"`
	FailureCode    string   `json:"failure_code,omitempty"`
	Command        string   `json:"command,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	SampleCount    int      `json:"sample_count,omitempty"`
	BatchSize      int      `json:"batch_size,omitempty"`
	BatchCount     int      `json:"batch_count,omitempty"`
	WarmupBatches  int      `json:"warmup_batches,omitempty"`
	BatchAPI       bool     `json:"batch_api"`
	EmbeddingDim   int      `json:"embedding_dim,omitempty"`
	WarmupMS       int64    `json:"warmup_ms,omitempty"`
	ElapsedMS      int64    `json:"elapsed_ms,omitempty"`
	BatchP50MS     int64    `json:"batch_p50_ms,omitempty"`
	BatchP95MS     int64    `json:"batch_p95_ms,omitempty"`
	ItemsPerSecond float64  `json:"items_per_second,omitempty"`
	Scope          string   `json:"scope,omitempty"`
	NotMeasured    []string `json:"not_measured,omitempty"`
}

// firstSupportGateArtifact is the bounded M4-pilot-only decision projection.
// It does not alter the generic Pinax evaluation receipt or activation gate.
type firstSupportGateArtifact struct {
	SchemaVersion           string  `json:"schema_version"`
	Status                  string  `json:"status"`
	FailureCode             string  `json:"failure_code,omitempty"`
	EvaluationK             int     `json:"evaluation_k"`
	M4PreflightStatus       string  `json:"m4_preflight_status"`
	ProviderBenchmarkStatus string  `json:"provider_benchmark_status"`
	EvaluationStatus        string  `json:"evaluation_status"`
	MetricsValid            bool    `json:"metrics_valid"`
	RecallAt5               float64 `json:"recall_at_5"`
	MRRAt10                 float64 `json:"mrr_at_10"`
	CitationCoverage        float64 `json:"citation_coverage"`
	FailureCount            int     `json:"failure_count"`
	ActivationUnchanged     bool    `json:"activation_unchanged"`
}

type inferrumProviderBenchmarkEnvelope struct {
	Command string `json:"command"`
	Status  string `json:"status"`
	Data    struct {
		Provider       string  `json:"provider"`
		Model          string  `json:"model"`
		SampleCount    int     `json:"sample_count"`
		BatchSize      int     `json:"batch_size"`
		BatchCount     int     `json:"batch_count"`
		WarmupBatches  int     `json:"warmup_batches"`
		BatchAPI       bool    `json:"batch_api"`
		EmbeddingDim   int     `json:"embedding_dim"`
		WarmupMS       int64   `json:"warmup_ms"`
		ElapsedMS      int64   `json:"elapsed_ms"`
		BatchP50MS     int64   `json:"batch_p50_ms"`
		BatchP95MS     int64   `json:"batch_p95_ms"`
		ItemsPerSecond float64 `json:"items_per_second"`
		Scope          string  `json:"scope"`
	} `json:"data"`
}

const (
	m4ProviderBenchmarkSamples = 64
	m4ProviderBenchmarkBatch   = 8
	m4ProviderBenchmarkWarmup  = 1
	m4ProviderBenchmarkTimeout = 6 * time.Minute
	providerBenchmarkSchemaV1  = "pinax.kb.provider-benchmark.v1"
	providerBenchmarkCommand   = "inferrum.provider.benchmark"
	providerBenchmarkScope     = "synthetic_provider_benchmark"
	firstSupportGateSchemaV1   = "pinax.kb.first-support-gate.v1"
	firstSupportEvaluationK    = 5
	firstSupportRecallAt5Gate  = 0.80
	firstSupportMRRAt10Gate    = 0.65
	firstSupportCoverageGate   = 1.00
)

func providerBenchmarkNotMeasuredFacts() []string {
	return []string{"retrieval_quality", "lancedb", "owner_corpus", "memory_pressure"}
}

// activationSnapshot captures only the bounded identity needed to prove that
// the evidence runner did not change the vault's active pointer. It never
// includes a vault path, descriptor path, corpus data, or receipt contents.
type activationSnapshot struct {
	Sequence           uint64
	ActiveGenerationID string
}

func (s activationSnapshot) equal(other activationSnapshot) bool {
	return s.Sequence == other.Sequence && s.ActiveGenerationID == other.ActiveGenerationID
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
	realCorpus := flag.Bool("real-corpus", false, "explicitly evaluate an existing vault and suite without activation")
	m4Preflight := flag.Bool("m4-preflight", false, "require and record target Mac M4 embedded LanceDB compatibility before real-corpus evaluation")
	m4PreflightOnly := flag.Bool("m4-preflight-only", false, "record target Mac M4 embedded LanceDB compatibility without accessing a vault")
	vault := flag.String("vault", "", "existing vault used only with --real-corpus")
	suite := flag.String("suite", "", "versioned evaluation suite used only with --real-corpus")
	pinaxBinary := flag.String("pinax-binary", "", "compiled pinax binary; defaults to a temporary CGO-disabled build")
	inferrumBinary := flag.String("inferrum-binary", "", "compiled inferrum binary required with --m4-preflight")
	runID := flag.String("run-id", "", "evidence run id used by the child")
	flag.Parse()
	opts := canaryOptions{
		Provider:           *provider,
		Model:              *model,
		AllowDiskHighWater: *allowDisk,
		RealCorpus:         *realCorpus,
		M4Preflight:        *m4Preflight,
		M4PreflightOnly:    *m4PreflightOnly,
		Vault:              *vault,
		Suite:              *suite,
		PinaxBinary:        *pinaxBinary,
		InferrumBinary:     *inferrumBinary,
		RunID:              *runID,
	}
	if *child {
		if opts.RealCorpus {
			opts.Vault = firstNonEmpty(opts.Vault, os.Getenv("PINAX_KB_EVIDENCE_VAULT"))
			opts.Suite = firstNonEmpty(opts.Suite, os.Getenv("PINAX_KB_EVIDENCE_SUITE"))
		}
		opts.PinaxBinary = firstNonEmpty(opts.PinaxBinary, os.Getenv("PINAX_KB_EVIDENCE_BINARY"))
		opts.InferrumBinary = firstNonEmpty(opts.InferrumBinary, os.Getenv("PINAX_KB_EVIDENCE_INFERRUM_BINARY"))
		if err := runChild(opts); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "kb canary failed:", safeFailure(err))
			os.Exit(1)
		}
		return
	}
	if err := validateCanaryOptions(opts); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kb canary configuration invalid:", safeFailure(err))
		os.Exit(2)
	}
	restoreInputs, err := applyChildInputEnvironment(opts)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kb canary configuration invalid: input_environment_failed")
		os.Exit(2)
	}
	defer restoreInputs()

	runIDValue := time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	args := []string{"run", "./internal/testkit/kblocalevidence", "--child", "--run-id", runIDValue, "--provider", *provider, "--model", *model}
	if *allowDisk {
		args = append(args, "--allow-disk-high-water")
	}
	if *realCorpus {
		args = append(args, "--real-corpus")
	}
	if *m4Preflight {
		args = append(args, "--m4-preflight")
	}
	if *m4PreflightOnly {
		args = append(args, "--m4-preflight-only")
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
			"synthetic_canary":                 !*realCorpus && !*m4PreflightOnly,
			"real_corpus_candidate_evaluation": *realCorpus,
			"real_ollama_embed":                *provider == "ollama" && !*m4PreflightOnly,
			"inferrum_sidecar_v1":              true,
			"candidate_evaluation":             !*m4PreflightOnly,
			"automatic_activation":             false,
			"answer_mode":                      "not_generated",
			"real_corpus_quality_proven":       false,
			"m4_preflight_requested":           *m4Preflight || *m4PreflightOnly,
			"m4_compatibility_only":            *m4PreflightOnly,
			"m4_provider_benchmark_requested":  *realCorpus && *m4Preflight,
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
	if err := validateCanaryOptions(opts); err != nil {
		return writeCanaryFailure(artifactDir, "input", err)
	}
	if opts.M4PreflightOnly {
		return runM4PreflightOnlyChild(opts, artifactDir)
	}
	if opts.RealCorpus {
		return runRealCorpusChild(opts, artifactDir)
	}
	return runSyntheticCanaryChild(opts, artifactDir)
}

func runM4PreflightOnlyChild(opts canaryOptions, artifactDir string) error {
	return runM4PreflightOnlyChildWithHost(opts, artifactDir, observedHostProfile())
}

func runM4PreflightOnlyChildWithHost(opts canaryOptions, artifactDir string, host hostProfile) error {
	result, err := runM4Preflight(opts, host, "")
	if writeErr := writeArtifact(artifactDir, "m4-preflight.json", result); writeErr != nil {
		return errors.New("artifact_write_failed")
	}
	if err != nil {
		return writeCanaryFailure(artifactDir, "m4_preflight", err)
	}
	return nil
}

func runSyntheticCanaryChild(opts canaryOptions, artifactDir string) error {
	vault, err := os.MkdirTemp("", "pinax-kb-canary-")
	if err != nil {
		return errors.New("canary_vault_failed")
	}
	defer func() { _ = os.RemoveAll(vault) }()
	pinaxPath, binaryMode, cleanupPinax, err := resolvePinaxBinary(opts.PinaxBinary)
	if err != nil {
		return writeCanaryFailure(artifactDir, "binary", err)
	}
	defer cleanupPinax()
	if _, err := runPinaxMeasured(pinaxPath, "init", vault, "--title", "KB Canary", "--json"); err != nil {
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
	rebuildMeasurement, err := runPinaxMeasured(pinaxPath, rebuildArgs...)
	if err != nil {
		return writeCanaryFailure(artifactDir, "rebuild", err)
	}
	rebuild := rebuildMeasurement.Envelope
	generationID, err := nestedString(rebuild, "facts", "generation_id")
	if err != nil {
		return writeCanaryFailure(artifactDir, "rebuild", errors.New("generation_id_missing"))
	}
	evaluationMeasurement, err := runPinaxMeasured(pinaxPath, "kb", "evaluate", "--suite", suiteRef, "--generation", generationID, "--vault", vault, "--json")
	if err != nil {
		return writeCanaryFailure(artifactDir, "evaluate", err)
	}
	evaluation := evaluationMeasurement.Envelope
	runID, err := nestedString(evaluation, "facts", "run_id")
	if err != nil {
		return writeCanaryFailure(artifactDir, "evaluate", errors.New("evaluation_run_id_missing"))
	}
	activationMeasurement, err := runPinaxMeasured(pinaxPath, "kb", "activate", "--generation", generationID, "--suite", suiteRef, "--run-id", runID, "--vault", vault, "--json")
	if err != nil {
		return writeCanaryFailure(artifactDir, "activate", err)
	}
	activation := activationMeasurement.Envelope
	searchMeasurement, err := runPinaxMeasured(pinaxPath, "kb", "search", "Inferrum local retrieval", "--generation", generationID, "--vault", vault, "--provider", opts.Provider, "--model", opts.Model, "--json")
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
		"measurement_scope":           "compiled_pinax_command_process",
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
		"pinax_binary_mode":     binaryMode,
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

func runRealCorpusChild(opts canaryOptions, artifactDir string) error {
	host := observedHostProfile()
	var preflight *m4PreflightArtifact
	var providerBenchmark *providerBenchmarkArtifact
	var pinaxPath, binaryMode string
	if opts.M4Preflight {
		var cleanupPinax func()
		var err error
		pinaxPath, binaryMode, cleanupPinax, err = resolvePinaxBinary(opts.PinaxBinary)
		if err != nil {
			result, _ := failedM4Preflight(host, safeFailure(err))
			result = withM4PreflightScope(result, opts)
			if writeErr := writeArtifact(artifactDir, "m4-preflight.json", result); writeErr != nil {
				return errors.New("artifact_write_failed")
			}
			return writeCanaryFailure(artifactDir, "m4_preflight", err)
		}
		defer cleanupPinax()
		result, err := runM4Preflight(opts, host, pinaxPath)
		if writeErr := writeArtifact(artifactDir, "m4-preflight.json", result); writeErr != nil {
			return errors.New("artifact_write_failed")
		}
		if err != nil {
			return writeCanaryFailure(artifactDir, "m4_preflight", err)
		}
		preflight = &result
		benchmarkResult, err := runInferrumProviderBenchmark(opts.InferrumBinary, opts.Provider, opts.Model)
		if writeErr := writeArtifact(artifactDir, "provider-benchmark.json", benchmarkResult); writeErr != nil {
			return errors.New("artifact_write_failed")
		}
		if err != nil {
			return writeCanaryFailure(artifactDir, "provider_benchmark", err)
		}
		providerBenchmark = &benchmarkResult
	}
	info, err := os.Stat(opts.Vault)
	if err != nil || !info.IsDir() {
		return writeCanaryFailure(artifactDir, "input", errors.New("real_corpus_vault_unavailable"))
	}
	beforeActivation, err := readActivationSnapshot(opts.Vault)
	if err != nil {
		return writeCanaryFailure(artifactDir, "activation_before", err)
	}
	if pinaxPath == "" {
		var cleanupPinax func()
		pinaxPath, binaryMode, cleanupPinax, err = resolvePinaxBinary(opts.PinaxBinary)
		if err != nil {
			return writeCanaryFailure(artifactDir, "binary", err)
		}
		defer cleanupPinax()
	}

	sampler := startOllamaResourceSampler()
	defer sampler.Close()
	doctorMeasurement, err := runPinaxMeasured(pinaxPath, "kb", "provider", "doctor", opts.Provider, "--model", opts.Model, "--vault", opts.Vault, "--json")
	if err != nil {
		return writeCanaryFailure(artifactDir, "provider_doctor", err)
	}
	rebuildArgs := []string{"kb", "rebuild", "--vault", opts.Vault, "--backend", "lancedb", "--provider", opts.Provider, "--model", opts.Model, "--json"}
	if opts.AllowDiskHighWater {
		rebuildArgs = append(rebuildArgs, "--allow-disk-high-water")
	}
	rebuildMeasurement, err := runPinaxMeasured(pinaxPath, rebuildArgs...)
	if err != nil {
		return writeCanaryFailure(artifactDir, "rebuild", err)
	}
	rebuild := rebuildMeasurement.Envelope
	generationID, err := nestedString(rebuild, "facts", "generation_id")
	if err != nil {
		return writeCanaryFailure(artifactDir, "rebuild", errors.New("generation_id_missing"))
	}
	evaluationMeasurement, err := runPinaxMeasured(pinaxPath, realCorpusEvaluationArgs(opts, generationID)...)
	if err != nil {
		return writeCanaryFailure(artifactDir, "evaluate", err)
	}
	afterActivation, err := readActivationSnapshot(opts.Vault)
	if err != nil {
		return writeCanaryFailure(artifactDir, "activation_after", err)
	}
	resources := realCorpusResourceSummary(sampler, doctorMeasurement, rebuildMeasurement, evaluationMeasurement)
	var firstSupportGate *firstSupportGateArtifact
	if opts.M4Preflight {
		gate := evaluateM4FirstSupportGate(preflight, providerBenchmark, evaluationMeasurement.Envelope, beforeActivation, afterActivation)
		firstSupportGate = &gate
	}
	result := realCorpusArtifact(opts, binaryMode, host, preflight, providerBenchmark, firstSupportGate, rebuild, evaluationMeasurement.Envelope, beforeActivation, afterActivation, resources)
	if err := writeArtifact(artifactDir, "real-corpus.json", result); err != nil {
		return errors.New("artifact_write_failed")
	}
	if !beforeActivation.equal(afterActivation) {
		return writeCanaryFailure(artifactDir, "activation_after", errors.New("activation_state_changed"))
	}
	if err := enforceM4FirstSupportGate(artifactDir, firstSupportGate); err != nil {
		return err
	}
	return nil
}

func realCorpusEvaluationArgs(opts canaryOptions, generationID string) []string {
	args := []string{"kb", "evaluate", "--suite", opts.Suite, "--generation", generationID, "--vault", opts.Vault}
	if opts.M4Preflight {
		args = append(args, "--k", strconv.Itoa(firstSupportEvaluationK))
	}
	return append(args, "--json")
}

func enforceM4FirstSupportGate(artifactDir string, gate *firstSupportGateArtifact) error {
	if gate == nil || gate.Status == "passed" {
		return nil
	}
	code := strings.TrimSpace(gate.FailureCode)
	if code == "" {
		code = "first_support_gate_failed"
	}
	return writeCanaryFailure(artifactDir, "first_support_gate", errors.New(code))
}

func realCorpusArtifact(opts canaryOptions, binaryMode string, host hostProfile, preflight *m4PreflightArtifact, providerBenchmark *providerBenchmarkArtifact, firstSupportGate *firstSupportGateArtifact, rebuild, evaluation map[string]any, beforeActivation, afterActivation activationSnapshot, resources map[string]any) map[string]any {
	evaluationStatus := nestedStringOrEmpty(evaluation, "facts", "status")
	activationUnchanged := beforeActivation.equal(afterActivation)
	result := map[string]any{
		"schema_version":              "pinax.kb.real-corpus-evidence.v1",
		"scope":                       "real_corpus_candidate_evaluation",
		"provider":                    opts.Provider,
		"model":                       opts.Model,
		"pinax_binary_mode":           binaryMode,
		"host":                        host,
		"target_m4_status":            host.TargetM4Status,
		"target_m4_air_status":        host.TargetM4AirStatus,
		"generation_id":               nestedStringOrEmpty(rebuild, "facts", "generation_id"),
		"documents":                   nestedStringOrEmpty(rebuild, "facts", "documents"),
		"chunks":                      nestedStringOrEmpty(rebuild, "facts", "chunks"),
		"embedding_dim":               nestedStringOrEmpty(rebuild, "facts", "embedding_dim"),
		"protocol":                    nestedStringOrEmpty(rebuild, "facts", "protocol"),
		"model_manifest_digest":       nestedStringOrEmpty(rebuild, "facts", "model_manifest_digest"),
		"profile_hash":                nestedStringOrEmpty(rebuild, "facts", "profile_hash"),
		"base_model_digest":           nestedStringOrEmpty(rebuild, "facts", "base_model_digest"),
		"daemon_version":              nestedStringOrEmpty(rebuild, "facts", "daemon_version"),
		"evaluation_run_id":           nestedStringOrEmpty(evaluation, "facts", "run_id"),
		"evaluation_status":           evaluationStatus,
		"suite_id":                    nestedStringOrEmpty(evaluation, "facts", "suite_id"),
		"suite_version":               nestedStringOrEmpty(evaluation, "facts", "suite_version"),
		"recall_at_5":                 nestedStringOrEmpty(evaluation, "facts", "recall_at_5"),
		"mrr_at_10":                   nestedStringOrEmpty(evaluation, "facts", "mrr_at_10"),
		"citation_coverage":           nestedStringOrEmpty(evaluation, "facts", "citation_coverage"),
		"failure_count":               nestedStringOrEmpty(evaluation, "facts", "failure_count"),
		"quality_verdict":             realCorpusQualityVerdict(evaluationStatus, activationUnchanged),
		"activation_status":           "not_attempted",
		"active_generation_before":    beforeActivation.ActiveGenerationID,
		"active_generation_after":     afterActivation.ActiveGenerationID,
		"active_sequence_before":      beforeActivation.Sequence,
		"active_sequence_after":       afterActivation.Sequence,
		"active_generation_unchanged": activationUnchanged,
		"corpus_content_in_evidence":  false,
		"queries_in_evidence":         false,
		"raw_vectors_in_evidence":     false,
		"resources":                   resources,
	}
	if preflight != nil {
		result["m4_preflight"] = preflight
		if preflight.PinaxBinaryArchitecture != "" {
			result["pinax_binary_architecture"] = preflight.PinaxBinaryArchitecture
		}
	}
	if providerBenchmark != nil {
		result["provider_benchmark"] = providerBenchmark
	}
	if firstSupportGate != nil {
		result["first_support_gate"] = firstSupportGate
	}
	return result
}

func realCorpusQualityVerdict(status string, activationUnchanged bool) string {
	if !activationUnchanged {
		return "activation_state_changed"
	}
	if status == "passed" {
		return "candidate_gate_passed_not_activated"
	}
	return "candidate_gate_not_passed"
}

func evaluateM4FirstSupportGate(preflight *m4PreflightArtifact, benchmark *providerBenchmarkArtifact, evaluation map[string]any, beforeActivation, afterActivation activationSnapshot) firstSupportGateArtifact {
	result := firstSupportGateArtifact{
		SchemaVersion:           firstSupportGateSchemaV1,
		Status:                  "failed",
		EvaluationK:             firstSupportEvaluationK,
		ActivationUnchanged:     beforeActivation.equal(afterActivation),
		EvaluationStatus:        nestedStringOrEmpty(evaluation, "facts", "status"),
		M4PreflightStatus:       firstSupportArtifactStatus(preflight),
		ProviderBenchmarkStatus: firstSupportBenchmarkStatus(benchmark),
	}
	if result.M4PreflightStatus != "passed" {
		result.FailureCode = "m4_preflight_not_passed"
		return result
	}
	if result.ProviderBenchmarkStatus != "passed" {
		result.FailureCode = "provider_benchmark_not_passed"
		return result
	}
	if !result.ActivationUnchanged {
		result.FailureCode = "activation_state_changed"
		return result
	}
	if result.EvaluationStatus != "passed" {
		result.FailureCode = "evaluation_status_not_passed"
		return result
	}
	recall, mrr, coverage, failureCount, ok := parseFirstSupportMetrics(evaluation)
	if !ok {
		result.FailureCode = "first_support_metrics_invalid"
		return result
	}
	result.MetricsValid = true
	result.RecallAt5 = recall
	result.MRRAt10 = mrr
	result.CitationCoverage = coverage
	result.FailureCount = failureCount
	if recall < firstSupportRecallAt5Gate {
		result.FailureCode = "recall_at_5_below_gate"
		return result
	}
	if mrr < firstSupportMRRAt10Gate {
		result.FailureCode = "mrr_at_10_below_gate"
		return result
	}
	if coverage != firstSupportCoverageGate {
		result.FailureCode = "citation_coverage_incomplete"
		return result
	}
	if failureCount != 0 {
		result.FailureCode = "evaluation_failures_present"
		return result
	}
	result.Status = "passed"
	return result
}

func firstSupportArtifactStatus(preflight *m4PreflightArtifact) string {
	if preflight == nil {
		return "unavailable"
	}
	return strings.TrimSpace(preflight.Status)
}

func firstSupportBenchmarkStatus(benchmark *providerBenchmarkArtifact) string {
	if benchmark == nil {
		return "unavailable"
	}
	return strings.TrimSpace(benchmark.Status)
}

func parseFirstSupportMetrics(evaluation map[string]any) (float64, float64, float64, int, bool) {
	recall, recallOK := parseFirstSupportUnitInterval(nestedStringOrEmpty(evaluation, "facts", "recall_at_5"))
	mrr, mrrOK := parseFirstSupportUnitInterval(nestedStringOrEmpty(evaluation, "facts", "mrr_at_10"))
	coverage, coverageOK := parseFirstSupportUnitInterval(nestedStringOrEmpty(evaluation, "facts", "citation_coverage"))
	failureCount, failureCountOK := parseFirstSupportFailureCount(nestedStringOrEmpty(evaluation, "facts", "failure_count"))
	return recall, mrr, coverage, failureCount, recallOK && mrrOK && coverageOK && failureCountOK
}

func parseFirstSupportUnitInterval(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return 0, false
	}
	return value, true
}

func parseFirstSupportFailureCount(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func readActivationSnapshot(vault string) (activationSnapshot, error) {
	descriptor, err := app.ReadKBActivationDescriptor(vault)
	if err != nil {
		return activationSnapshot{}, errors.New("activation_state_unavailable")
	}
	snapshot := activationSnapshot{Sequence: descriptor.Sequence}
	if descriptor.Active != nil {
		snapshot.ActiveGenerationID = descriptor.Active.GenerationID
	}
	return snapshot, nil
}

func realCorpusResourceSummary(sampler *ollamaResourceSampler, doctor, rebuild, evaluation pinaxCommandMeasurement) map[string]any {
	resourceStats := sampler.Close()
	afterOllama, afterOllamaErr := fetchOllamaProcessSnapshot()
	resourceObservation := "observed"
	if afterOllamaErr != nil {
		resourceObservation = "unavailable"
	}
	var maxRSSBytes, cpuUserMS, cpuSystemMS int64
	for _, measurement := range []pinaxCommandMeasurement{doctor, rebuild, evaluation} {
		maxRSSBytes = maxInt64(maxRSSBytes, measurement.MaxRSSBytes)
		cpuUserMS += measurement.CPUUserMS
		cpuSystemMS += measurement.CPUSystemMS
	}
	resources := map[string]any{
		"measurement_scope":       "compiled_pinax_command_process",
		"max_rss_bytes":           maxRSSBytes,
		"cpu_user_ms":             cpuUserMS,
		"cpu_system_ms":           cpuSystemMS,
		"ollama_ps_observation":   resourceObservation,
		"ollama_ps_samples":       resourceStats.Samples,
		"ollama_peak_model_bytes": resourceStats.PeakModelBytes,
		"ollama_peak_vram_bytes":  resourceStats.PeakVRAMBytes,
		"ollama_unload_status":    ollamaUnloadStatus(afterOllama),
		"ollama_models_after":     afterOllama.ModelCount,
		"system_memory_pressure":  "not_measured_by_runner",
	}
	for field, value := range realCorpusPerformanceFacts(doctor, rebuild, evaluation) {
		resources[field] = value
	}
	return resources
}

// realCorpusPerformanceFacts keeps command-level timing separate from quality
// metrics. The names deliberately avoid unproven warm/cold claims: the target
// machine, provider daemon, and model residency determine that state.
func realCorpusPerformanceFacts(doctor, rebuild, evaluation pinaxCommandMeasurement) map[string]any {
	return map[string]any{
		"provider_doctor_duration_ms":      doctor.DurationMS,
		"candidate_rebuild_duration_ms":    rebuild.DurationMS,
		"candidate_evaluation_duration_ms": evaluation.DurationMS,
	}
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

func validateCanaryOptions(opts canaryOptions) error {
	if !safeProviderOrModel(opts.Provider) || !safeProviderOrModel(opts.Model) {
		return errors.New("model_invalid")
	}
	if opts.M4Preflight && opts.M4PreflightOnly {
		return errors.New("m4_preflight_mode_conflict")
	}
	if opts.M4PreflightOnly {
		if opts.RealCorpus || strings.TrimSpace(opts.Vault) != "" || strings.TrimSpace(opts.Suite) != "" {
			return errors.New("m4_preflight_only_no_corpus_inputs")
		}
		if strings.TrimSpace(opts.InferrumBinary) == "" || strings.ContainsAny(opts.InferrumBinary, "\r\n") {
			return errors.New("m4_preflight_inferrum_binary_required")
		}
		return nil
	}
	if opts.M4Preflight && !opts.RealCorpus {
		return errors.New("m4_preflight_real_corpus_required")
	}
	if !opts.RealCorpus {
		if strings.TrimSpace(opts.Vault) != "" || strings.TrimSpace(opts.Suite) != "" {
			return errors.New("real_corpus_confirmation_required")
		}
		return nil
	}
	if strings.TrimSpace(opts.Vault) == "" || strings.TrimSpace(opts.Suite) == "" || strings.ContainsAny(opts.Vault, "\r\n") || strings.ContainsAny(opts.Suite, "\r\n") {
		return errors.New("real_corpus_inputs_required")
	}
	if !strings.EqualFold(strings.TrimSpace(opts.Provider), "ollama") {
		return errors.New("real_corpus_ollama_required")
	}
	if opts.M4Preflight && (strings.TrimSpace(opts.InferrumBinary) == "" || strings.ContainsAny(opts.InferrumBinary, "\r\n")) {
		return errors.New("m4_preflight_inferrum_binary_required")
	}
	return nil
}

func safeProviderOrModel(value string) bool {
	if strings.TrimSpace(value) == "" || len(value) > 160 {
		return false
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			continue
		}
		switch character {
		case '.', '-', '_', ':', '/':
			continue
		default:
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func applyChildInputEnvironment(opts canaryOptions) (func(), error) {
	values := map[string]string{"PINAX_KB_EVIDENCE_BINARY": opts.PinaxBinary}
	if opts.RealCorpus {
		values["PINAX_KB_EVIDENCE_VAULT"] = opts.Vault
		values["PINAX_KB_EVIDENCE_SUITE"] = opts.Suite
	}
	if opts.M4Preflight || opts.M4PreflightOnly {
		values["PINAX_KB_EVIDENCE_INFERRUM_BINARY"] = opts.InferrumBinary
	}
	type priorValue struct {
		value string
		set   bool
	}
	prior := make(map[string]priorValue, len(values))
	for key, value := range values {
		previous, exists := os.LookupEnv(key)
		prior[key] = priorValue{value: previous, set: exists}
		if err := os.Setenv(key, value); err != nil {
			for restoreKey, restoreValue := range prior {
				if restoreValue.set {
					_ = os.Setenv(restoreKey, restoreValue.value)
				} else {
					_ = os.Unsetenv(restoreKey)
				}
			}
			return nil, err
		}
	}
	return func() {
		for key, previous := range prior {
			if previous.set {
				_ = os.Setenv(key, previous.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}, nil
}

func resolvePinaxBinary(raw string) (string, string, func(), error) {
	if configured := strings.TrimSpace(raw); configured != "" {
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", "", nil, errors.New("pinax_binary_unavailable")
		}
		return configured, "provided", func() {}, nil
	}
	dir, err := os.MkdirTemp("", "pinax-kb-evidence-bin-")
	if err != nil {
		return "", "", nil, errors.New("pinax_binary_build_unavailable")
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "pinax")
	command := exec.Command("go", "build", "-trimpath", "-o", path, "./cmd/pinax")
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if err := command.Run(); err != nil {
		cleanup()
		return "", "", nil, errors.New("pinax_binary_build_failed")
	}
	return path, "compiled_temp", cleanup, nil
}

func runM4Preflight(opts canaryOptions, host hostProfile, pinaxBinary string) (m4PreflightArtifact, error) {
	if host.TargetM4Status != "confirmed" {
		artifact, err := failedM4Preflight(host, "target_m4_unconfirmed")
		return withM4PreflightScope(artifact, opts), err
	}
	if host.TargetM4AirStatus != "confirmed" {
		artifact, err := failedM4Preflight(host, "target_m4_air_unconfirmed")
		return withM4PreflightScope(artifact, opts), err
	}
	macOSVersion, err := localVersion("sw_vers", "-productVersion")
	if err != nil {
		artifact, failure := failedM4Preflight(host, "macos_version_unavailable")
		return withM4PreflightScope(artifact, opts), failure
	}
	sidecar := strings.TrimSpace(os.Getenv("PINAX_KB_SIDECAR"))
	if sidecar == "" {
		artifact, failure := failedM4Preflight(host, "sidecar_unavailable")
		return withM4PreflightScope(artifact, opts), failure
	}
	pythonVersion, pythonArchitecture, err := sidecarPythonFacts(sidecar)
	if err != nil {
		artifact, failure := failedM4Preflight(host, safeFailure(err))
		return withM4PreflightScope(artifact, opts), failure
	}
	if safeArchitectureFact(pythonArchitecture) != "arm64" {
		artifact, failure := failedM4PreflightWithArchitectures(host, "sidecar_python_arch_unsupported", pythonArchitecture, "", "")
		return withM4PreflightScope(artifact, opts), failure
	}
	inferrumArchitecture, err := inferrumBinaryArchitecture(opts.InferrumBinary)
	if err != nil {
		artifact, failure := failedM4PreflightWithArchitectures(host, safeFailure(err), pythonArchitecture, inferrumArchitecture, "")
		return withM4PreflightScope(artifact, opts), failure
	}
	pinaxArchitecture := ""
	if opts.RealCorpus {
		pinaxArchitecture, err = pinaxBinaryArchitecture(pinaxBinary)
		if err != nil {
			artifact, failure := failedM4PreflightWithArchitectures(host, safeFailure(err), pythonArchitecture, inferrumArchitecture, pinaxArchitecture)
			return withM4PreflightScope(artifact, opts), failure
		}
	}
	validation, err := runInferrumEmbeddedValidation(opts.InferrumBinary, sidecar)
	if err != nil {
		artifact, failure := failedM4PreflightWithArchitectures(host, safeFailure(err), pythonArchitecture, inferrumArchitecture, pinaxArchitecture)
		return withM4PreflightScope(artifact, opts), failure
	}
	artifact, evaluationErr := evaluateM4Preflight(host, macOSVersion, pythonVersion, pythonArchitecture, inferrumArchitecture, pinaxArchitecture, opts.RealCorpus, validation)
	return withM4PreflightScope(artifact, opts), evaluationErr
}

func withM4PreflightScope(artifact m4PreflightArtifact, opts canaryOptions) m4PreflightArtifact {
	if opts.M4PreflightOnly {
		artifact.Scope = "compatibility_only"
		return artifact
	}
	artifact.Scope = "real_corpus_preflight"
	return artifact
}

func failedM4Preflight(host hostProfile, code string) (m4PreflightArtifact, error) {
	return m4PreflightArtifact{
		SchemaVersion:     "pinax.kb.m4-preflight.v1",
		Status:            "failed",
		FailureCode:       code,
		TargetM4Status:    host.TargetM4Status,
		TargetM4AirStatus: host.TargetM4AirStatus,
	}, errors.New(code)
}

func failedM4PreflightWithArchitectures(host hostProfile, code, pythonArchitecture, inferrumArchitecture, pinaxArchitecture string) (m4PreflightArtifact, error) {
	artifact, err := failedM4Preflight(host, code)
	artifact.SidecarPythonArchitecture = safeArchitectureFact(pythonArchitecture)
	artifact.InferrumBinaryArchitecture = safeArchitectureFact(inferrumArchitecture)
	artifact.PinaxBinaryArchitecture = safeArchitectureFact(pinaxArchitecture)
	return artifact, err
}

func evaluateM4Preflight(host hostProfile, macOSVersion, pythonVersion, pythonArchitecture, inferrumArchitecture, pinaxArchitecture string, requirePinaxBinary bool, validation inferrumEmbeddedValidation) (m4PreflightArtifact, error) {
	artifact := m4PreflightArtifact{
		SchemaVersion:     "pinax.kb.m4-preflight.v1",
		Status:            "failed",
		TargetM4Status:    host.TargetM4Status,
		TargetM4AirStatus: host.TargetM4AirStatus,
	}
	if host.TargetM4Status != "confirmed" {
		artifact.FailureCode = "target_m4_unconfirmed"
		return artifact, errors.New(artifact.FailureCode)
	}
	if host.TargetM4AirStatus != "confirmed" {
		artifact.FailureCode = "target_m4_air_unconfirmed"
		return artifact, errors.New(artifact.FailureCode)
	}
	macMajor, macMinor, ok := parseMajorMinor(macOSVersion)
	if !ok {
		artifact.FailureCode = "macos_version_unavailable"
		return artifact, errors.New(artifact.FailureCode)
	}
	artifact.MacOSVersion = fmt.Sprintf("%d.%d", macMajor, macMinor)
	if !versionAtLeast(macMajor, macMinor, 14, 0) {
		artifact.FailureCode = "macos_version_unsupported"
		return artifact, errors.New(artifact.FailureCode)
	}
	pythonMajor, pythonMinor, ok := parseMajorMinor(pythonVersion)
	if !ok {
		artifact.FailureCode = "python_version_unavailable"
		return artifact, errors.New(artifact.FailureCode)
	}
	artifact.PythonVersion = fmt.Sprintf("%d.%d", pythonMajor, pythonMinor)
	if !versionAtLeast(pythonMajor, pythonMinor, 3, 10) {
		artifact.FailureCode = "python_version_unsupported"
		return artifact, errors.New(artifact.FailureCode)
	}
	artifact.SidecarPythonArchitecture = safeArchitectureFact(pythonArchitecture)
	if artifact.SidecarPythonArchitecture != "arm64" {
		artifact.FailureCode = "sidecar_python_arch_unsupported"
		return artifact, errors.New(artifact.FailureCode)
	}
	artifact.InferrumBinaryArchitecture = safeArchitectureFact(inferrumArchitecture)
	if artifact.InferrumBinaryArchitecture != "arm64" {
		artifact.FailureCode = "inferrum_binary_arch_unsupported"
		return artifact, errors.New(artifact.FailureCode)
	}
	if requirePinaxBinary {
		artifact.PinaxBinaryArchitecture = safeArchitectureFact(pinaxArchitecture)
		if artifact.PinaxBinaryArchitecture != "arm64" {
			artifact.FailureCode = "pinax_binary_arch_unsupported"
			return artifact, errors.New(artifact.FailureCode)
		}
	}
	if !embeddedValidationContractValid(validation) {
		artifact.FailureCode = "embedded_validation_contract_invalid"
		return artifact, errors.New(artifact.FailureCode)
	}
	artifact.Status = "passed"
	artifact.EmbeddedBackend = validation.Data.Backend
	artifact.EmbeddedDependency = validation.Data.Dependency
	artifact.EmbeddedEmbeddingDim = validation.Data.EmbeddingDim
	artifact.EmbeddedRows = validation.Data.Rows
	artifact.EmbeddedHits = validation.Data.Hits
	artifact.EmbeddedStoreState = validation.Data.StoreState
	artifact.EmbeddedStoreURIScope = validation.Data.StoreURIScope
	artifact.EmbeddedTable = validation.Data.Table
	return artifact, nil
}

func localVersion(command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, command, args...).Output()
	if ctx.Err() != nil {
		return "", errors.New("version_probe_timeout")
	}
	if err != nil {
		return "", errors.New("version_probe_failed")
	}
	major, minor, ok := parseMajorMinor(string(output))
	if !ok {
		return "", errors.New("version_probe_invalid")
	}
	return fmt.Sprintf("%d.%d", major, minor), nil
}

func sidecarPythonFacts(sidecar string) (string, string, error) {
	file, err := os.Open(sidecar)
	if err != nil {
		return "", "", errors.New("sidecar_unavailable")
	}
	defer func() { _ = file.Close() }()
	line, _ := bufio.NewReader(io.LimitReader(file, 512)).ReadString('\n')
	interpreter := pythonExecutableFromShebang(line)
	version, err := localVersion(interpreter, "--version")
	if err != nil {
		return "", "", errors.New("python_version_unavailable")
	}
	architecture, err := pythonArchitecture(interpreter)
	if err != nil {
		return "", "", err
	}
	return version, architecture, nil
}

func pythonArchitecture(interpreter string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, interpreter, "-c", "import platform; print(platform.machine())").Output()
	if ctx.Err() != nil {
		return "", errors.New("sidecar_python_arch_unavailable")
	}
	if err != nil {
		return "", errors.New("sidecar_python_arch_unavailable")
	}
	architecture := safeArchitectureFact(string(output))
	if architecture == "" {
		return "", errors.New("sidecar_python_arch_unavailable")
	}
	return architecture, nil
}

func inferrumBinaryArchitecture(path string) (string, error) {
	return machOBinaryArchitecture(path, "inferrum_binary_arch_unavailable", "inferrum_binary_arch_unsupported")
}

func pinaxBinaryArchitecture(path string) (string, error) {
	return machOBinaryArchitecture(path, "pinax_binary_arch_unavailable", "pinax_binary_arch_unsupported")
}

func machOBinaryArchitecture(path, unavailableCode, unsupportedCode string) (string, error) {
	architectures, err := machOArchitectures(path)
	if err != nil {
		return "", errors.New(unavailableCode)
	}
	for _, architecture := range architectures {
		if architecture == macho.CpuArm64 {
			return "arm64", nil
		}
	}
	for _, architecture := range architectures {
		if architecture == macho.CpuAmd64 {
			return "amd64", errors.New(unsupportedCode)
		}
	}
	return "unknown", errors.New(unsupportedCode)
}

func machOArchitectures(path string) ([]macho.Cpu, error) {
	if fat, err := macho.OpenFat(path); err == nil {
		defer func() { _ = fat.Close() }()
		architectures := make([]macho.Cpu, 0, len(fat.Arches))
		for _, architecture := range fat.Arches {
			architectures = append(architectures, architecture.Cpu)
		}
		return architectures, nil
	}
	thin, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = thin.Close() }()
	return []macho.Cpu{thin.Cpu}, nil
}

func safeArchitectureFact(value string) string {
	return strings.ToLower(sanitizeHostFact(value))
}

func pythonExecutableFromShebang(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#!"))
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "python3"
	}
	if filepath.Base(fields[0]) == "env" {
		for _, field := range fields[1:] {
			if strings.HasPrefix(filepath.Base(field), "python") {
				return field
			}
		}
		return "python3"
	}
	if strings.HasPrefix(filepath.Base(fields[0]), "python") {
		return fields[0]
	}
	return "python3"
}

func parseMajorMinor(raw string) (int, int, bool) {
	for _, field := range strings.Fields(raw) {
		field = strings.TrimLeft(field, "vV")
		parts := strings.Split(field, ".")
		if len(parts) < 2 {
			continue
		}
		major, majorErr := strconv.Atoi(parts[0])
		minor, minorErr := strconv.Atoi(parts[1])
		if majorErr == nil && minorErr == nil && major >= 0 && minor >= 0 {
			return major, minor, true
		}
	}
	return 0, 0, false
}

func versionAtLeast(major, minor, minimumMajor, minimumMinor int) bool {
	return major > minimumMajor || (major == minimumMajor && minor >= minimumMinor)
}

func runInferrumEmbeddedValidation(inferrumBinary, sidecar string) (inferrumEmbeddedValidation, error) {
	info, err := os.Stat(inferrumBinary)
	if err != nil || info.IsDir() {
		return inferrumEmbeddedValidation{}, errors.New("inferrum_binary_unavailable")
	}
	info, err = os.Stat(sidecar)
	if strings.TrimSpace(sidecar) == "" || err != nil || info.IsDir() {
		return inferrumEmbeddedValidation{}, errors.New("sidecar_unavailable")
	}
	root, err := os.MkdirTemp("", "pinax-kb-m4-preflight-")
	if err != nil {
		return inferrumEmbeddedValidation{}, errors.New("embedded_validation_root_unavailable")
	}
	defer func() { _ = os.RemoveAll(root) }()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, inferrumBinary, "validate", "embedded", "--root", root, "--sidecar", sidecar, "--json").Output()
	if ctx.Err() != nil {
		return inferrumEmbeddedValidation{}, errors.New("embedded_validation_timeout")
	}
	if err != nil {
		return inferrumEmbeddedValidation{}, errors.New("embedded_validation_failed")
	}
	return parseInferrumEmbeddedValidation(output)
}

func runInferrumProviderBenchmark(inferrumBinary, provider, model string) (providerBenchmarkArtifact, error) {
	info, err := os.Stat(inferrumBinary)
	if err != nil || info.IsDir() {
		artifact := failedProviderBenchmark("provider_benchmark_binary_unavailable")
		return artifact, errors.New(artifact.FailureCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), m4ProviderBenchmarkTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, inferrumBinary,
		"provider", "benchmark", provider,
		"--model", model,
		"--samples", strconv.Itoa(m4ProviderBenchmarkSamples),
		"--batch-size", strconv.Itoa(m4ProviderBenchmarkBatch),
		"--warmup", strconv.Itoa(m4ProviderBenchmarkWarmup),
		"--json",
	).Output()
	if ctx.Err() != nil {
		artifact := failedProviderBenchmark("provider_benchmark_timeout")
		return artifact, errors.New(artifact.FailureCode)
	}
	if err != nil {
		artifact := failedProviderBenchmark("provider_benchmark_failed")
		return artifact, errors.New(artifact.FailureCode)
	}
	return parseInferrumProviderBenchmark(output, provider, model)
}

func parseInferrumProviderBenchmark(output []byte, provider, model string) (providerBenchmarkArtifact, error) {
	var envelope inferrumProviderBenchmarkEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		artifact := failedProviderBenchmark("provider_benchmark_output_invalid")
		return artifact, errors.New(artifact.FailureCode)
	}
	data := envelope.Data
	if envelope.Command != providerBenchmarkCommand ||
		envelope.Status != "success" ||
		data.Provider != provider ||
		data.Model != model ||
		data.SampleCount != m4ProviderBenchmarkSamples ||
		data.BatchSize != m4ProviderBenchmarkBatch ||
		data.BatchCount != m4ProviderBenchmarkSamples/m4ProviderBenchmarkBatch ||
		data.WarmupBatches != m4ProviderBenchmarkWarmup ||
		data.EmbeddingDim <= 0 ||
		data.WarmupMS < 0 ||
		data.ElapsedMS < 0 ||
		data.BatchP50MS < 0 ||
		data.BatchP95MS < data.BatchP50MS ||
		data.ItemsPerSecond <= 0 ||
		math.IsNaN(data.ItemsPerSecond) ||
		math.IsInf(data.ItemsPerSecond, 0) ||
		data.Scope != providerBenchmarkScope {
		artifact := failedProviderBenchmark("provider_benchmark_output_invalid")
		return artifact, errors.New(artifact.FailureCode)
	}
	return providerBenchmarkArtifact{
		SchemaVersion:  providerBenchmarkSchemaV1,
		Status:         "passed",
		Command:        providerBenchmarkCommand,
		Provider:       data.Provider,
		Model:          data.Model,
		SampleCount:    data.SampleCount,
		BatchSize:      data.BatchSize,
		BatchCount:     data.BatchCount,
		WarmupBatches:  data.WarmupBatches,
		BatchAPI:       data.BatchAPI,
		EmbeddingDim:   data.EmbeddingDim,
		WarmupMS:       data.WarmupMS,
		ElapsedMS:      data.ElapsedMS,
		BatchP50MS:     data.BatchP50MS,
		BatchP95MS:     data.BatchP95MS,
		ItemsPerSecond: data.ItemsPerSecond,
		Scope:          providerBenchmarkScope,
		NotMeasured:    providerBenchmarkNotMeasuredFacts(),
	}, nil
}

func failedProviderBenchmark(code string) providerBenchmarkArtifact {
	return providerBenchmarkArtifact{
		SchemaVersion: providerBenchmarkSchemaV1,
		Status:        "failed",
		FailureCode:   code,
	}
}

func parseInferrumEmbeddedValidation(output []byte) (inferrumEmbeddedValidation, error) {
	var validation inferrumEmbeddedValidation
	if err := json.Unmarshal(output, &validation); err != nil {
		return inferrumEmbeddedValidation{}, errors.New("embedded_validation_output_invalid")
	}
	if validation.Command != "inferrum.validate.embedded" || strings.TrimSpace(validation.Status) == "" {
		return inferrumEmbeddedValidation{}, errors.New("embedded_validation_output_invalid")
	}
	return validation, nil
}

func embeddedValidationContractValid(validation inferrumEmbeddedValidation) bool {
	return validation.Command == "inferrum.validate.embedded" &&
		validation.Status == "success" &&
		validation.Data.Backend == "lancedb" &&
		validation.Data.Dependency == "lancedb" &&
		validation.Data.EmbeddingDim > 0 &&
		validation.Data.Rows > 0 &&
		validation.Data.Hits > 0 &&
		validation.Data.StoreState == "available" &&
		validation.Data.StoreURIScope == "relative_to_root" &&
		strings.TrimSpace(validation.Data.Table) != ""
}

func observedHostProfile() hostProfile {
	brand := ""
	machineModel := ""
	memoryBytes := int64(0)
	if runtime.GOOS == "darwin" {
		brand = sysctlFact("machdep.cpu.brand_string")
		machineModel = sysctlFact("hw.model")
		if rawMemory := sysctlFact("hw.memsize"); rawMemory != "" {
			memoryBytes, _ = strconv.ParseInt(rawMemory, 10, 64)
		}
	}
	return hostProfileFromFacts(runtime.GOOS, runtime.GOARCH, brand, machineModel, runtime.NumCPU(), memoryBytes)
}

func hostProfileFromFacts(goos, arch, cpuBrand, machineModel string, cpuCores int, memoryBytes int64) hostProfile {
	profile := hostProfile{
		OS:                goos,
		Arch:              arch,
		CPUCores:          cpuCores,
		CPUBrand:          sanitizeHostFact(cpuBrand),
		MemoryBytes:       maxInt64(memoryBytes, 0),
		TargetM4Status:    "not_macos",
		TargetM4AirStatus: "not_macos",
		MachineModel:      sanitizeHostFact(machineModel),
	}
	if goos == "darwin" {
		profile.TargetM4Status = "unconfirmed"
		profile.TargetM4AirStatus = "unconfirmed"
		if arch == "arm64" && strings.Contains(strings.ToLower(profile.CPUBrand), "apple m4") {
			profile.TargetM4Status = "confirmed"
			if supportedM4AirModel(profile.MachineModel) {
				profile.TargetM4AirStatus = "confirmed"
			}
		}
	}
	return profile
}

func supportedM4AirModel(machineModel string) bool {
	switch strings.TrimSpace(machineModel) {
	case "Mac16,12", "Mac16,13":
		return true
	default:
		return false
	}
}

func sysctlFact(key string) string {
	output, err := exec.Command("sysctl", "-n", key).Output()
	if err != nil {
		return ""
	}
	return sanitizeHostFact(string(output))
}

func sanitizeHostFact(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 96 {
		value = value[:96]
	}
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == ' ' || character == '-' || character == '.' || character == '_' || character == ',' {
			builder.WriteRune(character)
		}
	}
	return strings.TrimSpace(builder.String())
}

func runPinaxMeasured(executable string, args ...string) (pinaxCommandMeasurement, error) {
	cmd := exec.Command(executable, args...)
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
	maxRSSBytes = maxRSSBytesForOS(runtime.GOOS, int64(usage.Maxrss))
	cpuUserMS = int64(usage.Utime.Sec)*1000 + int64(usage.Utime.Usec)/1000
	cpuSystemMS = int64(usage.Stime.Sec)*1000 + int64(usage.Stime.Usec)/1000
	return maxRSSBytes, cpuUserMS, cpuSystemMS
}

func maxRSSBytesForOS(goos string, maxrss int64) int64 {
	if maxrss <= 0 {
		return 0
	}
	if goos == "darwin" {
		return maxrss
	}
	return maxrss * 1024
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
		if current == nil {
			return ""
		}
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
