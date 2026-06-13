## MODIFIED Requirements

### Requirement: Local index database is initialized and rebuilt through Pinax
Pinax SHALL create and maintain a local SQLite/GORM index projection for notebook search and organization without making the database the source of truth. The index SHALL rebuild from the record ledger plus Markdown content plus version evidence, using ledger note identity and lifecycle facts as machine truth and Markdown files as body content input.

#### Scenario: Initialize index database
- **WHEN** a user runs `pinax index init --vault ./my-notes --json`
- **THEN** Pinax SHALL create `.pinax/index.sqlite` through the application service
- **AND** the database SHALL contain schema metadata for the supported index version
- **AND** stdout SHALL include index path, schema version, record ledger schema version, version backend, current revision, and status facts.

#### Scenario: Rebuild index with full note projection
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL scan Markdown notes inside the vault boundary and rebuild note, text, tag, token, link, attachment, lifecycle, record consistency, version evidence, and dimension count projections through GORM
- **AND** indexed note identity SHALL come from the record ledger when a matching record exists
- **AND** indexed rows SHALL store ledger sequence, index epoch, version backend, revision id, worktree state, file blob id when available, diff summary hash when available, and content hash
- **AND** Markdown notes without records SHALL be indexed as unregistered candidates with consistency facts
- **AND** system index notes SHALL be marked so ordinary note statistics and orphan detection can exclude them.

#### Scenario: Index status reports freshness
- **WHEN** a user runs `pinax index status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `fresh`, `stale`, `missing`, or `unreadable`
- **AND** stale results SHALL include evidence such as changed note path, modified time, size, content hash, ledger sequence, registry version, version backend revision, diff summary hash, or record consistency issue.

## ADDED Requirements

### Requirement: Search explains record consistency risks
Pinax SHALL expose whether search results came from consistent ledger-backed records, unregistered Markdown files, missing files, or stale projections.

#### Scenario: Search returns consistency facts
- **WHEN** a user runs `pinax search "设计" --vault ./my-notes --json`
- **THEN** each result SHALL include stable consistency facts such as `record_status`, `lifecycle_status`, `record_version`, `ledger_seq`, `index_epoch`, `content_hash`, `path_status`, `version_backend`, `revision_id`, and `worktree_state`
- **AND** results with unresolved ledger or frontmatter issues SHALL include a next action for record status or repair planning.

#### Scenario: Search excludes deleted records by default
- **GIVEN** the record ledger contains trashed or deleted notes
- **WHEN** a user runs `pinax search "设计" --vault ./my-notes --json`
- **THEN** Pinax SHALL exclude deleted lifecycle states by default
- **AND** it SHALL include them only when the user selects an explicit trash, deleted, or all lifecycle filter.

### Requirement: Search supports version-aware queries
Pinax SHALL support version-aware local search by using version backend evidence and SHALL clearly report when historical content cannot be read by the configured backend.

#### Scenario: Search at current HEAD
- **GIVEN** the vault is Git-backed and the current HEAD is readable
- **WHEN** a user runs `pinax search "设计" --at HEAD --vault ./my-notes --json`
- **THEN** Pinax SHALL search content associated with HEAD or a HEAD index snapshot
- **AND** each result SHALL include revision id, file blob id when available, ledger sequence, index epoch, and whether dirty worktree changes were excluded.

#### Scenario: Search including dirty changes
- **GIVEN** the vault has uncommitted Markdown changes
- **WHEN** a user runs `pinax search "设计" --include-dirty --vault ./my-notes --json`
- **THEN** Pinax SHALL search the current working tree projection
- **AND** each affected result SHALL include `worktree_state=dirty` and diff summary hash facts.

#### Scenario: Search changed notes since revision
- **GIVEN** the version backend can list changed paths since revision `abc123`
- **WHEN** a user runs `pinax search "索引" --changed-since abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL restrict candidate parsing to notes changed since that revision before applying query filters
- **AND** stdout SHALL include changed path count, indexed candidate count, backend revision facts, and fallback status.

#### Scenario: Historical search unavailable
- **GIVEN** the version backend cannot read files at revision `abc123`
- **WHEN** a user runs `pinax search "索引" --revision abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `version_read_unavailable`
- **AND** no index database, Markdown file, record asset, Git state, provider state, or remote service SHALL be modified.

### Requirement: Index maintenance is incremental and bounded
Pinax SHALL optimize index maintenance by skipping unchanged notes, batching writes, bounding worker concurrency, and exposing progress and freshness diagnostics.

#### Scenario: Incremental refresh skips unchanged notes
- **GIVEN** the index has recorded ledger sequence, content hashes, file sizes, modified times, and version evidence for notes
- **WHEN** a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL parse only notes with changed ledger facts, changed version evidence, changed mtime/size, changed content hash, or missing projection rows
- **AND** stdout SHALL include scanned count, changed count, skipped count, indexed count, batch count, and duration facts.

#### Scenario: Rebuild reports checkpoint progress
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL process notes in batches and expose progress facts for total candidates, completed candidates, current batch, ledger sequence, index epoch, and last checkpoint
- **AND** a failed rebuild SHALL leave enough checkpoint or status evidence for a later command to explain whether it must restart or can resume.

#### Scenario: Low-memory index mode bounds concurrency
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --memory-budget low --json`
- **THEN** Pinax SHALL use bounded parse workers, bounded result queues, smaller SQLite batches, and streaming file reads
- **AND** it SHALL NOT retain all Markdown bodies, snippets, binary payloads, or historical revision contents in memory.

#### Scenario: SQLite projection uses batched writer semantics
- **WHEN** Pinax rebuilds or refreshes the local index
- **THEN** parse, hash, token, link, and property extraction MAY run concurrently
- **AND** SQLite projection writes SHALL be committed through bounded batch transactions that preserve one index epoch transition per completed batch or rebuild.

### Requirement: Search latency favors projection reads over full scans
Pinax SHALL serve ordinary search, list, table, and database-view queries from SQLite projections when fresh and SHALL use full Markdown scans only as an explicit fallback.

#### Scenario: Fresh search avoids full vault scan
- **GIVEN** `.pinax/index.sqlite` is fresh for the current ledger sequence and version evidence
- **WHEN** a user runs `pinax search "设计" --vault ./my-notes --json`
- **THEN** Pinax SHALL query the SQLite/FTS projection rather than scanning every Markdown file
- **AND** stdout SHALL include `engine=index`, index epoch, ledger sequence, and freshness facts.

#### Scenario: Fallback scan is bounded and explicit
- **GIVEN** the index is missing or stale
- **WHEN** a user runs `pinax search "设计" --allow-scan --memory-budget low --vault ./my-notes --json`
- **THEN** Pinax SHALL run a bounded local scan with limited workers and streaming reads
- **AND** stdout SHALL include `engine=scan`, index status, scanned count, and warning facts.
