# pinax Specification Delta

## ADDED Requirements

### Requirement: Pinax exposes a readonly local MCP server

Pinax SHALL expose a local stdio MCP server for agent read workflows while routing through the same application services as CLI commands.

#### Scenario: starting MCP serve
- **GIVEN** a user has a local Pinax vault
- **WHEN** they run `pinax mcp serve --vault ./my-notes`
- **THEN** Pinax SHALL accept MCP JSON-RPC requests over stdio
- **AND** it SHALL advertise only read-only resources and tools in MVP

#### Scenario: listing readonly resources
- **GIVEN** an MCP client calls `resources/list`
- **WHEN** Pinax responds
- **THEN** it SHALL include compact resources such as `pinax://vault/current`, `pinax://note/{note_id}`, `pinax://search/{query}`, and `pinax://organize/plan`
- **AND** it SHALL NOT return every note body by default

#### Scenario: calling readonly tools
- **GIVEN** an MCP client calls `pinax.search`, `pinax.note.read`, `pinax.organize.plan`, or `pinax.git.snapshot_plan`
- **WHEN** the tool is handled
- **THEN** Pinax SHALL route through `internal/app` services
- **AND** it SHALL NOT modify Markdown files, `.pinax/` state, Git state, or provider state

#### Scenario: rejecting write tools
- **GIVEN** an MCP client asks for write-capable behavior
- **WHEN** approval metadata or explicit local write flags are missing
- **THEN** Pinax SHALL return an approval-required or method-not-found error
- **AND** it MAY include a human-runnable CLI command for the user to apply manually
