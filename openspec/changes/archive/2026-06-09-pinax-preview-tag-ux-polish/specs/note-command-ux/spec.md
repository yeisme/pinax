## ADDED Requirements

### Requirement: Note preview shows scannable content and tags
Pinax SHALL render note preview output in default human mode as a concise Chinese metadata summary followed by the preview body and visible tag context.

#### Scenario: Preview note with tags in default mode
- **WHEN** a user runs `pinax note preview "Tagged Preview" --vault ./my-notes`
- **THEN** stdout SHALL include the note title, tag list, rendered view, and preview body
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Preview note keeps machine output structured
- **WHEN** a user runs `pinax note preview "Tagged Preview" --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with command `note.preview`, note metadata, view, and body data
- **AND** stdout SHALL NOT contain human table decoration outside JSON.

### Requirement: Note tag dimensions are visually scannable
Pinax SHALL make default note tag dimension output scannable with count, percentage, and a plain-text heat bar while preserving stable machine output.

#### Scenario: List note tags in default mode
- **WHEN** a user runs `pinax note tags --vault ./my-notes`
- **THEN** stdout SHALL include a concise Chinese summary and a table with tag value, count, percentage, and heat bar
- **AND** the table SHALL remain readable without ANSI color.

#### Scenario: List note tags as JSON
- **WHEN** a user runs `pinax note tags --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with dimension facts and item counts
- **AND** stdout SHALL NOT include the human-only heat bar text as localized prose.
