# project-board-workspace Specification

## Purpose
Define Pinax local project board workspace behavior: bounded board and note projections, controlled Markdown work item writes, planning snapshot integration, dashboard/MCP readonly access, localhost REST/RPC projection adapters, and fixture-first verification without remote provider dependencies.
## Requirements
### Requirement: Pinax exposes local project board projections

Pinax SHALL render project board views from local project metadata, Markdown notes, typed index/query projections, planning snapshots, and optional TaskBridge facts without requiring remote provider credentials.

#### Scenario: Showing a project board
- **GIVEN** a Pinax vault has a project `research` and notes tagged or frontmatter-linked to that project
- **WHEN** the user runs `pinax project board show research --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command=project.board.show`, project facts, board columns, item counts, warnings, index status, and next actions
- **AND** no Markdown file, `.pinax` asset, Git state, TaskBridge state, provider state, or remote service SHALL be modified.

#### Scenario: Default human board summary
- **GIVEN** a project board can be built
- **WHEN** the user runs `pinax project board show research --vault ./my-notes`
- **THEN** stdout SHALL contain a concise Chinese summary with counts for `next`, `doing`, `blocked`, `review`, and `done`
- **AND** it SHALL include at most one recommended next action by default.

#### Scenario: Board rows use bounded note cards
- **GIVEN** a project board contains items linked to local notes
- **WHEN** the user runs `pinax project board show research --note-display card --vault ./my-notes --json`
- **THEN** each board item that references a note SHALL include a bounded note card with title, path, note id, project, kind, status, tags, updated time, excerpt, and board column when available
- **AND** the board projection SHALL NOT include full note bodies, raw provider payloads, hidden prompts, or secrets.

#### Scenario: Degrading when index is stale
- **GIVEN** the local SQLite/GORM index is missing or stale
- **WHEN** the user runs `pinax project board show research --vault ./my-notes --json`
- **THEN** Pinax SHALL either rebuild through an explicit command path or degrade to a bounded Markdown scan
- **AND** the projection SHALL report `index_status=missing` or `index_status=stale`
- **AND** it SHALL include a runnable `pinax index rebuild --vault ./my-notes` next action.

### Requirement: Pinax provides a shared NoteDisplay surface

Pinax SHALL expose local notes through a shared `NoteDisplay` projection so note read/show, project board, dashboard, and MCP surfaces present consistent bounded information.

#### Scenario: Read note as card
- **GIVEN** a Pinax-managed note `note_123` exists
- **WHEN** the user runs `pinax note read note_123 --display card --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command=note.show`, `facts.display=card`, and bounded note identity facts
- **AND** `data.note` SHALL include title, path, note id, project, kind, status, tags, updated time, excerpt, and board column when available
- **AND** it SHALL NOT include full note body.

#### Scenario: Read note detail with context
- **GIVEN** a Pinax-managed note has links, backlinks, attachments, and related project notes
- **WHEN** the user runs `pinax note read note_123 --display detail --vault ./my-notes --json` or `pinax note read note_123 --display context --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with bounded `NoteDisplay` facts selected for the requested display level
- **AND** the projection SHALL NOT include full note bodies.

#### Scenario: Read note body only when explicit
- **GIVEN** a Pinax-managed note `note_123` exists
- **WHEN** the user runs `pinax note show note_123 --display body --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `facts.display=body` and the note body under `data.note.body`
- **AND** default board, dashboard, MCP, and `--agent` surfaces SHALL NOT include that body unless a future explicitly approved body-capable command is added.

#### Scenario: Preserve source and rendered view compatibility
- **GIVEN** existing scripts use `pinax note show note_123 --view source --vault ./my-notes --json` or `pinax note show note_123 --view rendered --vault ./my-notes --json`
- **WHEN** display profiles are implemented
- **THEN** existing source/rendered view behavior SHALL remain compatible
- **AND** `--display` SHALL control information breadth rather than replacing the existing source/rendered body semantics.

### Requirement: Board configuration is CLI-authored
Pinax SHALL persist and apply project board configuration through CLI commands or application services rather than requiring agents to hand-write metadata.

#### Scenario: Configured columns drive board projection
- **GIVEN** a project `investing` has a subproject `stock-learning`
- **AND** the user runs `pinax project board configure investing --subproject stock-learning --columns inbox,planned,learning,practice,review,retrospective,done --vault ./my-notes --json`
- **WHEN** the user runs `pinax project board show investing --subproject stock-learning --vault ./my-notes --json`
- **THEN** `data.board.columns` SHALL use the configured columns in order
- **AND** `facts` SHALL include optional `column.<id>` counts for configured columns
- **AND** existing facts such as `next`, `doing`, `blocked`, `review`, and `done` SHALL remain present for compatibility.

#### Scenario: Project items accept configured columns
- **GIVEN** a subproject board is configured with column `learning`
- **WHEN** the user runs `pinax project item add investing "学习 K 线基础" --subproject stock-learning --column learning --vault ./my-notes --json`
- **THEN** Pinax SHALL create a managed project item in column `learning`
- **AND** the item SHALL appear under `learning` in the next scoped board projection.

### Requirement: Project items are managed local Markdown work items

Pinax SHALL support controlled project item creation and movement without treating arbitrary Markdown checklist lines as writable tasks.

