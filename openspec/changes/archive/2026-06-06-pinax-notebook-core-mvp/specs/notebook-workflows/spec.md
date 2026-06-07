## ADDED Requirements

### Requirement: Daily workflow supports review and capture
Pinax SHALL provide local daily note workflows without requiring external services.

#### Scenario: Open or create today's daily note
- **WHEN** a user runs `pinax daily open --vault ./my-notes --editor fake-editor --json`
- **THEN** Pinax SHALL create `notes/daily/YYYY-MM-DD.md` if missing through the application service
- **AND** it SHALL open the daily note with the configured editor
- **AND** stdout SHALL contain one JSON envelope with command `daily.open`, daily note path, date, and editor facts.

#### Scenario: Show today's daily note without editing
- **WHEN** a user runs `pinax daily show --vault ./my-notes --json`
- **THEN** Pinax SHALL return the current daily note projection if it exists
- **AND** it SHALL NOT execute an editor or write provider state.

#### Scenario: Append to daily note
- **WHEN** a user runs `pinax daily append --body "复盘" --vault ./my-notes --json`
- **THEN** Pinax SHALL append the body to today's daily note inside the vault boundary
- **AND** it SHALL refresh the local index projection.

### Requirement: Inbox workflow supports fast capture and triage
Pinax SHALL provide an inbox workflow for quick capture and later organization.

#### Scenario: Capture inbox note
- **WHEN** a user runs `pinax inbox capture "想法" --body "正文" --tags inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL create a note under `notes/inbox/`
- **AND** the note frontmatter SHALL include `kind: inbox` and `status: inbox`
- **AND** the created note SHALL be added to the daily index and local index.

#### Scenario: List inbox notes
- **WHEN** a user runs `pinax inbox list --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes with inbox status or inbox kind
- **AND** the JSON facts SHALL include total and returned counts.

#### Scenario: Triage inbox note into project folder
- **WHEN** a user runs `pinax inbox triage note_123 --group work --folder ideas --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL update the note frontmatter and move it into the selected group/folder path through the application service
- **AND** it SHALL fail with `note_path_conflict` if the target path already exists.

### Requirement: Notebook organization views are discoverable
Pinax SHALL expose local organization dimensions as first-class readable views.

#### Scenario: List tags with counts
- **WHEN** a user runs `pinax tag list --vault ./my-notes --json`
- **THEN** Pinax SHALL return tags and note counts from the current vault index or scan fallback
- **AND** stdout SHALL contain no human prose outside the JSON envelope.

#### Scenario: List folders with counts
- **WHEN** a user runs `pinax folder list --vault ./my-notes --json`
- **THEN** Pinax SHALL return vault-relative note folders and counts
- **AND** it SHALL NOT include `.pinax`, `.git`, `dist`, or paths outside the vault.

#### Scenario: List kinds and groups
- **WHEN** a user runs `pinax kind list --vault ./my-notes --json` or `pinax group list --vault ./my-notes --json`
- **THEN** Pinax SHALL return kind or group values with counts
- **AND** missing values SHALL be represented with a stable empty bucket fact rather than crashing.

### Requirement: Links and backlinks are inspectable
Pinax SHALL let users inspect note links, backlinks, orphan notes, and unresolved references from local Markdown content.

#### Scenario: Show note outgoing links
- **WHEN** a user runs `pinax note links note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return wiki links and Markdown links found in the note body
- **AND** each link SHALL include source path, target text, resolved target path when available, and broken status.

#### Scenario: Show note backlinks
- **WHEN** a user runs `pinax note backlinks note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes that link to the target note by title, path, note id, or wiki reference
- **AND** it SHALL include stable facts for backlink count and unresolved count.

#### Scenario: List orphan notes
- **WHEN** a user runs `pinax note orphans --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes with no incoming or outgoing note links
- **AND** system index notes SHALL NOT be counted as ordinary orphans.

### Requirement: Attachments are managed inside the vault
Pinax SHALL let users attach local files to notes while keeping attachments inside the vault boundary.

#### Scenario: Attach local file to note
- **WHEN** a user runs `pinax note attach note_123 ./diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL copy the file into a vault attachment directory
- **AND** it SHALL append or return a Markdown reference to the attachment
- **AND** stdout SHALL include source path, vault-relative attachment path, and note path without leaking external secrets.

#### Scenario: List note attachments
- **WHEN** a user runs `pinax note attachments note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return attachment references found in the note body
- **AND** each item SHALL include whether the target file exists inside the vault.

#### Scenario: Reject attachment outside allowed source path when missing
- **WHEN** a user runs `pinax note attach note_123 ./missing.png --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `attachment_source_missing`
- **AND** no note body or vault attachment file SHALL be modified.

### Requirement: Saved views store reusable local filters
Pinax SHALL let users save and reuse common note list filters through CLI-authored structured assets.

#### Scenario: Save a note view
- **WHEN** a user runs `pinax view save active-work --group work --status active --kind reference --sort updated --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/views.json` through the application service
- **AND** the saved view SHALL store filters rather than note result snapshots.

#### Scenario: Show a saved view
- **WHEN** a user runs `pinax view show active-work --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve the saved filters and return current matching notes
- **AND** it SHALL report the saved view name and filter facts.

#### Scenario: Delete a saved view with approval
- **WHEN** a user runs `pinax view delete active-work --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove only that view from `.pinax/views.json`
- **AND** it SHALL NOT delete notes or attachments.

### Requirement: Local import and export preserve Markdown portability
Pinax SHALL support local Markdown import and export without external provider dependencies.

#### Scenario: Dry run Markdown directory import
- **WHEN** a user runs `pinax import markdown ./source --group research --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report planned note paths, conflicts, and skipped files
- **AND** it SHALL NOT write notes, `.pinax` receipts, Git state, or provider state.

#### Scenario: Apply Markdown import
- **WHEN** a user runs `pinax import markdown ./source --group research --conflict rename --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL copy Markdown files into the vault with Pinax frontmatter normalized when needed
- **AND** it SHALL record a redacted import receipt through the application service.

#### Scenario: Export Markdown bundle
- **WHEN** a user runs `pinax export markdown ./out --tag research --vault ./my-notes --json`
- **THEN** Pinax SHALL export matching Markdown notes and referenced attachments into the output directory
- **AND** it SHALL write an export receipt without storing provider credentials or raw external payloads.
