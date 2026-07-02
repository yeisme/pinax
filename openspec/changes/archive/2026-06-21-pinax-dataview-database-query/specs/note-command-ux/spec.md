# note-command-ux 增量规格

## MODIFIED Requirements

### Requirement: Query commands expose stable database output
Pinax SHALL expose database query, Dataview-compatible query, and table view commands with stable human and machine output.

#### Scenario: Query run supports SQL v2 aggregation
- **WHEN** a user runs `pinax query run 'SELECT status, COUNT(*) AS count FROM notes WHERE tags CONTAINS "project" GROUP BY status LIMIT 20' --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `query.run`
- **AND** the envelope SHALL preserve existing top-level fields and existing `query.run` facts
- **AND** `data` SHALL include selected columns, grouped rows, aggregate facts, page facts, engine, and index status
- **AND** stdout SHALL NOT include full note bodies, raw SQL execution traces, secrets, raw prompts, provider payloads, or full chain-of-thought.

#### Scenario: Query explain reports unsupported SQL safely
- **WHEN** a user runs `pinax query explain 'SELECT title FROM notes JOIN tasks ON notes.id = tasks.note_id' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `sql_unsupported_clause`
- **AND** the next action SHALL recommend a supported `FROM notes` or `FROM tasks` query instead of exposing raw SQLite details.

### Requirement: Database command help is user-runnable
Pinax SHALL document database query and Dataview-compatible commands with real local examples and without requiring external provider credentials.

#### Scenario: Database help shows Dataview workflow
- **WHEN** a user runs `pinax dataview --help`, `pinax query --help`, or `pinax database view --help`
- **THEN** help output SHALL include local examples for `dataview run`, `dataview explain`, `query run`, `query explain`, and `database view save/show/render`
- **AND** each example SHALL be directly runnable against a local vault path
- **AND** help output SHALL NOT require Notion API tokens, Obsidian plugin execution, JavaScript, Lark, firecrawl, or external network access.

## ADDED Requirements

### Requirement: Dataview-compatible query commands are available
Pinax SHALL provide a safe Dataview-compatible command surface for common Markdown database queries while compiling every query into Pinax's bounded query engine.

#### Scenario: Run Dataview table query
- **WHEN** a user runs `pinax dataview run 'TABLE title, status, due FROM #project WHERE status != "done" SORT due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `dataview.run`
- **AND** facts SHALL include language `dataview`, result kind `table`, source `notes`, row count, columns, engine, index status, and page facts
- **AND** rows SHALL contain bounded note/database projections without full note bodies.

#### Scenario: Run Dataview task query
- **WHEN** a user runs `pinax dataview run 'TASK FROM #project WHERE !completed SORT due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL return task rows derived from local Markdown task list items
- **AND** each row SHALL include source note path, line number, task text, completion state, due/scheduled dates when available, and stable note identity
- **AND** the command SHALL NOT call external Todo providers or write the vault.

#### Scenario: Reject DataviewJS
- **WHEN** a user runs `pinax dataview run 'dataviewjs await fetch("https://example.test")' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `dataviewjs_unsupported`
- **AND** no network, shell, JavaScript, provider, `.pinax` metadata, Git, or remote state SHALL be touched.

### Requirement: Database views support Dataview-style result kinds
Pinax SHALL let users save and render local database views for table, list, task, calendar, and board-style projections through CLI-authored view metadata.

#### Scenario: Save Dataview-backed database view
- **WHEN** a user runs `pinax database view save project-dashboard --query 'TABLE title, status FROM #project LIMIT 20' --language dataview --kind table --vault ./my-notes --json`
- **THEN** Pinax SHALL write or update `.pinax/views.json` through the application service
- **AND** the view definition SHALL record query language, query text, result kind, selected columns, display options, created/updated timestamps, and schema version
- **AND** stdout SHALL include command `database.view.save`, write facts, and a next action to show or render the view.

#### Scenario: Render saved database view as Markdown
- **WHEN** a user runs `pinax database view render project-dashboard --format markdown --vault ./my-notes --json`
- **THEN** Pinax SHALL execute the saved query through the same bounded query service
- **AND** `data` SHALL include a Markdown artifact or rendered block plus row/page facts
- **AND** rendering SHALL NOT write note files unless a separate explicit refresh/apply command is used.

### Requirement: Dataview managed blocks refresh safely
Pinax SHALL support managed Dataview-style fenced blocks in notes without allowing unmanaged body rewrites.

#### Scenario: Preview Dataview block without writing
- **WHEN** a note contains a fenced `pinax-dataview` query block
- **AND** a user runs `pinax note preview Dashboard --vault ./my-notes --json`
- **THEN** Pinax SHALL render a bounded preview of the query output
- **AND** no Markdown file, `.pinax` metadata, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Refresh managed Dataview block
- **WHEN** a note contains a `pinax:managed` Dataview block
- **AND** a user runs `pinax note refresh Dashboard --rendered --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the managed block output generated by Pinax
- **AND** stdout SHALL include changed block count, query count, index status, and record/index update facts
- **AND** user-authored prose outside the managed block SHALL remain unchanged.