#### Scenario: Adding a project item
- **GIVEN** a project `research` exists
- **WHEN** the user runs `pinax project item add research "实现看板 projection" --column next --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Pinax-managed Markdown note or managed item block through the application service
- **AND** the created item SHALL include stable item id, project slug, title, column, status, created time, updated time, and note reference facts
- **AND** stdout SHALL contain one JSON envelope with `command=project.item.add`.

#### Scenario: Moving a managed item
- **GIVEN** a Pinax-managed item `item_abc123` exists in column `next`
- **WHEN** the user runs `pinax project item move item_abc123 doing --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the managed item metadata through the application service
- **AND** it SHALL append redacted event evidence
- **AND** the next board projection SHALL place the item in `doing`.

#### Scenario: Refusing to modify unmanaged checklist lines
- **GIVEN** a board item was inferred from an arbitrary Markdown checklist line not owned by Pinax
- **WHEN** the user runs `pinax project item move <inferred-item-id> done --vault ./my-notes --json`
- **THEN** Pinax SHALL refuse the write with a stable error code such as `project_item_unmanaged`
- **AND** it SHALL provide a safe next action such as creating a managed item or opening the note manually.

### Requirement: Risky board writes require approval and snapshot protection

Pinax SHALL keep project board write operations explicit and recoverable.

#### Scenario: Archive requires approval
- **GIVEN** a managed item exists
- **WHEN** the user runs `pinax project item archive item_abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with `approval_required`
- **AND** no Markdown file, `.pinax` asset, Git state, TaskBridge state, provider state, or remote service SHALL be modified.

#### Scenario: Snapshot required for high-risk move
- **GIVEN** an item move would archive, delete, batch-change, or rewrite managed Markdown
- **WHEN** the user runs `pinax project item move item_abc123 done --yes --vault ./my-notes --json` without recent Pinax snapshot evidence
- **THEN** Pinax SHALL fail with `snapshot_required`
- **AND** the projection SHALL include a runnable `pinax version snapshot --vault ./my-notes --message "看板更新前快照"` next action.

#### Scenario: Approved protected write succeeds
- **GIVEN** a recent Pinax Git snapshot exists
- **WHEN** the user runs `pinax project item archive item_abc123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL archive only the managed item inside the vault boundary
- **AND** it SHALL record redacted event evidence.

### Requirement: Project board integrates with planning without becoming Todo storage

Pinax SHALL let planning workflows consume project board snapshots and facts while keeping TaskBridge as the execution control plane.

#### Scenario: Saving a board planning snapshot
- **GIVEN** a project board can be generated
- **WHEN** the user runs `pinax project board plan research --vault ./my-notes --save --json`
- **THEN** Pinax SHALL write a redacted board snapshot through the planning or project board service
- **AND** the snapshot SHALL include schema version, snapshot id, project slug, source facts, column counts, risks, next actions, and evidence refs
- **AND** it SHALL NOT include provider tokens, Authorization headers, raw prompts, hidden system prompts, raw provider payloads, tool private parameters, or complete chain-of-thought.

#### Scenario: Planning reads board facts
- **GIVEN** a saved board snapshot exists for project `research`
- **WHEN** the user runs `pinax plan weekly --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** the planning decision MAY include board facts such as blocked count, next count, overdue project items, and evidence refs
- **AND** it SHALL NOT automatically create or modify remote Todo tasks.

#### Scenario: TaskBridge remains optional
- **GIVEN** TaskBridge is unavailable
- **WHEN** the user runs `pinax project board show research --vault ./my-notes --json`
- **THEN** local board projection SHALL still work from Markdown vault and index facts
- **AND** TaskBridge unavailability SHALL be reported only as a warning when TaskBridge facts were requested.

### Requirement: Project board follows the AI-native CLI output contract

Pinax project board and item commands SHALL render human and machine outputs from one command projection.

#### Scenario: Machine output mode
- **GIVEN** a project board or item command supports `--json`, `--agent`, `--events`, or `--explain`
- **WHEN** a machine output mode is selected
- **THEN** stdout SHALL contain only the selected machine format
- **AND** progress, diagnostics, external command stderr, and non-structured errors SHALL go to stderr
- **AND** errors SHALL include stable status and error code fields.

#### Scenario: Agent output is bounded
- **GIVEN** the user runs `pinax project board show research --vault ./my-notes --agent`
- **WHEN** output is rendered
- **THEN** stdout SHALL contain low-token key=value facts for project, column counts, risk counts, index status, snapshot id when available, and next actions
- **AND** it SHALL NOT include full note bodies or raw provider payloads.

#### Scenario: Note display fields are stable
- **GIVEN** a command returns a `NoteDisplay` payload through `--json` or `--agent`
- **WHEN** output is rendered
- **THEN** stable fields SHALL include `note_id`, `title`, `path`, `project`, `kind`, `status`, `tags`, `updated_at`, `display`, `exposure`, `excerpt`, `board_column`, `links_count`, `backlinks_count`, `attachments_count`, `related_count`, and `redaction_warnings` when available
- **AND** new fields SHALL be optional unless a future major output contract version is declared.

#### Scenario: Human note display is readable but not a machine contract
- **GIVEN** the user runs `pinax note read note_123 --display detail --vault ./my-notes`
- **WHEN** default human output is rendered
- **THEN** stdout SHALL contain a concise Chinese metadata summary, readable note facts, and at most one recommended next action before any requested body content
- **AND** scripts and agents SHALL use `--json` or `--agent` rather than parsing localized human text.

### Requirement: Dashboard and MCP expose readonly board surfaces

Pinax SHALL expose project board context through readonly dashboard and MCP surfaces backed by the same application service projection.

#### Scenario: Dashboard board API is readonly
- **GIVEN** dashboard is running for a Pinax vault
- **WHEN** a user requests the project board endpoint for `research`
- **THEN** it SHALL return bounded board facts and next actions
- **AND** write-like HTTP methods SHALL be rejected without modifying Markdown, `.pinax`, Git, TaskBridge, provider, or remote state.

#### Scenario: Dashboard note drilldown is bounded
- **GIVEN** dashboard shows a project board item linked to `note_123`
- **WHEN** the dashboard requests note detail for that item
- **THEN** Pinax SHALL return `NoteDisplay` with display `card`, `detail`, or `context`
- **AND** it SHALL NOT return full body content unless a future explicit local body endpoint with approval and redaction rules is designed.

#### Scenario: MCP board resource is readonly
- **GIVEN** `pinax mcp serve --vault ./my-notes` is running
- **WHEN** an MCP client reads `pinax://project/research/board` or calls `pinax.project.board`
- **THEN** Pinax SHALL route through the project board application service
- **AND** it SHALL return bounded facts without full note bodies
- **AND** it SHALL NOT expose write-capable board tools in MVP.

