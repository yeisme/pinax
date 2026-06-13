## MODIFIED Requirements

### Requirement: Record events are append-only and replayable
Pinax SHALL write note record changes as append-only events and SHALL maintain materialized registry files as replayable projections of those events.

#### Scenario: Append note lifecycle event
- **WHEN** a CLI-approved note create, move, rename, archive, delete, restore, metadata, tag, or schema operation succeeds
- **THEN** Pinax SHALL append one redacted record event with schema version, event id, sequence, kind, actor, source command, note id, before facts, after facts, content hash, and created time
- **AND** it SHALL update the registry projection through the ledger service.

#### Scenario: Tag mutation records metadata evidence
- **WHEN** a user runs `pinax note tag add note_123 research --vault ./my-notes --json`, `pinax note tag remove note_123 inbox --vault ./my-notes --json`, or `pinax note tag set note_123 work reference --vault ./my-notes --json`
- **THEN** Pinax SHALL append a record metadata event or a dedicated tag metadata event after the Markdown write succeeds
- **AND** stdout SHALL include stable facts for `record_event`, `ledger_seq`, `record_version`, and version evidence when available
- **AND** repeated no-op tag operations SHALL be idempotent and SHALL NOT duplicate downstream side effects.

#### Scenario: Rejected tag input does not write ledger
- **WHEN** a tag operation fails validation before writing Markdown
- **THEN** Pinax SHALL NOT append a record event or update the registry projection
- **AND** the error projection SHALL include a stable error code and a runnable correction hint.
