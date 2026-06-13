# folder-operations Specification

## Purpose
TBD - created by archiving change pinax-folder-cli-api-operations. Update Purpose after archive.
## Requirements
### Requirement: Folder operations are Pinax-authored vault operations
Pinax SHALL expose folder lifecycle operations through `pinax folder` commands and application services rather than requiring users or agents to run raw filesystem commands.

#### Scenario: Create folder through Pinax
- **WHEN** a user runs `pinax folder create projects/research --purpose notes --vault ./my-notes --json`
- **THEN** Pinax SHALL create the vault-relative directory through the application service
- **AND** stdout SHALL include command `folder.create`, folder path, purpose, write status, registry evidence, and index update facts.

#### Scenario: Reject unsafe folder path
- **WHEN** a user runs `pinax folder create ../outside --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `unsafe_folder_path`
- **AND** it SHALL NOT create, move, remove, or register any path outside the vault boundary.

#### Scenario: List folders from Pinax projection
- **WHEN** a user runs `pinax folder list --include-empty --vault ./my-notes --json`
- **THEN** Pinax SHALL return folders discovered from the vault filesystem and CLI-authored folder registry
- **AND** each folder SHALL include path, purpose, managed status, empty state, note count, asset count, and evidence source when available.

### Requirement: Folder mutations are plan-aware and hookable
Pinax SHALL make non-trivial folder mutations previewable, approvable, auditable, and index-aware.

#### Scenario: Rename folder with dry-run
- **WHEN** a user runs `pinax folder rename inbox archive --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report matched files, affected notes, target paths, conflicts, link risks, snapshot requirement, and planned hook events
- **AND** it SHALL NOT modify Markdown files, folder registry, index database, Git state, provider state, or remote services.

#### Scenario: Apply folder rename with approval
- **WHEN** a user runs `pinax folder rename inbox archive --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL apply the rename through the application service
- **AND** it SHALL update affected note frontmatter folder values, append folder lifecycle events, update folder registry, and refresh or update affected index projections.

#### Scenario: Delete folder defaults to empty-only safety
- **WHEN** a user runs `pinax folder delete drafts --empty-only --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove the folder only when it contains no vault files and no registered empty-folder evidence that must be preserved
- **AND** non-empty folders SHALL fail with `folder_not_empty` and return a repair or review plan action rather than recursively deleting content.

### Requirement: Folder operations are exposed through registered REST and RPC capabilities
Pinax SHALL expose folder read and mutation capabilities through the same remote capability registry used by API route listing and OpenAPI export.

#### Scenario: Folder routes appear in API capabilities
- **WHEN** a user runs `pinax api routes --vault ./my-notes --json`
- **THEN** the route registry SHALL include REST and RPC folder capabilities for list, show, create, rename, move, delete, adopt, and repair-plan
- **AND** each route SHALL include readonly, body_allowed, approval_required, snapshot_required, command, capability id, and schema version metadata.

#### Scenario: REST folder create uses application service
- **WHEN** a local API client sends `POST /v1/folders` with JSON body `{"path":"projects/research","purpose":"notes","yes":true}`
- **THEN** the REST handler SHALL call the folder application service and return a Pinax projection envelope
- **AND** the handler SHALL NOT call filesystem APIs directly.

#### Scenario: RPC folder rename uses the same projection
- **WHEN** an RPC client calls `Pinax.Folder.Rename` with `path`, `target_path`, `dry_run`, and `yes` params
- **THEN** the dispatcher SHALL return the same command projection shape as `pinax folder rename`
- **AND** RPC and REST capability metadata SHALL remain aligned.

### Requirement: Remote folder writes are explicitly gated
Pinax SHALL keep remote folder mutations disabled by default and require explicit approval, optional snapshot evidence, and idempotency when writes are enabled.

#### Scenario: Readonly API rejects folder mutation
- **WHEN** `pinax api serve --readonly --vault ./my-notes` receives a folder mutation request
- **THEN** Pinax SHALL return failed projection error code `write_disabled`
- **AND** no vault file, folder registry, index database, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Remote mutation requires approval
- **WHEN** an API client requests `POST /v1/folders/inbox:rename` without `yes=true`
- **THEN** Pinax SHALL return failed projection error code `approval_required`
- **AND** the response SHALL include a safe dry-run action or CLI equivalent.

#### Scenario: Remote high-risk mutation requires snapshot evidence
- **WHEN** a remote folder rename, move, or delete would affect existing files or registered notes
- **AND** the request lacks valid `snapshot_id` or equivalent version evidence
- **THEN** Pinax SHALL return failed projection error code `snapshot_required`
- **AND** it SHALL include a runnable `pinax version snapshot --vault <vault>` action.

#### Scenario: Remote mutation is idempotent
- **WHEN** a remote client retries the same folder mutation with the same idempotency key
- **THEN** Pinax SHALL NOT repeat filesystem side effects
- **AND** it SHALL return the previous projection or current terminal state with stable evidence.

### Requirement: Folder registry preserves managed empty folders and operation evidence
Pinax SHALL maintain CLI-authored folder metadata for folder facts that cannot be derived reliably from ordinary files alone.

#### Scenario: Empty created folder remains visible
- **WHEN** a user creates an empty folder through `pinax folder create inbox --purpose notes --vault ./my-notes --json`
- **AND** then runs `pinax folder list --include-empty --vault ./my-notes --json`
- **THEN** Pinax SHALL include that folder using CLI-authored registry evidence even if the folder contains no files.

#### Scenario: Agents do not hand-write folder registry
- **WHEN** an agent needs to register, rename, delete, or adopt folder metadata
- **THEN** it SHALL invoke `pinax folder ...` CLI or API capabilities
- **AND** it SHALL NOT directly edit `.pinax/folders.json`, index SQLite rows, event JSONL, or hook metadata.

