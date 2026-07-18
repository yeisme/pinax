# pinax-backend-provider-cli Specification

## Purpose
TBD - created by archiving change pinax-backend-provider-cli. Update Purpose after archive.
## Requirements
### Requirement: Pinax exposes a unified backend command namespace

Pinax SHALL provide `pinax backend` as the primary CLI namespace for backend provider profile management, capability checks, diagnostics, sync plans, controlled backend operations, raw object inspection, and visible note-like object inspection.

#### Scenario: backend command exists

- **GIVEN** a user has a Pinax CLI build
- **WHEN** the user runs `pinax backend notes --help`
- **THEN** the command SHALL exist
- **AND** help SHALL list `summary`, `list`, and `stat`
- **AND** help text SHALL use English CLI wording.

### Requirement: Backend profiles are CLI-authored structured assets

Pinax SHALL create and modify backend provider metadata through application services and CLI commands rather than direct agent-written JSON/YAML/JSONL metadata.

#### Scenario: adding an S3 backend profile
- **GIVEN** a local vault exists at `./my-notes`
- **WHEN** the user runs `pinax backend add s3 --name work-s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json`
- **THEN** Pinax SHALL write or update `.pinax/backends.json` through the backend service
- **AND** stdout SHALL contain one JSON envelope with `command="backend.add"`
- **AND** the envelope SHALL include backend name, kind, credential source, capability summary, and next action
- **AND** it SHALL NOT include S3 access key, secret key, session token, Authorization header, or raw provider payload.

#### Scenario: adding an rclone backend profile
- **GIVEN** a local vault exists at `./my-notes`
- **WHEN** the user runs `pinax backend add rclone --name work-drive --remote workdrive:pinax --vault ./my-notes`
- **THEN** Pinax SHALL store the remote reference and credential source only
- **AND** it SHALL NOT copy rclone config secrets into the vault
- **AND** it SHALL recommend `pinax backend doctor --name work-drive --vault ./my-notes` as the next command.

#### Scenario: adding an OneDrive backend profile
- **GIVEN** OneDrive is represented by an rclone remote
- **WHEN** the user runs `pinax backend add onedrive --name personal-drive --remote onedrive:Pinax --vault ./my-notes`
- **THEN** Pinax SHALL store an `onedrive` backend profile whose credential source is `rclone_config`
- **AND** Pinax SHALL NOT store Microsoft OAuth tokens or implement native Microsoft Graph in this change.

### Requirement: Backend provider adapters expose stable capabilities and doctor results

Each backend provider SHALL expose normalized capability and doctor projections so users and agents can determine whether a backend can list, diff, pull, push, delete, or only provide configuration diagnostics.

#### Scenario: checking rclone capabilities
- **GIVEN** a vault has a backend profile named `work-drive`
- **WHEN** the user runs `pinax backend capabilities --name work-drive --vault ./my-notes --agent`
- **THEN** stdout SHALL include `spec_version`, `mode=agent`, `command=backend.capabilities`, `status`, `fact.backend.name`, `fact.backend.kind`, and capability facts
- **AND** stdout SHALL NOT include localized prose, ANSI color, raw rclone output, provider tokens, or secrets.

#### Scenario: diagnosing a missing rclone executable
- **GIVEN** a backend profile uses kind `rclone`
- **AND** the `rclone` executable is unavailable
- **WHEN** the user runs `pinax backend doctor --name work-drive --vault ./my-notes --json`
- **THEN** Pinax SHALL return a JSON envelope with `status="partial"` or `status="failed"`
- **AND** the error code SHALL be `RCLONE_NOT_FOUND`
- **AND** the envelope SHALL include a safe next action to install or configure rclone
- **AND** it SHALL NOT fail unrelated local note commands.

#### Scenario: diagnosing S3 profile without network access
- **GIVEN** a backend profile uses kind `s3`
- **WHEN** the user runs `pinax backend doctor --name work-s3 --vault ./my-notes --json`
- **THEN** Pinax SHALL validate required profile fields and credential source without requiring public network access by default
- **AND** it SHALL report whether network validation was skipped, unavailable, or explicitly requested
- **AND** it SHALL NOT read or print credential secret values.

