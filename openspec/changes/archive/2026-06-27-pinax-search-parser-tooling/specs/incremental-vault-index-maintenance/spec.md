# incremental-vault-index-maintenance Delta

## ADDED Requirements

### Requirement: Index refresh uses bounded parsing concurrency and single-writer commits
Pinax SHALL parse changed Markdown notes with bounded worker concurrency while keeping SQLite writes under a single writer boundary.

#### Scenario: Concurrent parser results are cancelled on error
- **WHEN** an index refresh worker encounters an unreadable changed note
- **THEN** Pinax SHALL cancel remaining parse work for the current refresh
- **AND** it SHALL preserve the last committed projection where possible
- **AND** it SHALL report `index_status=partial` with failed path evidence.

#### Scenario: Batch refresh reports performance facts
- **WHEN** a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** stdout facts SHALL include scanned, changed, skipped, indexed, batches, and duration facts
- **AND** implementation SHALL avoid opening and migrating the database once per note.

### Requirement: Markdown note parsing is centralized
Pinax SHALL use a shared Markdown note parser for note metadata, AST-derived structure, and projection inputs.

#### Scenario: Parser handles common Markdown note structure
- **WHEN** Pinax parses a registered note with YAML frontmatter, headings, links, assets, tasks, inline properties, and fenced query blocks
- **THEN** the parser SHALL return stable structured fields for those elements
- **AND** index/search/link/property/task projections SHALL consume the shared parse result rather than maintaining unrelated parsers for the same note body.
