# vault-record-ledger Specification

## Purpose
TBD - created by archiving change pinax-vault-record-ledger. Update Purpose after archive.
## Requirements
### Requirement: Record ledger owns note identity and lifecycle
Pinax SHALL treat the CLI-authored record ledger as the machine source of truth for note identity, lifecycle state, schema constraints, tombstones, and repair evidence while keeping Markdown files as the source of truth for user-authored body content.

#### Scenario: Initialize record ledger
- **WHEN** a user runs `pinax record init --vault ./my-notes --json`
- **THEN** Pinax SHALL create `.pinax/records/events.jsonl`, `.pinax/records/notes.json`, `.pinax/records/schemas.json`, and `.pinax/records/tombstones.json` through the application service
- **AND** stdout SHALL include ledger path, schema version, record count, event sequence, and status facts.

#### Scenario: Note identity is resolved from ledger
- **GIVEN** a Markdown note and the record ledger disagree about `note_id`
- **WHEN** Pinax resolves the note for machine operations
- **THEN** Pinax SHALL prefer the ledger identity for machine behavior
- **AND** it SHALL report `record_frontmatter_mismatch` instead of silently trusting the edited frontmatter.

### Requirement: Record events are append-only and replayable
Pinax SHALL write note record changes as append-only events and SHALL maintain materialized registry files as replayable projections of those events.

#### Scenario: Append note lifecycle event
- **WHEN** a CLI-approved note create, move, rename, archive, delete, restore, metadata, tag, or schema operation succeeds
- **THEN** Pinax SHALL append one redacted record event with schema version, event id, sequence, kind, actor, source command, note id, before facts, after facts, content hash, and created time
- **AND** it SHALL update the registry projection through the ledger service.

#### Scenario: Tag mutation records metadata evidence
- **WHEN** a user runs `pinax note tag add note_123 research --vault ./my-notes --json`, `pinax note tag remove note_123 inbox --vault ./my-notes --json`, or `pinax note tag set note_123 work reference --vault ./my-notes --json`
- **THEN** Pinax SHALL append a record metadata event or a dedicated tag metadata event after the Markdown write succeeds
- **AND** stdout SHALL include stable facts for `record_event`, `ledger_seq`, `record_version`, and version evidence when available
- **AND** repeated no-op tag operations SHALL be idempotent and SHALL NOT duplicate downstream side effects.

#### Scenario: Rejected tag input does not write ledger
- **WHEN** a tag operation fails validation before writing Markdown
- **THEN** Pinax SHALL NOT append a record event or update the registry projection
- **AND** the error projection SHALL include a stable error code and a runnable correction hint.

### Requirement: Record events capture version evidence
Pinax SHALL attach version evidence to record events when a supported version backend is available, and SHALL preserve enough evidence for stale detection, audit, restore planning, and version-aware search without defaulting to full diff storage.

#### Scenario: Capture Git version evidence for a mutation
- **GIVEN** the vault is inside a Git repository
- **WHEN** a CLI-approved note mutation succeeds
- **THEN** the appended record event SHALL include version backend, HEAD revision, worktree state, file blob id when available, content hash, and diff summary hash when the worktree is dirty
- **AND** the event SHALL NOT embed raw full diff text unless a command explicitly requests a saved diff artifact.

#### Scenario: Capture fallback version evidence without Git
- **GIVEN** the vault has no supported version backend
- **WHEN** a CLI-approved note mutation succeeds
- **THEN** the appended record event SHALL include `version.backend=none`, ledger sequence, content hash, file size, and modified time facts
- **AND** stdout SHALL include a next action for configuring Git or another version backend when version-aware restore is requested.

#### Scenario: Capture binary artifact evidence
- **GIVEN** a note references a binary attachment managed by a supported version backend or content-addressed store
- **WHEN** Pinax records attachment or note mutation evidence
- **THEN** the record event SHALL store attachment object id, backend revision, checksum, size, and MIME facts
- **AND** it SHALL NOT write binary payload bytes into Markdown, record events, search index text columns, stdout, stderr, or fixtures.

### Requirement: Version backend capabilities are explicit
Pinax SHALL route Git and non-Git version management through a version backend adapter and SHALL expose backend capabilities in machine-readable output.

#### Scenario: Detect Git backend capabilities
- **WHEN** a user runs `pinax record status --vault ./my-notes --json` inside a Git-backed vault
- **THEN** stdout SHALL include `version_backend=git`, current HEAD, worktree state, and capability facts for snapshot, diff summary, file revision, and read-at-revision.

#### Scenario: Detect unsupported history reads
- **GIVEN** the configured version backend cannot read files at historical revisions
- **WHEN** a user requests record history or version-aware search for a historical revision
- **THEN** Pinax SHALL fail with stable error code `version_read_unavailable`
- **AND** it SHALL include backend capability facts and a local next action.

### Requirement: Ledger mutations are single-writer and observable
Pinax SHALL serialize ledger event sequence allocation and registry materialization through one mutation coordinator while allowing independent read paths and bounded pre-processing workers.

#### Scenario: Concurrent note mutations preserve event order
- **GIVEN** multiple CLI operations attempt to create, rename, archive, or delete notes concurrently in the same vault
- **WHEN** Pinax accepts the mutations
- **THEN** record events SHALL receive strictly increasing sequences without gaps or duplicates
- **AND** registry materialization SHALL reflect the same event order.

