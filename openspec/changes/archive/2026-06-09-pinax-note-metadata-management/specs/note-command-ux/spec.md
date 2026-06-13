## ADDED Requirements

### Requirement: Note properties are manageable from the CLI
Pinax SHALL let users set and remove non-reserved note frontmatter properties through CLI commands backed by the application service.

#### Scenario: Set note property
- **WHEN** a user runs `pinax note property set note_123 priority 2 --vault ./my-notes --json`
- **THEN** Pinax SHALL update the target note frontmatter with `priority: 2`
- **AND** stdout SHALL contain one JSON envelope with command `note.property`, property name, operation, record facts, and index update facts.

#### Scenario: Remove note property
- **WHEN** a user runs `pinax note property remove note_123 priority --vault ./my-notes --agent`
- **THEN** Pinax SHALL remove the `priority` frontmatter field when present
- **AND** stdout SHALL include stable agent facts for command, operation, property, and index update status.

#### Scenario: Reserved property is rejected
- **WHEN** a user runs `pinax note property set note_123 tags urgent --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with a stable validation error
- **AND** it SHALL NOT bypass the dedicated tag management commands for structured tags.

### Requirement: Tag taxonomy supports controlled bulk updates
Pinax SHALL support controlled bulk tag rename and delete operations across registered notes while requiring an explicit preview or confirmation mode.

#### Scenario: Dry-run tag rename
- **WHEN** a user runs `pinax note tags rename old new --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report matched and changed note counts without modifying Markdown files, `.pinax/` state, provider state, or remote services.

#### Scenario: Confirmed tag rename
- **WHEN** a user runs `pinax note tags rename old new --yes --vault ./my-notes --agent`
- **THEN** Pinax SHALL update matching note frontmatter tags through the application service
- **AND** stdout SHALL include stable facts for old tag, new tag, matched count, changed count, write status, record event count, and index update status.

#### Scenario: Confirmed tag delete
- **WHEN** a user runs `pinax note tags delete stale --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove that tag from all registered notes that contain it
- **AND** it SHALL refresh the local index projection after writing changed notes.

#### Scenario: Bulk tag write requires confirmation
- **WHEN** a user runs `pinax note tags delete stale --vault ./my-notes --json` without `--dry-run` or `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown file, record ledger, index database, provider state, or remote service SHALL be modified.
