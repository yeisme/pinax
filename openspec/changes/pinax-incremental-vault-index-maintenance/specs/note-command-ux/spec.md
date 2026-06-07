## MODIFIED Requirements

### Requirement: Single-note maintenance operations are safe
Pinax SHALL support common single-note maintenance operations while protecting vault boundaries, destructive actions, and index projection consistency.

#### Scenario: Rename note title and path
- **WHEN** a user runs `pinax note rename note_123 "New Title" --vault ./my-notes --json`
- **THEN** Pinax SHALL update frontmatter title and choose a safe target path inside the vault
- **AND** it SHALL fail with stable error code `note_path_conflict` if target path already exists
- **AND** it SHALL process or enqueue a structured index event for the rename with old path, new path, note id, and content hash evidence.

#### Scenario: Archive note without moving file
- **WHEN** a user runs `pinax note archive note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL set frontmatter `status: archived`
- **AND** it SHALL NOT move or delete the Markdown file
- **AND** it SHALL update status property and search/list projection incrementally.

#### Scenario: Delete note moves to trash by default
- **WHEN** a user runs `pinax note delete note_123 --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL move the note to `.pinax/trash/` through the application service
- **AND** it SHALL append redacted event evidence
- **AND** it SHALL process or enqueue a delete index event that removes ordinary note projection and updates affected backlinks.

#### Scenario: Hard delete requires explicit hard approval
- **WHEN** a user runs `pinax note delete note_123 --hard --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** the note file SHALL remain unchanged.

#### Scenario: Move note updates index facts
- **WHEN** a user runs `pinax note move note_123 archive --vault ./my-notes --json`
- **THEN** Pinax SHALL move the note inside the vault and update selected frontmatter fields when requested
- **AND** JSON facts SHALL include path, note id, index event kind, index update status, and affected projection counts when available.

#### Scenario: Editor mutation refreshes changed note projection
- **WHEN** a user runs `pinax note edit note_123 --editor fake-editor --vault ./my-notes --json`
- **AND** the editor changes the Markdown file
- **THEN** Pinax SHALL detect the changed content hash after editor exit
- **AND** it SHALL update or enqueue incremental projection refresh for that note.

### Requirement: Note commands expose index update facts
Pinax SHALL expose stable index update facts for note commands that mutate Markdown files.

#### Scenario: Mutation reports committed index update
- **WHEN** a note mutation command updates the index projection synchronously
- **THEN** stdout facts SHALL include `index_update=committed`, `index_status=fresh`, and the relevant event kind
- **AND** machine output SHALL remain parseable under the CLI output contract.

#### Scenario: Mutation reports queued index update
- **WHEN** a note mutation command cannot wait for incremental index completion
- **THEN** stdout facts SHALL include `index_update=queued` and a next action for `pinax index status --refresh`
- **AND** the command SHALL NOT claim the index is fresh until the update has committed.