### Requirement: Backend diff, push, and pull are dry-run first and approval gated

Pinax SHALL generate backend sync plans before writing local files or remote provider state, and SHALL require explicit approval for writes.

#### Scenario: previewing backend diff
- **GIVEN** a vault has a backend profile named `work-drive`
- **WHEN** the user runs `pinax backend diff --name work-drive --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command="backend.diff"`
- **AND** the projection SHALL include creates, updates, deletes, skips, conflicts, risks, evidence, and recommended next action when known
- **AND** Pinax SHALL NOT write local notes, provider state, receipts, Git state, or remote objects.

#### Scenario: push requires explicit approval
- **GIVEN** a backend push plan has remote writes
- **WHEN** the user runs `pinax backend push --name work-drive --vault ./my-notes` without `--dry-run` or `--yes`
- **THEN** Pinax SHALL NOT write remote state
- **AND** it SHALL return a stable approval-required projection with error code `APPROVAL_REQUIRED`
- **AND** it SHALL recommend a dry-run or `--yes` command.

#### Scenario: pull conflicts are not silently overwritten
- **GIVEN** local and remote versions conflict
- **WHEN** the user runs `pinax backend pull --name work-drive --vault ./my-notes --yes`
- **THEN** Pinax SHALL refuse silent overwrite
- **AND** it SHALL write or update conflict queue state through the application service
- **AND** it SHALL render `CONFLICT_DETECTED` with redacted conflict refs and a safe next action.

### Requirement: Backend commands follow the AI-native CLI output contract

All backend commands SHALL render human and machine outputs from one command projection.

#### Scenario: JSON backend output is machine-only
- **GIVEN** a user runs any `pinax backend ... --json` command
- **WHEN** the command completes or fails
- **THEN** stdout SHALL contain exactly one valid JSON object
- **AND** the JSON object SHALL include `spec_version`, `mode="json"`, `command`, and `status`
- **AND** progress, diagnostics, logs, and external CLI stderr SHALL not be written to JSON stdout.

#### Scenario: default backend output is concise Chinese
- **GIVEN** a user runs `pinax backend status --name work-drive --vault ./my-notes`
- **WHEN** the command completes
- **THEN** stdout SHALL show a concise Chinese human summary
- **AND** it SHALL include state, backend kind, credential source summary, capability/risk summary, evidence, and one recommended next action when useful
- **AND** it SHALL NOT expose secrets, raw provider payload, or full external command output.

#### Scenario: explain output is a redacted reasoning summary
- **GIVEN** a user runs `pinax backend doctor --name work-drive --vault ./my-notes --explain`
- **WHEN** the command completes
- **THEN** stdout SHALL include Chinese sections for conclusion, evidence, confidence, risk, tradeoff, and next action
- **AND** it SHALL NOT include full chain-of-thought, raw prompts, hidden system prompts, provider payloads, tokens, cookies, Authorization headers, or private tool arguments.

### Requirement: Backend implementation is testable without real providers

Backend provider behavior SHALL be covered by local unit, command, and e2e tests that do not require production credentials or public network access.

#### Scenario: testing rclone through a fake executable
- **GIVEN** backend e2e tests cover rclone behavior
- **WHEN** the tests run
- **THEN** they SHOULD use a fake `rclone` executable or fixture harness
- **AND** they SHALL verify capability, doctor, diff, dry-run, approval gate, and redaction behavior
- **AND** they SHALL NOT require a real rclone config, real OneDrive account, real S3 bucket, or public network.

#### Scenario: preserving local-only Pinax behavior
- **GIVEN** no backend provider is configured
- **WHEN** a user runs local commands such as `pinax note new`, `pinax search`, `pinax index rebuild`, `pinax template init`, or readonly `pinax mcp`
- **THEN** those commands SHALL continue to work without backend provider credentials or network access
- **AND** backend provider failures SHALL not block local Markdown vault workflows.