#### Scenario: MCP note context is bounded
- **GIVEN** an MCP client asks for project note context
- **WHEN** Pinax returns note display facts
- **THEN** the response SHALL use display `card`, `detail`, or `context` with exposure `agent`
- **AND** it SHALL NOT include full note bodies, raw prompts, hidden system prompts, provider payloads, tool private parameters, or complete chain-of-thought.

### Requirement: REST and RPC surfaces are projection adapters

Pinax SHALL expose REST and RPC interfaces as thin adapters over application service projections rather than maintaining separate remote business models.

#### Scenario: REST board endpoint matches CLI projection
- **GIVEN** a project `research` exists
- **WHEN** a local REST client calls `GET /v1/projects/research/board?note_display=card`
- **THEN** Pinax SHALL route through the same application service path as `pinax project board show research --note-display card --vault ./my-notes --json`
- **AND** the response SHALL be a JSON projection envelope with the same command, status, facts keys, actions, and bounded note display fields
- **AND** the REST handler SHALL NOT directly parse Markdown, read `.pinax` structured assets, or call GORM repositories.

#### Scenario: REST note endpoint gates body exposure
- **GIVEN** a Pinax-managed note `note_123` exists
- **WHEN** a local REST client calls `GET /v1/notes/note_123?display=detail`
- **THEN** the response SHALL include `NoteDisplay` detail facts without full note body
- **AND** `display=body` SHALL be allowed only for explicitly local body exposure and SHALL be redacted according to the same policy as CLI `--json`.

#### Scenario: RPC method returns the same envelope
- **GIVEN** RPC is enabled for a local Pinax vault
- **WHEN** an RPC client calls `Pinax.ProjectBoard.Show` with project `research` and `note_display=card`
- **THEN** the response SHALL use the same projection envelope schema as REST and CLI JSON output
- **AND** RPC SHALL NOT invent a separate response shape for board items, note display, errors, actions, or evidence.

#### Scenario: Transport status does not replace command errors
- **GIVEN** a REST or RPC request has invalid parameters
- **WHEN** Pinax returns an error
- **THEN** the transport status MAY be 400, 403, 404, 409, or 500 as appropriate
- **AND** the response body SHALL still contain a failed projection with stable `error.code`, Chinese `error.message`, optional `error.hint`, and runnable next actions when useful.

### Requirement: Remote capabilities are discoverable and versioned

Pinax SHALL publish remote route and RPC method capabilities from a single registry so clients can adapt without scraping docs or localized human output.

#### Scenario: Listing API capabilities
- **GIVEN** local API support is enabled
- **WHEN** the user runs `pinax api routes --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command=api.routes`
- **AND** each route SHALL include route id, surface, method, path or RPC method, projection command, capability id, schema version, readonly flag, body_allowed flag, approval requirement, snapshot requirement, and stable error codes.

#### Scenario: Exporting OpenAPI schema
- **GIVEN** REST routes are registered
- **WHEN** the user runs `pinax api schema export --format openapi --vault ./my-notes --json`
- **THEN** Pinax SHALL return or write a schema derived from the route registry and projection schema
- **AND** each registered REST route with a path SHALL appear exactly once under `data.schema.paths`
- **AND** each operation method SHALL equal the lowercase registered route method
- **AND** each operation SHALL include `operationId`, `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`
- **AND** schema output SHALL NOT be hand-maintained separately from handler behavior without contract tests catching drift.

#### Scenario: REST transport errors preserve projection envelopes
- **WHEN** a local REST client requests an unknown API path or uses an unsupported method for a registered path
- **THEN** Pinax SHALL return HTTP `404` with `error.code=route_not_found` or HTTP `405` with `error.code=method_not_allowed`
- **AND** the response body SHALL remain a failed Pinax projection envelope.

