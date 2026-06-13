## ADDED Requirements

### Requirement: Draft workflow supports reviewable authoring

Pinax SHALL provide a draft workflow for user-authored notes that are not ready for ordinary active notebook surfaces, while keeping Markdown notes as the source of truth.

#### Scenario: Create draft note
- **WHEN** a user runs `pinax draft create "草稿想法" --body "先写一版" --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered Markdown note under `drafts/` or an equivalent safe draft folder through the application service
- **AND** the note frontmatter SHALL include `status: draft`
- **AND** Pinax SHALL NOT force `kind: draft` when the user or selected template provides another note kind.

#### Scenario: List draft notes
- **WHEN** a user runs `pinax draft list --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes whose managed lifecycle status is `draft`
- **AND** the JSON envelope facts SHALL include total count, returned count, status filter, index status when available, and one next action for previewing or promoting a draft.

#### Scenario: Show draft note through bounded note display
- **WHEN** a user runs `pinax draft show note_123 --view rendered --vault ./my-notes --json`
- **THEN** Pinax SHALL return the same bounded note display projection used by `pinax note show`
- **AND** it SHALL include stable facts for note id, path, title, status, lifecycle status, view, and body exposure mode.

#### Scenario: Promote draft to active note
- **WHEN** a user runs `pinax draft promote note_123 --status active --folder research --kind reference --vault ./my-notes --json`
- **THEN** Pinax SHALL update the note frontmatter and optional path through the application service
- **AND** it SHALL append redacted event and record metadata evidence
- **AND** it SHALL refresh the local index after the successful write.

#### Scenario: Discard draft without hard delete
- **WHEN** a user runs `pinax draft discard note_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL set the note lifecycle status to `discarded` through the application service
- **AND** it SHALL NOT hard delete the Markdown file or attachments
- **AND** stdout facts SHALL include `deleted=false` and a next action for `pinax note delete` if the user wants real deletion.

### Requirement: Inbox workflow supports review actions

Pinax SHALL extend inbox capture and triage with review actions that let users and remote clients inspect, promote, or discard inbox items without direct file manipulation.

#### Scenario: Show inbox item
- **WHEN** a user runs `pinax inbox show note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return a bounded note display projection for the inbox item
- **AND** the projection SHALL include status `inbox`, lifecycle status `inbox`, path, title, tags, and recommended next actions.

#### Scenario: Promote inbox item to draft
- **WHEN** a user runs `pinax inbox promote note_123 --to draft --vault ./my-notes --json`
- **THEN** Pinax SHALL update only controlled metadata and optional safe path fields through the application service
- **AND** the resulting note SHALL have lifecycle status `draft`
- **AND** the local index SHALL be refreshed after the successful write.

#### Scenario: Promote inbox item to active note
- **WHEN** a user runs `pinax inbox promote note_123 --to active --group work --folder ideas --kind reference --vault ./my-notes --json`
- **THEN** Pinax SHALL set the note status to `active` and move it to the selected safe target path through the application service
- **AND** it SHALL fail with stable error code `note_path_conflict` if the target path already exists.

#### Scenario: Discard inbox item without deleting content
- **WHEN** a user runs `pinax inbox discard note_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL set lifecycle status `discarded`
- **AND** it SHALL NOT hard delete Markdown, attachments, `.pinax` structured assets, Git state, provider state, or remote service state.

### Requirement: Review lifecycle transitions are service-owned

Pinax SHALL enforce inbox and draft lifecycle transitions through application services and SHALL reject direct or invalid workflow transitions.

#### Scenario: Invalid lifecycle transition is rejected
- **WHEN** a user runs `pinax draft promote note_123 --status inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_lifecycle_transition`
- **AND** it SHALL NOT modify Markdown, `.pinax` assets, index projection, Git state, provider state, or remote services.

#### Scenario: Dry-run lifecycle transition has no side effects
- **WHEN** a user runs `pinax inbox promote note_123 --to active --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return a reviewable transition plan with planned status, planned path, risk, and required approval facts
- **AND** it SHALL NOT write Markdown, `.pinax` events, record metadata, index projection, Git state, provider state, or remote services.

#### Scenario: Successful lifecycle transition records evidence
- **WHEN** Pinax completes an approved inbox or draft lifecycle transition
- **THEN** it SHALL append a redacted event, append record metadata evidence when record ledger is available, and refresh the local index
- **AND** stdout SHALL include stable facts for old status, new status, path, writes, record event, and index update status.
