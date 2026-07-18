## ADDED Requirements

### Requirement: Projects can be deleted and restored through trash
Pinax SHALL allow users to delete empty or obsolete projects through a recoverable trash lifecycle rather than hand-editing `.pinax/projects.json`.

#### Scenario: Delete empty project from registry
- **GIVEN** a project `history` exists with no subprojects and no managed notes under its notes prefix
- **WHEN** the user runs `pinax project delete history --vault ./my-notes --yes --json`
- **THEN** `pinax project list --vault ./my-notes --json` SHALL NOT include `history` in active projects
- **AND** `pinax trash list --vault ./my-notes --json` SHALL include a restorable `project/history` tombstone
- **AND** the JSON output SHALL include `command=project.delete`, `local_write=true`, `remote_write=false`, `trash_path`, and redacted evidence refs.

#### Scenario: Current project deletion clears or switches current project
- **GIVEN** `history` is the current project
- **WHEN** the user runs `pinax project delete history --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL either clear `current_project` or switch to the next active project deterministically
- **AND** stdout facts SHALL identify the resulting `current_project` state.

#### Scenario: Show deleted project points to restore
- **GIVEN** project `history` was moved to trash
- **WHEN** the user runs `pinax project show history --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `project_not_found`
- **AND** it SHALL include a next action `pinax trash restore project/history --vault ./my-notes --json` when a tombstone exists.

### Requirement: Subprojects can be deleted and restored through trash
Pinax SHALL route subproject workspace deletion through snapshot protection, trash backup, tombstone, and index refresh.

#### Scenario: Delete subproject workspace
- **GIVEN** project `history-learning` has subproject `history-info`
- **AND** a recent Pinax version snapshot exists
- **WHEN** the user runs `pinax project subproject delete history-learning history-info --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL remove `history-info` from active subproject listings
- **AND** it SHALL move the workspace directory and workspace registry fragment to `.pinax/trash/<date>/`
- **AND** it SHALL write a tombstone for `subproject/history-learning/history-info`.

#### Scenario: Non-empty subproject deletion requires snapshot
- **GIVEN** subproject `history-info` contains Markdown files or managed project items
- **WHEN** the user runs `pinax project subproject delete history-learning history-info --vault ./my-notes --yes --json` without recent snapshot evidence
- **THEN** Pinax SHALL fail with stable error code `snapshot_required`
- **AND** it SHALL include a runnable next action for `pinax version snapshot --vault ./my-notes --message "before subproject delete"`.

#### Scenario: Restore subproject workspace
- **GIVEN** subproject `history-learning/history-info` was moved to trash
- **WHEN** the user runs `pinax trash restore subproject/history-learning/history-info --vault ./my-notes --json`
- **THEN** Pinax SHALL restore the workspace directory, workspace registry, and board config fragments when no active conflict exists
- **AND** `pinax project subproject list history-learning --vault ./my-notes --json` SHALL include `history-info` again.
