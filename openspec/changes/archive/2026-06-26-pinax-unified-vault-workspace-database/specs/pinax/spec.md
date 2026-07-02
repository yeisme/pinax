# pinax Delta Spec

## ADDED Requirements

### Requirement: Pinax SHALL expose a unified vault workspace model

Pinax SHALL let one local Markdown vault contain multiple projects, subprojects, collections, saved views, task views, database views, publish profiles, and sync policies through a shared workspace model while preserving Markdown as the content source of truth.

#### Scenario: Workspace summary is bounded

- **GIVEN** a vault contains projects, subprojects, board views, database views, and note collections
- **WHEN** the user runs `pinax workspace show --vault ./my-notes --json`
- **THEN** stdout SHALL contain one projection envelope with workspace identity, project counts, active project, view counts, task counts, index status, warnings, and next actions
- **AND** it SHALL NOT include full note bodies, raw provider payloads, Authorization headers, cookies, tokens, hidden system prompts, private tool arguments, or complete chain-of-thought.

#### Scenario: Workspace registry is CLI-authored

- **WHEN** Pinax creates or updates workspace registry, project workspace registry, database view registry, task adoption ledger, event ledger, sync state, publish profile, or receipt metadata
- **THEN** the write SHALL happen through a Pinax command or application service
- **AND** agents SHALL NOT be required to hand-write `.pinax/**` JSON, YAML, TOML, or JSONL assets.

#### Scenario: Workspace paths stay inside the vault

- **WHEN** a user creates a project, subproject, collection, saved view, managed task, database view, template output, or publish source path
- **THEN** Pinax SHALL validate the resulting path is vault-relative and outside reserved directories such as `.pinax`, `.git`, `temp`, `dist`, `node_modules`, and `vendor`
- **AND** path traversal, absolute paths, and vault-external writes SHALL fail with stable machine-readable error codes.

### Requirement: Pinax SHALL evolve workspace contracts additively

Pinax SHALL implement unified workspace, task, database, API, MCP, dashboard, and sync surfaces as backward-compatible additions unless a future OpenSpec change explicitly approves migration, deprecation, and rollback for a breaking change.

#### Scenario: CLI output gains optional workspace facts

- **WHEN** a command begins returning workspace, task view, database view, graph, or compatibility facts
- **THEN** those facts SHALL be added as optional fields under the existing projection envelope or optional `--agent` keys
- **AND** existing envelope top-level fields, status enum values, and previously documented `--agent` keys SHALL remain valid.

#### Scenario: API capability additions are discoverable

- **WHEN** workspace, task, database, graph, or compatibility capabilities become available through REST, RPC, dashboard, MCP, or Remote API Mode
- **THEN** the capability SHALL be registered in the shared capability registry
- **AND** `pinax api routes --vault ./my-notes --json` SHALL expose route or RPC method, command, readonly status, body allowance, approval requirement, snapshot requirement, and stable error codes when applicable.

#### Scenario: Index schema changes are additive

- **WHEN** unified workspace or database features require new index projection storage
- **THEN** Pinax SHALL add GORM-managed tables, nullable columns, or indexes rather than dropping, renaming, narrowing, or repurposing existing projection schema in the same change
- **AND** deleting and rebuilding the index SHALL NOT delete Markdown source content.
