## MODIFIED Requirements

### Requirement: Note commands support ergonomic creation
Pinax SHALL let users create notes from multiple safe content sources while preserving Markdown body content as the content source and creating record ledger facts for machine identity and lifecycle.

#### Scenario: Create note from inline body
- **WHEN** a user runs `pinax note new "研究日志" --body "正文" --tags research --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note with Pinax frontmatter and the provided body
- **AND** Pinax SHALL append a record ledger event and update the note registry for the created note
- **AND** stdout SHALL contain one JSON envelope with command `note.new`, created path, note id, record version, version backend, revision id, worktree state, title, and next actions.

#### Scenario: Create note from stdin
- **WHEN** a user pipes Markdown to `pinax note create "会议" --stdin --vault ./my-notes --json`
- **THEN** Pinax SHALL read note body from stdin through the command layer
- **AND** the application service SHALL create the note and its record ledger facts without reading external network resources.

#### Scenario: Reject conflicting note body sources
- **WHEN** a user runs `pinax note new "x" --body a --from ./a.md --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `note_source_conflict`
- **AND** no note file or record ledger event SHALL be created.

#### Scenario: Dry run note creation
- **WHEN** a user runs `pinax note new "Draft" --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return planned path, frontmatter, record event preview, registry preview, and body preview
- **AND** it SHALL NOT write Markdown files, `.pinax/` state, Git state, provider state, or remote services.

### Requirement: Note creation builds notebook information architecture
Pinax SHALL make newly created notes immediately discoverable through group, folder, kind, tags, daily index, record ledger, and local index projections.

#### Scenario: Create note with group folder and kind
- **WHEN** a user runs `pinax note new "工具笔记" --group work --folder inbox --kind reference --tags pinax,cli --vault ./my-notes --json`
- **THEN** Pinax SHALL create the note under the selected group/project prefix and folder
- **AND** the note frontmatter SHALL include `project`, `folder`, `kind`, and `tags`
- **AND** the record ledger SHALL store the note id, path, lifecycle state, record version, schema version, and content hash
- **AND** the JSON envelope facts SHALL include group, folder, kind, daily index path, record update status, version evidence status, and index update status.

#### Scenario: Created note is added to daily index
- **WHEN** a note is created without `--dry-run`
- **THEN** Pinax SHALL update `notes/daily/YYYY-MM-DD.md` through the application service
- **AND** the daily index SHALL include the note title, path, tags, group, folder, kind, and note id.

#### Scenario: Created note refreshes local index
- **WHEN** a note is created without `--dry-run`
- **THEN** Pinax SHALL refresh `.pinax/index.sqlite` through the GORM index service using ledger and Markdown inputs
- **AND** a following `pinax stats --vault ./my-notes --json` SHALL report `index_status=fresh`.

### Requirement: Single-note maintenance operations are safe
Pinax SHALL support common single-note maintenance operations while protecting vault boundaries, record ledger invariants, and destructive actions.

#### Scenario: Rename note title and path
- **WHEN** a user runs `pinax note rename note_123 "New Title" --vault ./my-notes --json`
- **THEN** Pinax SHALL update frontmatter title and choose a safe target path inside the vault
- **AND** it SHALL append a record event for the rename and update the registry current path and title mirror
- **AND** it SHALL attach version evidence for the before and after path when the configured backend supports file revision facts
- **AND** it SHALL fail with stable error code `note_path_conflict` if target path already exists.

#### Scenario: Archive note without moving file
- **WHEN** a user runs `pinax note archive note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL set frontmatter `status: archived` when the mirror is enabled
- **AND** it SHALL update the ledger lifecycle state to `archived`
- **AND** it SHALL NOT move or delete the Markdown file.

#### Scenario: Delete note moves to trash by default
- **WHEN** a user runs `pinax note delete note_123 --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL move the note to `.pinax/trash/` through the application service
- **AND** it SHALL append redacted event evidence
- **AND** it SHALL update the record ledger lifecycle state, tombstone evidence, and version evidence.

#### Scenario: Hard delete requires explicit hard approval
- **WHEN** a user runs `pinax note delete note_123 --hard --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** the note file and record ledger SHALL remain unchanged.