#### Scenario: RPC capabilities match REST capabilities
- **GIVEN** REST and RPC expose the same project board capability
- **WHEN** capabilities are listed
- **THEN** both surfaces SHALL point to the same projection command and response schema version
- **AND** differences such as streaming support or body exposure SHALL be explicit capability fields.

### Requirement: Remote writes stay explicit and recoverable

Pinax SHALL keep REST and RPC write-capable operations behind the same dry-run, approval, and Git snapshot gates as CLI commands.

#### Scenario: Remote write defaults to plan
- **GIVEN** a remote client asks to move a project item
- **WHEN** it calls `POST /v1/project-items/item_abc123:move` without approval fields
- **THEN** Pinax SHALL return a dry-run or failed projection rather than modifying Markdown or `.pinax` assets
- **AND** the projection SHALL include the equivalent CLI next action when user confirmation is needed.

#### Scenario: Remote write requires snapshot when risky
- **GIVEN** moving or archiving an item would rewrite managed Markdown
- **WHEN** a REST or RPC client requests the write without recent snapshot evidence
- **THEN** Pinax SHALL return `snapshot_required`
- **AND** no Markdown file, `.pinax` asset, Git state, TaskBridge state, provider state, or remote service SHALL be modified.

#### Scenario: Remote server is localhost-only by default
- **GIVEN** a user starts the local API server
- **WHEN** they run `pinax api serve --vault ./my-notes --readonly --port 0`
- **THEN** Pinax SHALL bind to `127.0.0.1` by default
- **AND** non-loopback bind, CORS, TLS, token auth, multi-user access, and hosted API gateway behavior SHALL be rejected or reported unsupported until a dedicated security design exists.

#### Scenario: API serve lifecycle output is mode-safe
- **GIVEN** a user starts the local API server with `pinax api serve --readonly --port 0 --vault ./my-notes`
- **WHEN** no machine output mode is selected
- **THEN** stdout SHALL remain empty and the local URL SHALL be written to stderr
- **AND** `--events` SHALL emit `start`, `ready`, and `shutdown` or `error` NDJSON events on stdout
- **AND** `--json` and `--agent` SHALL either emit one startup projection and keep stdout quiet afterward or return `unsupported_output_mode` without mixing logs, URL banners, or human prose into machine stdout
- **AND** omitting `--readonly` SHALL return `readonly_required` without starting a server.

### Requirement: Project board implementation has fixture-first tests

Project board workflows SHALL be testable without real Todo provider credentials, real TaskBridge stores, remote networks, or the user's vault.

#### Scenario: Testing board workflows
- **GIVEN** project board commands are implemented
- **WHEN** tests are added
- **THEN** command e2e tests SHOULD use `github.com/rogpeppe/go-internal/testscript`
- **AND** tests SHALL use fixture vaults, temporary Git repositories, fake TaskBridge executables when needed, and redaction assertions
- **AND** tests SHALL cover readonly projection, save snapshot, configure columns, item add/move/archive, note display card/detail/context/body, approval gate, snapshot guard, stdout/stderr separation, unsupported source handling, dashboard readonly behavior, MCP readonly behavior, and body exposure gating.

#### Scenario: Testing REST and RPC contracts with evidence
- **GIVEN** REST or RPC server behavior is implemented
- **WHEN** integration or component tests run
- **THEN** each run SHALL write redacted evidence under `temp/integration-test-runs/<run-id>/`
- **AND** evidence SHALL include `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, and `artifacts/`
- **AND** tests SHALL cover capabilities, board endpoint, note display endpoint, RPC board method, dry-run write behavior, approval and snapshot errors, transport status mapping, stdout/stderr separation, and redaction.

#### Scenario: Requiring comments for non-obvious logic
- **GIVEN** future implementation touches column mapping, source normalization, TaskBridge protocol conversion, managed Markdown patching, approval/snapshot decisions, or non-obvious fixtures
- **WHEN** code is added or changed
- **THEN** implementation tasks SHALL require succinct Chinese comments explaining the non-obvious decision or recovery boundary.

### Requirement: Pinax API routes human output is scannable

Pinax SHALL render `pinax api routes` default human output with enough route detail for users to inspect local REST/RPC capabilities without switching to JSON.

#### Scenario: API routes summary includes endpoint evidence

- **WHEN** a user runs `pinax api routes --vault ./my-notes`
- **THEN** stdout SHALL include a Chinese summary and route count facts
- **AND** stdout SHALL include readable route evidence containing REST method/path or RPC method name plus the projection command.

#### Scenario: API routes machine output remains complete

- **WHEN** a user runs `pinax api routes --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `command=api.routes`
- **AND** `data.routes` and `data.capabilities` SHALL remain the machine-readable route registry for scripts and agents.

### Requirement: REST and RPC expose inbox and draft readonly projections

Pinax SHALL expose inbox and draft readonly operations through the local REST/RPC projection adapter without creating a separate remote business model.

#### Scenario: REST lists inbox items
- **WHEN** a local REST client calls `GET /v1/inbox?limit=20`
- **THEN** Pinax SHALL return the same projection envelope schema as `pinax inbox list --vault ./my-notes --json`
- **AND** the REST handler SHALL NOT directly parse Markdown, read `.pinax` structured assets, call GORM repositories, or mutate vault state.

#### Scenario: REST shows draft item
- **WHEN** a local REST client calls `GET /v1/drafts/note_123?view=rendered`
- **THEN** Pinax SHALL return a bounded note display projection for the draft
- **AND** the response SHALL include stable facts for note id, path, title, status, lifecycle status, display mode, and body exposure.

