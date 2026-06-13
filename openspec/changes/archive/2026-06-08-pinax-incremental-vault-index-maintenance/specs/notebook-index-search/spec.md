## MODIFIED Requirements

### Requirement: Local index database is initialized and rebuilt through Pinax
Pinax SHALL create and maintain a local SQLite/GORM index projection for notebook search and organization without making the database the source of truth, and SHALL support incremental maintenance after the initial rebuild.

#### Scenario: Initialize index database
- **WHEN** a user runs `pinax index init --vault ./my-notes --json`
- **THEN** Pinax SHALL create `.pinax/index.sqlite` through the application service
- **AND** the database SHALL contain schema metadata for the supported index version
- **AND** stdout SHALL include index path, schema version, and status facts.

#### Scenario: Rebuild index with full note projection
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL scan Markdown notes inside the vault boundary and rebuild note, text, tag, token, link, attachment, and dimension count projections through GORM
- **AND** system index notes SHALL be marked so ordinary note statistics and orphan detection can exclude them.

#### Scenario: Index status reports freshness
- **WHEN** a user runs `pinax index status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `fresh`, `stale`, `partial`, `missing`, or `unreadable`
- **AND** stale or partial results SHALL include evidence such as changed note path, deleted note path, new note path, modified time, size, content hash, schema version mismatch, or pending index events.

#### Scenario: Sync external file changes incrementally
- **WHEN** Markdown files are changed, moved, renamed, restored, or deleted outside Pinax
- **AND** a user runs `pinax index sync --vault ./my-notes --json`
- **THEN** Pinax SHALL scan file facts, reconcile differences against indexed facts, and apply safe incremental updates
- **AND** stdout SHALL include counts for created, changed, moved, renamed, deleted, restored, skipped, candidates, and failed events.

#### Scenario: Sync reports ambiguous external changes
- **WHEN** `pinax index sync` cannot uniquely classify an external path change as move, rename, delete, or create
- **THEN** Pinax SHALL report stable candidate facts and mark index status as `partial` or `stale`
- **AND** it SHALL include next actions for `pinax index repair` or `pinax index rebuild`.

### Requirement: Search uses local index with safe fallbacks
Pinax SHALL search the local notebook using the index when fresh and degrade to local scan or ripgrep fallback when needed.

#### Scenario: Search through fresh index
- **WHEN** a user runs `pinax search "认证" --tag auth --kind reference --limit 20 --vault ./my-notes --json`
- **THEN** Pinax SHALL query the local index projection first
- **AND** stdout SHALL include `engine=index`, `index_status=fresh`, total count, returned count, selected filters, result scores, matched fields, snippets, and note projections.

#### Scenario: Search with stale index warning
- **WHEN** the index is stale and a user runs `pinax search "认证" --allow-stale --vault ./my-notes --json`
- **THEN** Pinax SHALL return index results with status `partial`
- **AND** stdout SHALL include `index_status=stale` and an action recommending `pinax index sync` or `pinax index rebuild`.

#### Scenario: Search falls back without index
- **WHEN** `.pinax/index.sqlite` is missing and a user runs `pinax search "认证" --vault ./my-notes --json`
- **THEN** Pinax SHALL use `rg` when available or in-process scan otherwise
- **AND** facts SHALL identify the fallback engine without requiring external network access.

#### Scenario: Search after move uses updated path
- **WHEN** a note is moved and the move has been processed by incremental index maintenance
- **AND** a user runs `pinax search` or `pinax note list --json`
- **THEN** returned note projections SHALL use the new path
- **AND** stale old path rows SHALL NOT appear in ordinary results.

## ADDED Requirements

### Requirement: Index sync reconciles file facts
Pinax SHALL reconcile current vault file facts with indexed file facts to detect external changes.

#### Scenario: Hash unchanged skips work
- **WHEN** a Markdown file has unchanged path, size, modified time, and content hash compared with the index record
- **THEN** `pinax index sync` SHALL skip parsing and writing that note
- **AND** it SHALL report the skip count in machine output.

#### Scenario: Content changed updates self projection
- **WHEN** a Markdown file path is unchanged but content hash changes
- **THEN** `pinax index sync` SHALL update that note's text, token, tag, link, attachment, property, and dimension projection
- **AND** it SHALL update affected incoming link state when title, aliases, note id, or path-derived fields changed.
