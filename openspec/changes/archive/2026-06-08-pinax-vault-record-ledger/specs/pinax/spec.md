## MODIFIED Requirements

### Requirement: Machine-readable assets are CLI-authored

Pinax SHALL create and update machine-readable vault assets through commands or application services rather than requiring agents to hand-write JSON, YAML, or JSONL metadata. Pinax SHALL treat record ledger assets as CLI-authored machine records that own note identity, lifecycle, schema, tombstone, version evidence, sync evidence, and repair evidence.

#### Scenario: adding structured asset behavior
- **GIVEN** an implementation change adds config, provider profile, mapping, sync-state, event, record ledger, note registry, schema registry, tombstone, version backend config, diff evidence, snapshot receipt, briefing receipt, delivery receipt, feedback, or MCP evidence
- **WHEN** tasks are written
- **THEN** they SHALL include a command or service path that authors the asset
- **AND** tests SHALL validate schema version, redaction, path boundaries, ledger sequence or identity constraints where relevant, and stable machine-readable errors.

### Requirement: Pinax initializes and validates a local Markdown vault

Pinax SHALL initialize and validate local Markdown vaults without requiring provider credentials, remote services, or agent-written metadata files. Pinax SHALL preserve Markdown as the content source while initializing CLI-authored record assets for machine identity and lifecycle facts.

#### Scenario: initializing a vault
- **GIVEN** a user runs `pinax init ./my-notes --title "我的知识库"`
- **WHEN** the vault is initialized
- **THEN** Pinax SHALL create `notes/`, `.pinax/config.yaml`, `.pinax/events.jsonl`, and `.pinax/records/` assets through CLI services
- **AND** it SHALL NOT overwrite existing Markdown note bodies

#### Scenario: validating a vault
- **GIVEN** a vault contains Markdown notes and Pinax record assets
- **WHEN** the user runs `pinax validate --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with validation facts, record ledger facts, issues, and next actions
- **AND** path, metadata, record sequence, note identity, and schema errors SHALL use stable machine-readable error codes

### Requirement: Core note creation

Pinax SHALL create Markdown notes from the CLI while preserving Markdown body content as the content source and creating CLI-authored record ledger facts as the machine source for identity and lifecycle.

#### Scenario: Create a note with frontmatter

- **WHEN** a user runs `pinax note new "研究日志" --tags research,pinax --vault <vault>`
- **THEN** Pinax creates a Markdown file under the vault
- **AND** the file contains YAML frontmatter with `schema_version`, `note_id`, `title`, `tags`, `created_at`, and `updated_at`
- **AND** Pinax appends a record ledger event and updates the note registry for the new note id, path, lifecycle state, and content hash.
