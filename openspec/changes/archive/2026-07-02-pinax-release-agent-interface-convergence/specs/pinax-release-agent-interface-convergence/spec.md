# pinax-release-agent-interface-convergence Specification

## Purpose

定义 Pinax 可发布版本的 CLI-first agent 交互体验：核心需求功能必须落回 CLI，Local REST/RPC、MCP、Workbench 和 agent integrations 必须作为 CLI application service projection 的派生面运行，并保留 proof loop 的审批、快照、回执和恢复边界。

## ADDED Requirements

### Requirement: 发布版 SHALL lead with the CLI proof loop

Pinax 发布版 SHALL 把本地 Markdown vault proof loop 作为首要产品路径，并且 SHALL NOT 要求用户先配置 Cloud Sync、provider token、daemon、dashboard、MCP client、Workbench、plugin runtime 或 hosted service。

#### Scenario: 新用户运行五分钟 proof loop

- **GIVEN** 用户已安装 `pinax` 二进制并拥有一个空临时目录
- **WHEN** 用户运行 `pinax version`、`pinax init ./my-notes --title "My Knowledge Base"`、`pinax note add "First Note" --body "My first Pinax note." --vault ./my-notes`、`pinax proof loop run --vault ./my-notes --json`
- **THEN** 每个机器输出命令 SHALL 返回稳定 projection envelope
- **AND** proof loop preview SHALL 包含 `proof_loop_run_id`、stage facts、bounded next action
- **AND** 该路径 SHALL NOT 需要 Cloud Sync、provider credentials、MCP、dashboard、daemon 或源码 checkout。

#### Scenario: README and quickstart present proof loop first

- **WHEN** 用户阅读 README、中文 README 或 `docs/quickstart.md`
- **THEN** 第一条完整路径 SHALL 展示 capture、retrieve、diagnose、plan、snapshot、apply、restore
- **AND** Cloud Sync、publish、plugin runtime、provider-backed synthesis、realtime daemon 和 Workbench SHALL 出现在高级、预览或未来路径中，而不是作为首发必需步骤。

### Requirement: Release core capabilities SHALL be discoverable from one registry

Pinax SHALL publish release core capabilities from a shared capability registry. CLI docs、Local API discovery、OpenAPI export、Remote API Mode、Workbench capability explorer 和 agent handoff 文档 SHALL NOT 维护互相冲突的能力表。

#### Scenario: API route discovery exposes release core metadata

- **GIVEN** 一个有效 Pinax vault
- **WHEN** 用户运行 `pinax api routes --vault ./my-notes --json`
- **THEN** release core capabilities SHALL include release_core, command, capability_id, readonly, body_allowed, approval_required, snapshot_required, errors, copy_command, and local_only_reason when applicable
- **AND** every proof-loop scenario (vault bootstrap, capture, retrieve, diagnose, plan, apply safely, discover) SHALL be represented by at least one `release_core=true` capability
- **AND** CLI-local proof-loop capabilities (no REST/RPC route) SHALL carry a `local_only_reason` so agents know they are CLI-gated
- **AND** planned or future capabilities SHALL remain discoverable metadata only when no route exists.

#### Scenario: OpenAPI export is derived from real REST routes

- **GIVEN** route registry contains release core REST routes and planned local-only capabilities
- **WHEN** 用户运行 `pinax api schema export --format openapi --vault ./my-notes --json`
- **THEN** exported paths SHALL include only implemented REST paths from the registry
- **AND** each exported operation SHALL include `x-pinax-command`、`x-pinax-capability`、`x-pinax-release-core`、`x-pinax-readonly`、`x-pinax-body-allowed`、`x-pinax-approval-required`、`x-pinax-snapshot-required`
- **AND** OpenAPI export SHALL NOT fabricate HTTP paths for planned MCP-only, local-only, future-owner, or future-contract capabilities.

### Requirement: CLI SHALL remain the stable source for core user and agent workflows

Every release core workflow SHALL have a CLI entry point, shared projection envelope, machine output mode, docs example, and test evidence before being treated as release-ready.

#### Scenario: Core commands expose JSON and agent output

- **WHEN** release core commands such as `vault validate`、`note add`、`search`、`memory context`、`vault doctor`、`repair plan --save`、`version snapshot`、`repair apply --yes`、`version restore --plan`、`api routes` run with `--json` or `--agent`
- **THEN** stdout SHALL contain only the selected machine format
- **AND** diagnostics SHALL go to stderr
- **AND** output SHALL NOT leak raw tokens, Authorization headers, cookies, provider payloads, hidden system prompts, private tool arguments, full chain-of-thought, or unapproved full note bodies.

#### Scenario: Unsupported release capability cannot be documented as available

- **GIVEN** a capability is not implemented in CLI application services
- **WHEN** docs, route discovery, MCP tools, or Workbench metadata mention it
- **THEN** the capability SHALL be marked with a planned, preview, experimental, local-only, or future-owner status
- **AND** docs SHALL NOT provide it as a current runnable success path.

### Requirement: Local REST/RPC SHALL be a projection adapter

Pinax Local REST/RPC SHALL expose existing application service projections and SHALL NOT become a separate persistence, business logic, or hosted cloud model.

#### Scenario: REST and RPC route through application services

- **GIVEN** `pinax api serve --vault ./my-notes --readonly --port 8787` is running
- **WHEN** a client calls a registered read route or RPC method
- **THEN** the handler SHALL route through `internal/app` service behavior
- **AND** the response SHALL be a Pinax projection envelope compatible with CLI JSON output
- **AND** the handler SHALL NOT directly read or write Markdown、`.pinax/**`、SQLite/GORM repositories、Git、provider state、remote services or raw note files.

