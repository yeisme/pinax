## MODIFIED Requirements

### Requirement: Pinax local commands follow the AI-native CLI output contract
Pinax SHALL render human, agent, JSON, events, and explain outputs from one command projection, and note commands SHALL expose stable machine-readable facts for editor execution, mutation outcomes, trash paths, sorting semantics, and ambiguous candidates.

#### Scenario: rendering machine output
- **GIVEN** a local vault command supports `--json` or `--agent`
- **WHEN** that output mode is selected
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr

#### Scenario: rendering note hardening facts
- **GIVEN** a note command executes an editor, mutates a note, moves a note to trash, lists recent notes, or returns ambiguous candidates
- **WHEN** `--json` or `--agent` is selected
- **THEN** stdout SHALL include stable fields for the relevant path, note id, editor executable or args, trash path, sort facts, mutation outcome, or candidate path/title/note id
- **AND** stdout SHALL NOT include raw provider credentials, shell-expanded secrets, or unredacted trace payloads.
