# pinax-cli-remote-api-mode Specification

## Purpose
Define Pinax local API remote mode: HTTP RPC dispatch, CLI forwarding to a running `pinax api serve` process, safe bearer-token handling, and output-contract parity for JSON and agent consumers.
## Requirements
### Requirement: Local API HTTP RPC transport

Pinax SHALL expose an HTTP RPC route for the local API server that dispatches registered RPC capabilities through application services and returns Projection envelopes.

#### Scenario: Call registered RPC method over HTTP

- **GIVEN** `pinax api serve` is running for a vault
- **WHEN** a client sends `POST /v1/rpc` with method `Pinax.Folder.List`
- **THEN** the server SHALL dispatch through the existing RPC dispatcher and application service
- **AND** the response SHALL be a valid Pinax Projection envelope with command `folder.list`.

#### Scenario: Reject unknown RPC method

- **GIVEN** `pinax api serve` is running
- **WHEN** a client sends `POST /v1/rpc` with an unregistered method
- **THEN** the server SHALL return a failed Projection with error code `rpc_method_not_found`
- **AND** the server SHALL NOT execute any vault mutation.

#### Scenario: Enforce write gates for RPC

- **GIVEN** the RPC method is write-capable
- **WHEN** the server was not started with `--allow-write`
- **THEN** the server SHALL return `write_disabled`
- **AND** the vault SHALL remain unchanged.

#### Scenario: Require confirmation for remote write RPC

- **GIVEN** the server was started with `--allow-write`
- **WHEN** a write RPC is sent without `yes=true` and without `dry_run=true`
- **THEN** the server SHALL return `approval_required`
- **AND** the vault SHALL remain unchanged.

### Requirement: CLI remote mode selection

Pinax SHALL provide a CLI remote mode that forwards supported ordinary commands to a running local API service when an API URL is configured.

#### Scenario: Enable remote mode with flag

- **GIVEN** `pinax api serve` is running at `http://127.0.0.1:8787`
- **WHEN** the user runs `pinax --api-url http://127.0.0.1:8787 folder list --json`
- **THEN** the CLI SHALL call the remote API instead of reading a local vault
- **AND** stdout SHALL contain only the returned JSON Projection envelope.

#### Scenario: Forward note list query flags

- **GIVEN** `pinax api serve` is running at `http://127.0.0.1:8787`
- **WHEN** the user runs `pinax --api-url http://127.0.0.1:8787 note list --status active --tag research --limit 20 --json`
- **THEN** the CLI SHALL call RPC method `Pinax.Note.List`
- **AND** the server SHALL return the `note.list` Projection filtered by the supplied list flags.

#### Scenario: Enable remote mode with environment variable

- **GIVEN** `PINAX_API_URL` is set to `http://127.0.0.1:8787`
- **WHEN** the user runs a supported command such as `pinax inbox list --json`
- **THEN** the CLI SHALL execute the command through the remote API service.

#### Scenario: Enable remote mode with user configuration

- **GIVEN** user config contains `remote.api_url: http://127.0.0.1:8787`
- **WHEN** the user runs a supported command such as `pinax folder list --json`
- **THEN** the CLI SHALL execute the command through the remote API service without requiring `--api-url` or `PINAX_API_URL`.

#### Scenario: Keep local control commands editable with configured remote URL

- **GIVEN** user config contains `remote.api_url: http://127.0.0.1:8787`
- **WHEN** the user runs a local control command such as `pinax config get remote.api_url --agent`
- **THEN** the CLI SHALL execute the control command locally so the configured endpoint remains inspectable and editable.

#### Scenario: Reject explicit vault conflict

- **GIVEN** remote mode is enabled with `--api-url`
- **WHEN** the user also supplies an explicit `--vault`
- **THEN** the CLI SHALL return `remote_vault_conflict`
- **AND** the CLI SHALL NOT execute against either local or remote vault.

#### Scenario: Unsupported command does not fallback

- **GIVEN** remote mode is enabled
- **WHEN** the user runs a command not supported by remote mode
- **THEN** the CLI SHALL return `remote_command_unsupported`
- **AND** the CLI SHALL NOT execute the command locally.

#### Scenario: Configured remote command still rejects unsupported business commands

- **GIVEN** remote mode is enabled only by `remote.api_url`
- **WHEN** the user runs a non-control command that is not supported by remote mode
- **THEN** the CLI SHALL return `remote_command_unsupported`
- **AND** the CLI SHALL NOT execute the command locally.

