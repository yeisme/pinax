# pinax-web-client-contracts Delta Spec

## ADDED Requirements

### Requirement: Web 工作台只能消费 Pinax bounded projection

Pinax SHALL expose Web/workbench-facing state through Local REST/RPC, CLI JSON, MCP/dashboard shared projections, or copyable real `pinax ...` commands, and SHALL NOT require a Web client to read `.pinax/**`, SQLite, LanceDB, token files, provider config, sync state, or other structured assets directly.

#### Scenario: Web client discovers workbench capabilities

- **WHEN** a client runs `pinax api routes --vault ./my-notes --json`
- **THEN** the projection SHALL identify registered capabilities relevant to workbench screens
- **AND** each capability SHALL expose enough bounded metadata for a client to understand readonly/write mode, body exposure default, required approval, required snapshot, and local-only status when applicable.

#### Scenario: Web client does not parse private local assets

- **GIVEN** a vault contains `.pinax/`, SQLite indexes, KB projection files, token files, provider config, and sync state
- **WHEN** the Web design needs vault, index, provider, graph, board, editor, search, or proof state
- **THEN** the documented data source SHALL be a Pinax command, REST/RPC capability, or application service projection
- **AND** the design SHALL NOT instruct clients to read or write those local structured assets directly.

### Requirement: Settings 设置中心暴露配置来源和受控写入

Pinax SHALL support a Settings/control projection that lets a future Web client display configuration source, effective value, writable scope, validation state, secret-reference boundary, Cloud Sync diagnostics, Publish diagnostics, and danger-zone readiness without directly editing local config, sync state, profile files, token files, publish receipts, or other structured assets.

#### Scenario: Settings page discovers config source and save scope

- **WHEN** a client displays Settings for a vault
- **THEN** Pinax SHALL expose bounded facts for each supported setting, including effective value, source such as `user`、`project`、`env`、`flag` or `default`, writable status, allowed save scopes, validation result, and safe next action when applicable
- **AND** writes SHALL go through Pinax commands or application services such as `pinax config set output.theme high-contrast --scope user`
- **AND** the Settings contract SHALL NOT require clients to edit user config, project config, profile files, token files, `.pinax/**`, receipts, or sync metadata directly.

#### Scenario: Appearance settings use existing config keys

- **WHEN** a client renders theme and appearance controls
- **THEN** Pinax SHALL map CLI/output appearance to existing config keys such as `output.theme`、`output.color`、`output.markdown.style` and `themes.custom.*`
- **AND** documented examples SHALL use real commands such as `pinax config get output.theme --vault ./my-notes --json` and `pinax config set themes.custom.accent cyan --scope user`
- **AND** Web-only visual preferences MAY remain client-local until Pinax defines typed config keys for them.

#### Scenario: Keymap settings do not invent unsupported CLI commands

- **WHEN** a client displays keyboard shortcut settings
- **THEN** Pinax SHALL distinguish Web-client keymap preferences from Pinax CLI configuration
- **AND** the first Settings contract SHALL only rely on existing CLI config such as `pinax config get editor.command --vault ./my-notes --json` and `pinax config set editor.command "code --wait" --scope user`
- **AND** documentation and UI command previews SHALL NOT present an unsupported `pinax keymap` command.

#### Scenario: Cloud Sync settings distinguish local API from distributed sync

- **WHEN** a client displays Cloud Sync settings
- **THEN** Pinax SHALL show backend, workspace, device, secret reference status, doctor result, diff readiness, daemon status, recent redacted logs, conflict state, and dangerous action gates through `pinax cloud ...` and `pinax sync ...` capabilities
- **AND** safe examples SHALL include `pinax cloud status --vault ./my-notes --json`, `pinax cloud doctor --vault ./my-notes --json`, `pinax sync diff --target cloud --vault ./my-notes --json`, and `pinax sync push --target cloud --vault ./my-notes --dry-run --json`
- **AND** daemon run, push, pull, backend changes, and conflict resolution SHALL require explicit confirmation and SHALL NOT expose raw sync payloads, tokens, Authorization headers, provider stderr, or full note bodies.

