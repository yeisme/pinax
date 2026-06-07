## ADDED Requirements

### Requirement: Note commands support ergonomic creation
Pinax SHALL let users create notes from multiple safe content sources while preserving Markdown files as the source of truth.

#### Scenario: Create note from inline body
- **WHEN** a user runs `pinax note new "研究日志" --body "正文" --tags research --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note with Pinax frontmatter and the provided body
- **AND** stdout SHALL contain one JSON envelope with command `note.new`, created path, note id, title, and next actions.

#### Scenario: Create note from stdin
- **WHEN** a user pipes Markdown to `pinax note create "会议" --stdin --vault ./my-notes --json`
- **THEN** Pinax SHALL read note body from stdin through the command layer
- **AND** the application service SHALL create the note without reading external network resources.

#### Scenario: Reject conflicting note body sources
- **WHEN** a user runs `pinax note new "x" --body a --from ./a.md --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `note_source_conflict`
- **AND** no note file SHALL be created.

#### Scenario: Dry run note creation
- **WHEN** a user runs `pinax note new "Draft" --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return planned path, frontmatter, and body preview
- **AND** it SHALL NOT write Markdown files, `.pinax/` state, Git state, provider state, or remote services.

### Requirement: Note references resolve without requiring exact paths
Pinax SHALL resolve note references by note id, path, `notes/` prefix tolerant path, exact title, or unique title match.

#### Scenario: Show note by unique title
- **WHEN** a vault contains one note titled `研究日志`
- **AND** a user runs `pinax note show "研究日志" --vault ./my-notes --json`
- **THEN** Pinax SHALL read that note and include its path and note id in the projection.

#### Scenario: Ambiguous title returns candidates
- **WHEN** a vault contains multiple notes titled `会议`
- **AND** a user runs `pinax note show "会议" --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `note_ref_ambiguous`
- **AND** the error projection SHALL include candidate paths or note ids.

### Requirement: Note list is filterable and scannable
Pinax SHALL let users list notes by useful local vault dimensions.

#### Scenario: List recent notes with filters
- **WHEN** a user runs `pinax note list --tag research --project work --status active --recent --limit 20 --vault ./my-notes`
- **THEN** stdout SHALL contain a concise Chinese human summary and a scannable list of matching notes
- **AND** diagnostics SHALL go to stderr.

#### Scenario: List notes as JSON
- **WHEN** a user runs `pinax note list --tag research --limit 20 --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with filter facts, total count, returned count, and notes
- **AND** stdout SHALL NOT contain human prose outside JSON.

### Requirement: Note editing uses an explicit editor boundary
Pinax SHALL open notes in an editor only when requested and SHALL keep editor execution testable and local.

#### Scenario: Open existing note in editor
- **WHEN** a user runs `pinax note edit note_123 --editor fake-editor --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve the note inside the vault and execute the editor with the local note path
- **AND** stdout SHALL contain a projection with editor command, note path, and status.

#### Scenario: Missing editor fails clearly
- **WHEN** a user runs `pinax note edit note_123 --vault ./my-notes` and no editor is configured
- **THEN** Pinax SHALL fail with stable error code `editor_not_configured`
- **AND** it SHALL suggest setting `$EDITOR` or passing `--editor`.

### Requirement: Single-note maintenance operations are safe
Pinax SHALL support common single-note maintenance operations while protecting vault boundaries and destructive actions.

#### Scenario: Rename note title and path
- **WHEN** a user runs `pinax note rename note_123 "New Title" --vault ./my-notes --json`
- **THEN** Pinax SHALL update frontmatter title and choose a safe target path inside the vault
- **AND** it SHALL fail with stable error code `note_path_conflict` if target path already exists.

#### Scenario: Archive note without moving file
- **WHEN** a user runs `pinax note archive note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL set frontmatter `status: archived`
- **AND** it SHALL NOT move or delete the Markdown file.

#### Scenario: Delete note moves to trash by default
- **WHEN** a user runs `pinax note delete note_123 --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL move the note to `.pinax/trash/` through the application service
- **AND** it SHALL append redacted event evidence.

#### Scenario: Hard delete requires explicit hard approval
- **WHEN** a user runs `pinax note delete note_123 --vault ./my-notes --hard --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** the note file SHALL remain unchanged.

### Requirement: Note tags are manageable from the CLI
Pinax SHALL let users add, remove, and set frontmatter tags without hand-editing machine-readable metadata.

#### Scenario: Add note tag
- **WHEN** a user runs `pinax note tag add note_123 research --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the target note frontmatter tags through the application service
- **AND** duplicate tags SHALL NOT be added.

#### Scenario: Remove note tag
- **WHEN** a user runs `pinax note tag remove note_123 inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL remove the tag if present
- **AND** it SHALL return a stable projection even if the tag was already absent.
