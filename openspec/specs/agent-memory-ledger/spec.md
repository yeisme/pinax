# agent-memory-ledger Specification

## Purpose
TBD - created by archiving change pinax-agent-memory-ledger. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL provide a non-vector agent memory ledger

Pinax SHALL provide a local-first `pinax memory` command group that stores typed agent memory records without requiring embeddings, LanceDB, or any vector backend.

#### Scenario: Capture confirmed fact memory
- **WHEN** the user runs `pinax memory capture --type fact --subject pinax --predicate release_workflow --object "tag push triggers GitHub Actions" --source docs/operations/release-packaging.md --vault ./my-notes --json`
- **THEN** Pinax SHALL write a confirmed memory record through the app service
- **AND** the record SHALL include type, subject, predicate, object, status, source citation, created timestamp, and vault scope
- **AND** JSON facts SHALL include `command=memory.capture`, record id, type, status, and source.

#### Scenario: Dry-run capture is read-only
- **WHEN** the user runs `pinax memory capture --type decision --subject pinax --object "Use structured memory" --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return the planned record
- **AND** it SHALL NOT write Markdown, SQLite rows, FTS rows, receipts, or remote objects.

### Requirement: Memory records SHALL preserve source citations and lifecycle state

Pinax SHALL store memory records with source citations and lifecycle state so agent context can distinguish confirmed facts from drafts, superseded records, expired records, and rejected candidates.

#### Scenario: Default recall excludes unconfirmed or obsolete records
- **WHEN** the memory ledger contains records with `draft`, `confirmed`, `superseded`, `expired`, and `rejected` statuses
- **AND** the user runs `pinax memory recall "release workflow" --vault ./my-notes --json`
- **THEN** Pinax SHALL return only matching `confirmed` records by default
- **AND** it SHALL omit `draft`, `superseded`, `expired`, and `rejected` records unless the user explicitly asks for them.

#### Scenario: Superseded memory remains auditable
- **WHEN** a confirmed record supersedes an older record
- **THEN** Pinax SHALL preserve the older record with `status=superseded`
- **AND** `pinax memory list --include-superseded --json` SHALL be able to show both records and the supersession link.

### Requirement: Memory recall SHALL be explainable without vectors

Pinax SHALL rank memory recall using deterministic non-vector signals: vault scope, type/entity filters, FTS keyword score, recency, confidence, source authority, and lifecycle state.

#### Scenario: Context includes recall reasons
- **WHEN** the user runs `pinax memory context "prepare next release" --entity pinax --limit 12 --vault ./my-notes --json`
- **THEN** each returned memory item SHALL include a `recall_reason`
- **AND** the reason SHALL name the matching signals, such as entity match, type match, source kind, status, confidence, or keyword score.

#### Scenario: Agent output is bounded and machine-readable
- **WHEN** the user runs `pinax memory context "prepare next release" --entity pinax --limit 12 --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value facts for command, status, scope, match count, record ids, and memory types
- **AND** stdout SHALL NOT include localized prose, raw prompts, provider payloads, secrets, cookies, Authorization headers, or full private note bodies.

### Requirement: Memory ledger SHALL be a local rebuildable projection

Pinax SHALL treat `.pinax/memory/` as a local CLI-authored structured asset and SHALL NOT require it to be synchronized as authoritative Cloud Sync data.

#### Scenario: Projection can be rebuilt from sources
- **WHEN** a user has Markdown notes, OpenSpec docs, release evidence, and git metadata available in the vault or repository
- **THEN** Pinax MAY rebuild memory ledger projections from those sources
- **AND** the rebuild SHALL preserve source citations and shall not invent confirmed facts from unconfirmed inference.

#### Scenario: Cloud Sync does not upload memory projection as authority
- **WHEN** Cloud Sync publishes encrypted vault revisions
- **THEN** it SHALL NOT treat `.pinax/memory/` SQLite or FTS files as authoritative cross-device memory state
- **AND** another device MAY rebuild or recapture local memory from synchronized Markdown and project evidence.

### Requirement: Memory recall SHALL use deterministic multi-signal ranking

Pinax SHALL rank memory recall with deterministic non-vector signals while keeping Memory separate from KB semantic search.

#### Scenario: Ranking combines query, entity, source, confidence, and freshness

- **WHEN** the memory ledger contains multiple confirmed records matching `release workflow`
- **AND** the user runs `pinax memory recall "release workflow" --entity pinax --vault ./my-notes --json`
- **THEN** Pinax SHALL rank candidates using keyword match, entity match, type affinity, source authority, confidence, freshness, lifecycle, and task-fitness signals
- **AND** the result SHALL be stable across repeated runs with the same ledger and query
- **AND** it SHALL NOT use embeddings, LanceDB, provider calls, remote services, or raw note body search outside the local memory projection.

#### Scenario: Source authority and confidence affect tie-breaks

- **WHEN** two confirmed records have equivalent query and entity matches
- **AND** one record cites an OpenSpec source while the other cites a generic file source
- **THEN** the OpenSpec-sourced record SHOULD rank higher
- **AND** `recall_reason` or `signals` SHALL explain the source and confidence contribution.

### Requirement: Memory recall SHALL collapse obsolete and duplicate records by default

Pinax SHALL avoid filling agent context with duplicate or obsolete records while preserving auditability through list commands.

#### Scenario: Superseded records remain auditable but are not default context

- **WHEN** a confirmed memory record supersedes an older record
- **AND** the user runs `pinax memory context "prepare next release" --entity pinax --vault ./my-notes --json`
- **THEN** Pinax SHALL return the current confirmed record by default
- **AND** it SHALL omit the superseded old record from default context
- **AND** `pinax memory list --include-superseded --vault ./my-notes --json` SHALL still be able to show the old record and supersession link.

#### Scenario: Duplicate subject and predicate records are collapsed

- **WHEN** multiple confirmed records share the same normalized subject and predicate
- **THEN** default recall SHALL return the highest-scoring record for that subject and predicate
- **AND** lower-scoring duplicates SHALL remain stored and auditable, not deleted.

### Requirement: Memory recall explanations SHALL expose bounded signal breakdowns

Pinax SHALL expose recall explanations that help agents and users understand why records were selected without leaking private note bodies or provider payloads.

#### Scenario: JSON output includes optional signal breakdown

- **WHEN** the user runs `pinax memory recall "release workflow" --entity pinax --vault ./my-notes --json`
- **THEN** each returned match SHALL keep `score` and `recall_reason`
- **AND** each match MAY include a `signals` object with bounded numeric contributions such as keyword, entity, source, confidence, freshness, lifecycle, and task_fitness
- **AND** adding `signals` SHALL NOT remove or rename existing JSON envelope fields or existing match fields.

#### Scenario: Agent output remains low-token and body-safe

- **WHEN** the user runs `pinax memory context "prepare next release" --entity pinax --limit 12 --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value facts for command, status, scope, match count, memory types, optional top score, and bounded recall reasons
- **AND** stdout SHALL NOT include localized prose, full memory bodies, raw prompts, provider payloads, Authorization headers, cookies, tokens, hidden system prompts, private tool arguments, or complete chain-of-thought.

