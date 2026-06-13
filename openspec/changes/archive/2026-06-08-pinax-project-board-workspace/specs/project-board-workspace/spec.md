# project-board-workspace Specification

## Purpose

Pinax project board workspace SHALL provide a local-first project board projection and controlled project item workflow while preserving Markdown vault ownership, CLI-authored structured assets, output contract stability, and TaskBridge execution boundaries.

## ADDED Requirements

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
- **THEN** stdout SHALL contain one JSON envelope with `command=note.read`, `facts.display=card`, and bounded note identity facts
- **AND** `data.note` SHALL include title, path, note id, project, kind, status, tags, updated time, excerpt, and board column when available
- **AND** it SHALL NOT include full note body.

#### Scenario: Read note detail with context
- **GIVEN** a Pinax-managed note has links, backlinks, attachments, and related project notes
- **WHEN** the user runs `pinax note read note_123 --display detail --with-context --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with relationship counts, selected properties, bounded related note cards, evidence refs, and next actions
- **AND** related notes SHALL be bounded by default limits and SHALL NOT include full note bodies.

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

Pinax SHALL persist project board configuration through CLI commands or application services rather than requiring agents to hand-write metadata.

#### Scenario: Configuring board columns
- **GIVEN** a project `research` exists
- **WHEN** the user runs `pinax project board configure research --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json`
- **THEN** Pinax SHALL write `.pinax/project-boards/research.json` through the project board service
- **AND** the asset SHALL include `schema_version=pinax.project_board.v1`, project slug, columns, updated time, and redacted event evidence
- **AND** stdout SHALL contain one JSON envelope with the saved configuration facts.

#### Scenario: Validating board assets
- **GIVEN** project board configuration or saved board snapshots exist
- **WHEN** the user runs `pinax validate --vault ./my-notes --json`
- **THEN** Pinax SHALL validate schema versions, required fields, enum values, path boundaries, project references, note references, and redaction rules
- **AND** invalid assets SHALL return stable machine-readable error codes.

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
- **AND** the projection SHALL include a runnable `pinax git snapshot --vault ./my-notes --message "看板更新前快照"` next action.

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
- **AND** schema output SHALL NOT be hand-maintained separately from handler behavior without contract tests catching drift.

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
