## MODIFIED Requirements

### Requirement: Pinax SHALL manage a local semantic KB projection

Pinax SHALL provide a `pinax kb` command group that keeps Markdown vault content as the source of truth while maintaining a rebuildable local semantic projection with `backend=lancedb` through Inferrum.

#### Scenario: Rebuild local semantic projection
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider fake --vault ./my-notes --json`
- **THEN** Pinax SHALL scan registered Markdown notes through app-owned vault behavior
- **AND** it SHALL write a local projection under `.pinax/kb/` through `github.com/yeisme/inferrum`
- **AND** JSON facts SHALL include backend, provider, model, document count, chunk count, and `sync_vectors=false`.

#### Scenario: Missing LanceDB sidecar is actionable
- **WHEN** the user runs `pinax kb rebuild --backend lancedb --vault ./my-notes --json` without an available `inferrum-lancedb-sidecar`
- **THEN** Pinax SHALL return `error.code=kb_sidecar_unavailable`
- **AND** it SHALL include an Inferrum install or configuration next step.

#### Scenario: Sidecar receives bounded projection data
- **WHEN** Pinax calls `inferrum-lancedb-sidecar rebuild`
- **THEN** the request SHALL use `schema_version=inferrum.sidecar.v1`, `domain=kb`, the Pinax KB table, and canonical `records`
- **AND** it SHALL include vectors, opaque safe metadata, and bounded previews
- **AND** it SHALL NOT include `chunk_text`, full note bodies, raw provider payloads, Authorization headers, cookies, credentials, absolute private paths, or unknown metadata fields.

## REMOVED Requirements

### Requirement: LanceDB sidecar protocol SHALL remain backward-compatible

**Reason**: Pinax no longer owns or distributes a Python LanceDB sidecar protocol. New writes use the Inferrum-owned `inferrum.sidecar.v1` contract exclusively.

**Migration**: Install or configure `inferrum-lancedb-sidecar`. Existing `pinax.kb.sidecar.v1` projection data may only be read through the Pinax-owned `legacy_v1_readonly` parser; Pinax never starts or writes through the retired binary.
