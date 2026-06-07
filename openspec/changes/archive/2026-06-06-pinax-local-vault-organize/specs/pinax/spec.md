# pinax Specification Delta

## ADDED Requirements

### Requirement: Pinax initializes and validates a local Markdown vault

Pinax SHALL initialize and validate local Markdown vaults without requiring provider credentials, remote services, or agent-written metadata files.

#### Scenario: initializing a vault
- **GIVEN** a user runs `pinax init ./my-notes --title "我的知识库"`
- **WHEN** the vault is initialized
- **THEN** Pinax SHALL create `notes/`, `.pinax/config.yaml`, and `.pinax/events.jsonl` through CLI services
- **AND** it SHALL NOT overwrite existing Markdown note bodies

#### Scenario: validating a vault
- **GIVEN** a vault contains Markdown notes
- **WHEN** the user runs `pinax validate --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with validation facts, issues, and next actions
- **AND** path and metadata errors SHALL use stable machine-readable error codes

### Requirement: Pinax plans and applies metadata safely

Pinax SHALL plan metadata normalization before writing frontmatter and SHALL require explicit approval for writes.

#### Scenario: planning metadata changes
- **GIVEN** Markdown notes are missing Pinax metadata
- **WHEN** the user runs `pinax metadata plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report planned frontmatter additions without modifying files

#### Scenario: applying metadata changes
- **GIVEN** a metadata plan exists for local notes
- **WHEN** the user runs `pinax metadata apply --vault ./my-notes --yes`
- **THEN** Pinax SHALL update only Markdown files inside the vault
- **AND** it SHALL append redacted event evidence through the event service

### Requirement: Pinax organizes note files with Git protection

Pinax SHALL generate an organize plan before moving or renaming notes, and SHALL require explicit Git snapshot protection before true apply.

#### Scenario: previewing organize changes
- **GIVEN** notes have titles that imply normalized paths
- **WHEN** the user runs `pinax organize plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report planned moves, skips, and conflicts without modifying files

#### Scenario: refusing unprotected organize apply
- **GIVEN** no recent Pinax Git snapshot evidence exists
- **WHEN** the user runs `pinax organize apply --vault ./my-notes --yes`
- **THEN** Pinax SHALL refuse the write with a stable error code and a runnable `pinax git snapshot` next action

#### Scenario: applying organize changes after snapshot
- **GIVEN** the user has run `pinax git snapshot --vault ./my-notes --message "整理前快照"`
- **WHEN** the user runs `pinax organize apply --vault ./my-notes --yes`
- **THEN** Pinax SHALL move only files within the vault boundary
- **AND** it SHALL record redacted event evidence for each applied move

### Requirement: Pinax local commands follow the AI-native CLI output contract

Pinax SHALL render human, agent, JSON, events, and explain outputs from one command projection.

#### Scenario: rendering machine output
- **GIVEN** a local vault command supports `--json` or `--agent`
- **WHEN** that output mode is selected
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr
