## ADDED Requirements

### Requirement: KB projection activation SHALL be atomic and reversible

Pinax SHALL build each semantic projection as a distinct generation and SHALL NOT replace the active generation until the complete generation passes source, embedding, index, citation, permission, and configured evaluation gates.

#### Scenario: Successful candidate generation becomes active
- **WHEN** a KB rebuild completes all chunks with one exact provider/model identity/dimension, the source digest remains unchanged, the LanceDB row count matches the plan, and a `passed` evaluation receipt matches the candidate generation id, source snapshot/digest, provider, exact model tag, resolved model-manifest digest, profile hash, dimension, suite id/version, gate config hash, and generation manifest hash
- **THEN** Pinax SHALL acquire the vault mutation lock and compare-and-swap one authoritative activation descriptor containing both active and previous generation references
- **AND** it SHALL preserve the previous generation as a bounded rollback target
- **AND** machine output SHALL report generation id, protocol, provider, exact model tag, base model digest when applicable, resolved model-manifest digest, profile hash, dimension, source snapshot, row count, evaluation receipt hash, activation sequence, and activation status.

#### Scenario: Candidate receipt is absent or belongs to another generation
- **WHEN** activation receives no passed evaluation receipt or any bound generation, source, model, suite, gate, dimension, or manifest fact differs from the candidate
- **THEN** Pinax SHALL reject activation with a stable mismatch or gate-not-satisfied error
- **AND** it SHALL NOT update the activation descriptor or reuse an evaluation of the current active generation.

#### Scenario: Failed staging generation preserves active projection
- **WHEN** embedding, indexing, source-drift validation, citation validation, permission validation, disk admission, or evaluation fails for a staging generation
- **THEN** Pinax SHALL mark that generation failed or rejected
- **AND** it SHALL NOT replace or partially overwrite the active generation
- **AND** a subsequent search SHALL continue to use the previously active generation.

#### Scenario: Operator rolls back to previous generation
- **WHEN** an authorized local rollback command selects the previous generation recorded in the activation descriptor with the expected sequence
- **THEN** Pinax SHALL use the same vault lock and compare-and-swap transition to exchange active and previous without modifying canonical Markdown, Git state, Cloud Sync state, provider credentials, or remote objects
- **AND** it SHALL write a redacted rollback receipt.

#### Scenario: Activation is interrupted or races with another command
- **WHEN** activate, rollback, or prune is interrupted before or after descriptor rename, or another mutation presents a stale sequence
- **THEN** recovery SHALL observe either the complete old descriptor or the complete new descriptor, never a mixed active/previous pair
- **AND** a stale concurrent mutation SHALL fail instead of overwriting the newer state
- **AND** a single search/context request SHALL stay pinned to one descriptor snapshot.

### Requirement: KB permission filtering SHALL fail closed before Inferrum search

Pinax SHALL resolve KB permissions from Pinax-owned source metadata before invoking Inferrum search and SHALL distinguish unresolved permission state from an explicitly empty allowed-id result.

#### Scenario: Empty allowed-id set returns zero results
- **WHEN** Pinax successfully resolves a query's allowed ids and the resulting set is empty
- **THEN** Pinax SHALL return zero hits with a stable `permission_empty` fact or equivalent
- **AND** it SHALL NOT invoke the Inferrum sidecar with an empty allowed-id list.

#### Scenario: Permission resolution failure does not search broadly
- **WHEN** Pinax cannot resolve permission candidates or the permission filter is invalid
- **THEN** Pinax SHALL return a stable failure
- **AND** it SHALL NOT omit the filter, send a nil filter as unrestricted, or query all records.

### Requirement: KB citations SHALL be safe and traceable

Pinax SHALL return citations that identify the canonical source fragment without returning full note bodies, absolute private paths, embedding vectors, or unsafe metadata.

#### Scenario: Search result contains a bounded citation
- **WHEN** `pinax kb search` or `pinax kb context` returns a hit
- **THEN** the hit SHALL include stable chunk id, safe source ref, title, heading/page/span when available, bounded preview, score, source version or digest, provider, model, and active generation id
- **AND** it SHALL NOT include full `body`, `raw_body`, absolute vault path, vector values, provider payloads, credentials, Authorization headers, cookies, raw prompts, or chain-of-thought.

#### Scenario: Unknown metadata is not projected
- **WHEN** a source adapter or old projection contains metadata outside the Pinax KB safe-field allowlist
- **THEN** Pinax SHALL omit the unknown fields from Inferrum records, CLI output, API projection, Web fixtures, manifests, and evaluation evidence
- **AND** it SHALL NOT use a denylist-only policy to pass new fields by default.

## MODIFIED Requirements

### Requirement: Pinax SHALL manage a local semantic KB projection

Pinax SHALL provide a `pinax kb` command group that keeps Markdown vault content as the source of truth while maintaining a rebuildable local semantic projection through `github.com/yeisme/inferrum`, `inferrum-lancedb-sidecar`, and `inferrum.sidecar.v1`.

#### Scenario: Rebuild local semantic projection through Inferrum
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider fake --vault ./my-notes --json`
- **THEN** Pinax SHALL scan registered Markdown notes through app-owned vault behavior
- **AND** it SHALL map Pinax chunks to domain-agnostic Inferrum records with opaque safe metadata
- **AND** it SHALL build a staging generation under `.pinax/kb/generations/<generation-id>/`
- **AND** JSON facts SHALL include backend, protocol, provider, model, embedding dimension, document count, chunk count, generation status, and `sync_vectors=false`.

#### Scenario: Missing unified LanceDB sidecar is actionable
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --vault ./my-notes --json` without an available `inferrum-lancedb-sidecar`
- **THEN** Pinax SHALL return `error.code=kb_sidecar_unavailable` or a compatible stable code
- **AND** it SHALL include an install or configuration next step for the unified sidecar
- **AND** it SHALL NOT silently fall back to the Pinax v1 write path.