#### Scenario: RPC readonly methods match CLI projections
- **WHEN** an RPC client calls `Pinax.Inbox.List`, `Pinax.Inbox.Show`, `Pinax.Draft.List`, or `Pinax.Draft.Show`
- **THEN** the response SHALL use the same projection envelope schema as REST and CLI JSON output
- **AND** RPC SHALL NOT invent a separate response shape for notes, errors, actions, or evidence.

### Requirement: Remote inbox and draft writes stay gated

Pinax SHALL expose inbox and draft write-like operations only as explicit gated projection adapter routes.

#### Scenario: Readonly server rejects inbox capture
- **GIVEN** a user starts the local API server without `--allow-write`
- **WHEN** a REST client calls `POST /v1/inbox:capture` with a title and body
- **THEN** Pinax SHALL return a failed projection with stable error code `write_disabled`
- **AND** no Markdown, `.pinax` asset, index projection, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Write server still requires approval for lifecycle mutation
- **GIVEN** a user starts the local API server with `--allow-write`
- **WHEN** a REST client calls `POST /v1/drafts/note_123:discard` without `yes=true`
- **THEN** Pinax SHALL return a failed projection with stable error code `approval_required`
- **AND** no Markdown, `.pinax` event, record metadata, index projection, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Dry-run remote promote returns plan only
- **GIVEN** a user starts the local API server with `--allow-write`
- **WHEN** a REST client calls `POST /v1/inbox/note_123:promote?to=active&dry_run=true`
- **THEN** Pinax SHALL return a transition plan projection with `writes=false`
- **AND** it SHALL NOT write Markdown, `.pinax` assets, index projection, Git state, provider state, or remote services.

#### Scenario: Approved remote promote calls application service
- **GIVEN** a user starts the local API server with `--allow-write`
- **WHEN** a REST client calls `POST /v1/drafts/note_123:promote?status=active&yes=true`
- **THEN** Pinax SHALL call the same application service used by `pinax draft promote`
- **AND** it SHALL return a projection envelope with old status, new status, path, writes, record event, and index update facts.

### Requirement: Route registry advertises inbox and draft capabilities

Pinax SHALL publish inbox and draft REST/RPC capabilities from the shared route registry so remote clients can discover allowed operations without scraping localized help text.

#### Scenario: API routes includes inbox and draft routes
- **WHEN** a user runs `pinax api routes --vault ./my-notes --json`
- **THEN** the route registry projection SHALL include REST and RPC entries for inbox and draft list, show, capture/create, promote, archive, and discard operations
- **AND** each route SHALL include route id, surface, method, path or RPC method, projection command, capability id, schema version, readonly flag, body allowed flag, approval requirement, snapshot requirement, and stable error codes.

