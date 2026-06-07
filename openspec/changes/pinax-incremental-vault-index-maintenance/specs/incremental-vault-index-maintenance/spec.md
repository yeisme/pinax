## ADDED Requirements

### Requirement: Pinax maintains index projection from vault file lifecycle events
Pinax SHALL maintain local index projection through structured vault file lifecycle events while keeping Markdown files as the source of truth.

#### Scenario: Pinax command emits known move event
- **WHEN** a user runs `pinax note move note_123 archive --vault ./my-notes --json`
- **THEN** Pinax SHALL emit or process a `note_moved` index event with note id, old path, new path, old hash when available, new hash, and evidence source `pinax_command`
- **AND** the command projection SHALL include index update facts.

#### Scenario: Pinax command emits known rename event
- **WHEN** a user runs `pinax note rename note_123 "New Title" --vault ./my-notes --json`
- **THEN** Pinax SHALL emit or process a `note_renamed` or `note_changed` index event with note id, old path, new path when changed, title evidence, and source `pinax_command`
- **AND** affected title, path, property, FTS, and link projection SHALL be updated incrementally.

#### Scenario: Pinax command emits delete event
- **WHEN** a user runs `pinax note delete note_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL emit or process a `note_deleted` index event with note id, old path, deletion mode, trash path when applicable, and evidence source `pinax_command`
- **AND** the deleted note's projection SHALL be removed or tombstoned without a full rebuild.

### Requirement: Note identity is stable across path changes
Pinax SHALL use note id as the primary note identity and SHALL NOT treat path as the sole identity for indexed notes.

#### Scenario: Move preserves note identity
- **WHEN** a note with frontmatter `note_id: note_123` moves from `notes/a.md` to `notes/archive/a.md`
- **THEN** Pinax SHALL preserve the same note identity in index projection
- **AND** it SHALL update path, folder, file properties, outgoing relative link resolution, and incoming path-based links as affected projections.

#### Scenario: External move with note id is recognized
- **WHEN** a user moves a Markdown file outside Pinax and keeps its `note_id`
- **AND** a user runs `pinax index sync --vault ./my-notes --json`
- **THEN** Pinax SHALL recognize the move as a strong match
- **AND** it SHALL update index projection incrementally rather than requiring a full rebuild.

#### Scenario: Ambiguous external move is not guessed
- **WHEN** external file changes create multiple possible rename or move candidates without a unique note id or content hash match
- **AND** a user runs `pinax index sync --vault ./my-notes --json`
- **THEN** Pinax SHALL report candidate moves with stable issue facts
- **AND** it SHALL NOT guess one candidate automatically.

### Requirement: Deleted notes use tombstone evidence
Pinax SHALL maintain short-lived tombstone evidence for deleted notes to support restore detection and backlink reclassification.

#### Scenario: Delete reclassifies incoming links
- **WHEN** a note is deleted from the vault
- **AND** other notes linked to that note
- **THEN** Pinax SHALL remove the deleted note's outgoing edges and content/property projection
- **AND** it SHALL reclassify incoming links as broken, ambiguous, or unresolved according to current vault state.

#### Scenario: Restore clears tombstone
- **WHEN** a deleted note reappears with the same note id or strong content hash evidence
- **AND** a user runs `pinax index sync --vault ./my-notes --json`
- **THEN** Pinax SHALL treat it as restore or move
- **AND** it SHALL clear or update the matching tombstone.

### Requirement: Incremental results match full rebuild
Pinax SHALL make incremental updates converge to the same queryable projection as a full rebuild for the same final vault state.

#### Scenario: Move rename delete sequence matches rebuild
- **WHEN** a fixture vault reaches a final state through note changed, moved, renamed, deleted, and restored events
- **AND** the same final state is indexed by full rebuild
- **THEN** note list, search, backlinks, database query, attachment, and property projection results SHALL be equivalent.

#### Scenario: Old epoch result is discarded
- **WHEN** a worker result from an old index epoch completes after a newer epoch has started
- **THEN** Pinax SHALL discard the old result before writer commit
- **AND** it SHALL NOT overwrite newer projection rows.

### Requirement: Incremental failures are recoverable
Pinax SHALL report stale or partial status when incremental maintenance cannot safely update projection.

#### Scenario: Incremental update failure marks partial
- **WHEN** an incremental writer transaction fails
- **THEN** Pinax SHALL preserve the last committed projection when possible
- **AND** index status SHALL be `partial` or `stale` with evidence and a next action for repair or rebuild.

#### Scenario: Repair chooses safe recovery
- **WHEN** index consistency checks find stale path rows, orphan tombstones, or ambiguous external move candidates
- **THEN** Pinax SHALL expose repair operations or rebuild next actions
- **AND** it SHALL NOT modify Markdown files without an explicit approved maintenance command.