### Requirement: Remote API client safety

Pinax SHALL keep remote transport credentials and request bodies out of user output, logs, fixtures, and errors.

#### Scenario: Send bearer token without leaking it

- **GIVEN** a bearer token is configured through `--api-token`, `--api-token-file`, or `PINAX_API_TOKEN`
- **WHEN** the CLI calls the remote API
- **THEN** the client SHALL send the token only as an Authorization header
- **AND** no stdout, stderr, Projection error, audit log, or test fixture SHALL contain the raw token or Authorization header.

#### Scenario: Decode non-2xx projection response

- **GIVEN** the remote API returns a non-2xx HTTP status with a Projection envelope
- **WHEN** the CLI receives the response
- **THEN** the CLI SHALL render that Projection through the selected output mode
- **AND** it SHALL NOT replace it with an unstructured transport error.

#### Scenario: Report invalid remote response

- **GIVEN** the remote API response is not a valid Projection envelope
- **WHEN** the CLI receives the response
- **THEN** the CLI SHALL return `remote_api_invalid_response`
- **AND** the error SHALL NOT include credentials or raw request body.

### Requirement: RPC operation logging

Pinax SHALL log enough RPC operation metadata to diagnose remote calls without exposing request details or note contents.

#### Scenario: Log RPC method and outcome

- **GIVEN** `pinax api serve` receives `POST /v1/rpc` for a registered method
- **WHEN** the RPC completes or fails at an API gate
- **THEN** stderr diagnostics SHALL include an `api.rpc` log with RPC method, optional RPC id, mapped command, group, readonly flag, HTTP status, duration, and projection error code when present
- **AND** the log SHALL NOT include RPC params, request body, response body, note content, Authorization header, cookies, tokens, or provider payloads.

### Requirement: Remote output contract parity

Remote mode SHALL preserve the existing Pinax CLI output modes.

#### Scenario: JSON output stays machine-only

- **GIVEN** remote mode is enabled
- **WHEN** the user runs a supported command with `--json`
- **THEN** stdout SHALL contain one valid JSON Projection object
- **AND** stdout SHALL NOT contain diagnostics, logs, ANSI decoration, or prose outside the JSON object.

#### Scenario: Agent output remains key-value

- **GIVEN** remote mode is enabled
- **WHEN** the user runs a supported command with `--agent`
- **THEN** stdout SHALL contain stable key=value lines rendered from the returned Projection
- **AND** stdout SHALL NOT contain human prose or credentials.

### Requirement: Project workspace REST routes SHALL be projection adapters

Pinax SHALL expose project workspace routes through the local REST/RPC projection adapter without creating a separate remote project management model.

#### Scenario: Read project workspace routes
- **WHEN** local API clients call `GET /v1/projects`, `GET /v1/projects/{project}`, `GET /v1/projects/{project}/subprojects`, `GET /v1/projects/{project}/subprojects/{subproject}`, or `GET /v1/project-items/{item_id}`
- **THEN** Pinax SHALL route through the application service
- **AND** responses SHALL be Pinax projection envelopes
- **AND** handlers SHALL NOT directly parse Markdown, read `.pinax` structured assets, query GORM repositories, call Git, or invoke providers.

#### Scenario: Project board route supports optional subproject query
- **WHEN** a local API client calls `GET /v1/projects/research/board?subproject=stock-learning&note_display=card`
- **THEN** the response SHALL match the bounded projection returned by `pinax project board show research --subproject stock-learning --note-display card --vault <vault> --json`
- **AND** omitting `subproject` SHALL preserve existing project-wide board behavior.

### Requirement: Project workspace write routes SHALL be gated

Pinax SHALL expose only controlled write-plan or confirmed write routes for project workspace operations.

#### Scenario: Readonly API rejects workspace writes
- **GIVEN** `pinax api serve --vault yeisme-notes` is running in default readonly mode
- **WHEN** a client calls `POST /v1/project-items?project=research&subproject=stock-learning&yes=true`
- **THEN** Pinax SHALL return a failed projection with `error.code=write_disabled`
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Allow-write API still requires confirmation
- **GIVEN** `pinax api serve --vault yeisme-notes --allow-write` is running
- **WHEN** a client calls a project workspace write route without `yes=true`
- **THEN** Pinax SHALL return `approval_required`
- **AND** it SHALL include a runnable CLI next action when useful.

