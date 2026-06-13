## ADDED Requirements

### Requirement: Folder taxonomy supports controlled bulk rename
Pinax SHALL support controlled bulk folder rename across registered notes while preserving Markdown files as the source of truth.

#### Scenario: Dry-run folder rename
- **WHEN** a user runs `pinax note folders rename inbox archive --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report matched note count, changed note count, old folder, new folder, and planned target paths
- **AND** it SHALL NOT modify Markdown files, record ledger, index database, provider state, or remote services.

#### Scenario: Confirmed folder rename
- **WHEN** a user runs `pinax note folders rename inbox archive --yes --vault ./my-notes --agent`
- **THEN** Pinax SHALL move matching note files into the target folder and update frontmatter `folder: archive`
- **AND** stdout SHALL include stable facts for old folder, new folder, matched count, changed count, write status, record event count, and index update status.

#### Scenario: Folder rename requires confirmation
- **WHEN** a user runs `pinax note folders rename inbox archive --vault ./my-notes --json` without `--dry-run` or `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown file, record ledger, index database, provider state, or remote service SHALL be modified.

#### Scenario: Folder rename rejects path conflicts before writing
- **WHEN** a confirmed folder rename would overwrite an existing note path or make two notes target the same path
- **THEN** Pinax SHALL fail with stable error code `note_path_conflict`
- **AND** it SHALL NOT partially move notes or update frontmatter.