#### Scenario: Publish settings require plan/build validation before deploy

- **WHEN** a client displays Publish settings
- **THEN** Pinax SHALL expose publish profiles, target, renderer, theme, validation result, secret scan status, latest receipt, plan/build/serve/deploy readiness, and safe next action
- **AND** safe examples SHALL include `pinax publish profile validate public --vault ./my-notes --json`, `pinax publish theme list --vault ./my-notes --json`, `pinax publish plan --profile public --target github-pages --vault ./my-notes --json`, and `pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json`
- **AND** deploy SHALL remain a danger-zone action that requires `--yes`, latest valid receipt, safe output path, target repository/path validation, and redaction of secrets, provider payloads, private body content, and local absolute paths where possible.

### Requirement: 右侧 Agent 侧栏使用可审查的上下文和动作合同

Pinax SHALL support a right-side Agent interaction model based on bounded context, provider status, registered capability preview, reviewable plan/diff, snapshot requirement, receipt, and restore hint, rather than arbitrary command execution.

#### Scenario: Agent context is bounded by default

- **WHEN** a client asks for context for a note, search result, project card, graph entity, canvas object reference, or editor selection
- **THEN** Pinax SHALL return bounded facts such as title, path, tags, heading path, snippets, refs, evidence, status, confidence, and actions
- **AND** it SHALL NOT return a full note body unless the request explicitly asks for body exposure.

#### Scenario: Agent plan cannot apply silently

- **GIVEN** a proposed Agent action would write Markdown, `.pinax/**`, index projection, provider state, sync state, or version state
- **WHEN** the request omits explicit approval or required snapshot evidence
- **THEN** Pinax SHALL return a plan-only projection or a stable gate error such as `approval_required`, `write_disabled`, or `snapshot_required`
- **AND** it SHALL include a safe next action when one is available.

#### Scenario: Agent command preview uses real user commands

- **WHEN** the Agent side panel displays a command preview
- **THEN** the command SHALL be a real command a user can run directly, such as `pinax index refresh --vault ./my-notes --json`
- **AND** the preview SHALL NOT expose internal execution prefixes, shell wrappers, raw tool arguments, or agent runtime-only commands.

### Requirement: BYOK 和 local provider 状态不得泄密

Pinax SHALL expose BYOK/local provider status through provider list and provider doctor projections that show configuration status and credential source type without revealing credential values.

#### Scenario: Provider list is safe for UI display

- **WHEN** a client runs `pinax kb provider list --vault ./my-notes --json`
- **THEN** the projection SHALL include provider names, default models, configured status, local-only status, and credential source type
- **AND** it SHALL NOT include provider key values, bearer tokens, cookies, raw provider payloads, or raw prompts.

#### Scenario: Missing provider credentials produce doctor next action

- **WHEN** `pinax kb provider doctor openai --vault ./my-notes --json` detects missing credentials
- **THEN** Pinax SHALL return a stable provider-not-configured result or error
- **AND** the next action SHALL point to a real diagnostic command such as `pinax kb provider doctor openai --vault ./my-notes --json`
- **AND** it SHALL NOT tell a Web client to collect or persist the raw key in a Web form.

#### Scenario: Local model provider remains visibly local

- **WHEN** a client inspects `ollama` provider status
- **THEN** Pinax SHALL mark the provider as local-only or local-service-backed when applicable
- **AND** the projection SHALL expose reachability and model defaults without requiring a token.

### Requirement: Pinax Editor 使用显式 body exposure 和 proof gate

Pinax SHALL support Pinax Editor through source-first Markdown projections that separate preview/detail reading, explicit body exposure, diff review, managed block refresh, attachment handling, snapshot creation, and apply receipts.

#### Scenario: Editor preview does not expose full body by default