#### Scenario: High-risk workspace write requires snapshot
- **GIVEN** a project workspace write would archive, delete, batch-change, or rewrite managed Markdown
- **WHEN** the request includes `yes=true` but no recent snapshot evidence exists
- **THEN** Pinax SHALL return `snapshot_required`
- **AND** the response body SHALL remain a failed Pinax projection envelope.

### Requirement: Project workspace capabilities SHALL be discoverable

Pinax SHALL publish project workspace routes and RPC methods from the shared route registry.

#### Scenario: API route list includes workspace capabilities
- **WHEN** the user runs `pinax api routes --vault yeisme-notes --json`
- **THEN** project workspace routes SHALL include route id, surface, method, path or RPC method, command, capability id, schema version, readonly, body_allowed, approval_required, snapshot_required, and stable errors.

#### Scenario: OpenAPI export includes workspace routes
- **WHEN** the user runs `pinax api schema export --format openapi --vault yeisme-notes --json`
- **THEN** project workspace REST routes SHALL appear in `data.schema.paths`
- **AND** operations SHALL include `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`.

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

### Requirement: 客户端 CLI 覆盖通过 capability registry 增量扩展

Pinax SHALL evolve client CLI parity by registering safe capabilities in `RemoteCapabilities()` / `RemoteRoutes()` and SHALL NOT expose a generic remote shell or arbitrary command runner.

#### Scenario: 新客户端能力必须可发现

- **WHEN** a CLI capability is made available to Remote API clients
- **THEN** `pinax api routes --vault <vault> --json` SHALL list its capability id, command, route or RPC method, readonly status, body allowance, approval requirement, snapshot requirement, and stable errors
- **AND** `pinax api schema export --format openapi --vault <vault> --json` SHALL derive REST operation metadata from the same registry when a REST route exists.

#### Scenario: unsupported command never falls back locally

- **GIVEN** remote mode is enabled with `--api-url`, `PINAX_API_URL`, or `remote.api_url`
- **WHEN** the user runs a command that is not registered for remote mode and is not explicitly local-only
- **THEN** Pinax SHALL return `remote_command_unsupported`
- **AND** it SHALL NOT execute the command against a local vault as a fallback.

#### Scenario: local control commands remain local

- **GIVEN** remote mode is enabled only by persisted `remote.api_url`
- **WHEN** the user runs `pinax config`, `pinax api`, `pinax token`, `pinax profile`, `pinax vault`, `pinax cloud`, `pinax sync daemon`, completion, foreground server, or editor commands
- **THEN** Pinax SHALL keep those commands local unless a dedicated safe capability explicitly covers the operation.

### Requirement: 客户端写操作复用 CLI 安全门禁

Pinax SHALL keep every remote client write behind the same application service, approval, dry-run, snapshot, receipt, and redaction boundaries as the equivalent CLI command.

#### Scenario: readonly server rejects writes

- **GIVEN** `pinax api serve --vault <vault>` is running without `--allow-write`
- **WHEN** a client calls a write-capable REST route or RPC method with `yes=true`
- **THEN** Pinax SHALL return `write_disabled`
- **AND** no Markdown, `.pinax/**`, SQLite index, Git state, provider state, sync-state, token file, or remote service SHALL be modified.

#### Scenario: allow-write still requires confirmation

- **GIVEN** `pinax api serve --vault <vault> --allow-write` is running
- **WHEN** a write-capable client call omits both `yes=true` and `dry_run=true`
- **THEN** Pinax SHALL return `approval_required`
- **AND** the returned projection SHALL include a safe next action when one is available.

#### Scenario: risky writes require snapshot evidence

- **GIVEN** a remote client write would rename, move, delete, archive, apply repairs, apply organize plans, restore, publish, deploy, or batch-modify managed content
- **WHEN** the request lacks required snapshot evidence
- **THEN** Pinax SHALL return `snapshot_required` or an equivalent plan-only projection
- **AND** it SHALL NOT perform the risky write.

### Requirement: 客户端覆盖矩阵可审计

Pinax SHALL provide or test a coverage matrix that compares the CLI command tree with remote capability support.

#### Scenario: command parity audit classifies every command

- **WHEN** the parity audit runs
- **THEN** every user-visible CLI command SHALL be classified as `remote_supported`, `local_only`, or `unsupported`
- **AND** every `remote_supported` command SHALL point to a capability id or RPC method
- **AND** every `local_only` command SHALL include a reason such as runtime control, credential control, foreground process, editor, completion, local filesystem diagnostic, or daemon lifecycle.

