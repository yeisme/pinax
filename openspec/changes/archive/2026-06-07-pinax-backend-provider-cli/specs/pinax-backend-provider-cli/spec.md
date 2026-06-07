## ADDED Requirements

### Requirement: Pinax exposes a unified backend command namespace

Pinax SHALL provide `pinax backend` as the primary CLI namespace for backend provider profile management, capability checks, diagnostics, sync plans, and controlled backend operations.

#### Scenario: backend command exists
- **GIVEN** a user has a Pinax CLI build
- **WHEN** the user runs `pinax backend --help`
- **THEN** the command SHALL exist
- **AND** help SHALL list `list`, `add`, `status`, `doctor`, `capabilities`, `diff`, `push`, `pull`, and `remove`
- **AND** help text SHALL explain backend provider interaction in Chinese.

#### Scenario: storage remains compatible
- **GIVEN** existing users run `pinax storage status --vault ./my-notes`
- **WHEN** the storage command executes
- **THEN** Pinax SHALL keep the command available during this change
- **AND** it SHALL use the backend service or compatible projection internally
- **AND** it SHALL not break existing `storage set-s3`, `storage status`, or `storage doctor` machine output fields.

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
