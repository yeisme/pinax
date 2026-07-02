# pinax-dataview-database Delta Spec

## ADDED Requirements

### Requirement: Pinax database SHALL support Notion-style local views safely

Pinax SHALL provide local database views over Markdown/frontmatter/task/link/asset/index facts with table, board, list, and calendar displays while preserving Markdown and `.pinax/**` registries as the source inputs. A saved database view SHALL be the stable tab unit for Markdown pages, dashboard, MCP, API, and future client surfaces.

#### Scenario: Save table view with typed properties

- **WHEN** the user runs `pinax database view save active-projects --query 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" LIMIT 50' --language sql --display table --vault ./my-notes --json`
- **THEN** Pinax SHALL write a CLI-authored database view definition
- **AND** the view SHALL store query text, query language, display kind, columns, limit, sort, filters, and typed property facts rather than result rows.
- **AND** existing `--kind` behavior SHALL remain readable as a compatibility alias or legacy field; `--display` SHALL be the canonical display contract for query-backed database views.

#### Scenario: Saved database view is a tab

- **WHEN** the user runs `pinax database view save sprint-board --query 'SELECT title, status, due FROM notes WHERE tags CONTAINS "sprint" LIMIT 50' --language sql --display board --group-by status --vault ./my-notes --json`
- **THEN** Pinax SHALL persist a saved view that can be addressed as the `sprint-board` tab
- **AND** optional display metadata such as `display.mode`, `display.tab_label`, `display.tab_order`, and `display.icon` SHALL be additive registry fields
- **AND** older saved view fields such as `kind`, `language`, `query`, `columns`, `group_by`, `calendar_field`, and `board_column` SHALL remain readable.

#### Scenario: Render board view from database query

- **WHEN** the user runs `pinax database view render active-projects --display board --group-by status --vault ./my-notes --json`
- **THEN** Pinax SHALL render bounded board columns from current index/query results
- **AND** the projection SHALL include selected source, display kind, columns, groups, row count, index status, warnings, and next actions
- **AND** temporary render flags such as `--display`, `--group-by`, `--calendar-field`, and `--board-column` SHALL NOT be written back to the saved view registry unless the user explicitly runs a save/apply command
- **AND** it SHALL NOT mutate Markdown, `.pinax/**`, Git state, provider state, sync state, or remote services.

#### Scenario: Calendar view requires a date property

- **WHEN** the user runs `pinax database view render active-projects --display calendar --calendar-field due --vault ./my-notes --json`
- **THEN** Pinax SHALL use the configured date property to group entries
- **AND** invalid or missing date values SHALL be returned as warnings or excluded according to documented stable rules rather than causing raw panics.

#### Scenario: Calendar view without date field is rejected

- **WHEN** the user runs `pinax database view render active-projects --display calendar --vault ./my-notes --json`
- **AND** the saved view does not define `calendar_field`
- **THEN** Pinax SHALL fail with stable error code `calendar_field_required`
- **AND** the next action SHALL recommend `pinax database view save active-projects --display calendar --calendar-field <property> --vault ./my-notes --json` or a non-calendar display.

#### Scenario: Markdown page renders multiple saved database tabs

- **GIVEN** a Markdown note contains two fences, `pinax-database-view active-projects` and `pinax-database-view sprint-board`
- **WHEN** the user runs `pinax note show Dashboard --view rendered --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve each fence to a saved database view and render a multi-tab projection in the same document order
- **AND** each tab projection SHALL include stable tab id, view name, display, row count, columns, warnings, and index status
- **AND** missing saved views SHALL produce `database_tab_view_not_found` without rewriting the note body.

### Requirement: Property schemas SHALL support a safe local subset

Pinax SHALL support property schema inference and explicit schema configuration for common local database workflows without executing unsafe formulas or reading secrets.

#### Scenario: Configure select and date properties

- **WHEN** the user runs `pinax database schema set status --type select --values inbox,next,doing,blocked,review,done --vault ./my-notes --json`
- **AND** the user runs `pinax database schema set due --type date --vault ./my-notes --json`
- **THEN** Pinax SHALL persist schema metadata through the database schema service
- **AND** subsequent query and view rendering SHALL use those property types for comparison, sorting, grouping, and validation.

#### Scenario: Safe relation and rollup

- **WHEN** a database view uses a relation or rollup property
- **THEN** relation targets SHALL be limited to vault-local note, task, project, subproject, collection, or view references
- **AND** rollup functions SHALL be limited to bounded local aggregates such as count, min, max, latest, and status summary
- **AND** formula or rollup evaluation SHALL NOT access files, network, environment variables, provider payloads, raw prompts, hidden system prompts, secrets, or full note bodies.

#### Scenario: Unsafe formula is rejected

- **WHEN** the user configures or runs a formula that attempts network access, file access, environment access, arbitrary JavaScript, DataviewJS, SQL write, PRAGMA, provider payload access, or secret access
- **THEN** Pinax SHALL reject the formula with a stable error code such as `formula_unsupported_clause` or `formula_forbidden_access`
- **AND** no Markdown file, index database, structured asset, provider state, Git state, sync state, or remote service SHALL be modified.

### Requirement: Database output SHALL remain agent-safe

Pinax SHALL keep database view output bounded across default human, JSON, agent, events, explain, saved view, managed block, API, MCP, and dashboard surfaces.

#### Scenario: Agent output for database view is low-token

- **WHEN** the user runs `pinax database view render active-projects --vault ./my-notes --agent`
- **THEN** stdout SHALL include key=value facts for command, status, view name, tab id, language, display, source, rows, columns, groups, warnings, index status, and next actions when useful
- **AND** stdout SHALL NOT include localized prose, ANSI tables, full note bodies, raw prompts, provider payloads, Authorization headers, cookies, tokens, hidden system prompts, private tool arguments, or complete chain-of-thought.

#### Scenario: JSON output exposes database tab data additively

- **WHEN** the user runs `pinax database view render active-projects --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope whose existing top-level fields remain unchanged
- **AND** `data.database_view`, `data.database_tab`, and `facts.database_tab.*` style facts MAY be added as optional fields
- **AND** existing facts such as `view`, `rows`, `columns`, and `index_status` SHALL remain available for older consumers.

#### Scenario: Explain output is a redacted review summary

- **WHEN** the user runs `pinax database view render active-projects --vault ./my-notes --explain`
- **THEN** stdout SHALL contain conclusion, selected source, parsed query shape, filters, display, warnings, risk, confidence, and next step
- **AND** it SHALL NOT contain full chain-of-thought or private provider/tool payloads.