#### Scenario: OpenAPI schema exports review routes
- **WHEN** a user runs `pinax api schema export --format openapi --vault ./my-notes --json`
- **THEN** the exported schema SHALL include inbox and draft REST paths with methods derived from the shared route registry
- **AND** each operation SHALL include `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, and `x-pinax-approval-required` metadata.

#### Scenario: Remote errors preserve projection envelopes
- **WHEN** a REST or RPC inbox/draft route receives invalid parameters, unknown note refs, invalid lifecycle targets, disabled writes, or missing approval
- **THEN** Pinax SHALL return a failed projection envelope with a stable error code
- **AND** transport status mapping SHALL NOT replace, localize, or drop the projection error details.

### Requirement: Pinax API OpenAPI schema is derived from the route registry

Pinax SHALL derive local API OpenAPI REST paths, methods, operation ids, and Pinax extension metadata from the same remote route registry returned by `pinax api routes`.

#### Scenario: OpenAPI method matches registered REST method
- **GIVEN** a REST route is registered with `surface=rest`, `method=POST`, and path `/v1/project-items/{ref}:{action}`
- **WHEN** a user runs `pinax api schema export --format openapi --vault ./my-notes --json`
- **THEN** the exported OpenAPI schema SHALL include `/v1/project-items/{ref}:{action}` with a `post` operation
- **AND** it SHALL NOT export that route as a `get` operation.

#### Scenario: Every registered REST route appears in OpenAPI
- **GIVEN** REST routes are registered in the remote route registry
- **WHEN** OpenAPI schema is exported
- **THEN** each registered REST route with a path SHALL appear exactly once under `data.schema.paths`
- **AND** the operation method SHALL equal the lowercase registered route method.

#### Scenario: OpenAPI operation includes Pinax route metadata
- **GIVEN** a REST route is exported to OpenAPI
- **WHEN** a client inspects the operation object
- **THEN** the operation SHALL include `operationId`, `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`
- **AND** those values SHALL match the registered route and capability metadata.

### Requirement: Pinax local REST routes preserve projection error envelopes

Pinax SHALL map local REST transport status codes without replacing or dropping the command projection error envelope.

#### Scenario: Unknown REST route returns projection 404
- **WHEN** a local REST client requests an unknown API path
- **THEN** Pinax SHALL return HTTP 404
- **AND** the response body SHALL be a failed projection envelope with `error.code=route_not_found`.

#### Scenario: Unsupported REST method returns projection 405
- **WHEN** a local REST client uses an unsupported method for a registered path
- **THEN** Pinax SHALL return HTTP 405
- **AND** the response body SHALL be a failed projection envelope with `error.code=method_not_allowed`.

#### Scenario: REST route registry matches handler behavior
- **GIVEN** routes are listed by `pinax api routes --json`
- **WHEN** contract tests exercise representative HTTP requests for each registered REST route
- **THEN** each route SHALL reach the handler for its projection command
- **AND** the response body SHALL be valid JSON using the Pinax projection envelope.

### Requirement: Pinax local RPC methods preserve registry contracts

Pinax SHALL keep registered RPC methods aligned with the RPC dispatcher and equivalent REST capability metadata.

#### Scenario: Every registered RPC method is dispatchable
- **GIVEN** RPC routes are registered in the remote route registry
- **WHEN** contract tests call each registered RPC method with representative fixture params
- **THEN** the dispatcher SHALL return a Pinax projection envelope
- **AND** it SHALL NOT return `rpc_method_not_found` for any registered method.

#### Scenario: Unknown RPC method returns stable error
- **WHEN** a local RPC client calls an unknown method
- **THEN** Pinax SHALL return a failed projection envelope with `error.code=rpc_method_not_found`
- **AND** the error hint SHALL direct the user to inspect `pinax api routes`.

#### Scenario: REST and RPC capability metadata stays aligned
- **GIVEN** a capability is exposed by both REST and RPC surfaces
- **WHEN** capabilities are listed
- **THEN** the REST and RPC routes SHALL point to the same projection command and response schema version
- **AND** any difference in readonly, body exposure, approval, or snapshot requirements SHALL be explicit in route or capability metadata.

### Requirement: Pinax remote write gates use explicit transport status without side effects

Pinax SHALL keep remote write-like REST and RPC operations behind dry-run, approval, and snapshot gates, and SHALL use explicit transport status while preserving failed projection details.

#### Scenario: Approval gate has failed projection and no writes
- **GIVEN** a project item exists in a fixture vault
- **WHEN** a REST client calls `POST /v1/project-items/{ref}:archive` without approval fields
- **THEN** Pinax SHALL return a non-2xx transport status with `error.code=approval_required`
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Snapshot gate has failed projection and no writes
- **GIVEN** a project item exists in a fixture vault and approval is supplied
- **WHEN** a REST or RPC client requests an archive or risky move without required snapshot evidence
- **THEN** Pinax SHALL return a non-2xx transport status with `error.code=snapshot_required`
- **AND** the projection SHALL include a runnable `pinax version snapshot` next action or hint
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Remote gate output is redacted
- **WHEN** REST or RPC gate responses include evidence, hint, actions, or diagnostics
- **THEN** stdout, stderr, response body, test fixtures, and integration evidence SHALL NOT include provider tokens, Authorization headers, cookies, webhook URLs, raw provider payloads, or hidden prompt content.

### Requirement: Pinax API serve lifecycle output is mode-safe

Pinax SHALL treat `pinax api serve --readonly` as a long-running local server command with explicit stdout and stderr behavior for each output mode.

#### Scenario: Serve requires readonly mode
- **WHEN** a user runs `pinax api serve --vault ./my-notes` without `--readonly`
- **THEN** Pinax SHALL return a failed projection with `error.code=readonly_required`
- **AND** it SHALL NOT start a server.

#### Scenario: Serve binds localhost and reports URL on stderr by default
- **WHEN** a user runs `pinax api serve --vault ./my-notes --readonly --port 0`
- **THEN** Pinax SHALL bind to `127.0.0.1`
- **AND** default human mode SHALL report the local URL on stderr
- **AND** stdout SHALL NOT contain logs, banners, or non-structured progress.

#### Scenario: Serve events mode emits lifecycle events
- **WHEN** a user runs `pinax api serve --vault ./my-notes --readonly --port 0 --events`
- **THEN** stdout SHALL be NDJSON events containing `start` and `ready`
- **AND** shutdown or startup failure SHALL emit `shutdown` or `error`
- **AND** diagnostics and logs SHALL remain on stderr.

#### Scenario: Serve machine modes do not mix logs into stdout
- **WHEN** a user runs `pinax api serve` with `--json` or `--agent`
- **THEN** Pinax SHALL either emit one startup projection and keep stdout otherwise quiet, or return a stable `unsupported_output_mode` failed projection
- **AND** it SHALL NOT mix local URL logs, diagnostics, or human prose into machine stdout.

### Requirement: Pinax SHALL support project-scoped subprojects as local workspaces

Pinax SHALL let a vault project contain subprojects that represent local workspaces for research, learning, content, planning, retrospectives, or tool-candidate workflows without creating a new Yeisme engineering project.

#### Scenario: Create a subproject workspace
- **GIVEN** a Pinax vault has project `research`
- **WHEN** the user runs `pinax project subproject create research stock-learning --title "Stock Learning" --template scenario --vault yeisme-notes --json`
- **THEN** Pinax SHALL create a subproject workspace through the application service
- **AND** it SHALL create or record the standard directories `00-charter`, `10-inbox`, `20-sources`, `30-runs`, `40-outputs`, `50-retros`, and `90-tool-candidates`
- **AND** stdout SHALL contain one projection envelope with command `project.subproject.create`, project, subproject, workspace path, created directory facts, and next actions.

#### Scenario: List and show subprojects
- **GIVEN** project `research` has subproject `stock-learning`
- **WHEN** the user runs `pinax project subproject list research --vault yeisme-notes --json` or `pinax project subproject show research stock-learning --vault yeisme-notes --json`
- **THEN** Pinax SHALL return bounded workspace facts without reading full note bodies
- **AND** it SHALL include charter path, directory presence, board configuration status, item counts when available, and safe next actions.

#### Scenario: Reject unsafe subproject paths
- **WHEN** a user attempts to create a subproject with an empty slug, path traversal, absolute path, or reserved directory target
- **THEN** Pinax SHALL fail with a stable machine-readable error code
- **AND** it SHALL NOT create Markdown files, `.pinax` structured assets, Git state, provider state, or remote state.

### Requirement: Project board SHALL support optional subproject scope

Pinax SHALL extend project board commands with an optional subproject scope while preserving existing project-wide board behavior.

#### Scenario: Show subproject board
- **GIVEN** project `research` has subproject `stock-learning` and managed project items
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault yeisme-notes --json`
- **THEN** stdout SHALL contain one projection envelope with command `project.board.show`
- **AND** facts SHALL include project, subproject, column counts, item counts, index status, warnings, and next actions
- **AND** returned items SHALL be scoped to `stock-learning`.

