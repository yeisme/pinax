# project-board-workspace Delta Spec

## ADDED Requirements

### Requirement: Project Manager subprojects SHALL be vault-local and visibly annotated

Pinax SHALL treat Project Manager subprojects as vault-local workspace directories, not repository subprojects, independent Git repositories, runtime services, or `.pinax/**` metadata folders.

#### Scenario: Create subproject shows the vault-local target path

- **GIVEN** the active vault root is `~/data/yeisme-notes`
- **WHEN** the user creates or previews a Project Manager subproject such as `stock-learning`
- **THEN** Pinax SHALL expose a vault-relative `workspace_path` and a full path preview under `~/data/yeisme-notes/`
- **AND** the projection, dashboard, or OD SHALL label the target as a Markdown workspace directory rather than a Git repository or Yeisme code subproject.

#### Scenario: Registry path is explained separately from content path

- **WHEN** Pinax writes `.pinax/project-workspaces/<project>/<subproject>.json`
- **THEN** the UI and docs SHALL describe that file as CLI-authored registry metadata
- **AND** user-authored notes, project artifacts, task notes, and managed blocks SHALL be described as living under `workspace_path` inside the vault.

#### Scenario: Default subproject directories are semantic rather than numbered

- **WHEN** Pinax creates a new Project Manager subproject workspace
- **THEN** it SHALL create semantic default directories such as `charter`, `inbox`, `sources`, `runs`, `outputs`, `retros`, and `tool-candidates`
- **AND** it SHALL NOT create numeric-prefix defaults such as `00-charter`, `10-inbox`, `20-sources`, `30-runs`, `40-outputs`, `50-retros`, or `90-tool-candidates`
- **AND** existing numeric-prefix directories in older vaults SHALL remain readable user content rather than being deleted, renamed, or treated as the only supported structure.

#### Scenario: Project Manager copy avoids ambiguous subproject language

- **WHEN** Project Manager renders empty states, create forms, detail panels, or confirmation dialogs for subprojects
- **THEN** it SHALL include concise annotations for `Vault root`, `Workspace path`, and `Full path preview`
- **AND** it SHALL NOT imply that Pinax will create a monorepo subproject, Git submodule, independent remote repository, `AGENTS.md`, `CLAUDE.md`, or development toolchain bootstrap for this vault-local workspace.

### Requirement: Project boards SHALL use explicit task ownership

Pinax SHALL distinguish managed tasks, adopted checklist tasks, and inferred checklist tasks so board writes never mutate arbitrary Markdown checklist lines without explicit user approval.

#### Scenario: Managed task can move across columns

- **GIVEN** a project board contains a Pinax-managed task `item_123` in column `next`
- **WHEN** the user runs `pinax project item move item_123 doing --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the managed task metadata or managed block through the application service
- **AND** the next board projection SHALL place the task in `doing`
- **AND** redacted event evidence SHALL be appended.

#### Scenario: Inferred checklist is readonly until adopted

- **GIVEN** Pinax inferred a board row from a user-authored Markdown checklist line that is not managed by Pinax
- **WHEN** the user runs `pinax project item move <inferred-id> done --vault ./my-notes --json`
- **THEN** Pinax SHALL refuse the write with `project_item_unmanaged` or `task_unmanaged`
- **AND** the projection SHALL include a safe next action such as `pinax task adopt <inferred-id> --plan --vault ./my-notes --json`.

#### Scenario: Task adoption is plan-gated

- **WHEN** the user runs `pinax task adopt <inferred-id> --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL return an adoption plan without modifying Markdown, `.pinax/**`, Git state, provider state, sync state, or remote services
- **AND** applying the adoption SHALL require an explicit command such as `pinax task adopt <inferred-id> --yes --vault ./my-notes --json`.

### Requirement: Project boards SHALL support saved task views

Pinax SHALL allow projects and subprojects to save reusable board views backed by filters and display options rather than saved result snapshots.

#### Scenario: Save board view

- **WHEN** the user runs `pinax project board view save research active --subproject stock-learning --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update a CLI-authored board view asset
- **AND** the view SHALL store source query, filters, columns, grouping, display options, and project/subproject scope
- **AND** it SHALL NOT store raw note bodies or a stale copy of result rows as the source of truth.

#### Scenario: Render board view from current facts

- **WHEN** the user runs `pinax project board view render research active --subproject stock-learning --vault ./my-notes --json`
- **THEN** Pinax SHALL compute current rows from the workspace, task, note, and index projections
- **AND** stdout SHALL include bounded cards, counts, warnings, index status, and next actions.

### Requirement: Daily review SHALL update only managed task blocks

Pinax SHALL support daily task review from project boards without rewriting arbitrary daily note content.

#### Scenario: Daily review writes a managed block only

- **GIVEN** today's daily note contains `<!-- pinax:managed name=daily-task-review -->` and `<!-- /pinax:managed -->`
- **WHEN** the user runs `pinax plan daily --tasks --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL update only that managed block with bounded task review facts
- **AND** all user-authored Markdown outside the block SHALL be preserved.

#### Scenario: Daily review refuses ambiguous write target

- **GIVEN** today's daily note does not contain a `daily-task-review` managed block
- **WHEN** the user runs `pinax plan daily --tasks --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL fail or return partial projection with stable error code `managed_block_missing`
- **AND** it SHALL NOT guess an insertion point or rewrite the note body.
