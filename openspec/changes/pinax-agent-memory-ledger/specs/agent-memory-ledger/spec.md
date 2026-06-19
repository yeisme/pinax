## ADDED Requirements

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