#### Scenario: Existing project-wide board remains compatible
- **GIVEN** existing scripts run `pinax project board show research --vault yeisme-notes --json`
- **WHEN** subproject support exists
- **THEN** the command SHALL keep returning the project-wide board unless `--subproject` is explicitly provided
- **AND** existing JSON fields and `--agent` keys SHALL remain compatible.

#### Scenario: Configure subproject board columns
- **WHEN** the user runs `pinax project board configure research --subproject stock-learning --columns inbox,next,doing,blocked,review,done --vault yeisme-notes --json`
- **THEN** Pinax SHALL write subproject-scoped board configuration through the project board service
- **AND** it SHALL NOT overwrite the project-wide board configuration.

### Requirement: Project items SHALL carry project management fields

Pinax SHALL support local project item metadata useful for project management while keeping Markdown and CLI-authored metadata as the source records.

#### Scenario: Add item with project management fields
- **WHEN** the user runs `pinax project item add research "跑第一次真实研究" --subproject stock-learning --column next --labels research,learning --milestone phase-1 --priority medium --vault yeisme-notes --json`
- **THEN** Pinax SHALL create a managed item through the application service
- **AND** the item SHALL include project, subproject, item id, title, column, status, labels, milestone, priority, optional due date, optional blockers, created time, updated time, and note reference facts.

#### Scenario: Move managed item between columns
- **GIVEN** a managed item exists in `next`
- **WHEN** the user runs `pinax project item move <item_id> doing --vault yeisme-notes --json`
- **THEN** Pinax SHALL update only managed item metadata
- **AND** the next board projection SHALL place the item in `doing`
- **AND** redacted event evidence SHALL be recorded.

#### Scenario: Refuse unmanaged checklist writes
- **GIVEN** an item was inferred from a Markdown checklist line not owned by Pinax
- **WHEN** the user runs `pinax project item move <inferred_item_id> done --vault yeisme-notes --json`
- **THEN** Pinax SHALL refuse the write with `project_item_unmanaged`
- **AND** it SHALL include a safe next action to create a managed item or edit the note manually.

### Requirement: Project workspace writes SHALL stay protected

Pinax SHALL keep subproject and board writes explicit, auditable, and recoverable.

#### Scenario: Archive requires approval
- **GIVEN** a managed item exists
- **WHEN** the user runs `pinax project item archive <item_id> --vault yeisme-notes --json`
- **THEN** Pinax SHALL fail with `approval_required`
- **AND** no Markdown file, `.pinax` asset, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Snapshot required for high-risk board write
- **GIVEN** a board operation would archive, batch-change, delete, or rewrite managed Markdown
- **WHEN** the user runs the operation with `--yes` and no recent snapshot evidence
- **THEN** Pinax SHALL fail with `snapshot_required`
- **AND** it SHALL include a runnable `pinax version snapshot --vault yeisme-notes --message "project workspace update"` next action.

### Requirement: Project Manager subprojects SHALL be vault-local and visibly annotated

Pinax SHALL treat Project Manager subprojects as vault-local workspace directories, not repository subprojects, independent Git repositories, runtime services, or `.pinax/**` metadata folders.

#### Scenario: Create subproject shows the vault-local target path

- **GIVEN** the active vault root is `~/data/yeisme-notes`
- **WHEN** the user creates or previews a Project Manager subproject such as `stock-learning`
- **THEN** Pinax SHALL expose a vault-relative `workspace_path` and a full path preview under `~/data/yeisme-notes/`
- **AND** the projection, dashboard, or OD SHALL label the target as a Markdown workspace directory rather than a Git repository or Yeisme code subproject.

#### Scenario: Registry path is explained separately from content path

- **WHEN** Pinax writes `.pinax/project-workspaces/<project>/<subproject>.json`
- **THEN** the UI and docs SHALL describe that file as CLI-authored registry metadata
- **AND** user-authored notes, project artifacts, task notes, and managed blocks SHALL be described as living under `workspace_path` inside the vault.

#### Scenario: Default subproject directories are semantic rather than numbered

