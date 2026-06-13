## ADDED Requirements

### Requirement: Query commands expose stable database output
Pinax SHALL expose database query and table view commands with stable human and machine output.

#### Scenario: Query run renders human table summary
- **WHEN** a user runs `pinax query run 'SELECT title, status FROM notes WHERE tags CONTAINS "project" LIMIT 10' --vault ./my-notes`
- **THEN** stdout SHALL contain a concise Chinese summary and a scannable table of selected columns
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Query run renders JSON envelope
- **WHEN** a user runs `pinax query run 'SELECT title, status FROM notes WHERE tags CONTAINS "project" LIMIT 10' --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `query.run`, status, facts, columns, rows, filters, sorts, page, engine, and index status
- **AND** stdout SHALL NOT contain human prose outside JSON.

#### Scenario: Query run renders agent facts
- **WHEN** a user runs `pinax query run 'SELECT title, status FROM notes WHERE tags CONTAINS "project" LIMIT 10' --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable low-token key=value facts for command, status, rows, columns, engine, index status, has more, and next cursor when present
- **AND** stdout SHALL NOT include full row bodies, localized prose, raw prompts, provider payloads, or secrets.

#### Scenario: Query explain renders reviewable summary
- **WHEN** a user runs `pinax query explain 'SELECT title FROM notes WHERE tags CONTAINS "project" AND status = "active" LIMIT 20' --vault ./my-notes --explain`
- **THEN** stdout SHALL contain a Chinese explanation with parsed query shape, selected indexes, risk, warnings, and recommended next action
- **AND** it SHALL NOT include full chain-of-thought, raw SQL, secrets, or hidden prompts.

### Requirement: Note list can include selected properties
Pinax SHALL let note list expose selected database properties while preserving existing note list behavior.

#### Scenario: Note list with selected properties
- **WHEN** a user runs `pinax note list --property status --property due --tag project --vault ./my-notes --json`
- **THEN** Pinax SHALL return matching notes with selected property values
- **AND** existing note identity fields such as path, title, note id, tags, kind, and status SHALL remain stable.

#### Scenario: Invalid property selection fails clearly
- **WHEN** a user runs `pinax note list --property unknown --strict-properties --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `property_not_found`
- **AND** no Markdown file, index database, or structured asset SHALL be modified.

### Requirement: Database command help is user-runnable
Pinax SHALL document database query commands with real local examples and without requiring external provider credentials.

#### Scenario: Database help shows local examples
- **WHEN** a user runs `pinax database --help` or `pinax query --help`
- **THEN** help output SHALL include local examples for `query run`, `query explain`, `database view save/show/list/delete`, and `database schema infer/set`
- **AND** help output SHALL NOT require Notion API tokens, external plugins, JavaScript execution, Lark, firecrawl, or external network access.

### Requirement: Completion assists local notebook workflows
Pinax SHALL provide shell completion for database, query, view, search, and note-list workflows using only local vault state and static option sets.

#### Scenario: Complete existing saved views
- **WHEN** a user completes `pinax database view show <TAB>` or `pinax view show <TAB>` against a local vault
- **THEN** completion SHALL list only existing saved view names from CLI-authored view metadata
- **AND** completion descriptions SHOULD include view kind or filter summary when available
- **AND** completion SHALL NOT create, mutate, rebuild, delete, or remotely sync any asset.

#### Scenario: Complete local dimensions and properties
- **WHEN** a user completes flags such as `--tag`, `--group`, `--folder`, `--kind`, `--status`, `--sort`, `--property`, `--column`, or `--type`
- **THEN** completion SHALL return matching local dimensions, inferred properties, or static enum values with concise descriptions
- **AND** it SHALL avoid suggesting missing journal dates, missing views, or non-existent local dimensions.

#### Scenario: Completion degrades without index
- **WHEN** the local index is missing or stale during completion
- **THEN** completion SHALL use cheap local metadata reads or static options when possible
- **AND** it SHALL NOT trigger lazy index rebuild or expensive query execution during shell completion.

### Requirement: Help guides first successful use
Pinax SHALL make common database/search/view workflows discoverable from help output and command errors.

#### Scenario: Query help shows workflow order
- **WHEN** a user runs `pinax query --help`, `pinax database --help`, or `pinax database view --help`
- **THEN** help output SHALL show a short local workflow: inspect/rebuild index, run `query explain`, run `query run`, save a view, show a view
- **AND** each example SHALL be directly runnable against a local vault path.

#### Scenario: Errors include next action
- **WHEN** a query, database view, or property selection fails because an index, schema, view, property, query, approval, or argument is missing
- **THEN** the projection SHALL include a stable error code and a concrete next command
- **AND** machine modes SHALL expose the same next action without parsing localized prose.
