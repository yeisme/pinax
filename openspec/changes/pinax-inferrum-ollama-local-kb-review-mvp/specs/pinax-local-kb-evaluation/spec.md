## ADDED Requirements

### Requirement: Pinax SHALL manage versioned local KB evaluation suites

Pinax SHALL support local, versioned evaluation suites containing stable question ids, expected citation refs, optional expected answer points, tags, and suite metadata without making evaluation evidence a copy of the knowledge base.

#### Scenario: Evaluation suite is loaded from a versioned local source
- **WHEN** the user selects an evaluation suite for a vault
- **THEN** Pinax SHALL resolve a stable suite id and version or digest
- **AND** it SHALL validate unique question ids and safe expected citation refs before running
- **AND** it SHALL NOT upload the suite or canonical note content to a remote service.

#### Scenario: Dataset is too small for a quality verdict
- **WHEN** a suite contains fewer questions than its configured minimum or lacks enough expected citations
- **THEN** Pinax SHALL report `insufficient_dataset`
- **AND** it SHALL NOT label the embedding model or generation as having passed a quality gate.

### Requirement: Pinax SHALL replay evaluation questions through the production retrieval path

`pinax kb evaluate` SHALL execute the same provider, permission, search, bounded context, and citation path used by normal KB retrieval against either the current active generation or one explicitly selected immutable candidate generation, while persisting only a Pinax-owned sanitized receipt.

#### Scenario: Evaluation run uses active generation facts
- **WHEN** the user runs `pinax kb evaluate --suite <suite> --vault ./my-notes --json`
- **THEN** every question SHALL use the active generation's provider, exact model tag, resolved model-manifest digest, profile hash, dimension, protocol, source snapshot, permission rules, limit, and retrieval stages
- **AND** output SHALL include a stable run id and suite version.

#### Scenario: Evaluation run targets a candidate before activation
- **WHEN** the user runs `pinax kb evaluate --suite <suite> --generation <candidate-id> --vault ./my-notes --json` or the activation service requests the equivalent operation
- **THEN** every question SHALL read the immutable candidate manifest/store without changing the active descriptor
- **AND** the resulting receipt SHALL bind candidate generation id, source snapshot/digest, provider, exact model tag, resolved model-manifest digest, profile hash, dimension, suite id/version, gate config hash, and generation manifest hash
- **AND** only a terminal `passed` receipt with all matching facts SHALL be eligible for candidate activation.

#### Scenario: Timeout result is not automatically retried
- **WHEN** an evaluation question times out after admission and its terminal result is unknown
- **THEN** Pinax SHALL record an `unknown` or `timeout` result with the stable run/question id
- **AND** it SHALL NOT automatically submit the same Ollama computation again
- **AND** it SHALL provide a status or rerun command requiring explicit operator action.

#### Scenario: Empty permission result remains a valid zero-hit observation
- **WHEN** a question's allowed-id resolution returns an explicit empty set
- **THEN** the evaluation SHALL record `permission_empty` and zero hits
- **AND** it SHALL NOT query the sidecar without a restriction.

### Requirement: Evaluation SHALL compute retrieval and citation metrics truthfully

Pinax SHALL calculate metrics only from observed ranked citations and SHALL distinguish missing data, failures, and unsupported answer-generation capability from successful retrieval.

#### Scenario: Run reports ranked citation metrics
- **WHEN** an evaluation run reaches a terminal state
- **THEN** its summary SHALL include total questions, terminal status counts, Recall@K, MRR@K, citation coverage, no-hit count, permission-empty count, error classifications, and available latency summaries
- **AND** every metric SHALL identify the suite version, generation id, K value, and denominator.

#### Scenario: Missing phase latency is not emitted as zero
- **WHEN** the runtime cannot observe a stage-specific latency such as embed, search, or assemble
- **THEN** the field SHALL be absent or explicitly unavailable
- **AND** it SHALL NOT be emitted as `0` or inferred from another phase.

