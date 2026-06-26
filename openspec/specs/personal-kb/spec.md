# personal-kb Specification

## Purpose
TBD - created by archiving change pinax-lancedb-personal-kb. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL manage a local semantic KB projection

Pinax SHALL provide a `pinax kb` command group that keeps Markdown vault content as the source of truth while maintaining a rebuildable local semantic projection with `backend=lancedb`.

#### Scenario: Rebuild local semantic projection
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider fake --vault ./my-notes --json`
- **THEN** Pinax SHALL scan registered Markdown notes through app-owned vault behavior
- **AND** it SHALL write a local projection under `.pinax/kb/lancedb/`
- **AND** JSON facts SHALL include backend, provider, model, document count, chunk count, and `sync_vectors=false`.

#### Scenario: Missing LanceDB sidecar is actionable
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --vault ./my-notes --json` without an available sidecar
- **THEN** Pinax SHALL return `error.code=kb_sidecar_unavailable`
- **AND** it SHALL include an install or configuration next step.

#### Scenario: Sidecar receives bounded projection data
- **WHEN** Pinax calls `pinax-lancedb-sidecar rebuild`
- **THEN** the request SHALL use `schema_version=pinax.kb.sidecar.v1`
- **AND** it SHALL include vectors, metadata, and bounded previews
- **AND** it SHALL NOT include `chunk_text`, full note bodies, raw provider payloads, Authorization headers, cookies, or credentials.

### Requirement: Pinax SHALL import Markdown and text into the vault before indexing

Pinax SHALL import Markdown and plain-text sources into the Markdown vault before they become part of the semantic KB projection.

#### Scenario: Dry-run import is read-only
- **WHEN** the user runs `pinax kb import ./source --include "*.txt" --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return an import plan
- **AND** it SHALL NOT write Markdown, `.pinax/` receipts, LanceDB projection files, provider state, or remote objects.

#### Scenario: Confirmed import writes normalized Markdown
- **WHEN** the user runs `pinax kb import ./source --include "*.txt" --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL write normalized Pinax Markdown notes through the app service
- **AND** it SHALL refresh the local index and write redacted receipt evidence.

### Requirement: Semantic search and context SHALL be bounded for agents

Pinax SHALL expose semantic `search` and `context` commands that return bounded chunk previews and metadata rather than full note bodies by default.

#### Scenario: Agent reads semantic facts
- **WHEN** the user runs `pinax kb search "semantic projection" --agent`
- **THEN** stdout SHALL include stable key=value facts for command, status, backend, provider, model, and match counts
- **AND** stdout SHALL NOT include localized prose, provider payloads, secrets, or raw note bodies.

#### Scenario: Unknown embedding provider is rejected
- **WHEN** the user runs `pinax kb rebuild --provider gemni --vault ./my-notes --json`
- **THEN** Pinax SHALL return `error.code=provider_invalid`
- **AND** it SHALL NOT silently fall back to another provider.

#### Scenario: Context output is bounded
- **WHEN** the user runs `pinax kb context "task" --json`
- **THEN** JSON data SHALL include bounded hit previews and source metadata
- **AND** it SHALL NOT include full `body`, `raw_body`, authorization headers, cookies, or embedding provider payloads.

### Requirement: Cloud Sync SHALL not synchronize semantic projection files

Pinax SHALL treat LanceDB projection files as local rebuildable artifacts. Cloud Sync SHALL continue to synchronize encrypted vault revisions and SHALL NOT sync `.pinax/kb/lancedb/` as authoritative data.

#### Scenario: Multi-device semantic rebuild
- **WHEN** a device pulls a committed Cloud Sync revision
- **THEN** it MAY run `pinax kb refresh --vault <vault>` to rebuild its local semantic projection
- **AND** the remote transport SHALL NOT receive plaintext notes, LanceDB files, vectors, Gemini payloads, or provider credentials.

### Requirement: KB SHALL expose additive embedding provider discovery

Pinax SHALL expose KB embedding provider discovery without changing the existing `gemini`, `fake`, `lancedb`, or fake backend defaults.

#### Scenario: Provider list reports configured status without secrets

- **WHEN** the user runs `pinax kb provider list --vault ./my-notes --json`
- **THEN** stdout SHALL be a valid Pinax JSON envelope
- **AND** the response SHALL include provider ids for `gemini`, `openai`, `ollama`, and `fake`
- **AND** provider facts SHALL include default model, local-only status, and configured status where available
- **AND** stdout, stderr, events, docs, fixtures, and integration evidence SHALL NOT include raw tokens, Authorization headers, provider payloads, cookies, hidden prompts, private tool arguments, or full note bodies.

#### Scenario: Provider doctor diagnoses missing credentials safely

- **WHEN** the user runs `pinax kb provider doctor --provider openai --model text-embedding-3-small --vault ./my-notes --json` without configured credentials
- **THEN** Pinax SHALL return a valid failure envelope with a stable error code and a concrete next action
- **AND** the diagnostic SHALL mention only credential source type or env var name, not the credential value.

### Requirement: KB SHALL support OpenAI and Ollama embedding providers

Pinax SHALL support cloud and local embedding providers through the semantic provider registry while keeping Markdown vaults and KB projections local-first.

#### Scenario: Rebuild with OpenAI provider writes only local projection

- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json`
- **THEN** Pinax SHALL scan local notes, request embeddings through the OpenAI provider, and rebuild `.pinax/kb/lancedb/` through the LanceDB sidecar
- **AND** the command SHALL NOT write Markdown note bodies, provider state, Git state, Cloud Sync state, or remote sync objects
- **AND** machine output SHALL include provider/model/backend facts without raw provider payloads or credentials.

#### Scenario: Rebuild with Ollama provider is local-only

- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider ollama --model nomic-embed-text --vault ./my-notes --json`
- **THEN** Pinax SHALL use the configured local Ollama endpoint for embeddings
- **AND** failure to reach the endpoint SHALL return a stable diagnostic instead of requiring cloud credentials
- **AND** tests SHALL use a fake local HTTP server rather than a real user Ollama instance.

### Requirement: LanceDB sidecar protocol SHALL remain backward-compatible

Pinax SHALL keep the LanceDB sidecar protocol compatible while allowing optional provider metadata for diagnostics and future index tuning.

#### Scenario: Sidecar accepts old and new rebuild requests

- **WHEN** `pinax-lancedb-sidecar rebuild` receives a `pinax.kb.sidecar.v1` request without provider metadata
- **THEN** it SHALL continue to rebuild the local store successfully
- **AND** when the request includes optional provider, model, embedding dimension, distance metric, or collection metadata, the sidecar SHALL accept those fields without requiring existing clients to send them.

#### Scenario: Search remains bounded and provider-aware

- **WHEN** the user runs `pinax kb search "release workflow" --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value facts for command, status, backend, provider, model, matches, and total
- **AND** stdout SHALL NOT include full note bodies, raw provider payloads, credentials, Authorization headers, cookies, or hidden prompts.