### Requirement: Backend note inspection commands expose visible cloud note state

Pinax SHALL provide read-only backend note inspection commands that summarize, list, and stat note-like Markdown objects visible through a configured backend profile.

#### Scenario: summarize visible backend notes

- **GIVEN** a vault has a backend profile named `work-s3`
- **AND** `work-s3` is the default backend
- **WHEN** the user runs `pinax backend notes summary --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope with `command="backend.notes.summary"`
- **AND** facts SHALL include `backend`, `kind`, `objects`, `notes`, and `bytes`
- **AND** the command SHALL NOT write local notes, `.pinax/**` state, remote objects, sync receipts, or provider credentials.

#### Scenario: list visible backend Markdown notes

- **GIVEN** a backend contains `notes/cloud-sync.md` and non-note objects
- **AND** that backend is the default backend
- **WHEN** the user runs `pinax backend notes list --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope with `command="backend.notes.list"`
- **AND** `data.notes[]` SHALL include note-like objects whose visible key ends with `.md`
- **AND** each note row SHALL include `path`, `key`, and `size_bytes`, with `revision` and `updated_at` when the backend provides them
- **AND** non-Markdown objects SHALL NOT appear in `data.notes[]`.

#### Scenario: stat one visible backend note object

- **GIVEN** a backend contains `notes/cloud-sync.md`
- **WHEN** the user runs `pinax backend notes stat work-s3 notes/cloud-sync.md --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope with `command="backend.notes.stat"`
- **AND** facts SHALL include `backend`, `path`, `key`, and `revision` when the backend provides a revision
- **AND** missing objects SHALL return `note_object_not_found` rather than a generic internal error.

#### Scenario: complete backend note inspection arguments

- **GIVEN** a vault has a backend profile named `work-s3`
- **AND** that backend exposes visible note-like object `notes/cloud-sync.md`
- **WHEN** shell completion is requested for `pinax backend notes stat <TAB> --vault ./my-notes`
- **THEN** Pinax SHALL suggest backend profile names with backend kind descriptions
- **WHEN** shell completion is requested for `pinax backend notes stat work-s3 notes/ --vault ./my-notes`
- **THEN** Pinax SHALL suggest visible note-like Markdown paths such as `notes/cloud-sync.md`
- **AND** it SHALL NOT suggest non-Markdown objects as note paths.

#### Scenario: complete backend note prefixes

- **GIVEN** a backend exposes visible note-like object `notes/research/cloud-sync.md`
- **WHEN** shell completion is requested for `pinax backend notes list work-s3 <TAB> --vault ./my-notes`
- **THEN** Pinax SHALL suggest note prefixes such as `notes/` and `notes/research/`
- **AND** completion SHALL remain read-only.

### Requirement: Backend provider failures are normalized for cloud note inspection

Pinax SHALL map backend provider access failures to stable Pinax command errors without leaking raw provider payloads or secrets.

#### Scenario: invalid S3 region is actionable

- **GIVEN** an S3-compatible backend profile has an invalid region value or runtime region resolution fails
- **WHEN** the user runs `pinax backend object list default-s3 pinax-storage/ --vault ./my-notes --json`
- **THEN** stdout SHALL be one failed JSON envelope
- **AND** `error.code` SHALL be `backend_region_invalid` or another stable backend provider code
- **AND** the hint SHALL include a runnable command such as `pinax backend show default-s3 --vault ./my-notes`
- **AND** stdout/stderr SHALL NOT include S3 access key, secret key, session token, Authorization header, cookies, or provider raw payload.

#### Scenario: S3 profile fields are used by runtime client

- **GIVEN** an S3 backend profile stores `bucket`, `prefix`, `region`, `endpoint`, and `profile`
- **WHEN** Pinax creates the runtime backend store for object or note inspection
- **THEN** it SHALL pass `region`, `endpoint`, and `profile` into the S3 backend client options
- **AND** custom endpoint backends SHALL use path-style addressing by default.

