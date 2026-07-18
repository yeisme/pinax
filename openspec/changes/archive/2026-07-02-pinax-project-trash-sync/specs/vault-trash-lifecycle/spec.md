## ADDED Requirements

### Requirement: Vault objects use a recoverable trash lifecycle
Pinax SHALL route destructive vault object operations through a CLI-authored trash lifecycle by default. A vault object includes notes, projects, subprojects, project board configuration, templates, views, and future structured registry assets.

#### Scenario: Default delete moves object to trash
- **WHEN** a user runs `pinax project delete history --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL remove `history` from active project listings
- **AND** it SHALL write a tombstone with object kind, object id, old registry facts, trash path, deleted time, source command, and version evidence
- **AND** it SHALL preserve recoverable registry and content fragments under `.pinax/trash/<date>/`.

#### Scenario: Delete without approval is rejected
- **WHEN** a user runs `pinax project delete history --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown file, `.pinax` asset, registry, index database, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Hard delete is explicit and bounded
- **WHEN** a user runs `pinax trash purge project/history --hard --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL permanently remove only the matching trash backup and tombstone after validation
- **AND** active vault objects SHALL NOT be hard-deleted directly by default.

### Requirement: Trash contents are inspectable and restorable
Pinax SHALL provide commands to inspect, restore, and purge trash entries without requiring users or agents to edit `.pinax/**` files by hand.

#### Scenario: List trash entries
- **WHEN** a user runs `pinax trash list --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with command `trash.list`
- **AND** each entry SHALL include object id, object kind, deleted time, source command, trash path, restore status, and redacted evidence refs.

#### Scenario: Restore trashed project
- **GIVEN** project `history` was moved to trash
- **WHEN** a user runs `pinax trash restore project/history --vault ./my-notes --json`
- **THEN** Pinax SHALL restore the project registry entry and recoverable content fragments through the application service
- **AND** it SHALL refresh affected index projections
- **AND** the restored project SHALL appear in `pinax project list --vault ./my-notes --json`.

#### Scenario: Restore conflict is explicit
- **GIVEN** project `history` is in trash
- **AND** an active project with slug `history` already exists
- **WHEN** a user runs `pinax trash restore project/history --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `restore_conflict`
- **AND** it SHALL NOT overwrite the active project or delete the trash backup.

### Requirement: Trash surfaces follow the CLI output contract
Trash commands SHALL render all human and machine output from one projection without leaking sensitive payloads.

#### Scenario: Machine output is stable
- **WHEN** a user runs `pinax trash list --vault ./my-notes --agent`
- **THEN** stdout SHALL contain key=value facts including `spec_version`, `mode=agent`, `command=trash.list`, `status`, entry count, and next actions
- **AND** stdout SHALL NOT include ANSI, localized prose blocks, raw note bodies, provider payloads, hidden prompts, tokens, or private tool arguments.
