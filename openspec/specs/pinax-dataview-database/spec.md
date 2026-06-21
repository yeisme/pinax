# pinax-dataview-database Specification

## Purpose
TBD - created by archiving change pinax-dataview-database-query. Update Purpose after archive.
## Requirements
### Requirement: Pinax SQL v2 remains safe and bounded
Pinax SHALL support a larger read-only SQL subset for notes database workflows while preventing raw SQLite access and unsafe functions.

#### Scenario: Type-aware filtering
- **WHEN** a vault contains frontmatter and inline fields with strings, numbers, booleans, dates, lists, tags, and links
- **AND** a user runs `pinax query run 'SELECT title, priority, due FROM notes WHERE priority >= 2 AND due <= "2026-06-30" LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL compare values according to inferred or configured property types
- **AND** mixed or invalid values SHALL be returned as warnings or excluded according to stable documented rules, not cause raw panics.

#### Scenario: Group and aggregate query
- **WHEN** a user runs `pinax query run 'SELECT status, COUNT(*) AS count FROM notes WHERE tags CONTAINS "project" GROUP BY status LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL return grouped rows with aggregate values
- **AND** facts SHALL include selected columns, group count, aggregate names, row count, and page facts.

#### Scenario: Unsafe SQL is rejected
- **WHEN** a user runs `pinax query run 'DROP TABLE notes' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `sql_unsupported_clause`
- **AND** no Markdown file, index database, structured asset, provider state, Git state, or remote service SHALL be modified.

### Requirement: Query sources cover notebook database primitives
Pinax SHALL expose bounded query sources for notes, tasks, links, backlinks, and assets.

#### Scenario: Query links source
- **WHEN** a user runs `pinax query run 'SELECT source, target, status FROM links WHERE status != "resolved" LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL return link rows from the maintained relationship projection
- **AND** rows SHALL include source path, raw target, resolved target when available, link kind, and status
- **AND** the command SHALL NOT mutate links or repair note bodies.

#### Scenario: Query assets source
- **WHEN** a user runs `pinax query run 'SELECT path, media_type, linked_notes FROM assets WHERE missing = true LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL return bounded asset rows from the asset/index projection
- **AND** missing and orphan facts SHALL be inspectable without deleting or moving files.

### Requirement: Query output is agent-safe
Pinax SHALL keep database query output bounded and safe across default, JSON, agent, events, explain, saved view, and managed block surfaces.

#### Scenario: Agent output contains low-token facts
- **WHEN** a user runs `pinax dataview run 'TABLE title FROM #project LIMIT 5' --vault ./my-notes --agent`
- **THEN** stdout SHALL include `spec_version`, `mode=agent`, `command=dataview.run`, `status`, `fact.language`, `fact.kind`, `fact.source`, `fact.rows`, `fact.columns`, and next action keys when useful
- **AND** stdout SHALL NOT include localized prose, ANSI tables, full note bodies, raw prompts, provider payloads, Authorization headers, secrets, or full chain-of-thought.

#### Scenario: Explain output is reviewable
- **WHEN** a user runs `pinax dataview explain 'TABLE title FROM #project LIMIT 5' --vault ./my-notes --explain`
- **THEN** stdout SHALL contain a reviewable explanation with conclusion, parsed query shape, selected source, warnings, risk, confidence, and next step
- **AND** it SHALL NOT contain full chain-of-thought, raw prompts, hidden system prompts, private tool arguments, provider payloads, cookies, Authorization headers, or secrets.

