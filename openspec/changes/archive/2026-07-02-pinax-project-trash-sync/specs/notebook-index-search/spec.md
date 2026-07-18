## ADDED Requirements

### Requirement: Index maintenance respects trash lifecycle
Pinax index refresh and rebuild SHALL use ledger lifecycle and trash tombstones to remove or hide deleted object projections without treating the index as the source of truth.

#### Scenario: Project delete removes board/search projection references
- **GIVEN** project `history` exists in the project registry and index projections
- **WHEN** the user runs `pinax project delete history --vault ./my-notes --yes --json`
- **AND** Pinax refreshes the index
- **THEN** project board, search, graph, and recent projections SHALL NOT return `history` as an active project
- **AND** trash-aware commands MAY still expose the tombstone.

#### Scenario: Rebuild excludes trash backups from ordinary notes
- **GIVEN** `.pinax/trash/20260627/projects/history/` contains Markdown backups
- **WHEN** the user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL NOT index trash backup Markdown as ordinary active notes
- **AND** it SHALL preserve restore metadata for `pinax trash list` or `pinax trash show`.

#### Scenario: Deleted lifecycle filter is explicit
- **WHEN** a user runs `pinax search "历史" --vault ./my-notes --json`
- **THEN** search SHALL exclude trashed and deleted lifecycle states by default
- **AND** future trash/deleted filters SHALL be explicit rather than silently mixed into ordinary search results.
