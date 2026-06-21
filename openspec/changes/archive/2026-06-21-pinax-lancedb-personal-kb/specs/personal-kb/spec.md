## ADDED Requirements

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
