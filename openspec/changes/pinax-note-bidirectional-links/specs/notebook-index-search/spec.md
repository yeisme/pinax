## MODIFIED Requirements

### Requirement: Local index database is initialized and rebuilt through Pinax
Pinax SHALL create and maintain a local SQLite/GORM index projection for notebook search, organization, and bidirectional link graph queries without making the database the source of truth.

#### Scenario: Initialize index database
- **WHEN** a user runs `pinax index init --vault ./my-notes --json`
- **THEN** Pinax SHALL create `.pinax/index.sqlite` through the application service
- **AND** the database SHALL contain schema metadata for the supported index version
- **AND** stdout SHALL include index path, schema version, and status facts.

#### Scenario: Rebuild index with full note projection
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL scan Markdown notes inside the vault boundary and rebuild note, text, tag, token, link, attachment, and dimension count projections through GORM
- **AND** link projection SHALL preserve source path, raw target, normalized target text, alias, heading, resolved target path, resolved target note id when available, link kind, line number when available, and status `resolved`, `broken`, `ambiguous`, `external`, or `ignored`
- **AND** system index notes SHALL be marked so ordinary note statistics and orphan detection can exclude them.

#### Scenario: Index status reports freshness
- **WHEN** a user runs `pinax index status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `fresh`, `stale`, `missing`, or `unreadable`
- **AND** stale results SHALL include evidence such as changed note path, modified time, size, content hash, or schema version mismatch.

#### Scenario: Old link schema is stale
- **WHEN** `.pinax/index.sqlite` exists but lacks the supported bidirectional link schema version
- **AND** a user runs `pinax index status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `index_status=stale`
- **AND** stdout SHALL include a next action recommending `pinax index rebuild`.

#### Scenario: Incremental update after note content changes
- **WHEN** a user changes one Markdown note after a fresh full index rebuild
- **AND** Pinax receives or detects a `note_changed` index event for that note
- **THEN** Pinax SHALL update only that note's note, text, tag, token, link, attachment, dimension, and FTS projections plus source notes whose link resolution is affected by the changed note
- **AND** it SHALL NOT rescan unrelated Markdown notes.

#### Scenario: Incremental update skips unchanged content
- **WHEN** a note write event has the same content hash, path, modified time, and size as the current index record
- **THEN** Pinax SHALL skip parser and writer work for that note
- **AND** index status SHALL remain `fresh`.

#### Scenario: Deleted note updates backlinks incrementally
- **WHEN** a Markdown note is deleted after a fresh full index rebuild
- **AND** other notes linked to the deleted note
- **THEN** Pinax SHALL remove the deleted note projection and its outgoing edges
- **AND** it SHALL reclassify affected backlinks as `broken` or `ambiguous` without rebuilding the full vault index.

#### Scenario: Stale epoch results are discarded
- **WHEN** an old rebuild or incremental worker result completes after a newer index epoch has started
- **THEN** Pinax SHALL discard the old result before writer commit
- **AND** it SHALL NOT overwrite the newer index projection.

#### Scenario: Incremental update reports runtime facts
- **WHEN** a user runs `pinax index status --vault ./my-notes --json` after incremental indexing
- **THEN** stdout SHALL include stable facts for index status, schema version, indexed note count, last indexed time, and optional runtime counters such as queued, parsed, indexed, failed, and epoch when available.

#### Scenario: Full rebuild and incremental result match
- **WHEN** a fixture vault is indexed by full rebuild
- **AND** the same final vault state is reached through incremental note changed, moved, and deleted events
- **THEN** Pinax SHALL produce equivalent note, link, dimension, and search query results for the fixture.

### Requirement: Search filters cover notebook organization dimensions
Pinax SHALL let users combine full-text query with local notebook filters, including bidirectional link relationship filters.

#### Scenario: Filter search by organization dimensions
- **WHEN** a user runs `pinax search "设计" --group work --folder architecture --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL return only matching notes
- **AND** JSON facts SHALL include stable keys for group, folder, kind, and status filters.

#### Scenario: Filter search by links and attachments
- **WHEN** a user runs `pinax search "" --link-target "Auth" --has-attachment --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes with matching resolved or unresolved link targets and at least one attachment reference
- **AND** each result SHALL include link and attachment summary counts.

#### Scenario: Filter search by resolved backlink target
- **WHEN** a user runs `pinax search "" --link-target note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes whose outgoing link graph resolves to the selected note
- **AND** facts SHALL include `link_target`, `resolved`, `broken`, `ambiguous`, `engine`, and `index_status`.

#### Scenario: Filter search by ambiguous link target
- **WHEN** a user runs `pinax search "" --link-target "会议" --vault ./my-notes --json`
- **AND** multiple notes can satisfy the target
- **THEN** Pinax SHALL fail with stable error code `link_target_ambiguous` or return partial facts with candidate paths
- **AND** it SHALL NOT choose one target automatically.

#### Scenario: Invalid search filter fails clearly
- **WHEN** a user runs `pinax search "x" --updated-after not-a-date --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_date_filter`
- **AND** no index database or Markdown file SHALL be modified.
