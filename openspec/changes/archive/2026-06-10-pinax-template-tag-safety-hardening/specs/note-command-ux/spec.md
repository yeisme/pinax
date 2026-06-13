## MODIFIED Requirements

### Requirement: Note tags are manageable from the CLI
Pinax SHALL let users add, remove, and set frontmatter tags without hand-editing machine-readable metadata, and SHALL ensure tag updates cannot corrupt CLI-authored YAML frontmatter or inject unrelated metadata fields.

#### Scenario: Add note tag
- **WHEN** a user runs `pinax note tag add note_123 research --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the target note frontmatter tags through the application service
- **AND** duplicate tags SHALL NOT be added
- **AND** stdout SHALL include stable facts for updated tags and either index update status or explicit stale index status.

#### Scenario: Remove note tag
- **WHEN** a user runs `pinax note tag remove note_123 inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL remove the tag if present
- **AND** it SHALL return a stable projection even if the tag was already absent.

#### Scenario: Reject unsafe tag value
- **WHEN** a user runs `pinax note new "Unsafe" --tags $'safe,bad]\nstatus: archived' --vault ./my-notes --json` or `pinax note tag add note_123 $'bad]\nstatus: archived' --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_tag`
- **AND** no note file, frontmatter field, record event, index projection, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Tag frontmatter remains parseable YAML
- **WHEN** Pinax writes tags through `note new`, `note tag add`, `note tag remove`, `note tag set`, import defaults, repair apply, or organize metadata operations
- **THEN** the resulting frontmatter SHALL remain valid YAML with `tags` represented as a list of tag strings
- **AND** user-authored unrelated metadata fields SHALL be preserved where practical.
