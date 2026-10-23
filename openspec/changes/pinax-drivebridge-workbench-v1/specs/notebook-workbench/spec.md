## ADDED Requirements

### Requirement: Source-first rich notebook editing

A rich or composed view SHALL be a fixed-revision preview with `source_anchor` and `origin`. Editing SHALL target the original Markdown source and SHALL pass through proposal, human confirmation, apply, snapshot, revision guard and receipt. A renderer SHALL NOT write a composed projection or a second vault.

#### Scenario: Embedded source is unavailable

- **WHEN** an embedded target is unavailable, ambiguous, cyclic, beyond depth, or outside the authorized scope
- **THEN** the workbench shows an explicit unavailable/ambiguous/cycle/depth/unsupported state and does not submit a parent-document write

### Requirement: Closed desktop DTO boundary

Electron main/preload SHALL hold loopback bearer, session and file-selection tickets. Renderer DTOs SHALL contain opaque identities, revisions, bounded content projections and recovery keys only; they SHALL NOT contain `.pinax` paths, SQLite handles, bearer tokens, raw provider payloads or full reasoning.

#### Scenario: Draft and recovery

- **WHEN** a user edits a bound note or an apply response is lost
- **THEN** the draft remains memory-only and the recovery slot contains only the original opaque operation key for `operation.show` or `operation.reconcile`

### Requirement: Attachment proposal boundary

Attachment upload or replacement SHALL use the existing bounded input request contract and SHALL atomically guard note identity, MIME/size, revision, snapshot, body reference update and receipt. Renderer code SHALL never submit an arbitrary filesystem path as a write instruction.

#### Scenario: Unsupported attachment

- **WHEN** MIME, size, placement or note revision is invalid
- **THEN** Pinax rejects the proposal before mutation and leaves source bytes, references, revision and receipt unchanged
