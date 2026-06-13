## ADDED Requirements

### Requirement: CLI tree exposes version and asset primary paths
Pinax SHALL expose version control and asset management as primary command groups with Chinese help text and stable machine command names.

#### Scenario: Root help shows version and asset groups
- **WHEN** a user runs `pinax --help`
- **THEN** root help SHALL include `version` and `asset` in appropriate command groups
- **AND** it SHALL NOT show `git` as a primary command.

#### Scenario: Version help recommends safe workflows
- **WHEN** a user runs `pinax version --help`
- **THEN** help SHALL show status, snapshot, history, diff, show, changed, restore, and backends commands
- **AND** examples SHALL use `--plan`, `--yes`, and snapshot protection where writes are possible.

#### Scenario: Asset help avoids direct metadata editing
- **WHEN** a user runs `pinax asset --help`
- **THEN** help SHALL recommend `asset add/list/show/preview/link/backlinks/orphans/missing/move/remove/verify/repair`
- **AND** it SHALL NOT instruct users or agents to hand-edit `.pinax/assets/*.json`.

#### Scenario: Note attach help exposes attachment placement options
- **WHEN** a user runs `pinax note attach --help`
- **THEN** help SHALL describe `--placement`, `--link-style`, `--embed`, `--mode`, and `--rename`
- **AND** examples SHALL include `pinax note attach "认证方案" ./diagram.png --placement note-folder --embed --vault ./my-notes --json`.

#### Scenario: Note show help exposes attachment preview options
- **WHEN** a user runs `pinax note show --help`
- **THEN** help SHALL describe rendered preview flags such as `--embed-attachments`, `--max-embed-depth`, `--max-embed-bytes`, and `--max-preview-bytes`
- **AND** examples SHALL include `pinax note show "认证方案" --view rendered --embed-attachments markdown --vault ./my-notes`.

#### Scenario: Note preview is a readonly alias
- **WHEN** a user runs `pinax note preview --help`
- **THEN** help SHALL present it as a readonly rendered note preview command
- **AND** examples SHALL include `pinax note preview "认证方案" --embed-attachments markdown --vault ./my-notes`.

### Requirement: Existing actions migrate from git to version wording
Pinax SHALL update user-facing next actions, errors, examples, and docs to use `version` terminology for snapshot and history workflows.

#### Scenario: Snapshot-required error uses version command
- **WHEN** a command fails with `snapshot_required`
- **THEN** stdout SHALL include a next action using `pinax version snapshot --vault <vault> --message <message>`
- **AND** machine facts MAY include `version_backend` but SHALL NOT require users to run `pinax git`.

#### Scenario: Hidden git alias is absent from primary help
- **WHEN** a user runs `pinax git --help` during compatibility period
- **THEN** Pinax MAY show a deprecation message or route to version snapshot help
- **AND** `pinax --help` SHALL keep the git alias hidden from primary navigation.
