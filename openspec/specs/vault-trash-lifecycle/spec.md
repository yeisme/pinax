# vault-trash-lifecycle Specification

## Purpose
TBD - created by archiving change pinax-project-trash-sync. Update Purpose after archive.
## Requirements
### Requirement: Vault objects use a recoverable trash lifecycle

Pinax SHALL include notes in Cloud Sync delete marker handling so note soft deletes converge across devices without hard-deleting user content silently.

#### Scenario: Note soft delete produces a cloud delete marker

- **WHEN** a user runs `pinax note delete "Draft" --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL move the note to `.pinax/trash/`
- **AND** the next Cloud Sync manifest SHALL include a `note` delete marker for the old path and a trash backup blob.

#### Scenario: Remote note delete marker enters local trash

- **GIVEN** device A soft-deletes a note and pushes Cloud Sync
- **WHEN** device B runs `pinax sync pull --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL remove the active note path on device B
- **AND** it SHALL preserve the note body under local trash with a note tombstone.

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