#### Scenario: Evaluation does not claim answer quality without generation
- **WHEN** no generation model is configured for the run
- **THEN** question and summary projections SHALL report `answer_mode=not_generated`
- **AND** bounded retrieval context SHALL NOT be labeled as a generated answer
- **AND** answer-quality metrics SHALL be absent or unavailable.

### Requirement: Evaluation runs SHALL produce stable review receipts

Every evaluation run SHALL write a machine-readable receipt that can be compared across model, corpus, source snapshot, and generation changes without storing unsafe content.

#### Scenario: Completed run writes a reviewable receipt
- **WHEN** an evaluation run completes or fails
- **THEN** Pinax SHALL persist run id, suite id/version, generation id, provider/exact model tag/base digest when applicable/resolved model-manifest digest/profile hash/dimension, protocol, source snapshot/digest, gate config hash, generation manifest hash, start/finish time, duration, question status summaries, metrics, failure classifications, sanitized receipt refs/hashes, and redaction policy
- **AND** the receipt SHALL remain readable after the Ollama process or Pinax API stops.

#### Scenario: Raw Inferrum retrieval manifest is not persisted
- **WHEN** Inferrum returns an in-memory retrieval manifest containing a raw query or permission filter values
- **THEN** Pinax SHALL convert it to an allowlisted persisted receipt using question id/query digest and permission policy/version/status/allowed count/allowed-set digest
- **AND** it SHALL NOT persist raw query text, permission id lists, or a direct serialization of the Inferrum retrieval manifest
- **AND** any manifest ref or hash in evaluation evidence SHALL point to the sanitized Pinax receipt.

#### Scenario: Evidence excludes sensitive content
- **WHEN** evaluation command, integration runner, API fixture, or Workbench projection writes evidence
- **THEN** it SHALL exclude raw question text from integration logs, permission id lists, full expected answers, full note bodies, absolute vault paths, vectors, credentials, tokens, cookies, Authorization headers, provider payloads, raw prompts, hidden prompts, private tool arguments, and chain-of-thought
- **AND** it SHALL use question ids, safe citation refs, digests, bounded previews, and manifest refs instead.

### Requirement: Real evaluation runs SHALL leave project-owned integration evidence

Pinax SHALL treat real Ollama + LanceDB evaluation as a component or system test and SHALL generate the repository-standard evidence directory for both passing and failing runs.

