## MODIFIED Requirements

### Requirement: Local index database is initialized and rebuilt through Pinax
Pinax SHALL create and maintain a local SQLite/GORM index projection for notebook search, organization, database views, typed properties, and query planning without making the database the source of truth.

#### Scenario: Initialize index database
- **WHEN** a user runs `pinax index init --vault ./my-notes --json`
- **THEN** Pinax SHALL create `.pinax/index.sqlite` through the application service
- **AND** the database SHALL contain schema metadata for the supported index version
- **AND** stdout SHALL include index path, schema version, and status facts.

#### Scenario: Rebuild index with full note projection
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL scan Markdown notes inside the vault boundary and rebuild note, text, tag, token, link, attachment, dimension count, property definition, and typed property value projections through GORM
- **AND** system index notes SHALL be marked so ordinary note statistics and orphan detection can exclude them.

#### Scenario: Index status reports freshness
- **WHEN** a user runs `pinax index status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `fresh`, `stale`, `missing`, or `unreadable`
- **AND** stale results SHALL include evidence such as changed note path, modified time, size, content hash, schema version mismatch, or property projection schema mismatch.

#### Scenario: Rebuild property projection
- **WHEN** Markdown notes contain frontmatter, inline fields, tags, links, attachments, and system fields
- **AND** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL index queryable typed properties with stable property ids, normalized names, source kind, inferred type, and typed values
- **AND** property projection SHALL be rebuildable from Markdown notes and CLI-authored schema metadata.

### Requirement: Search uses local index with safe fallbacks
Pinax SHALL search and query the local notebook using the index when fresh and degrade to local scan or ripgrep fallback only for supported simple search paths when needed.

#### Scenario: Search through fresh index
- **WHEN** a user runs `pinax search "认证" --tag auth --kind reference --limit 20 --vault ./my-notes --json`
- **THEN** Pinax SHALL query the local index projection first
- **AND** stdout SHALL include `engine=index`, `index_status=fresh`, total count, returned count, selected filters, result scores, matched fields, snippets, and note projections.

#### Scenario: Search with stale index warning
- **WHEN** the index is stale and a user runs `pinax search "认证" --allow-stale --vault ./my-notes --json`
- **THEN** Pinax SHALL return index results with status `partial`
- **AND** stdout SHALL include `index_status=stale` and an action recommending `pinax index rebuild`.

#### Scenario: Search falls back without index
- **WHEN** `.pinax/index.sqlite` is missing and a user runs `pinax search "认证" --vault ./my-notes --json`
- **THEN** Pinax SHALL use `rg` when available or in-process scan otherwise
- **AND** facts SHALL identify the fallback engine without requiring external network access.

#### Scenario: Search can lazy-load index on first run
- **WHEN** `.pinax/index.sqlite` is missing or stale and a user runs `pinax search "认证" --vault ./my-notes --json`
- **AND** the query uses index-supported local filters and the vault is within the configured lazy-load cost budget
- **THEN** Pinax MAY rebuild or refresh the local index projection before searching
- **AND** stdout SHALL include `engine=index`, `index_status=fresh`, and a fact such as `index_loaded=lazy_rebuild`
- **AND** the lazy rebuild SHALL be bounded, cancellable via context, local-only, and SHALL NOT write Markdown notes, provider state, Git state, or remote services.

#### Scenario: Lazy-load search degrades when cost is high
- **WHEN** lazy index rebuild is disabled, over budget, unsupported, or fails
- **THEN** Pinax SHALL fall back to the existing local search behavior or return a stable index error depending on command semantics
- **AND** stdout SHALL include evidence and a next action recommending `pinax index rebuild --vault <vault>`.

#### Scenario: Database query requires queryable projection
- **WHEN** `.pinax/index.sqlite` is missing or lacks the typed property projection
- **AND** a user runs `pinax query run 'SELECT title FROM notes WHERE status = "active" LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `index_required` or `property_index_stale`
- **AND** stdout SHALL include a next action recommending `pinax index rebuild` rather than performing an expensive full-vault database query.

#### Scenario: Database query lazy-load is explicit
- **WHEN** `.pinax/index.sqlite` is missing or stale
- **AND** a user runs `pinax query run 'SELECT title FROM notes WHERE status = "active" LIMIT 20' --lazy-index --vault ./my-notes --json`
- **THEN** Pinax MAY rebuild the typed property projection before query execution when the query is safe and within budget
- **AND** the result SHALL include index load facts, timing or cost evidence, and the final engine/index status
- **AND** without `--lazy-index`, database queries SHALL prefer a clear index-required error over surprising expensive work.

## ADDED Requirements

### Requirement: Query planner uses safe indexed execution
Pinax SHALL plan database queries from a validated query AST and execute them through repository boundaries with parameter binding.

#### Scenario: Plan property-filtered table query
- **WHEN** a user runs `pinax query explain 'SELECT title, due FROM notes WHERE tags CONTAINS "project" AND status = "active" AND due <= date(today) ORDER BY due ASC LIMIT 20' --vault ./my-notes --json`
- **THEN** Pinax SHALL report source filters, property filters, selected indexes, selected properties, sort keys, limit, and fallback warnings
- **AND** it SHALL NOT expose raw SQL text by default.

#### Scenario: Execute selected property query
- **WHEN** a table query selects `title`, `status`, and `due`
- **THEN** the repository SHALL load only those selected properties plus row identity and required sort/filter fields
- **AND** it SHALL NOT load note bodies by default.

#### Scenario: Reject unsafe query at planner boundary
- **WHEN** query validation detects unsupported clauses, forbidden functions, path escape attempts, or excessive limits
- **THEN** Pinax SHALL fail before repository execution with a stable error code
- **AND** no index database or Markdown file SHALL be modified.

### Requirement: Query pagination and limits are stable
Pinax SHALL support stable limits and cursor pagination for database query results.

#### Scenario: Default query limit applies
- **WHEN** a query omits `LIMIT`
- **THEN** Pinax SHALL apply a safe default result limit
- **AND** stdout SHALL include an action or cursor for retrieving more rows when more matches exist.

#### Scenario: Cursor returns next page
- **WHEN** a query result includes `has_more=true`
- **AND** a user reruns the query with the returned cursor
- **THEN** Pinax SHALL return the next stable page for the same query shape
- **AND** stdout SHALL include updated pagination facts.
