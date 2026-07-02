# pinax-cli-remote-api-mode Delta Spec

## ADDED Requirements

### Requirement: Workspace, task, database, and graph capabilities SHALL use the shared registry

Pinax SHALL expose workspace, task, database view, graph, and Obsidian compatibility read surfaces through the existing capability registry when they are made available to REST, RPC, dashboard, MCP, or Remote API Mode clients.

#### Scenario: Workspace capability is discoverable

- **WHEN** workspace summary or project workspace read capability is added to Remote API Mode
- **THEN** `pinax api routes --vault ./my-notes --json` SHALL list capability id, command, route or RPC method, readonly flag, body allowance, approval requirement, snapshot requirement, and stable errors
- **AND** OpenAPI export SHALL derive route metadata from the same registry when a REST route exists.

#### Scenario: Database view render is readonly by default

- **WHEN** a client calls a registered database view render REST route or RPC method
- **THEN** Pinax SHALL route through the database view application service and return the same projection shape as CLI JSON output
- **AND** it SHALL NOT write Markdown, `.pinax/**`, index database, provider state, sync state, Git state, or remote services.

#### Scenario: Database tab projection is shared across clients

- **WHEN** a client calls a registered database view render capability for a saved view such as `active-projects`
- **THEN** the response SHALL include the same bounded database tab projection used by `pinax database view render active-projects --vault ./my-notes --json`
- **AND** the projection SHALL include optional tab metadata such as tab id, view name, display, row count, columns, groups, warnings, and index status
- **AND** REST, RPC, MCP, dashboard, and Remote API Mode SHALL NOT invent separate field names for the same tab facts.

#### Scenario: Task adoption and risky writes remain gated

- **GIVEN** `pinax api serve --vault ./my-notes --allow-write` is running
- **WHEN** a client requests task adoption, task move, archive, repair apply, organize apply, managed block refresh, publish deploy, or restore apply
- **THEN** Pinax SHALL require the same `yes=true`, dry-run, approval, snapshot, receipt, and redaction boundaries as the equivalent CLI command
- **AND** readonly servers SHALL return `write_disabled` for the same operation.

### Requirement: MCP and dashboard SHALL default to bounded readonly projections

Pinax SHALL expose unified workspace, task, database, graph, and compatibility information to MCP and dashboard as readonly projections unless a future explicit write-capable design is approved.

#### Scenario: MCP reads workspace context without body exposure

- **WHEN** an MCP client reads a resource such as `pinax://workspace/current`, `pinax://project/research/board`, `pinax://database/active-projects`, or `pinax://graph/current`
- **THEN** Pinax SHALL return bounded facts from application services
- **AND** it SHALL NOT include full note bodies, raw provider payloads, hidden prompts, private tool parameters, secrets, or complete chain-of-thought.

#### Scenario: Dashboard reads database tabs without owning business logic

- **WHEN** dashboard or a future local client renders a Markdown page that references saved database views as tabs
- **THEN** it SHALL request readonly database tab projections through the capability registry or application service
- **AND** it SHALL treat each saved database view as one tab rather than parsing `.pinax/**` registry files or Markdown query fences directly
- **AND** UI state such as active tab selection SHALL remain client-local unless a future explicit saved workspace layout design adds a CLI-authored registry.

#### Scenario: Dashboard cannot mutate workspace by default

- **WHEN** dashboard requests workspace, project board, database view, graph, backlink, or vault health endpoints
- **THEN** Pinax SHALL serve readonly projection data
- **AND** write-like HTTP methods SHALL be rejected unless a future explicit local write design adds approval, snapshot, receipt, and redaction gates.

### Requirement: Remote API Mode SHALL preserve local-control boundaries

Pinax SHALL prevent persisted remote configuration from hijacking local control commands even as workspace, task, database, and graph command coverage expands.

#### Scenario: Local control commands stay local under remote config

- **GIVEN** `remote.api_url` is configured
- **WHEN** the user runs `pinax config`, `pinax api`, `pinax token`, `pinax profile`, `pinax vault`, `pinax cloud`, `pinax sync daemon`, completion, foreground server, editor, or local filesystem diagnostic commands
- **THEN** Pinax SHALL keep those commands local unless a dedicated safe capability explicitly covers that operation
- **AND** unsupported remote commands SHALL return `remote_command_unsupported` rather than silently running against a local vault.
