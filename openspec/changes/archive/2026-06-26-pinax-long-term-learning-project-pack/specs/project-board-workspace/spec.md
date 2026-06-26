## MODIFIED Requirements

### Requirement: Board configuration is CLI-authored
Pinax SHALL persist and apply project board configuration through CLI commands or application services rather than requiring agents to hand-write metadata.

#### Scenario: Configured columns drive board projection
- **GIVEN** a project `investing` has a subproject `stock-learning`
- **AND** the user runs `pinax project board configure investing --subproject stock-learning --columns inbox,planned,learning,practice,review,retrospective,done --vault ./my-notes --json`
- **WHEN** the user runs `pinax project board show investing --subproject stock-learning --vault ./my-notes --json`
- **THEN** `data.board.columns` SHALL use the configured columns in order
- **AND** `facts` SHALL include optional `column.<id>` counts for configured columns
- **AND** existing facts such as `next`, `doing`, `blocked`, `review`, and `done` SHALL remain present for compatibility.

#### Scenario: Project items accept configured columns
- **GIVEN** a subproject board is configured with column `learning`
- **WHEN** the user runs `pinax project item add investing "学习 K 线基础" --subproject stock-learning --column learning --vault ./my-notes --json`
- **THEN** Pinax SHALL create a managed project item in column `learning`
- **AND** the item SHALL appear under `learning` in the next scoped board projection.

## ADDED Requirements

### Requirement: Pinax can initialize long-term learning project workspaces
Pinax SHALL provide a local-first learning project initializer that composes project, workspace, board, templates, and starter items through application services.

#### Scenario: Initialize a stock learning project pack
- **WHEN** the user runs `pinax project learning init investing stock-learning --title "学习炒股的全部笔记" --project-name "学习炒股" --notes-prefix notes/investing --preset stock-learning --vault ./stock-learning-notes --json`
- **THEN** Pinax SHALL create or reuse project `investing`
- **AND** it SHALL create subproject workspace `stock-learning` with template `long-term-learning`
- **AND** it SHALL configure the learning board columns
- **AND** it SHALL create starter notes and starter project items through Pinax services
- **AND** stdout SHALL contain one JSON envelope with `command=project.learning.init`.

#### Scenario: Learning init dry-run is read-only
- **WHEN** the user runs `pinax project learning init investing stock-learning --preset stock-learning --vault ./stock-learning-notes --dry-run --json`
- **THEN** Pinax SHALL return planned operations
- **AND** it SHALL NOT write Markdown, `.pinax` assets, Git state, provider state, or remote services.