#### Scenario: Sidecar receives domain-agnostic bounded records
- **WHEN** Pinax calls `inferrum-lancedb-sidecar rebuild`
- **THEN** the request SHALL use `schema_version=inferrum.sidecar.v1`, `domain=kb`, the Pinax KB table, and canonical `records`
- **AND** record metadata SHALL be treated as opaque by Inferrum
- **AND** the request SHALL NOT include `chunk_text`, full note bodies, absolute private paths, raw provider payloads, Authorization headers, cookies, credentials, or unknown metadata fields.

### Requirement: KB SHALL support OpenAI and Ollama embedding providers

Pinax SHALL support cloud and local embedding providers through the Inferrum provider registry while keeping Markdown vaults and KB projections local-first and binding each active generation to one exact provider/model identity/dimension.

#### Scenario: Rebuild with OpenAI provider writes only local projection
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json`
- **THEN** Pinax SHALL scan local notes, request embeddings through the Inferrum OpenAI provider, and build a local staging generation through the unified LanceDB sidecar
- **AND** the command SHALL NOT write Markdown note bodies, provider state, Git state, Cloud Sync state, or remote sync objects
- **AND** machine output SHALL include provider/model/backend/generation facts without raw provider payloads or credentials.

#### Scenario: Rebuild with Ollama provider is local-only and model-specific
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider ollama --model pinax-qwen3-embedding:lowmem --vault ./my-notes --json`
- **THEN** Pinax SHALL use the configured loopback Ollama endpoint for embeddings
- **AND** it SHALL separately record the exact model tag, base model digest when available, the exact tag's resolved model-manifest digest, normalized derived profile hash, Ollama daemon version, returned embedding dimension, and local-only status in the generation manifest
- **AND** failure to reach the endpoint, find the exact model, or produce a non-empty fixed-dimension embedding SHALL return a stable diagnostic instead of requiring cloud credentials.

#### Scenario: Provider doctor performs a real embed canary
- **WHEN** the user runs KB provider doctor for Ollama
- **THEN** Pinax SHALL distinguish daemon reachability, exact model availability, and real embedding readiness
- **AND** it SHALL NOT report the provider ready solely because a port, `/api/version`, or `/api/tags` returned success
- **AND** the canary input and output evidence SHALL be non-sensitive and SHALL NOT persist vector values.

#### Scenario: Query model mismatch fails instead of mixing vector spaces
- **WHEN** search or context resolves a provider, exact tag, model-manifest digest, derived profile hash, or dimension that differs from the active generation
- **THEN** Pinax SHALL return a stable model-mismatch failure
- **AND** it SHALL NOT compare a derived tag against only the base model digest
- **AND** it SHALL NOT silently embed the query in a different vector space.

#### Scenario: Provider unit tests do not require a user Ollama daemon
- **WHEN** ordinary Go unit or contract tests exercise the Ollama provider
- **THEN** tests SHALL use a fake local HTTP server or deterministic fixture
- **AND** the real user Ollama instance SHALL only be used by explicitly named component/system canaries that write integration evidence.

### Requirement: LanceDB sidecar protocol SHALL remain backward-compatible

Pinax SHALL migrate all new semantic projection writes to `inferrum.sidecar.v1`. During the documented compatibility window, it SHALL retain only a Pinax-owned `legacy_v1_readonly` parser for existing projection data; it SHALL NOT retain, start, or write through the old Python sidecar binary.

#### Scenario: New rebuild writes only Inferrum v1
- **WHEN** a migrated Pinax build creates or refreshes a KB projection
- **THEN** it SHALL invoke `inferrum-lancedb-sidecar` with `inferrum.sidecar.v1`
- **AND** it SHALL NOT dual-write or silently write `pinax.kb.sidecar.v1`.

#### Scenario: Existing v1 projection remains diagnosable
- **WHEN** a vault contains only a legacy v1 projection during the compatibility window
- **THEN** Pinax doctor/status SHALL report `protocol=pinax.kb.sidecar.v1` and the migration or rollback next action
- **AND** it SHALL report the introduced release N, compatible-through release N+1, removal-eligible release N+2, and current compatibility status; an unreleased development build SHALL NOT expire the window
- **AND** it SHALL NOT label the projection as a Inferrum v1 generation.

#### Scenario: Explicit legacy profile permits reads but rejects mutations
- **WHEN** an operator selects `legacy_v1_readonly` during N or N+1 and an existing valid v1 projection is available
- **THEN** doctor, status, search, and context MAY read that projection through the built-in historical parser with an explicit v1 protocol fact
- **AND** import, rebuild, refresh, activate, rollback, prune, or any other mutation SHALL return a stable read-only error without invoking an old sidecar
- **AND** canonical Markdown and source versions SHALL remain unchanged and normal non-legacy rebuild commands SHALL continue to target Inferrum v1.

#### Scenario: Protocol removal requires a later change
- **WHEN** the compatibility window ends
- **THEN** removal of the historical projection read path SHALL require a separate OpenSpec with migration, deprecation, consumer evidence, and rollback
- **AND** the already retired binary SHALL NOT be restored.
