# Pinax KB 解耦变更

## REMOVED Requirements

### Requirement: Pinax SHALL own an in-process vector KB

Pinax SHALL no longer own the vector database, embedding provider, Inferrum,
LanceDB, reranker, sidecar, semantic generation, or retrieval implementation
previously exposed by `pinax kb`.

#### Scenario: Vector rebuild is retired

- **WHEN** a user invokes a former vector rebuild, refresh, provider, evaluation,
  activation, rollback, or prune operation
- **THEN** Pinax SHALL NOT start a provider or sidecar and SHALL NOT read or
  write vector projection files

### Requirement: Pinax SHALL own KB review API data

Pinax SHALL no longer generate vector generation, citation, provider, or review
data for `/v1/kb/review/*`.

#### Scenario: Review route is retired

- **WHEN** a client requests a former KB review route
- **THEN** Pinax SHALL return HTTP 404 and no generation, citation, provider
  credential, vector payload, or note body

## ADDED Requirements

### Requirement: Pinax SHALL provide an explicit external RAG handoff

Pinax SHALL keep Markdown as the source of truth and SHALL expose
`pinax export markdown <output-dir>` as the handoff to an external RAG owner.
The external owner SHALL manage ingest, chunking, embedding, vector storage,
reranking, context, and evaluation.

#### Scenario: External RAG export

- **WHEN** a user needs semantic retrieval or generated context
- **THEN** Pinax SHALL export bounded Markdown and source metadata without
  persisting vectors, provider payloads, or external RAG state

### Requirement: Released KB command names SHALL fail closed during migration

Pinax SHALL remove the `pinax kb` command tree entirely rather than keep a
fail-closed compatibility stub. Former `pinax kb` invocations SHALL surface
cobra's unknown-command error.

#### Scenario: Old CLI is safely redirected

- **WHEN** a user runs a former `pinax kb` command
- **THEN** the command SHALL fail with an unknown-command error without
  invoking vector code, and `docs/commands/kb.md` SHALL document the Markdown
  export and deterministic local search replacements

### Requirement: Removed configuration SHALL be inert

Pinax SHALL remove `kb.sidecar.*` and `PINAX_KB_*` runtime configuration and
SHALL NOT read `kb.*` keys. Leftover `kb.*` keys in existing YAML configs
SHALL be inert.

#### Scenario: Old setting is inert

- **WHEN** a vault config still contains a `kb.sidecar.*` key
- **THEN** Pinax SHALL ignore it, SHALL NOT start any sidecar process, and
  default configuration SHALL NOT define any `kb.*` key

### Requirement: Historical vector artifacts SHALL be removed from Pinax-owned workspaces

The migration SHALL remove explicitly enumerated repository and test-workspace
`.pinax/kb/**` artifacts. It SHALL NOT delete Markdown vault content, encrypted
sync revisions, or object-storage data.

#### Scenario: Historical workspace cleanup

- **WHEN** the migration cleanup is applied
- **THEN** the enumerated repository and test-workspace `.pinax/kb/**` paths SHALL
  be absent and the Markdown vault and sync data SHALL remain unchanged

#### Scenario: Rollback after cleanup

- **WHEN** an external RAG migration needs to be paused
- **THEN** restoring the previous Pinax binary or commit SHALL restore code
  compatibility, while the external RAG projection SHALL be rebuilt from Markdown
  export if needed
