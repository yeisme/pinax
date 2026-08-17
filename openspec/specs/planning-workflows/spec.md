# planning-workflows Specification

## Purpose

Pinax 提供本地优先的日、周、月规划、规划快照和可审阅 action 草稿。规划输入来自 vault、索引和本地 project board；Pinax 不依赖外部任务运行时，不拥有远端任务 Provider，也不执行外部 action。

## Requirements

### Requirement: Planning uses local sources only

Pinax SHALL use local Markdown vault, local index projections, and local project-board facts for planning. It SHALL NOT invoke an external task runtime, probe external task executables, read external task stores, or require provider credentials.

#### Scenario: local daily preview

- **WHEN** the user runs `pinax plan daily --vault ./my-notes --dry-run --json`
- **THEN** Pinax SHALL return a planning projection with period, snapshot id, decision id, capacity facts, risks, and local evidence
- **AND** it SHALL leave Markdown, `.pinax` assets, Git state, and remote services unchanged

#### Scenario: local weekly and monthly preview

- **WHEN** the user runs `pinax plan weekly|monthly --vault ./my-notes --dry-run --json`
- **THEN** Pinax SHALL use local vault and project-board context
- **AND** it SHALL not require an external task executable

### Requirement: Planning snapshots are CLI-authored and redacted

Pinax SHALL persist normalized planning snapshots through application services when a command explicitly saves planning state.

#### Scenario: saving a local snapshot

- **WHEN** the user runs `pinax plan daily --vault ./my-notes --save --yes --json`
- **THEN** Pinax SHALL write `.pinax/planning/snapshots/<snapshot_id>.json`
- **AND** the snapshot SHALL contain schema version, source, capture time, local facts, risks, and evidence refs
- **AND** it SHALL NOT contain credentials, Authorization headers, raw provider payloads, hidden prompts, private tool arguments, or full chain-of-thought

### Requirement: Daily task review has an explicit write gate

Pinax SHALL render local project-board tasks into the `daily-task-review` managed block only after explicit approval.

#### Scenario: task-review dry-run

- **WHEN** the user runs `pinax plan daily --task-review --vault ./my-notes --json`
- **THEN** Pinax SHALL preview the replacement
- **AND** it SHALL leave the daily note unchanged

#### Scenario: task-review apply

- **WHEN** the user runs `pinax plan daily --task-review --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the `daily-task-review` managed block
- **AND** it SHALL preserve user-authored content outside the block

### Requirement: Plan actions are Pinax-owned drafts

Pinax SHALL generate `pinax.planning.actions.v1` drafts for review and SHALL NOT execute them or delegate them to an external task service.

#### Scenario: action draft dry-run

- **WHEN** the user runs `pinax plan actions --from daily --vault ./my-notes --json`
- **THEN** Pinax SHALL return a local action draft preview with source decision, source snapshot, task ids, reasons, and confirmation requirements
- **AND** it SHALL not write the actions directory

#### Scenario: saving an action draft

- **WHEN** the user runs `pinax plan actions --from weekly --save --vault ./my-notes --json`
- **THEN** Pinax SHALL write `.pinax/planning/actions/<action_id>.json` through the planning service
- **AND** the draft SHALL use `pinax.planning.actions.v1`
- **AND** the projection SHALL not include an external execute command

### Requirement: Planning commands follow the AI-native CLI output contract

Planning commands SHALL render human and machine outputs from one command projection.

#### Scenario: machine output mode

- **WHEN** a planning command is run with `--json`, `--agent`, `--events`, or `--explain`
- **THEN** stdout SHALL contain only the selected machine format
- **AND** errors SHALL include stable status and error code fields

### Requirement: Planning tests are fixture-first

Planning workflows SHALL be testable with temporary vaults and local fixtures without real provider credentials, external task stores, remote networks, or the user's vault.

#### Scenario: verification coverage

- **WHEN** planning behavior changes
- **THEN** tests SHALL cover dry-run/write gates, snapshots, managed-block conflicts, daily/weekly/monthly decisions, local action draft generation, and output modes
