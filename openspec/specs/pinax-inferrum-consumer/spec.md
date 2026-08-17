# Pinax external RAG ownership

## Purpose

Inferrum, LanceDB, embedding providers, and vector retrieval are no longer Pinax
dependencies. A separate RAG project may choose and own those components.

## Requirements

### Requirement: Pinax SHALL not consume Inferrum

Pinax SHALL NOT import the Inferrum Go module, execute an Inferrum sidecar, or
persist Inferrum/LanceDB projection data.

#### Scenario: Dependency audit

- **WHEN** Pinax builds the active CLI
- **THEN** the module graph and vendor tree SHALL contain no Inferrum package or
  sidecar dependency.

### Requirement: External RAG SHALL be reached through an explicit handoff

External RAG integration SHALL start from an explicit Markdown export rather than
Pinax provider calls.

#### Scenario: Handoff keeps ownership separate

- **WHEN** an agent needs semantic retrieval
- **THEN** it SHALL export Markdown with a Pinax command and call the external
  RAG contract; Pinax SHALL not expose provider credentials, vectors, or raw
  provider payloads.