#### Scenario: Readonly API rejects writes

- **GIVEN** `pinax api serve --vault ./my-notes --readonly --port 8787` is running
- **WHEN** a client calls a write-capable route with `yes=true`
- **THEN** the server SHALL return a failed projection with `error.code=write_disabled`
- **AND** no Markdown file、`.pinax/**` asset、Git state、provider state or remote service SHALL be modified.

#### Scenario: Allow-write API still requires approval and snapshot gates

- **GIVEN** `pinax api serve --vault ./my-notes --allow-write --port 8787` is running
- **WHEN** a client calls a write-capable route without `yes=true`
- **THEN** the server SHALL return `approval_required`
- **AND** when a write requires version protection and no valid snapshot evidence exists, it SHALL return `snapshot_required`
- **AND** the response SHALL include a runnable CLI next action when useful.

#### Scenario: Remote API Mode does not fallback to local execution

- **GIVEN** remote mode is enabled with `--api-url`, `PINAX_API_URL`, or user config `remote.api_url`
- **WHEN** the user runs an unsupported non-control command
- **THEN** Pinax SHALL return `remote_command_unsupported`
- **AND** it SHALL NOT silently execute the command against a local vault.

### Requirement: MCP SHALL be read-only and bounded in the release path

Pinax release MCP SHALL expose local stdio tools and resources for bounded agent read and plan-preview workflows. It SHALL NOT directly write vault state in the release path.

#### Scenario: MCP lists only bounded read and plan-preview tools

- **GIVEN** `pinax mcp serve --vault ./my-notes` is running
- **WHEN** an MCP client calls `tools/list` or `resources/list`
- **THEN** Pinax SHALL advertise only readonly release tools/resources and plan-preview tools
- **AND** it SHALL NOT advertise direct Markdown, `.pinax/**`, Git, provider, Cloud Sync, or remote write tools.

#### Scenario: MCP read tools do not expose full bodies by default

- **GIVEN** an MCP client calls a note, search, memory, KB, graph, brain context, brain answer, or sources tool
- **WHEN** the request does not explicitly allow full body exposure
- **THEN** Pinax SHALL return bounded projections with evidence refs, snippets, ranking facts, freshness/risk facts, and next actions
- **AND** it SHALL NOT return full private note bodies by default.

#### Scenario: MCP write attempts are rejected or converted to next command

- **GIVEN** an MCP client requests a vault mutation
- **WHEN** the release MCP surface handles the request
- **THEN** Pinax SHALL reject the tool call or return a plan-only projection with a CLI next command
- **AND** it SHALL NOT modify Markdown、`.pinax/**`、Git、provider state or remote services.

### Requirement: Agent write workflows SHALL preserve proof-loop gates

Pinax SHALL require agent-originated write workflows to pass through plan, approval, snapshot when required, apply receipt, and restore evidence.

#### Scenario: Agent repair apply is protected

- **GIVEN** an agent has generated or selected a repair plan
- **WHEN** the user runs `pinax repair apply --vault ./my-notes --plan repair-abc123 --yes --json`
- **THEN** Pinax SHALL apply only approved low-risk operations through application services
- **AND** it SHALL write receipt evidence linked to the plan and snapshot when required
- **AND** manual-review items SHALL NOT be silently deleted, merged, rewritten, or applied.

#### Scenario: Agent restore path is CLI-authored

- **GIVEN** an apply changed a local Markdown file
- **WHEN** the user runs `pinax version restore notes/example.md --revision HEAD --plan --vault ./my-notes --json` and then `pinax version restore apply --vault ./my-notes --plan restore-abc123 --yes --json`
- **THEN** Pinax SHALL restore through the CLI/application service path
- **AND** output SHALL report local write facts and remote write facts
- **AND** stale restore plans SHALL be rejected with a stable error code and next action.

### Requirement: Release evidence SHALL be project-owned and redacted

Pinax release integration, component, system, and e2e tests SHALL write redacted evidence under the Pinax subproject temp directory and preserve evidence on failure.

#### Scenario: Release integration run writes required evidence files

- **WHEN** maintainers run the release core integration/e2e command
- **THEN** Pinax SHALL write evidence under `temp/integration-test-runs/<run-id>/`
- **AND** the run directory SHALL contain `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`
- **AND** the command SHALL preserve evidence even when the original test exits non-zero.

#### Scenario: Release evidence is redacted

- **WHEN** release evidence, fixtures, stdout, stderr, event logs, receipts, screenshots, or golden files are generated
- **THEN** they SHALL NOT contain raw tokens, Authorization headers, cookies, webhook URLs, provider payloads, hidden system prompts, private tool arguments, full chain-of-thought, or unapproved full note bodies
- **AND** diagnostics SHALL show only source type, env var names, local config paths, keychain references, configured status, redacted digests, or evidence references.

### Requirement: Release gate SHALL validate docs, contracts, tests, build, and OpenSpec

Pinax release convergence SHALL NOT be considered complete until project-level validation proves the CLI-first agent path and derived surfaces work together.

#### Scenario: Running the release quality gate

- **GIVEN** implementation tasks for this change are complete
- **WHEN** maintainers run `task check` from `cli/pinax`
- **THEN** formatting, lint, tests, build, sidecar protocol checks, and `openspec validate --all` SHALL pass
- **AND** failures SHALL be fixed in the owning lane rather than documented around.

#### Scenario: Running OpenSpec strict validation

- **WHEN** maintainers run `openspec validate pinax-release-agent-interface-convergence --strict`
- **THEN** this change SHALL validate without missing proposal, design, tasks, spec scenarios, or malformed requirement structure.