- **WHEN** Pinax creates a new Project Manager subproject workspace
- **THEN** it SHALL create semantic default directories such as `charter`, `inbox`, `sources`, `runs`, `outputs`, `retros`, and `tool-candidates`
- **AND** it SHALL NOT create numeric-prefix defaults such as `00-charter`, `10-inbox`, `20-sources`, `30-runs`, `40-outputs`, `50-retros`, or `90-tool-candidates`
- **AND** existing numeric-prefix directories in older vaults SHALL remain readable user content rather than being deleted, renamed, or treated as the only supported structure.

#### Scenario: Project Manager copy avoids ambiguous subproject language

- **WHEN** Project Manager renders empty states, create forms, detail panels, or confirmation dialogs for subprojects
- **THEN** it SHALL include concise annotations for `Vault root`, `Workspace path`, and `Full path preview`
- **AND** it SHALL NOT imply that Pinax will create a monorepo subproject, Git submodule, independent remote repository, `AGENTS.md`, `CLAUDE.md`, or development toolchain bootstrap for this vault-local workspace.

### Requirement: Project boards SHALL use explicit task ownership

Pinax SHALL distinguish managed tasks, adopted checklist tasks, and inferred checklist tasks so board writes never mutate arbitrary Markdown checklist lines without explicit user approval.

#### Scenario: Managed task can move across columns

- **GIVEN** a project board contains a Pinax-managed task `item_123` in column `next`
- **WHEN** the user runs `pinax project item move item_123 doing --vault ./my-notes --json`
- **THEN** Pinax SHALL update only the managed task metadata or managed block through the application service
- **AND** the next board projection SHALL place the task in `doing`
- **AND** redacted event evidence SHALL be appended.

#### Scenario: Inferred checklist is readonly until adopted

- **GIVEN** Pinax inferred a board row from a user-authored Markdown checklist line that is not managed by Pinax
- **WHEN** the user runs `pinax project item move <inferred-id> done --vault ./my-notes --json`
- **THEN** Pinax SHALL refuse the write with `project_item_unmanaged` or `task_unmanaged`
- **AND** the projection SHALL include a safe next action such as `pinax task adopt <inferred-id> --plan --vault ./my-notes --json`.

#### Scenario: Task adoption is plan-gated

- **WHEN** the user runs `pinax task adopt <inferred-id> --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL return an adoption plan without modifying Markdown, `.pinax/**`, Git state, provider state, sync state, or remote services
- **AND** applying the adoption SHALL require an explicit command such as `pinax task adopt <inferred-id> --yes --vault ./my-notes --json`.

### Requirement: Project boards SHALL support saved task views

Pinax SHALL allow projects and subprojects to save reusable board views backed by filters and display options rather than saved result snapshots.

#### Scenario: Save board view

- **WHEN** the user runs `pinax project board view save research active --subproject stock-learning --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update a CLI-authored board view asset
- **AND** the view SHALL store source query, filters, columns, grouping, display options, and project/subproject scope
- **AND** it SHALL NOT store raw note bodies or a stale copy of result rows as the source of truth.

#### Scenario: Render board view from current facts

- **WHEN** the user runs `pinax project board view render research active --subproject stock-learning --vault ./my-notes --json`
- **THEN** Pinax SHALL compute current rows from the workspace, task, note, and index projections
- **AND** stdout SHALL include bounded cards, counts, warnings, index status, and next actions.

### Requirement: Daily review SHALL update only managed task blocks

Pinax SHALL support daily task review from project boards without rewriting arbitrary daily note content.

#### Scenario: Daily review writes a managed block only

- **GIVEN** today's daily note contains `<!-- pinax:managed name=daily-task-review -->` and `<!-- /pinax:managed -->`
- **WHEN** the user runs `pinax plan daily --tasks --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL update only that managed block with bounded task review facts
- **AND** all user-authored Markdown outside the block SHALL be preserved.

#### Scenario: Daily review refuses ambiguous write target

- **GIVEN** today's daily note does not contain a `daily-task-review` managed block
- **WHEN** the user runs `pinax plan daily --tasks --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL fail or return partial projection with stable error code `managed_block_missing`
- **AND** it SHALL NOT guess an insertion point or rewrite the note body.

### Requirement: Pinax can initialize long-term learning project workspaces
Pinax SHALL provide a local-first learning project initializer that composes project, workspace, board, templates, and starter items through application services.

#### Scenario: Initialize a stock learning project pack
- **WHEN** the user runs `pinax project learning init investing stock-learning --title "学习炒股的全部笔记" --project-name "学习炒股" --notes-prefix notes/investing --preset stock-learning --vault ./stock-learning-notes --json`
- **THEN** Pinax SHALL create or reuse project `investing`
- **AND** it SHALL create subproject workspace `stock-learning` with template `long-term-learning`
- **AND** it SHALL configure the learning board columns
- **AND** it SHALL create starter notes and starter project items through Pinax services
- **AND** stdout SHALL contain one JSON envelope with `command=project.learning.init`.

#### Scenario: Learning init dry-run is read-only
- **WHEN** the user runs `pinax project learning init investing stock-learning --preset stock-learning --vault ./stock-learning-notes --dry-run --json`
- **THEN** Pinax SHALL return planned operations
- **AND** it SHALL NOT write Markdown, `.pinax` assets, Git state, provider state, or remote services.

