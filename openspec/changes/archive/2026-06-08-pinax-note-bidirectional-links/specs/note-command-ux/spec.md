## MODIFIED Requirements

### Requirement: Note commands expose link and attachment subcommands
Pinax SHALL expose link, backlink, orphan, graph-context, and attachment inspection from the note command surface.

#### Scenario: Note help includes relationship commands
- **WHEN** a user runs `pinax note --help`
- **THEN** help output SHALL include links, backlinks, orphans, attach, and attachments commands
- **AND** help text SHALL describe local Markdown note relationships.

#### Scenario: Note relationship commands follow output contract
- **WHEN** a user runs a note relationship command with `--agent` or `--json`
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Note links supports relationship filters
- **WHEN** a user runs `pinax note links note_123 --broken-only --kind wiki --vault ./my-notes --json`
- **THEN** Pinax SHALL return only matching outgoing link edges
- **AND** stdout facts SHALL include path, note id when available, links, resolved, broken, ambiguous, ignored, kind filter, and engine.

#### Scenario: Note backlinks supports bounded output
- **WHEN** a user runs `pinax note backlinks note_123 --limit 20 --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable low-token key=value facts for backlink count, returned count, broken count, ambiguous count, index status, and next action when more results exist
- **AND** stdout SHALL NOT include localized prose, raw note bodies, provider payloads, or secrets.

#### Scenario: Ambiguous note reference returns candidates
- **WHEN** a user runs `pinax note links "会议" --vault ./my-notes --json`
- **AND** multiple notes match the note reference
- **THEN** Pinax SHALL fail with stable error code `note_ref_ambiguous`
- **AND** the error projection SHALL include candidate paths or note ids.

#### Scenario: Explain output summarizes link decisions
- **WHEN** a user runs `pinax note backlinks note_123 --vault ./my-notes --explain`
- **THEN** stdout SHALL contain a Chinese explanation summary with conclusion, evidence, risk, and recommended next action
- **AND** stdout SHALL NOT include full chain-of-thought, raw prompts, hidden system prompts, secrets, or provider payloads.
