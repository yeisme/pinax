# version-control Specification

## Purpose
TBD - created by archiving change pinax-versioned-vault-assets. Update Purpose after archive.
## Requirements
### Requirement: Pinax exposes vault version control without user-visible Git command coupling
Pinax SHALL expose vault version operations through `pinax version` commands and SHALL treat Git as an optional backend type rather than the user-facing command namespace.

#### Scenario: Show version backend status
- **WHEN** a user runs `pinax version status --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with command `version.status`, backend type, capability facts, current revision when available, worktree state, snapshot support, changed path support, and read-at-revision support.
- **AND** the command SHALL NOT invoke a system `git` binary or write vault files.

#### Scenario: Create a version snapshot
- **WHEN** a user runs `pinax version snapshot --vault ./my-notes --message "整理前快照" --json`
- **THEN** Pinax SHALL create snapshot evidence through the configured VersionBackend
- **AND** stdout SHALL include snapshot id, backend type, revision id when available, content evidence refs, and a next action for protected apply workflows.

#### Scenario: Hidden Git snapshot alias remains compatible
- **WHEN** a user runs `pinax git snapshot --vault ./my-notes --message "整理前快照" --json`
- **THEN** Pinax SHALL route to the same application service as `pinax version snapshot`
- **AND** root help and next actions SHALL recommend `pinax version snapshot` rather than `pinax git snapshot`.

### Requirement: VersionBackend is pure Go and capability-driven
Pinax SHALL route version status, snapshot, changed path, diff summary, and read-at-revision behavior through a pure Go VersionBackend interface with explicit capability reporting.

VersionBackend SHALL provide version evidence only and SHALL NOT become the truth source for Markdown bodies, asset bytes, record ledger events, or index projections. Unsupported capability errors SHALL report backend capability state without echoing raw diff text, raw revision ranges, provider payloads, tokens, or secret-bearing paths.

#### Scenario: Local backend records content evidence
- **WHEN** the configured backend is `local`
- **AND** a user runs `pinax version snapshot --vault ./my-notes --message "checkpoint" --json`
- **THEN** Pinax SHALL record content hashes, file sizes, modified times, index epoch when available, and record ledger sequence when available
- **AND** it SHALL NOT require Git, network access, provider credentials, or external binaries.

#### Scenario: Unsupported historical read fails clearly
- **WHEN** a user runs `pinax version show notes/a.md --revision abc123 --vault ./my-notes --json`
- **AND** the active backend does not support read-at-revision
- **THEN** Pinax SHALL fail with stable error code `version_read_unavailable`
- **AND** stdout SHALL include backend capability facts and a local next action.
- **AND** stdout and stderr SHALL NOT include raw file content, raw diff hunks, provider payloads, tokens, or unredacted secret-bearing revision strings.

#### Scenario: Changed paths are backend-scoped
- **WHEN** a user runs `pinax version changed --since abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return changed path candidates only if the active backend reports changed-path support
- **AND** each candidate SHALL include path, change kind when known, content evidence when available, and whether the path is a note, asset, or unmanaged file.

### Requirement: Version restore is planned before writing
Pinax SHALL make version restore a planned workflow before it writes Markdown, asset files, record assets, or index projections.

#### Scenario: Plan restore for a note or asset
- **WHEN** a user runs `pinax version restore yeisme --revision abc123 --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve `yeisme` through the shared vault object resolver
- **AND** stdout SHALL contain restore operations, before/after evidence, risk classification, and required snapshot action without modifying vault content.

#### Scenario: Reject ambiguous restore target
- **WHEN** `pinax version restore yeisme --revision abc123 --plan --vault ./my-notes --json` matches multiple notes or assets
- **THEN** Pinax SHALL fail with stable error code `vault_object_ref_ambiguous`
- **AND** stdout SHALL include candidate paths, object kinds, managed statuses, and match fields.