#### Scenario: Real local KB canary writes evidence
- **WHEN** the real local-KB integration entrypoint runs on the current server or CI canary
- **THEN** it SHALL create `temp/integration-test-runs/<run-id>/summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, and `artifacts/`
- **AND** summary status and exit code SHALL reflect the real terminal command result
- **AND** artifacts SHALL separately prove daemon version, exact model tag, base digest when applicable, resolved model-manifest digest, profile hash, non-empty embedding dimension, sidecar dependency, generation row count, retrieval citations, and evaluation metrics.

#### Scenario: Pre-migration baseline canary does not claim unified integration
- **WHEN** a Inferrum-owned runner separately exercises direct Inferrum v1, current Pinax v1, and a Web route baseline before the Pinax migration exists
- **THEN** its summary SHALL use a baseline/component scope and list explicit proven and not-proven claims
- **AND** it SHALL NOT claim Pinax-to-Inferrum-v1 generation, evaluation, activation, rollback, or KB review Web implementation success.

#### Scenario: Failed canary preserves evidence and active generation
- **WHEN** model pull/readiness, sidecar install, disk admission, embedding, rebuild, search, citation validation, or evaluation fails
- **THEN** the runner SHALL still write complete redacted evidence and return the original failing exit status
- **AND** it SHALL NOT activate the failed generation or overwrite previous evidence.

### Requirement: Real-corpus target-machine evidence SHALL require explicit confirmation and preserve activation state

The Pinax local-KB evidence runner SHALL provide an explicit real-corpus mode that requires an existing vault, a versioned evaluation suite, and an affirmative real-corpus flag. The operator workflow SHALL require a target compatibility preflight before treating a run as Mac M4 Air evidence: macOS 14+ on `arm64`, an observed Apple M4 CPU, a recognized current M4 Air hardware model, Python 3.10+, native `arm64` execution by the sidecar's actual Python interpreter, the resolved compiled Pinax Mach-O binary, and the supplied compiled Inferrum Mach-O binary, a macOS-arm64-compatible installation of the current sidecar LanceDB dependency range, and a successful compiled `inferrum validate embedded` run on a disposable root. `--m4-preflight --inferrum-binary <binary>` SHALL be additive to the existing candidate-only runner: when selected, it SHALL validate these facts, run one fixed safe `inferrum provider benchmark ollama` measurement (`samples=64`, `batch_size=8`, `warmup=1`), and write redacted `m4-preflight.json` plus `provider-benchmark.json` before reading the vault; it SHALL project only bounded benchmark facts into `real-corpus.json`. Callers that omit the new flag retain candidate-only compatibility but SHALL NOT use the result as M4 baseline or release-acceptance evidence. The runner SHALL use an explicitly supplied Pinax binary or build a temporary `CGO_ENABLED=0` binary for the run. It SHALL run provider doctor, candidate rebuild, and candidate evaluation, but SHALL NOT activate, roll back, copy, or persist the corpus through the evidence runner.

#### Scenario: A target Mac evaluates a real corpus candidate

- **WHEN** an operator runs the real-corpus evidence runner with an Ollama provider, a vault, a suite, and explicit confirmation
- **THEN** it SHALL write redacted component evidence containing safe model/generation/metric facts, top-level `target_m4_status`, optional `target_m4_air_status`, optional `pinax_binary_architecture`, and optional bounded `provider_benchmark`, host OS/arch/resource observations, `provider_doctor_duration_ms`, `candidate_rebuild_duration_ms`, `candidate_evaluation_duration_ms`, and `activation_status=not_attempted`
- **AND** the command-duration fields SHALL be optional additive facts and SHALL NOT be labeled as cold, warm, or P95 measurements
- **AND** it SHALL record independent M4 and M4 Air identity statuses from observed host facts; M4 confirmation alone SHALL NOT qualify the run as M4 Air evidence
- **AND** it SHALL compare the Pinax activation descriptor before and after candidate evaluation, retain a failed artifact with `activation_state_changed` if it differs, and otherwise leave the active generation unchanged until a separate explicit activation command consumes a matching passed receipt.

### Requirement: M4 first-support evidence SHALL enforce its citation-coverage gate independently

For an explicit `--real-corpus --m4-preflight` run, Pinax SHALL add a bounded optional `first_support_gate` projection to `real-corpus.json`. This gate is a pilot release-evidence decision and SHALL NOT change the existing generic `pinax kb evaluate` receipt status, receipt gate hash, or ordinary candidate activation semantics.

#### Scenario: A M4 candidate meets all first-support evidence gates

- **WHEN** M4 preflight and provider benchmark passed, activation is unchanged, and the candidate evaluation reports `status=passed`, Recall@5 `>=0.80`, MRR@10 `>=0.65`, citation coverage `=1.00`, and failure count `=0`
- **THEN** `first_support_gate.status` SHALL be `passed`
- **AND** the runner SHALL retain `activation_status=not_attempted` for a separate explicit activation decision.

#### Scenario: A generic evaluation pass has incomplete top-K citation coverage

- **WHEN** the generic evaluation status is `passed` but the real-corpus evidence reports citation coverage below `1.00` at its declared evaluation K
- **THEN** `first_support_gate.status` SHALL be `failed` with the bounded code `citation_coverage_incomplete`
- **AND** the runner SHALL write `real-corpus.json` and a redacted failure artifact, exit non-zero, and leave the active generation unchanged.

#### Scenario: Metrics are missing or invalid

- **WHEN** any required first-support metric is absent, non-finite, or outside its valid range
- **THEN** `first_support_gate.status` SHALL be `failed` with a bounded metric-validation failure code
- **AND** Pinax SHALL NOT infer a release decision from another metric or a generic evaluation status.

#### Scenario: A target Mac emits a compiled embedded-LanceDB preflight receipt

- **WHEN** the operator selects `--m4-preflight` with a compiled Inferrum binary and configured sidecar
- **THEN** the runner SHALL verify observed Apple M4 and M4 Air host identities, macOS 14+, the sidecar's actual Python 3.10+ interpreter, the resolved Pinax and supplied Inferrum binaries as native arm64 Mach-O executables, and a successful `inferrum validate embedded` projection before reading the vault
- **AND** it SHALL preserve `target_m4_status` and add `target_m4_air_status`, `sidecar_python_architecture`, `pinax_binary_architecture`, and `inferrum_binary_architecture` as bounded facts, while not persisting the raw hardware model identifier, binary path, or sidecar path
- **AND** it SHALL not persist binary paths, sidecar paths, disposable roots, vault paths, raw command stdout/stderr, source content, vectors, tokens, or provider payloads.

#### Scenario: A target Mac records a safe baseline provider benchmark before corpus access

- **WHEN** the operator selects `--real-corpus --m4-preflight` after the compiled embedded-LanceDB preflight passes
- **THEN** the runner SHALL invoke the supplied Inferrum binary once as `provider benchmark ollama` with `samples=64`, `batch_size=8`, `warmup=1`, and JSON output before reading the vault
- **AND** it SHALL persist a redacted `provider-benchmark.json` and optional `provider_benchmark` projection containing only command/status, provider/model, batch facts, observed dimension, timing percentiles, throughput, scope, and fixed not-measured facts
- **AND** benchmark failure, malformed output, model/provider mismatch, or invalid metrics SHALL fail before candidate rebuild without persisting raw command output, sample text, vectors, endpoint, credentials, or provider payloads.

#### Scenario: A target Mac records compatibility before any corpus is available

- **WHEN** the operator selects `--m4-preflight-only --inferrum-binary <binary>` with a configured sidecar
- **THEN** the runner SHALL reject any real-corpus, vault, or suite input and SHALL NOT invoke Pinax, Ollama, Inferrum provider benchmark, provider doctor, rebuild, evaluation, activation, or rollback
- **AND** it SHALL write redacted component evidence and `m4-preflight.json` with `scope=compatibility_only` after checking observed M4 and M4 Air identities, macOS, sidecar Python, and compiled embedded LanceDB
- **AND** a passed compatibility-only artifact SHALL be insufficient by itself for M4 baseline, capacity, quality, activation, or release-acceptance claims.

#### Scenario: A real-corpus preflight rejects a translated Pinax binary

- **WHEN** an operator selects `--real-corpus --m4-preflight` and the resolved Pinax Mach-O binary lacks an `arm64` slice
- **THEN** the runner SHALL fail before reading the vault with `pinax_binary_arch_unsupported` or `pinax_binary_arch_unavailable`
- **AND** it SHALL persist only the bounded architecture fact when available, without persisting the binary path

#### Scenario: M4 compatibility preflight is not complete

- **WHEN** an operator cannot satisfy the documented macOS/architecture/Python/sidecar/compiled-embedded-LanceDB preflight
- **THEN** the operator SHALL not use a real-corpus runner result as M4 baseline, capacity, or release-acceptance evidence
- **AND** the active generation SHALL remain unchanged until the preflight and a separate candidate evaluation succeed.

#### Scenario: A real-corpus invocation is incomplete or unsafe

- **WHEN** an operator omits explicit confirmation, the vault, the suite, or the Ollama provider requirement
- **THEN** the runner SHALL fail before candidate rebuild
- **AND** it SHALL not write source text, questions, expected answers, vault paths, vectors, credentials, provider payloads, or private tool arguments into its artifact.
