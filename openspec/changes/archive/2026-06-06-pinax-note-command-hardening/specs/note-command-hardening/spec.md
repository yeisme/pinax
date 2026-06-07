## ADDED Requirements

### Requirement: Note editor execution handles executable arguments safely
Pinax SHALL execute note editors through a parsed executable and argument list without invoking a shell.

#### Scenario: Edit note with EDITOR arguments
- **WHEN** a user runs `EDITOR="code --wait" pinax note edit note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL execute `code` with `--wait` and the resolved note path as separate arguments
- **AND** stdout SHALL contain one JSON envelope with editor executable, args, note path, and status.

#### Scenario: Reject missing editor clearly
- **WHEN** a user runs `pinax note edit note_123 --vault ./my-notes --json` without `$EDITOR` or `--editor`
- **THEN** Pinax SHALL fail with stable error code `editor_not_configured`
- **AND** no note file SHALL be modified.

#### Scenario: Editor execution does not use shell interpolation
- **WHEN** a user passes an editor string containing shell metacharacters
- **THEN** Pinax SHALL treat parsed tokens as executable arguments rather than executing shell syntax
- **AND** tests SHALL use a fake executable instead of a real editor.

### Requirement: Note mutations avoid half-written rename states
Pinax SHALL apply single-note metadata/path mutations in a way that avoids leaving users with partially updated files after a failed rename.

#### Scenario: Rename target move fails after content preparation
- **WHEN** `pinax note rename note_123 "New Title" --vault ./my-notes --json` cannot complete the target file move
- **THEN** Pinax SHALL return a failed projection with a stable error code
- **AND** the original note path SHALL remain readable with its original title and body whenever the failure occurs before final commit.

#### Scenario: Rename succeeds atomically enough for local CLI use
- **WHEN** a user runs `pinax note rename note_123 "New Title" --vault ./my-notes --json`
- **THEN** Pinax SHALL update the note title and path inside the vault
- **AND** it SHALL append redacted event evidence with old path and new path.

### Requirement: Note delete trash paths are unique and non-overwriting
Pinax SHALL move notes to a unique trash path when `note delete --yes` is used without `--hard`.

#### Scenario: Trash target already exists
- **WHEN** `.pinax/trash/YYYYMMDD/work/note.md` already exists
- **AND** a user runs `pinax note delete notes/work/note.md --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL choose a non-conflicting trash path such as `.pinax/trash/YYYYMMDD/work/note-2.md`
- **AND** it SHALL NOT overwrite existing trash files.

#### Scenario: Hard delete still requires explicit approval
- **WHEN** a user runs `pinax note delete note_123 --hard --vault ./my-notes --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** the note file SHALL remain unchanged.

### Requirement: Frontmatter patching preserves user-authored metadata where practical
Pinax SHALL update Pinax-managed frontmatter fields while preserving unrelated user-authored metadata and common YAML comments where practical.

#### Scenario: Tag update preserves unknown frontmatter fields
- **WHEN** a note contains frontmatter with an unknown field and a comment
- **AND** a user runs `pinax note tag add note_123 research --vault ./my-notes --json`
- **THEN** Pinax SHALL update the tags field
- **AND** the unknown field and common comment lines SHALL remain in the note frontmatter.

#### Scenario: Archive updates only status and timestamp fields
- **WHEN** a user runs `pinax note archive note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL set `status: archived` and update `updated_at`
- **AND** it SHALL preserve the note body and unrelated metadata.

### Requirement: Note recent listing semantics are explicit
Pinax SHALL make `note list --recent` a clear sorting request rather than an implicit time-window filter.

#### Scenario: Recent list reports sort facts
- **WHEN** a user runs `pinax note list --recent --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with facts indicating updated-time sorting
- **AND** it SHALL NOT silently filter out older notes unless a separate time-window flag is provided.

#### Scenario: Human recent list remains scannable
- **WHEN** a user runs `pinax note list --recent --vault ./my-notes`
- **THEN** stdout SHALL contain a concise Chinese summary and scannable note rows with path, title, status or tags, and updated time when available.
