## ADDED Requirements

### Requirement: Pinax SHALL support project-scoped subprojects as local workspaces

Pinax SHALL let a vault project contain subprojects that represent local workspaces for research, learning, content, planning, retrospectives, or tool-candidate workflows without creating a new Yeisme engineering project.

#### Scenario: Create a subproject workspace
- **GIVEN** a Pinax vault has project `research`
- **WHEN** the user runs `pinax project subproject create research stock-learning --title "Stock Learning" --template scenario --vault yeisme-notes --json`
- **THEN** Pinax SHALL create a subproject workspace through the application service
- **AND** it SHALL create or record the standard directories `00-charter`, `10-inbox`, `20-sources`, `30-runs`, `40-outputs`, `50-retros`, and `90-tool-candidates`
- **AND** stdout SHALL contain one projection envelope with command `project.subproject.create`, project, subproject, workspace path, created directory facts, and next actions.

#### Scenario: List and show subprojects
- **GIVEN** project `research` has subproject `stock-learning`
- **WHEN** the user runs `pinax project subproject list research --vault yeisme-notes --json` or `pinax project subproject show research stock-learning --vault yeisme-notes --json`
- **THEN** Pinax SHALL return bounded workspace facts without reading full note bodies
- **AND** it SHALL include charter path, directory presence, board configuration status, item counts when available, and safe next actions.

#### Scenario: Reject unsafe subproject paths
- **WHEN** a user attempts to create a subproject with an empty slug, path traversal, absolute path, or reserved directory target
- **THEN** Pinax SHALL fail with a stable machine-readable error code
- **AND** it SHALL NOT create Markdown files, `.pinax` structured assets, Git state, provider state, or remote state.

### Requirement: Project board SHALL support optional subproject scope

Pinax SHALL extend project board commands with an optional subproject scope while preserving existing project-wide board behavior.

#### Scenario: Show subproject board
- **GIVEN** project `research` has subproject `stock-learning` and managed project items
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault yeisme-notes --json`
- **THEN** stdout SHALL contain one projection envelope with command `project.board.show`
- **AND** facts SHALL include project, subproject, column counts, item counts, index status, warnings, and next actions
- **AND** returned items SHALL be scoped to `stock-learning`.

#### Scenario: Existing project-wide board remains compatible
- **GIVEN** existing scripts run `pinax project board show research --vault yeisme-notes --json`
- **WHEN** subproject support exists
- **THEN** the command SHALL keep returning the project-wide board unless `--subproject` is explicitly provided
- **AND** existing JSON fields and `--agent` keys SHALL remain compatible.

#### Scenario: Configure subproject board columns
- **WHEN** the user runs `pinax project board configure research --subproject stock-learning --columns inbox,next,doing,blocked,review,done --vault yeisme-notes --json`
- **THEN** Pinax SHALL write subproject-scoped board configuration through the project board service
- **AND** it SHALL NOT overwrite the project-wide board configuration.

### Requirement: Project items SHALL carry project management fields

Pinax SHALL support local project item metadata useful for project management while keeping Markdown and CLI-authored metadata as the source records.

#### Scenario: Add item with project management fields
- **WHEN** the user runs `pinax project item add research "跑第一次真实研究" --subproject stock-learning --column next --labels research,learning --milestone phase-1 --priority medium --vault yeisme-notes --json`
- **THEN** Pinax SHALL create a managed item through the application service
- **AND** the item SHALL include project, subproject, item id, title, column, status, labels, milestone, priority, optional due date, optional blockers, created time, updated time, and note reference facts.

#### Scenario: Move managed item between columns
- **GIVEN** a managed item exists in `next`
- **WHEN** the user runs `pinax project item move <item_id> doing --vault yeisme-notes --json`
- **THEN** Pinax SHALL update only managed item metadata
- **AND** the next board projection SHALL place the item in `doing`
- **AND** redacted event evidence SHALL be recorded.

#### Scenario: Refuse unmanaged checklist writes
- **GIVEN** an item was inferred from a Markdown checklist line not owned by Pinax
- **WHEN** the user runs `pinax project item move <inferred_item_id> done --vault yeisme-notes --json`
- **THEN** Pinax SHALL refuse the write with `project_item_unmanaged`
- **AND** it SHALL include a safe next action to create a managed item or edit the note manually.

### Requirement: Project workspace writes SHALL stay protected

Pinax SHALL keep subproject and board writes explicit, auditable, and recoverable.

#### Scenario: Archive requires approval
- **GIVEN** a managed item exists
- **WHEN** the user runs `pinax project item archive <item_id> --vault yeisme-notes --json`
- **THEN** Pinax SHALL fail with `approval_required`
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Snapshot required for high-risk board write
- **GIVEN** a board operation would archive, batch-change, delete, or rewrite managed Markdown
- **WHEN** the user runs the operation with `--yes` and no recent snapshot evidence
- **THEN** Pinax SHALL fail with `snapshot_required`
- **AND** it SHALL include a runnable `pinax version snapshot --vault yeisme-notes --message "project workspace update"` next action.

