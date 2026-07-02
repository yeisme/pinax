## ADDED Requirements

### Requirement: Notes MAY belong to a project subproject

Pinax SHALL allow notes and managed project items to carry an optional `subproject` field inside a project while preserving existing project-only note behavior.

#### Scenario: Add note to subproject directory
- **WHEN** the user runs `pinax note add "Stock Learning Charter" --project research --subproject stock-learning --dir projects/stock-learning/00-charter --body "目标：建立个人股票学习和研究流程。" --vault yeisme-notes --json`
- **THEN** Pinax SHALL create the note through the application service inside the vault content boundary
- **AND** frontmatter SHALL include project and subproject facts
- **AND** `.pinax` structured assets SHALL NOT be hand-written by the caller.

#### Scenario: Project-only notes remain compatible
- **WHEN** the user runs `pinax note add "Research Log" --project research --vault yeisme-notes --json`
- **THEN** Pinax SHALL preserve existing project-only behavior
- **AND** it SHALL NOT require a subproject field.

#### Scenario: Subproject directory cannot escape vault
- **WHEN** a note command combines `--project`, `--subproject`, and `--dir` with a path that escapes the vault or targets a reserved directory
- **THEN** Pinax SHALL fail with a stable error code
- **AND** it SHALL NOT write Markdown, `.pinax` assets, Git state, provider state, or remote state.

