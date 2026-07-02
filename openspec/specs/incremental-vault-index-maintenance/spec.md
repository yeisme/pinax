# incremental-vault-index-maintenance Specification

## Purpose
TBD - created by archiving change pinax-incremental-vault-index-maintenance. Update Purpose after archive.
## Requirements
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

### Requirement: Folder lifecycle events update local projections
Pinax SHALL update local folder, note, asset, and link projections from structured folder lifecycle events emitted by Pinax commands or APIs.

#### Scenario: Folder create updates folder projection
- **WHEN** a user runs `pinax folder create projects/research --purpose notes --vault ./my-notes --json`
- **THEN** Pinax SHALL emit or process a `folder_created` index event with folder path, purpose, managed status, and evidence source `pinax_command`
- **AND** `pinax folder list --include-empty --vault ./my-notes --json` SHALL include the new folder without requiring manual index file edits.

#### Scenario: Folder rename updates affected note projections
- **WHEN** a folder rename moves registered notes from `inbox/` to `archive/`
- **THEN** Pinax SHALL preserve note identity, update note path and folder properties, and mark affected relative link or attachment projections stale when necessary
- **AND** the command projection SHALL include index update or stale-index facts.

#### Scenario: Folder delete removes empty folder projection
- **WHEN** a CLI-authored empty folder is deleted through `pinax folder delete inbox --empty-only --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove or tombstone the folder projection and registry entry
- **AND** it SHALL NOT remove note, asset, or link projections unrelated to that folder.

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

