## ADDED Requirements

### Requirement: Custom frontmatter properties are queryable
Pinax SHALL index non-empty custom note frontmatter fields as typed properties while keeping Markdown as the source of truth.

#### Scenario: List notes with custom frontmatter properties
- **WHEN** a registered note contains frontmatter fields such as `rating: 5`, `done: false`, and `due: 2026-06-09`
- **AND** a user runs `pinax note list --property rating --property done --property due --strict-properties --vault ./my-notes --json`
- **THEN** Pinax SHALL return those selected property values in the note list projection
- **AND** it SHALL infer useful property types from frontmatter scalar values.

#### Scenario: Property command refreshes property projection
- **WHEN** a user runs `pinax note property set note_123 priority 2 --vault ./my-notes --json`
- **AND** then runs `pinax note list --property priority --strict-properties --vault ./my-notes --json`
- **THEN** Pinax SHALL expose the new `priority` property without requiring hand-edited index metadata
- **AND** the original Markdown file SHALL remain the authoritative storage for the property.