#### Scenario: Mutation coordinator exposes diagnostics
- **WHEN** a user runs `pinax record status --vault ./my-notes --json`
- **THEN** stdout SHALL include ledger sequence, registry version, last mutation duration when available, pending mutation count when available, and stale or replay diagnostics
- **AND** diagnostics SHALL NOT include raw diff text, binary payloads, provider secrets, or unredacted traces.

#### Scenario: Atomic values do not own lifecycle state
- **WHEN** Pinax updates note lifecycle, repair state, or tombstone state
- **THEN** the update SHALL go through domain/application transition rules
- **AND** atomics SHALL be limited to simple counters, epoch markers, cancellation flags, or snapshot pointers.

### Requirement: Record scanning respects memory budgets
Pinax SHALL scan large vaults with bounded worker queues and SHALL avoid retaining all Markdown bodies or binary payloads in memory.

#### Scenario: Low-memory record scan
- **WHEN** a user runs `pinax record status --vault ./my-notes --memory-budget low --json`
- **THEN** Pinax SHALL reduce worker count and batch size for scan work
- **AND** it SHALL stream file hashing and metadata extraction without retaining full note bodies after each note is processed.

#### Scenario: Cancel long record scan
- **WHEN** a long record scan receives context cancellation or process interruption
- **THEN** Pinax SHALL stop scheduling new scan work
- **AND** it SHALL return partial progress or checkpoint facts without corrupting record assets.

### Requirement: Existing Markdown notes can be adopted without losing portability
Pinax SHALL import existing Markdown notes into the record ledger through an explicit adoption plan rather than assuming all files are already trustworthy Pinax records.

#### Scenario: Plan record adoption
- **WHEN** a user runs `pinax record adopt --vault ./my-notes --plan --json`
- **THEN** Pinax SHALL scan Markdown notes inside the vault boundary and return adoption operations for unregistered notes, missing frontmatter mirrors, duplicate note ids, and path conflicts
- **AND** it SHALL NOT modify Markdown files, `.pinax/` assets, Git state, provider state, or remote services.

#### Scenario: Apply record adoption
- **WHEN** a user runs `pinax record adopt --vault ./my-notes --apply --yes --json`
- **THEN** Pinax SHALL create ledger records and record events for approved existing notes through the application service
- **AND** it SHALL leave frontmatter rewriting to an explicit metadata plan unless the adoption operation was approved to write the mirror.

### Requirement: Frontmatter is a portable mirror of ledger facts
Pinax SHALL keep Pinax-managed frontmatter as a portable mirror and validation checkpoint, not as unquestioned authority for machine identity or lifecycle facts.

#### Scenario: Repair missing mirror fields
- **GIVEN** a ledger record exists and the Markdown frontmatter is missing `schema_version` or `note_id`
- **WHEN** a user runs `pinax metadata plan --vault ./my-notes --json`
- **THEN** Pinax SHALL propose mirror additions from the ledger record
- **AND** it SHALL NOT write the Markdown file until `metadata apply` receives explicit approval.

#### Scenario: Detect conflicting mirror fields
- **GIVEN** a Markdown file has a `note_id` that conflicts with the ledger record for its path
- **WHEN** a user runs `pinax record status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `record_frontmatter_mismatch` with ledger note id, frontmatter note id, path, and recommended repair action.

### Requirement: External edits become reconciliation facts
Pinax SHALL treat filesystem moves, renames, deletes, and external Markdown edits as reconciliation input that creates issues or plans before machine records are changed.

#### Scenario: Detect external file move
- **GIVEN** a registered note file has moved outside a Pinax command
- **WHEN** a user runs `pinax record status --vault ./my-notes --json`
- **THEN** Pinax SHALL compare ledger path, filesystem path candidates, content hash, and frontmatter mirror
- **AND** it SHALL report a pending reconciliation issue rather than immediately rewriting the ledger.

#### Scenario: Detect active record without file
- **GIVEN** the ledger has an active note record whose current path no longer exists
- **WHEN** Pinax scans record status
- **THEN** Pinax SHALL report `record_file_missing`
- **AND** it SHALL include next actions for repair plan generation, restore, tombstone, or manual review.

### Requirement: Record adoption accepts scoped vault object queries
Pinax SHALL allow record adoption to target a specific unresolved Markdown candidate while preserving full-vault adoption planning when no query is provided.

#### Scenario: Plan adoption for a single Markdown file
- **WHEN** a user runs `pinax record adopt yeisme --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve `yeisme` using adoptable scope
- **AND** stdout SHALL include adoption operations only for the uniquely matched unmanaged Markdown file.
- **AND** no Markdown, record, asset, index, version, provider, or Git state SHALL be modified.

#### Scenario: Full-vault adoption remains available
- **WHEN** a user runs `pinax record adopt --plan --vault ./my-notes --json` without a query
- **THEN** Pinax SHALL scan all adoptable Markdown notes inside the vault boundary and return the full adoption plan.

### Requirement: Record events include version and asset evidence without payload leakage
Pinax SHALL attach version evidence and asset references to record events when available without embedding raw diff text or binary payload bytes.

#### Scenario: Note mutation records version evidence
- **WHEN** a CLI-approved note create, rename, move, archive, delete, restore, metadata, or adoption operation succeeds
- **THEN** the appended record event SHALL include backend type, snapshot or revision id when available, content hash, file size, ledger sequence, and worktree status when available
- **AND** it SHALL NOT depend on a system Git binary.

#### Scenario: Note references asset evidence
- **WHEN** a note operation creates or changes asset references
- **THEN** the record event SHALL include asset ids and content hashes as evidence refs
- **AND** it SHALL NOT write asset bytes into record event JSONL.

