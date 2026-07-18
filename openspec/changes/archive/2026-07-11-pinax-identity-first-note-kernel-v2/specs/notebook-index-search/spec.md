## ADDED Requirements

### Requirement: Local index relations are object-first
The local SQLite/GORM projection SHALL use canonical object IDs as logical relation keys for notes, Tags, links, tasks, assets and properties. Current paths SHALL remain indexed and unique for active objects but SHALL be mutable locators rather than logical primary identity.

#### Scenario: Rebuild preserves object identity
- **WHEN** a user rebuilds the local index after notes were renamed or moved
- **THEN** Pinax SHALL read canonical IDs from ledger/frontmatter reconciliation, rebuild relations under those IDs and SHALL NOT generate replacement IDs from current paths.

#### Scenario: Tag and classification queries use object relations
- **WHEN** a user filters notes by project, group, folder, kind, status or Tag
- **THEN** the query SHALL return the stable object ID and current path for each note and SHALL join multi-value relations by object ID.

### Requirement: Search resolves ID, path and human references consistently
Search and shared object resolution SHALL support canonical object ID, legacy ID during migration, current path, title and alias while returning which field matched and whether the result is unique.

#### Scenario: Resolve a moved note by canonical ID
- **WHEN** a user or Agent queries a note by canonical object ID after its path changed
- **THEN** Pinax SHALL return the same note at its current path without a fallback full-vault identity guess.

#### Scenario: Resolve a legacy ID during migration
- **WHEN** a caller supplies a legacy note ID with an active compatibility mapping
- **THEN** Pinax SHALL resolve the canonical object, disclose that a compatibility mapping was used and return a migration next action where appropriate.
