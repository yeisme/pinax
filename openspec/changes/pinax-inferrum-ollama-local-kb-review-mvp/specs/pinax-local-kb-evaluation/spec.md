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
