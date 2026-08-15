# Pinax local content and external RAG boundary

## Purpose

Pinax owns Markdown vault content, the local SQLite/GORM index, deterministic
text search, memory, and encrypted sync. Semantic RAG is an external capability
and is not implemented by this project.

## Requirements

### Requirement: Pinax SHALL not own vector retrieval

Pinax SHALL NOT import, start, or call a vector database, embedding provider,
Inferrum, LanceDB, reranker, or RAG sidecar.

#### Scenario: Local search remains available

- **WHEN** the user runs `pinax index refresh` or `pinax search <query>`
- **THEN** Pinax SHALL use its deterministic local projection and SHALL NOT make
  an embedding or vector-store call.

#### Scenario: External RAG handoff

- **WHEN** the user needs semantic retrieval or generated context
- **THEN** Pinax SHALL provide `pinax export markdown <output-dir>` and the
  external RAG owner SHALL manage ingest, embedding, vector storage, reranking,
  and evaluation.

### Requirement: Released KB names SHALL fail closed during migration

Pinax SHALL keep the released `pinax kb` command names parseable for one release
window and SHALL return `error.code=kb_decoupled` without vector I/O.

#### Scenario: Old command returns a handoff

- **WHEN** a user runs a released `pinax kb` subcommand
- **THEN** Pinax SHALL return `kb_decoupled` and a Markdown export next action.

### Requirement: Historical vector files SHALL not be part of the active workspace

Pinax SHALL not recreate or sync `.pinax/kb/**` vector artifacts. Explicitly
enumerated repository and test-workspace artifacts are removed during the
decoupling migration; Markdown vault content and sync data remain authoritative.

#### Scenario: External RAG rebuilds after cleanup

- **WHEN** the user restores a previous Pinax binary or starts an external RAG
  migration
- **THEN** the external owner SHALL rebuild any required projection from
  `pinax export markdown`, without requiring Pinax vector files.
