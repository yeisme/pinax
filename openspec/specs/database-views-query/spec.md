# database-views-query Specification

## Purpose
TBD - created by archiving change pinax-database-views-query. Update Purpose after archive.
## Requirements
### Requirement: Pinax indexes notes as typed database rows
Pinax SHALL project local Markdown notes into typed database rows while keeping Markdown files as the source of truth.

#### Scenario: Extract typed properties from frontmatter
- **WHEN** a note contains YAML frontmatter fields such as `status: active`, `rating: 5`, `done: false`, `due: 2026-06-06`, and `tags: [project, pinax]`
- **THEN** Pinax SHALL index those fields as typed properties for the note row
- **AND** the original Markdown file SHALL remain the source of truth.

#### Scenario: Extract system properties
- **WHEN** Pinax indexes a note
- **THEN** it SHALL expose stable system properties such as `file.path`, `file.name`, `file.folder`, `file.tags`, `file.created`, `file.updated`, `note.id`, `links`, `backlinks`, and `attachments`
- **AND** those properties SHALL be queryable without requiring users to write frontmatter.

#### Scenario: Report mixed property types
- **WHEN** the same property has incompatible value types across notes
- **THEN** Pinax SHALL preserve raw values and report a mixed type warning in schema inference or query explain
- **AND** it SHALL NOT silently drop rows.

### Requirement: Pinax supports a safe SQL-first local query language
Pinax SHALL support Pinax SQL for local database views and SHALL NOT execute user input as raw SQLite SQL.

#### Scenario: Run SQL table query
- **WHEN** a user runs `pinax query run 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with table columns, rows, row count, selected filters, sort facts, engine, and index status
- **AND** stdout SHALL NOT include human prose outside JSON.

#### Scenario: Parse SQL into query AST
- **WHEN** a user runs `pinax query run 'SELECT title, status FROM notes WHERE tags CONTAINS "project" ORDER BY updated DESC LIMIT 10' --vault ./my-notes --json`
- **THEN** Pinax SHALL parse the query into an internal query AST
- **AND** it SHALL return matching local note rows without passing the raw query string to SQLite.

#### Scenario: Reject non-SQL table syntax
- **WHEN** a user runs `pinax query run 'TABLE title FROM #project LIMIT 10' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with a stable error code such as `sql_unsupported_syntax`
- **AND** the next action SHALL recommend an equivalent `SELECT ... FROM notes ...` query.

#### Scenario: Reject unsupported SQL
- **WHEN** a user runs a query with unsupported clauses such as arbitrary joins, subqueries, shell calls, network calls, or user JavaScript
- **THEN** Pinax SHALL fail with a stable error code such as `sql_unsupported_clause` or `sql_forbidden_function`
- **AND** no Markdown file, `.pinax/` structured asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Explain query plan
- **WHEN** a user runs `pinax query explain 'SELECT title FROM notes WHERE status = "active" LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL return the parsed query shape, selected indexes, fallback warnings, unsupported features, estimated limits, and selected property list
- **AND** it SHALL NOT execute expensive full result rendering.

### Requirement: Query results are bounded and page-aware
Pinax SHALL bound database query output and support cursor pagination for large result sets.

#### Scenario: Paginate table results
- **WHEN** a query matches more rows than the requested limit
- **THEN** Pinax SHALL return only the requested page
- **AND** JSON output SHALL include `has_more` and an opaque cursor or next action for the next page.

#### Scenario: Load selected properties only
- **WHEN** a table query selects specific columns
- **THEN** Pinax SHALL load and return only those selected properties plus stable row identity fields
- **AND** it SHALL NOT include full note bodies by default.

### Requirement: Database views are CLI-authored structured assets
Pinax SHALL store reusable database views through CLI or application services rather than hand-written metadata.

#### Scenario: Save a table view
- **WHEN** a user runs `pinax database view save active-projects --query 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 50' --kind table --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update the view registry through the application service
- **AND** the saved view SHALL store query and display configuration rather than result snapshots.

#### Scenario: Show a table view
- **WHEN** a user runs `pinax database view show active-projects --vault ./my-notes --json`
- **THEN** Pinax SHALL execute the saved query against the current local index projection
- **AND** it SHALL return current rows, columns, filters, sort facts, and pagination facts.

#### Scenario: Delete a database view with approval
- **WHEN** a user runs `pinax database view delete active-projects --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove only that view definition through the application service
- **AND** it SHALL NOT delete notes, attachments, index rows, provider data, or remote state.

### Requirement: Schema inference and overrides are reviewable
Pinax SHALL infer database property schemas from local notes and let users declare property types through CLI-authored metadata.

#### Scenario: Infer database schema
- **WHEN** a user runs `pinax database schema infer --vault ./my-notes --json`
- **THEN** Pinax SHALL return discovered properties, inferred types, source counts, mixed type warnings, and sample values
- **AND** it SHALL NOT modify Markdown files or structured assets.

#### Scenario: Set property type override
- **WHEN** a user runs `pinax database schema set status --type select --values active,paused,done --vault ./my-notes --json`
- **THEN** Pinax SHALL update CLI-authored database schema metadata
- **AND** it SHALL append redacted event evidence.

### Requirement: Custom frontmatter properties are queryable
Pinax SHALL index non-empty custom note frontmatter fields as typed properties while keeping Markdown as the source of truth.

#### Scenario: List notes with custom frontmatter properties
- **WHEN** a registered note contains frontmatter fields such as `rating: 5`, `done: false`, and `due: 2026-06-09`
- **AND** a user runs `pinax note list --property rating --property done --property due --strict-properties --vault ./my-notes --json`
- **THEN** Pinax SHALL return those selected property values in the note list projection
- **AND** it SHALL infer useful property types from frontmatter scalar values.

#### Scenario: Property command refreshes property projection
- **WHEN** a user runs `pinax note property set note_123 priority 2 --vault ./my-notes --json`
- **AND** then runs `pinax note list --property priority --strict-properties --vault ./my-notes --json`
- **THEN** Pinax SHALL expose the new `priority` property without requiring hand-edited index metadata
- **AND** the original Markdown file SHALL remain the authoritative storage for the property.

