package main

import (
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
)

func TestWriteCanarySuiteUsesTwentyBoundedQuestions(t *testing.T) {
	vault := t.TempDir()
	if err := writeCanarySuite(vault); err != nil {
		t.Fatalf("write canary suite: %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(vault, ".pinax", "kb", "evaluation-suites", "local-canary.json"))
	if err != nil {
		t.Fatalf("read canary suite: %v", err)
	}
	var suite map[string]any
	if err := json.Unmarshal(payload, &suite); err != nil {
		t.Fatalf("suite json: %v", err)
	}
	questions, ok := suite["questions"].([]any)
	if !ok || len(questions) != 20 {
		t.Fatalf("questions = %#v", suite["questions"])
	}
}

func TestWriteCanaryFailureOnlyPersistsSafeStageAndCode(t *testing.T) {
	dir := t.TempDir()
	err := writeCanaryFailure(dir, "rebuild", os.ErrNotExist)
	if err == nil {
		t.Fatalf("writeCanaryFailure should return the terminal failure")
	}
	payload, readErr := os.ReadFile(filepath.Join(dir, "failure.json"))
	if readErr != nil {
		t.Fatalf("read failure artifact: %v", readErr)
	}
	text := string(payload)
	if !strings.Contains(text, `"stage": "rebuild"`) || !strings.Contains(text, `"status": "failed"`) || strings.Contains(text, "/") {
		t.Fatalf("failure artifact is not bounded: %s", text)
	}
}

func TestParseOllamaProcessSnapshotKeepsOnlyResourceFacts(t *testing.T) {
	payload := []byte(`{"models":[{"name":"pinax-qwen3-embedding:lowmem","size":123456,"size_vram":654321},{"name":"other","size":100,"size_vram":200}]}`)
	stats, err := parseOllamaProcessSnapshot(payload)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	if stats.ModelCount != 2 || stats.TotalModelBytes != 123556 || stats.TotalVRAMBytes != 654521 {
		t.Fatalf("resource stats = %#v", stats)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal resource stats: %v", err)
	}
	if strings.Contains(string(encoded), "pinax-qwen3") {
		t.Fatalf("resource stats retained a model name: %s", encoded)
	}
}

func TestOllamaUnloadStatusIsTruthful(t *testing.T) {
	if got := ollamaUnloadStatus(ollamaProcessStats{}); got != "unloaded" {
		t.Fatalf("empty process status = %q, want unloaded", got)
	}
	if got := ollamaUnloadStatus(ollamaProcessStats{ModelCount: 1}); got != "still_loaded" {
		t.Fatalf("loaded process status = %q, want still_loaded", got)
	}
}

func TestResourceHelpersDoNotInventUnavailableProcessFacts(t *testing.T) {
	maxRSS, user, system := processResourceUsage(nil)
	if maxRSS != 0 || user != 0 || system != 0 {
		t.Fatalf("nil process usage = %d/%d/%d, want zeros", maxRSS, user, system)
	}
	if got := maxInt64(3, 9, 4); got != 9 {
		t.Fatalf("maxInt64 = %d, want 9", got)
	}
}

func TestRealCorpusPerformanceFactsExposeComparableCommandDurations(t *testing.T) {
	facts := realCorpusPerformanceFacts(
		pinaxCommandMeasurement{DurationMS: 11},
		pinaxCommandMeasurement{DurationMS: 22},
		pinaxCommandMeasurement{DurationMS: 33},
	)
	if facts["provider_doctor_duration_ms"] != int64(11) || facts["candidate_rebuild_duration_ms"] != int64(22) || facts["candidate_evaluation_duration_ms"] != int64(33) {
		t.Fatalf("performance facts = %#v", facts)
	}
	for _, forbidden := range []string{"vault", "suite", "path", "query", "content"} {
		for key := range facts {
			if strings.Contains(strings.ToLower(key), forbidden) {
				t.Fatalf("performance facts key leaked %q: %#v", forbidden, facts)
			}
		}
	}
}

func TestRealCorpusResourceSummaryIncludesComparableCommandDurations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/ps" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	sampler := &ollamaResourceSampler{done: make(chan struct{})}
	resources := realCorpusResourceSummary(
		sampler,
		pinaxCommandMeasurement{DurationMS: 101},
		pinaxCommandMeasurement{DurationMS: 202},
		pinaxCommandMeasurement{DurationMS: 303},
	)
	if resources["provider_doctor_duration_ms"] != int64(101) || resources["candidate_rebuild_duration_ms"] != int64(202) || resources["candidate_evaluation_duration_ms"] != int64(303) {
		t.Fatalf("resource summary missing durations: %#v", resources)
	}
}

func TestRunShadowComparePersistsBoundedArtifact(t *testing.T) {
	artifactDir := t.TempDir()
	artifact, err := runShadowCompare(artifactDir, writeShadowCompareTestSidecar(t))
	if err != nil {
		t.Fatalf("run shadow compare: %v", err)
	}
	if artifact["citation_identity_match"] != true || artifact["safe_citations"] != true || artifact["permission_empty_semantics_match"] != true {
		t.Fatalf("shadow artifact facts = %#v", artifact)
	}
	if artifact["documents_legacy"] != 2 || artifact["documents_inferrum"] != 2 || artifact["chunks_legacy"] != 2 || artifact["chunks_inferrum"] != 2 {
		t.Fatalf("shadow artifact counts = %#v", artifact)
	}
	payload, err := os.ReadFile(filepath.Join(artifactDir, "shadow-compare.json"))
	if err != nil {
		t.Fatalf("read shadow artifact: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"query_vector", "chunk_text", "vault_path", "raw_prompt", "provider_payload", "SECRET"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("shadow artifact leaked %q: %s", forbidden, text)
		}
	}
}

func TestOllamaBaseURLUsesLoopbackDefaultAndTrimsSlash(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	if got := ollamaBaseURL(); got != "http://127.0.0.1:11434" {
		t.Fatalf("default Ollama URL = %q", got)
	}
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:11434/")
	if got := ollamaBaseURL(); got != "http://127.0.0.1:11434" {
		t.Fatalf("trimmed Ollama URL = %q", got)
	}
}

func TestValidateCanaryOptionsRequiresExplicitRealCorpusInputs(t *testing.T) {
	tests := []struct {
		name    string
		opts    canaryOptions
		wantErr string
	}{
		{name: "synthetic defaults", opts: canaryOptions{Provider: "ollama", Model: "qwen3-embedding:0.6b"}},
		{name: "vault without confirmation", opts: canaryOptions{Provider: "ollama", Model: "qwen3-embedding:0.6b", Vault: "/private/vault"}, wantErr: "real_corpus_confirmation_required"},
		{name: "missing vault", opts: canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", Suite: ".pinax/kb/evaluation-suites/m4.json"}, wantErr: "real_corpus_inputs_required"},
		{name: "missing suite", opts: canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", Vault: "/private/vault"}, wantErr: "real_corpus_inputs_required"},
		{name: "wrong provider", opts: canaryOptions{RealCorpus: true, Provider: "fake", Model: "qwen3-embedding:0.6b", Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json"}, wantErr: "real_corpus_ollama_required"},
		{name: "unsafe model", opts: canaryOptions{RealCorpus: true, Provider: "ollama", Model: "unsafe\nmodel", Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json"}, wantErr: "model_invalid"},
		{name: "ready", opts: canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCanaryOptions(test.opts)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCanaryOptions() error = %v", err)
				}
				return
			}
			if safeFailure(err) != test.wantErr {
				t.Fatalf("validateCanaryOptions() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateCanaryOptionsScopesM4PreflightToRealCorpusAndRequiresInferrumBinary(t *testing.T) {
	tests := []struct {
		name    string
		opts    canaryOptions
		wantErr string
	}{
		{
			name:    "synthetic preflight is rejected",
			opts:    canaryOptions{Provider: "ollama", Model: "qwen3-embedding:0.6b", M4Preflight: true},
			wantErr: "m4_preflight_real_corpus_required",
		},
		{
			name:    "real corpus preflight requires compiled inferrum",
			opts:    canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json", M4Preflight: true},
			wantErr: "m4_preflight_inferrum_binary_required",
		},
		{
			name: "real corpus preflight is ready with explicit binary",
			opts: canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json", M4Preflight: true, InferrumBinary: "./temp/bin/inferrum"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCanaryOptions(test.opts)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCanaryOptions() error = %v", err)
				}
				return
			}
			if safeFailure(err) != test.wantErr {
				t.Fatalf("validateCanaryOptions() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateCanaryOptionsAllowsStandaloneM4PreflightOnlyWithoutCorpus(t *testing.T) {
	tests := []struct {
		name    string
		opts    canaryOptions
		wantErr string
	}{
		{
			name:    "missing inferrum binary",
			opts:    canaryOptions{Provider: "ollama", Model: "qwen3-embedding:0.6b", M4PreflightOnly: true},
			wantErr: "m4_preflight_inferrum_binary_required",
		},
		{
			name: "standalone preflight is ready",
			opts: canaryOptions{
				Provider:        "ollama",
				Model:           "qwen3-embedding:0.6b",
				M4PreflightOnly: true,
				InferrumBinary:  "./temp/bin/inferrum",
			},
		},
		{
			name:    "real corpus is rejected",
			opts:    canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json", M4PreflightOnly: true, InferrumBinary: "./temp/bin/inferrum"},
			wantErr: "m4_preflight_only_no_corpus_inputs",
		},
		{
			name:    "preflight modes conflict",
			opts:    canaryOptions{Provider: "ollama", Model: "qwen3-embedding:0.6b", M4Preflight: true, M4PreflightOnly: true, InferrumBinary: "./temp/bin/inferrum"},
			wantErr: "m4_preflight_mode_conflict",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCanaryOptions(test.opts)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCanaryOptions() error = %v", err)
				}
				return
			}
			if safeFailure(err) != test.wantErr {
				t.Fatalf("validateCanaryOptions() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestM4PreflightOnlyChildFailsClosedWithoutCorpusAndWritesBoundedArtifact(t *testing.T) {
	artifactDir := t.TempDir()
	opts := canaryOptions{M4PreflightOnly: true, Provider: "ollama", Model: "qwen3-embedding:0.6b", InferrumBinary: "/private/unavailable-inferrum"}
	host := hostProfile{OS: "linux", Arch: "amd64", TargetM4Status: "not_macos", TargetM4AirStatus: "not_macos"}
	err := runM4PreflightOnlyChildWithHost(opts, artifactDir, host)
	if safeFailure(err) != "m4_preflight:target_m4_unconfirmed" {
		t.Fatalf("runM4PreflightOnlyChildWithHost() error = %v", err)
	}
	payload, readErr := os.ReadFile(filepath.Join(artifactDir, "m4-preflight.json"))
	if readErr != nil {
		t.Fatalf("read m4 preflight artifact: %v", readErr)
	}
	var artifact m4PreflightArtifact
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("parse m4 preflight artifact: %v", err)
	}
	if artifact.Scope != "compatibility_only" || artifact.Status != "failed" || artifact.FailureCode != "target_m4_unconfirmed" || artifact.TargetM4Status != "not_macos" || artifact.TargetM4AirStatus != "not_macos" {
		t.Fatalf("m4 preflight artifact = %#v", artifact)
	}
	if artifact.PinaxBinaryArchitecture != "" || strings.Contains(string(payload), "pinax_binary_architecture") {
		t.Fatalf("compatibility-only artifact unexpectedly recorded Pinax architecture: %s", payload)
	}
	for _, forbidden := range []string{"/private", "unavailable-inferrum", "vault", "suite", "query", "content"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(forbidden)) {
			t.Fatalf("m4 preflight artifact leaked %q: %s", forbidden, payload)
		}
	}
	failure, failureErr := os.ReadFile(filepath.Join(artifactDir, "failure.json"))
	if failureErr != nil || !strings.Contains(string(failure), `"stage": "m4_preflight"`) || strings.Contains(string(failure), "/private") {
		t.Fatalf("failure artifact = %s, err = %v", failure, failureErr)
	}
}

func TestEvaluateM4PreflightRequiresTargetCompatibilityAndEmbeddedValidation(t *testing.T) {
	validEmbedded := inferrumEmbeddedValidation{
		Command: "inferrum.validate.embedded",
		Status:  "success",
		Data: inferrumEmbeddedData{
			Backend:       "lancedb",
			Dependency:    "lancedb",
			EmbeddingDim:  1024,
			Rows:          3,
			Hits:          3,
			StoreState:    "available",
			StoreURIScope: "relative_to_root",
			Table:         "note_chunks",
		},
	}
	confirmed := hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,12", 10, 16<<30)
	tests := []struct {
		name               string
		host               hostProfile
		macOS              string
		python             string
		pythonArch         string
		inferrumArch       string
		pinaxArch          string
		requirePinaxBinary bool
		embedded           inferrumEmbeddedValidation
		wantErr            string
	}{
		{name: "real corpus pass", host: confirmed, macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", pinaxArch: "arm64", requirePinaxBinary: true, embedded: validEmbedded},
		{name: "compatibility-only pass without Pinax binary", host: confirmed, macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", embedded: validEmbedded},
		{name: "host not confirmed", host: hostProfileFromFacts("linux", "amd64", "", "", 8, 0), macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", embedded: validEmbedded, wantErr: "target_m4_unconfirmed"},
		{name: "M4 desktop is not M4 Air evidence", host: hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,10", 10, 16<<30), macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", embedded: validEmbedded, wantErr: "target_m4_air_unconfirmed"},
		{name: "old macos", host: confirmed, macOS: "13.6", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", embedded: validEmbedded, wantErr: "macos_version_unsupported"},
		{name: "old python", host: confirmed, macOS: "14.5", python: "3.9", pythonArch: "arm64", inferrumArch: "arm64", embedded: validEmbedded, wantErr: "python_version_unsupported"},
		{name: "translated Python is rejected", host: confirmed, macOS: "14.5", python: "3.12", pythonArch: "x86_64", inferrumArch: "arm64", embedded: validEmbedded, wantErr: "sidecar_python_arch_unsupported"},
		{name: "translated Inferrum binary is rejected", host: confirmed, macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "x86_64", embedded: validEmbedded, wantErr: "inferrum_binary_arch_unsupported"},
		{name: "translated Pinax binary is rejected for real corpus", host: confirmed, macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", pinaxArch: "x86_64", requirePinaxBinary: true, embedded: validEmbedded, wantErr: "pinax_binary_arch_unsupported"},
		{name: "embedded contract invalid", host: confirmed, macOS: "14.5", python: "3.12", pythonArch: "arm64", inferrumArch: "arm64", embedded: inferrumEmbeddedValidation{Command: "inferrum.validate.embedded", Status: "success"}, wantErr: "embedded_validation_contract_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact, err := evaluateM4Preflight(test.host, test.macOS, test.python, test.pythonArch, test.inferrumArch, test.pinaxArch, test.requirePinaxBinary, test.embedded)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("evaluateM4Preflight() error = %v", err)
				}
				if artifact.Status != "passed" || artifact.TargetM4Status != "confirmed" || artifact.TargetM4AirStatus != "confirmed" || artifact.MacOSVersion != "14.5" || artifact.PythonVersion != "3.12" || artifact.SidecarPythonArchitecture != "arm64" || artifact.InferrumBinaryArchitecture != "arm64" || artifact.EmbeddedBackend != "lancedb" || artifact.EmbeddedStoreURIScope != "relative_to_root" {
					t.Fatalf("preflight artifact = %#v", artifact)
				}
				if test.requirePinaxBinary && artifact.PinaxBinaryArchitecture != "arm64" {
					t.Fatalf("real corpus preflight Pinax architecture = %q", artifact.PinaxBinaryArchitecture)
				}
				if !test.requirePinaxBinary && artifact.PinaxBinaryArchitecture != "" {
					t.Fatalf("compatibility-only preflight unexpectedly inspected Pinax = %q", artifact.PinaxBinaryArchitecture)
				}
				return
			}
			if safeFailure(err) != test.wantErr || artifact.Status != "failed" || artifact.FailureCode != test.wantErr {
				t.Fatalf("preflight artifact/error = %#v/%v, want %q", artifact, err, test.wantErr)
			}
		})
	}
}

func TestParseInferrumEmbeddedValidationAcceptsOnlySafeProjection(t *testing.T) {
	validation, err := parseInferrumEmbeddedValidation([]byte(`{"command":"inferrum.validate.embedded","status":"success","data":{"backend":"lancedb","dependency":"lancedb","embedding_dim":1024,"rows":3,"hits":3,"store_state":"available","store_uri_scope":"relative_to_root","table":"note_chunks"}}`))
	if err != nil {
		t.Fatalf("parse validation: %v", err)
	}
	if validation.Data.Table != "note_chunks" || validation.Data.EmbeddingDim != 1024 {
		t.Fatalf("validation = %#v", validation)
	}
	if _, err := parseInferrumEmbeddedValidation([]byte(`{"status":"success","data":{}}`)); safeFailure(err) != "embedded_validation_output_invalid" {
		t.Fatalf("invalid validation error = %v", err)
	}
}

func TestRunInferrumProviderBenchmarkUsesFixedSafeContract(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "inferrum")
	body := `#!/bin/sh
if [ "$#" -ne 12 ] || [ "$1" != "provider" ] || [ "$2" != "benchmark" ] || [ "$3" != "ollama" ] || [ "$4" != "--model" ] || [ "$5" != "qwen3-embedding:0.6b" ] || [ "$6" != "--samples" ] || [ "$7" != "64" ] || [ "$8" != "--batch-size" ] || [ "$9" != "8" ] || [ "${10}" != "--warmup" ] || [ "${11}" != "1" ] || [ "${12}" != "--json" ]; then
  exit 9
fi
printf '%s\n' '{"spec_version":"1.1","mode":"json","command":"inferrum.provider.benchmark","status":"success","data":{"provider":"ollama","model":"qwen3-embedding:0.6b","sample_count":64,"batch_size":8,"batch_count":8,"warmup_batches":1,"batch_api":true,"input_characters":1430,"embedding_dim":1024,"warmup_ms":12,"elapsed_ms":87,"batch_p50_ms":10,"batch_p95_ms":16,"items_per_second":735.1,"scope":"synthetic_provider_benchmark","not_measured":["retrieval_quality","lancedb","owner_corpus","memory_pressure"]}}'
`
	if err := os.WriteFile(binary, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	artifact, err := runInferrumProviderBenchmark(binary, "ollama", "qwen3-embedding:0.6b")
	if err != nil {
		t.Fatalf("run benchmark: %v", err)
	}
	if artifact.SchemaVersion != "pinax.kb.provider-benchmark.v1" || artifact.Status != "passed" || artifact.Provider != "ollama" || artifact.Model != "qwen3-embedding:0.6b" || artifact.SampleCount != 64 || artifact.BatchSize != 8 || artifact.BatchCount != 8 || artifact.WarmupBatches != 1 || artifact.EmbeddingDim != 1024 || artifact.BatchP95MS != 16 || artifact.ItemsPerSecond != 735.1 || artifact.Scope != "synthetic_provider_benchmark" {
		t.Fatalf("provider benchmark artifact = %#v", artifact)
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{filepath.Dir(binary), "input_characters", "raw_sample", "provider_payload", "vector", "endpoint"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(forbidden)) {
			t.Fatalf("provider benchmark artifact leaked %q: %s", forbidden, payload)
		}
	}
}

func TestParseInferrumProviderBenchmarkRejectsUnsafeOrIncompatibleOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "wrong model",
			output: `{"command":"inferrum.provider.benchmark","status":"success","data":{"provider":"ollama","model":"other","sample_count":64,"batch_size":8,"batch_count":8,"warmup_batches":1,"embedding_dim":1024,"items_per_second":1,"scope":"synthetic_provider_benchmark"}}`,
		},
		{
			name:   "missing measured dimension",
			output: `{"command":"inferrum.provider.benchmark","status":"success","data":{"provider":"ollama","model":"qwen3-embedding:0.6b","sample_count":64,"batch_size":8,"batch_count":8,"warmup_batches":1,"items_per_second":1,"scope":"synthetic_provider_benchmark"}}`,
		},
		{
			name:   "wrong scope",
			output: `{"command":"inferrum.provider.benchmark","status":"success","data":{"provider":"ollama","model":"qwen3-embedding:0.6b","sample_count":64,"batch_size":8,"batch_count":8,"warmup_batches":1,"embedding_dim":1024,"items_per_second":1,"scope":"owner_corpus"}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact, err := parseInferrumProviderBenchmark([]byte(test.output), "ollama", "qwen3-embedding:0.6b")
			if safeFailure(err) != "provider_benchmark_output_invalid" || artifact.Status != "failed" || artifact.FailureCode != "provider_benchmark_output_invalid" {
				t.Fatalf("parse artifact/error = %#v/%v", artifact, err)
			}
		})
	}
}

func TestInferrumBinaryArchitectureRequiresMachOArm64(t *testing.T) {
	arm64 := writeMachOFixture(t, macho.CpuArm64)
	if architecture, err := inferrumBinaryArchitecture(arm64); err != nil || architecture != "arm64" {
		t.Fatalf("arm64 architecture = %q, %v", architecture, err)
	}
	x86 := writeMachOFixture(t, macho.CpuAmd64)
	if architecture, err := inferrumBinaryArchitecture(x86); safeFailure(err) != "inferrum_binary_arch_unsupported" || architecture != "amd64" {
		t.Fatalf("x86 architecture = %q, %v", architecture, err)
	}
	plain := filepath.Join(t.TempDir(), "not-a-binary")
	if err := os.WriteFile(plain, []byte("not a Mach-O"), 0o600); err != nil {
		t.Fatal(err)
	}
	if architecture, err := inferrumBinaryArchitecture(plain); safeFailure(err) != "inferrum_binary_arch_unavailable" || architecture != "" {
		t.Fatalf("plain architecture = %q, %v", architecture, err)
	}
}

func TestPinaxBinaryArchitectureRequiresMachOArm64(t *testing.T) {
	arm64 := writeMachOFixture(t, macho.CpuArm64)
	if architecture, err := pinaxBinaryArchitecture(arm64); err != nil || architecture != "arm64" {
		t.Fatalf("arm64 architecture = %q, %v", architecture, err)
	}
	x86 := writeMachOFixture(t, macho.CpuAmd64)
	if architecture, err := pinaxBinaryArchitecture(x86); safeFailure(err) != "pinax_binary_arch_unsupported" || architecture != "amd64" {
		t.Fatalf("x86 architecture = %q, %v", architecture, err)
	}
	plain := filepath.Join(t.TempDir(), "not-a-binary")
	if err := os.WriteFile(plain, []byte("not a Mach-O"), 0o600); err != nil {
		t.Fatal(err)
	}
	if architecture, err := pinaxBinaryArchitecture(plain); safeFailure(err) != "pinax_binary_arch_unavailable" || architecture != "" {
		t.Fatalf("plain architecture = %q, %v", architecture, err)
	}
}

func writeMachOFixture(t *testing.T, cpu macho.Cpu) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum")
	payload := make([]byte, 32)
	binary.LittleEndian.PutUint32(payload[0:4], 0xfeedfacf)
	binary.LittleEndian.PutUint32(payload[4:8], uint32(cpu))
	binary.LittleEndian.PutUint32(payload[12:16], 2)
	if err := os.WriteFile(path, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPythonExecutableFromSidecarShebangUsesSidecarInterpreterOrSafeFallback(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{line: "#!/private/venv/bin/python\n", want: "/private/venv/bin/python"},
		{line: "#!/usr/bin/env python3\n", want: "python3"},
		{line: "#!/usr/bin/env -S python3 -u\n", want: "python3"},
		{line: "#!/bin/sh\n", want: "python3"},
		{line: "", want: "python3"},
	}
	for _, test := range tests {
		if got := pythonExecutableFromShebang(test.line); got != test.want {
			t.Fatalf("pythonExecutableFromShebang(%q) = %q, want %q", test.line, got, test.want)
		}
	}
}

func TestRunInferrumEmbeddedValidationUsesDisposableRootAndPersistsNoPaths(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "inferrum-lancedb-sidecar")
	if err := os.WriteFile(sidecar, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	inferrum := filepath.Join(dir, "inferrum")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"command\":\"inferrum.validate.embedded\",\"status\":\"success\",\"data\":{\"backend\":\"lancedb\",\"dependency\":\"lancedb\",\"embedding_dim\":1024,\"rows\":3,\"hits\":3,\"store_state\":\"available\",\"store_uri_scope\":\"relative_to_root\",\"table\":\"note_chunks\"}}'\n"
	if err := os.WriteFile(inferrum, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	validation, err := runInferrumEmbeddedValidation(inferrum, sidecar)
	if err != nil {
		t.Fatalf("run validation: %v", err)
	}
	artifact, err := evaluateM4Preflight(hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,12", 10, 16<<30), "14.5", "3.12", "arm64", "arm64", "", false, validation)
	if err != nil {
		t.Fatalf("evaluate validation: %v", err)
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{dir, "inferrum-lancedb-sidecar", "Mac16,12", "--sidecar", "--root"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("preflight artifact leaked %q: %s", forbidden, payload)
		}
	}
}

func TestHostProfileMarksOnlyObservedM4AirFactsAsConfirmed(t *testing.T) {
	m4 := hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,12", 10, 16<<30)
	if m4.TargetM4Status != "confirmed" || m4.TargetM4AirStatus != "confirmed" || m4.CPUBrand != "Apple M4" || m4.MemoryBytes != 16<<30 {
		t.Fatalf("M4 profile = %#v", m4)
	}
	fifteenInchAir := hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,13", 10, 24<<30)
	if fifteenInchAir.TargetM4Status != "confirmed" || fifteenInchAir.TargetM4AirStatus != "confirmed" {
		t.Fatalf("15-inch M4 Air profile = %#v", fifteenInchAir)
	}
	m4Desktop := hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,10", 10, 16<<30)
	if m4Desktop.TargetM4Status != "confirmed" || m4Desktop.TargetM4AirStatus != "unconfirmed" {
		t.Fatalf("M4 desktop profile = %#v", m4Desktop)
	}
	nonM4 := hostProfileFromFacts("darwin", "arm64", "Apple M3", "Mac15,2", 8, 16<<30)
	if nonM4.TargetM4Status != "unconfirmed" || nonM4.TargetM4AirStatus != "unconfirmed" {
		t.Fatalf("non-M4 profile = %#v", nonM4)
	}
	linux := hostProfileFromFacts("linux", "amd64", "", "", 8, 0)
	if linux.TargetM4Status != "not_macos" || linux.TargetM4AirStatus != "not_macos" {
		t.Fatalf("linux profile = %#v", linux)
	}
}

func TestProcessRSSUsesMacOSBytesAndLinuxKiB(t *testing.T) {
	if got := maxRSSBytesForOS("darwin", 2048); got != 2048 {
		t.Fatalf("darwin max rss = %d, want 2048", got)
	}
	if got := maxRSSBytesForOS("linux", 2048); got != 2048*1024 {
		t.Fatalf("linux max rss = %d, want %d", got, 2048*1024)
	}
	if got := maxRSSBytesForOS("linux", -1); got != 0 {
		t.Fatalf("negative max rss = %d, want 0", got)
	}
}

func TestRealCorpusArtifactExcludesOperatorInputsAndPreservesCandidateOnlyState(t *testing.T) {
	opts := canaryOptions{
		RealCorpus: true,
		Provider:   "ollama",
		Model:      "qwen3-embedding:0.6b",
		Vault:      "/private/customer-vault",
		Suite:      ".pinax/kb/evaluation-suites/secret-evaluation.json",
	}
	beforeActivation := activationSnapshot{Sequence: 7, ActiveGenerationID: "active-previous"}
	preflight := &m4PreflightArtifact{SchemaVersion: "pinax.kb.m4-preflight.v1", Status: "passed", TargetM4Status: "confirmed", TargetM4AirStatus: "confirmed", MacOSVersion: "14.5", PythonVersion: "3.12", PinaxBinaryArchitecture: "arm64", EmbeddedBackend: "lancedb", EmbeddedStoreURIScope: "relative_to_root"}
	benchmark := &providerBenchmarkArtifact{SchemaVersion: "pinax.kb.provider-benchmark.v1", Status: "passed", Provider: "ollama", Model: "qwen3-embedding:0.6b", SampleCount: 64, BatchSize: 8, BatchCount: 8, WarmupBatches: 1, BatchAPI: true, EmbeddingDim: 1024, WarmupMS: 1, ElapsedMS: 2, BatchP50MS: 3, BatchP95MS: 4, ItemsPerSecond: 5, Scope: "synthetic_provider_benchmark", NotMeasured: []string{"retrieval_quality", "lancedb", "owner_corpus", "memory_pressure"}}
	evaluation := map[string]any{
		"facts": map[string]any{"run_id": "run-1", "status": "passed", "recall_at_5": "0.900000", "mrr_at_10": "0.850000", "citation_coverage": "1.000000", "failure_count": "0", "suite_id": "m4-evaluation", "suite_version": "v1"},
	}
	firstSupportGate := evaluateM4FirstSupportGate(preflight, benchmark, evaluation, beforeActivation, beforeActivation)
	artifact := realCorpusArtifact(opts, "compiled_temp", hostProfileFromFacts("darwin", "arm64", "Apple M4", "Mac16,12", 10, 16<<30), preflight, benchmark, &firstSupportGate, map[string]any{
		"facts": map[string]any{"generation_id": "candidate-1", "documents": "50", "chunks": "200", "embedding_dim": "1024", "protocol": "inferrum.sidecar.v1", "model_manifest_digest": "digest", "profile_hash": "profile"},
	}, evaluation, beforeActivation, beforeActivation, map[string]any{
		"max_rss_bytes":                    int64(123),
		"provider_doctor_duration_ms":      int64(11),
		"candidate_rebuild_duration_ms":    int64(22),
		"candidate_evaluation_duration_ms": int64(33),
	})
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, forbidden := range []string{"/private/customer-vault", "secret-evaluation", "Mac16,12", "query_text", "expected_answer", "provider_payload", "vector_values"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("real corpus artifact leaked %q: %s", forbidden, text)
		}
	}
	resources, resourcesOK := artifact["resources"].(map[string]any)
	if artifact["scope"] != "real_corpus_candidate_evaluation" || artifact["activation_status"] != "not_attempted" || artifact["quality_verdict"] != "candidate_gate_passed_not_activated" || artifact["pinax_binary_mode"] != "compiled_temp" || artifact["pinax_binary_architecture"] != "arm64" || artifact["target_m4_status"] != "confirmed" || artifact["target_m4_air_status"] != "confirmed" || artifact["active_generation_unchanged"] != true || artifact["active_generation_before"] != "active-previous" || artifact["active_generation_after"] != "active-previous" || artifact["m4_preflight"] != preflight || artifact["provider_benchmark"] != benchmark || artifact["first_support_gate"] != &firstSupportGate || !resourcesOK || resources["provider_doctor_duration_ms"] != int64(11) || resources["candidate_rebuild_duration_ms"] != int64(22) || resources["candidate_evaluation_duration_ms"] != int64(33) {
		t.Fatalf("unexpected real corpus artifact: %#v", artifact)
	}
}

func TestRealCorpusArtifactFailsVerdictWhenActivationStateChanges(t *testing.T) {
	artifact := realCorpusArtifact(canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b"}, "provided", hostProfile{}, nil, nil, nil, map[string]any{}, map[string]any{"facts": map[string]any{"status": "passed"}}, activationSnapshot{Sequence: 1, ActiveGenerationID: "before"}, activationSnapshot{Sequence: 2, ActiveGenerationID: "after"}, map[string]any{})
	if artifact["active_generation_unchanged"] != false || artifact["quality_verdict"] != "activation_state_changed" {
		t.Fatalf("changed activation artifact = %#v", artifact)
	}
}

func TestM4FirstSupportGateRequiresExactTopKCitationCoverage(t *testing.T) {
	preflight := &m4PreflightArtifact{SchemaVersion: "pinax.kb.m4-preflight.v1", Status: "passed"}
	benchmark := &providerBenchmarkArtifact{SchemaVersion: "pinax.kb.provider-benchmark.v1", Status: "passed", Provider: "ollama", Model: "qwen3-embedding:0.6b"}
	evaluation := map[string]any{
		"facts": map[string]any{
			"status":            "passed",
			"recall_at_5":       "0.800000",
			"mrr_at_10":         "0.650000",
			"citation_coverage": "1.000000",
			"failure_count":     "0",
		},
	}
	unchanged := activationSnapshot{Sequence: 5, ActiveGenerationID: "active-1"}
	passed := evaluateM4FirstSupportGate(preflight, benchmark, evaluation, unchanged, unchanged)
	if passed.Status != "passed" || passed.FailureCode != "" || !passed.MetricsValid || passed.CitationCoverage != 1 {
		t.Fatalf("passed gate = %#v", passed)
	}

	partialEvaluation := map[string]any{
		"facts": map[string]any{
			"status":            "passed",
			"recall_at_5":       "0.900000",
			"mrr_at_10":         "0.850000",
			"citation_coverage": "0.950000",
			"failure_count":     "0",
		},
	}
	partial := evaluateM4FirstSupportGate(preflight, benchmark, partialEvaluation, unchanged, unchanged)
	if partial.Status != "failed" || partial.FailureCode != "citation_coverage_incomplete" || !partial.MetricsValid || partial.CitationCoverage != 0.95 {
		t.Fatalf("partial citation gate = %#v", partial)
	}
}

func TestM4FirstSupportGateFailsClosedForInvalidMetricsAndStaysAdditive(t *testing.T) {
	preflight := &m4PreflightArtifact{SchemaVersion: "pinax.kb.m4-preflight.v1", Status: "passed"}
	benchmark := &providerBenchmarkArtifact{SchemaVersion: "pinax.kb.provider-benchmark.v1", Status: "passed", Provider: "ollama", Model: "qwen3-embedding:0.6b"}
	unchanged := activationSnapshot{Sequence: 5, ActiveGenerationID: "active-1"}
	invalid := evaluateM4FirstSupportGate(preflight, benchmark, map[string]any{
		"facts": map[string]any{
			"status":            "passed",
			"recall_at_5":       "NaN",
			"mrr_at_10":         "0.850000",
			"citation_coverage": "1.000000",
			"failure_count":     "0",
		},
	}, unchanged, unchanged)
	if invalid.Status != "failed" || invalid.FailureCode != "first_support_metrics_invalid" || invalid.MetricsValid {
		t.Fatalf("invalid metric gate = %#v", invalid)
	}

	artifact := realCorpusArtifact(
		canaryOptions{RealCorpus: true, Provider: "ollama", Model: "qwen3-embedding:0.6b"},
		"provided",
		hostProfile{},
		nil,
		nil,
		nil,
		map[string]any{},
		map[string]any{"facts": map[string]any{"status": "passed"}},
		unchanged,
		unchanged,
		map[string]any{},
	)
	if _, exists := artifact["first_support_gate"]; exists {
		t.Fatalf("candidate-only artifact unexpectedly contains first support gate: %#v", artifact)
	}
}

func TestM4FirstSupportEvaluationArgsPinTopKAndKeepCandidateOnlyCompatibility(t *testing.T) {
	pilotArgs := realCorpusEvaluationArgs(canaryOptions{M4Preflight: true, Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json"}, "candidate-1")
	pilot := " " + strings.Join(pilotArgs, " ") + " "
	if !strings.Contains(pilot, " --k 5 ") || !strings.Contains(pilot, " --generation candidate-1 ") || !strings.Contains(pilot, " --json ") {
		t.Fatalf("M4 pilot evaluation args = %#v", pilotArgs)
	}

	candidateOnlyArgs := realCorpusEvaluationArgs(canaryOptions{Vault: "/private/vault", Suite: ".pinax/kb/evaluation-suites/m4.json"}, "candidate-1")
	if strings.Contains(" "+strings.Join(candidateOnlyArgs, " ")+" ", " --k ") {
		t.Fatalf("candidate-only evaluation args unexpectedly pin first-support K: %#v", candidateOnlyArgs)
	}
}

func TestEnforceM4FirstSupportGatePersistsBoundedFailure(t *testing.T) {
	dir := t.TempDir()
	failed := &firstSupportGateArtifact{SchemaVersion: firstSupportGateSchemaV1, Status: "failed", FailureCode: "citation_coverage_incomplete"}
	if err := enforceM4FirstSupportGate(dir, failed); err == nil || !strings.Contains(err.Error(), "first_support_gate:citation_coverage_incomplete") {
		t.Fatalf("failed gate error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(dir, "failure.json"))
	if err != nil {
		t.Fatalf("read gate failure artifact: %v", err)
	}
	if !strings.Contains(string(payload), `"stage": "first_support_gate"`) || !strings.Contains(string(payload), `"code": "citation_coverage_incomplete"`) || strings.Contains(string(payload), "/private") {
		t.Fatalf("gate failure artifact is not bounded: %s", payload)
	}
	if err := enforceM4FirstSupportGate(dir, &firstSupportGateArtifact{SchemaVersion: firstSupportGateSchemaV1, Status: "passed"}); err != nil {
		t.Fatalf("passed first-support gate rejected: %v", err)
	}
}

func TestReadActivationSnapshotTreatsUnactivatedVaultAsSequenceZero(t *testing.T) {
	snapshot, err := readActivationSnapshot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Sequence != 0 || snapshot.ActiveGenerationID != "" {
		t.Fatalf("unactivated snapshot = %#v", snapshot)
	}
}

func TestReadActivationSnapshotCapturesActiveGenerationIdentity(t *testing.T) {
	vault := t.TempDir()
	descriptor := app.KBActivationDescriptor{
		SchemaVersion: app.KBActivationDescriptorSchema,
		Sequence:      1,
		Active: &app.KBActivationRef{
			GenerationID:           "candidate-1",
			Protocol:               "inferrum.sidecar.v1",
			Provider:               "ollama",
			Model:                  "qwen3-embedding:0.6b",
			ModelManifestDigest:    "sha256:model",
			ProfileHash:            "sha256:profile",
			EmbeddingDim:           1024,
			SourceSnapshot:         "snapshot-1",
			SourceDigest:           "sha256:source",
			GenerationManifestHash: "sha256:manifest",
			EvaluationReceiptHash:  "sha256:receipt",
		},
		ActivatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := app.CommitKBActivation(vault, 0, descriptor); err != nil {
		t.Fatalf("commit activation: %v", err)
	}
	snapshot, err := readActivationSnapshot(vault)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Sequence != 1 || snapshot.ActiveGenerationID != "candidate-1" {
		t.Fatalf("active snapshot = %#v", snapshot)
	}
}

func TestResolvePinaxBinaryAcceptsExplicitBinaryWithoutRecordingItsPath(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "pinax")
	if err := os.WriteFile(binary, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, mode, cleanup, err := resolvePinaxBinary(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if path != binary || mode != "provided" {
		t.Fatalf("resolved binary = %q/%q, want %q/provided", path, mode, binary)
	}
}

func writeShadowCompareTestSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, math, pathlib, sys

op = sys.argv[1]
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)

def cosine(left, right):
    dot = sum(a * b for a, b in zip(left, right))
    left_norm = math.sqrt(sum(a * a for a in left))
    right_norm = math.sqrt(sum(a * a for a in right))
    if left_norm == 0 or right_norm == 0:
        return 0.0
    return dot / (left_norm * right_norm)

if op == "rebuild":
    (store / "records.json").write_text(json.dumps(req.get("records", [])), encoding="utf-8")
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(req.get("records", []))}))
elif op == "search":
    rows = json.loads((store / "records.json").read_text(encoding="utf-8"))
    allowed = set(req.get("allowed_ids") or [])
    if allowed:
        rows = [row for row in rows if row.get("id") in allowed]
    query = req.get("query_vector") or []
    rows.sort(key=lambda row: (-cosine(query, row.get("vector") or []), str((row.get("metadata") or {}).get("source_ref") or "")))
    limit = int(req.get("limit") or 20)
    hits = [{"id": row["id"], "score": cosine(query, row.get("vector") or []), "metadata": row.get("metadata") or {}} for row in rows[:limit]]
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","total":len(hits),"hits":hits}))
else:
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"failed","error":{"code":"operation_invalid","message":"unknown operation"}}))
    sys.exit(2)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write shadow test sidecar: %v", err)
	}
	return path
}