- **WHEN** a client runs `pinax note read "Research Log" --display card --vault ./my-notes --json`
- **THEN** Pinax SHALL return bounded note metadata suitable for editor preview
- **AND** it SHALL NOT include the full note body.

#### Scenario: Editor source mode explicitly requests body

- **WHEN** a client runs `pinax note read "Research Log" --display body --vault ./my-notes --json`
- **THEN** Pinax MAY return the note body for explicit source editing
- **AND** the projection SHALL make the body exposure state visible to callers.

#### Scenario: Managed block refresh writes only managed content

- **WHEN** a client runs `pinax note refresh "Research Log" --rendered --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL refresh only the recognized managed block range
- **AND** it SHALL preserve user-authored Markdown outside the managed block.

#### Scenario: Editor-assisted write has snapshot and receipt path

- **GIVEN** an editor-assisted rewrite or replace would modify Markdown content
- **WHEN** the client asks to apply it
- **THEN** Pinax SHALL require the same approval, snapshot, diff, receipt, and restore boundaries as the equivalent CLI workflow
- **AND** it SHALL NOT let an Agent plan bypass the proof gate.

### Requirement: Kanban、图谱、搜索和画布视图复用同一投影真源

Pinax SHALL treat Kanban, knowledge graph, search, and infinite canvas as views over Markdown vault and Pinax projections, not as separate sources of truth.

#### Scenario: Kanban card operations use project service

- **WHEN** a client shows or moves a Kanban item using project board capabilities
- **THEN** Pinax SHALL derive columns and cards from project/work item projections
- **AND** move, add, archive, and batch operations SHALL go through application service gates rather than client-side file edits.

#### Scenario: Knowledge graph starts from an entity and evidence

- **WHEN** a client explores a graph entity
- **THEN** Pinax SHALL return bounded nodes, edges, relationship evidence, confidence or link status when available
- **AND** it SHALL NOT require loading or rendering the full graph by default.

#### Scenario: Search result snippets are bounded

- **WHEN** a client searches notes through `pinax search "authentication" --vault ./my-notes --json`
- **THEN** Pinax SHALL return grouped results, snippets, paths, scores, filters, and index status suitable for a search sidebar
- **AND** snippets SHALL NOT bypass body exposure or include `.pinax/**` and `.git/**` internals.

#### Scenario: Raw text fallback is service managed

- **WHEN** `rg` is used as a fallback for exact raw text scan
- **THEN** it SHALL be invoked or represented through a Pinax application service or diagnostic path
- **AND** the Web client SHALL NOT scan the vault directly from the browser.

#### Scenario: Canvas layout metadata is service-owned

- **WHEN** a future canvas feature stores note cards, search result groups, graph entities, project items, evidence snippets, frames, connectors, or annotations
- **THEN** Pinax SHALL store only object references, layout metadata, viewport state, and bounded annotations through service-owned structured assets
- **AND** it SHALL NOT persist full note bodies, search result full text, raw provider payloads, or raw prompts in canvas layout data.

### Requirement: Future client implementation ownership remains separate

Pinax SHALL document future Web/desktop client implementation as a separate owning subproject while `cli/pinax` remains the owner of CLI/API/projection contracts.

#### Scenario: OpenSpec handoff does not place client source in CLI project

- **WHEN** the Web/Open Design handoff is complete
- **THEN** the handoff SHALL state that future React/Web/desktop client source belongs in an independent client subproject
- **AND** `cli/pinax` SHALL remain responsible for stable commands, Local REST/RPC, projections, permission gates, and redaction contracts.

#### Scenario: Client implementation consumes capability matrix

- **WHEN** a future client project begins implementation
- **THEN** it SHALL consume `pinax api routes --vault <vault> --json`, OpenAPI export, and documented Pinax projections
- **AND** it SHALL NOT invent a parallel persistence model for notes, boards, graph, search, canvas, provider state, or proof receipts.
