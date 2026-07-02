# 任务

## 0. 全局约束

- Owner: `cli/pinax`。未来 Web/桌面客户端源码必须由独立客户端子项目拥有，本变更只交付 Pinax 侧 CLI/API/projection 合同、测试和文档。
- 合同策略: 只做 additive change；不得删除、重命名或重定义现有命令、flag、JSON envelope 顶层字段、`--agent` key、API route、RPC method、`.pinax/**` registry key 或 index schema。
- 写入边界: Markdown note body 可由用户显式编辑；`.pinax/**` structured assets、SQLite/GORM projection、LanceDB projection、events、receipts、sync state、provider config、token/profile metadata 必须由 CLI/application service 写入。
- 脱敏边界: 新增输出、日志、测试 fixture 和运行证据不得包含真实 token、Authorization header、cookie、provider key、raw provider payload、raw prompt、hidden system prompt、private tool arguments、完整 note body 或完整 chain-of-thought。
- 文档语言: 人类文档、OpenSpec proposal/design/tasks/spec 使用中文；命令名、flag、JSON key、provider id、model id、route id 和 code identifiers 保持英文或既有稳定名称。
- 集成证据: 新增或扩展 integration/component/e2e 入口时，证据写入 `temp/integration-test-runs/<run-id>/`，至少包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`。
- 完成门禁: 每个阶段运行 focused tests；收口运行 `openspec validate pinax-web-open-design-client-contracts --strict`、`openspec validate --all --strict`；触及 Go 代码后运行 `task check` 或记录现有无关失败和 focused tests 证据。

## P0: Open Design 合同基线

- [x] **0.1 审计 Web 开放设计与现有 capability 覆盖**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: none
  - Scope: 对齐 `docs/product/web-open-design.md`、已归档的 `pinax-client-cli-parity-realtime-sync`、已归档的 `pinax-unified-vault-workspace-database`、已归档的 `pinax-kb-provider-expansion` 和当前 `pinax api routes`，列出 Web 工作台需要的 capability 是否已存在。
  - Files: `docs/product/web-open-design.md`、`openspec/changes/archive/2026-06-26-pinax-client-cli-parity-realtime-sync/*`、`openspec/changes/archive/2026-06-26-pinax-unified-vault-workspace-database/*`、`openspec/changes/archive/2026-06-24-pinax-kb-provider-expansion/*`、`openspec/specs/personal-kb/spec.md`、`internal/app/remote.go`、`internal/api/http.go`、`internal/api/rpc.go`。
  - Acceptance: 形成 capability gap matrix，至少覆盖 `workbench.status`、`agent.context`、`provider.status`、`editor.note`、`board.view`、`graph.view`、`search.view`、`canvas.view`、`proof.gate`；每个 gap 标记为 `implemented`、`covered-by-active-change`、`new-task` 或 `future-client-only`。
  - Validation command: `openspec list && openspec validate pinax-web-open-design-client-contracts --strict && openspec validate --all --strict`
  - Expected result: OpenSpec validation 通过；如 active change 尚未完成，gap matrix 明确依赖，不重复实现同一合同。
  - Failure re-check: 如果现有 active change validation 失败，先记录阻塞项和 owner，不在本变更中改写无关 spec。
  - Evidence: 2026-06-26 审计 `docs/product/web-open-design.md`、`internal/app/remote.go`、`openspec/specs/pinax-cli-remote-api-mode/spec.md`、`openspec/specs/project-board-workspace/spec.md`、`openspec/specs/personal-kb/spec.md`、`openspec/specs/notebook-index-search/spec.md` 和已归档 changes，已在 `design.md` 增加 capability gap matrix，覆盖 `workbench.status`、`agent.context`、`provider.status`、`editor.note`、`board.view`、`graph.view`、`search.view`、`canvas.view`、`proof.gate`。当前矩阵无未纳入计划的 `new-task`；`canvas.view` 标记为 `future-client-only`，其余缺口由本 active change 后续任务覆盖。

- [x] **0.2 扩展 API discovery 的 Web-facing 分类**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 0.1
  - Scope: 在 capability registry 或 route projection 中新增 optional Web-facing metadata，例如 `ui_group`、`body_exposure_default`、`write_gate`、`copy_command`、`local_only_reason`，用于未来 Web 工作台生成状态栏、右侧 Agent 和命令预览。
  - Files: `internal/app/remote.go`、`internal/api/http.go`、`internal/api/rpc.go`、`internal/domain/types.go`、`cmd/pinax/api_command_test.go`、`docs/commands/api.md`。
  - Acceptance: `pinax api routes --vault ./my-notes --json` 可列出 capability 所属 UI 分组和安全门禁；旧字段保持不变；`pinax api schema export --format openapi --vault ./my-notes --json` 不泄露 vault path、token 或 provider payload。
  - Validation command: `go test ./internal/app ./internal/api ./cmd/pinax -run 'APIRoutes|Capability|OpenAPI|Remote|Web' -count=1`
  - Expected result: API discovery focused tests 通过；`--json` 仍是单个 projection envelope。
  - Failure re-check: 如果 OpenAPI schema 不适合承载 UI metadata，保留 schema 兼容，只在 `api routes` data 中新增 optional metadata。
  - Evidence: 2026-06-26 按 TDD 在 `internal/app/remote_test.go` 先补 RED 断言，要求 RemoteCapabilities/RemoteRoutes 暴露 optional `ui_group`、`body_exposure_default`、`write_gate`、`copy_command`，且 OpenAPI REST operation extension 同步 `x-pinax-ui-group`、`x-pinax-body-exposure`、`x-pinax-write-gate`。首轮 `go test ./internal/app -run 'Remote|Capability|OpenAPI|Web' -count=1` 因字段不存在失败；随后在 `domain.RemoteCapability`/`RemoteRoute` 中新增 optional 字段，通过 `decorateRemoteCapabilities` 填充 Web-facing metadata，并在 OpenAPI export 中输出 extension。重跑 `go test ./internal/app -run 'Remote|Capability|OpenAPI|Web' -count=1` 通过；运行 `go test ./internal/app ./internal/api ./cmd/pinax -run 'APIRoutes|Capability|OpenAPI|Remote|Web' -count=1`，退出码 0。`docs/commands/api.md` 已说明新增 optional metadata 字段。

## P1: 工作台 Shell 和状态栏合同

- [x] **1.1 新增 workbench status projection**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 0.2
  - Scope: 为未来 Web 顶栏/状态栏提供一个 read-only projection，聚合 vault、index freshness、sync status、write mode、body exposure 默认值、remote API readonly/allow-write、profile/token 状态和推荐 next actions。
  - Files: `internal/app/service.go`、`internal/app/remote.go`、`internal/api/rpc.go`、`internal/api/http.go`、`internal/output/render.go`、`cmd/pinax/api_command_test.go`、`docs/commands/api.md`。
  - Acceptance: `pinax api routes --vault ./my-notes --json` 或新增等价 status capability 能让客户端发现工作台状态；index missing/stale 返回 `pinax index refresh --vault ./my-notes --json` next action；readonly server 明确显示 writes disabled。
  - Validation command: `go test ./internal/app ./internal/api ./internal/output ./cmd/pinax -run 'Workbench|Status|Index|Readonly|Profile|Token|Remote' -count=1`
  - Expected result: workbench status JSON/agent/human 输出均不包含 note body、token 值或本机 secret。
  - Failure re-check: 如果某些 sync/provider 状态已有独立命令，只聚合 bounded facts 和 action，不重复读取内部文件。
  - Evidence: 2026-06-26 按 TDD 新增 `TestAPIWorkbenchStatusCLI`，首轮 `go test ./cmd/pinax -run TestAPIWorkbenchStatusCLI -count=1` 因缺 `api status` 命令失败；随后新增 `workbench.status` capability、REST route `/v1/workbench/status`、RPC method `Pinax.Workbench.Status`、`pinax api status` CLI 和共享 `WorkbenchStatus` projection。projection 输出 bounded facts：`vault_root`、`index_status`、`write_mode`、`body_exposure_default`、`profile_status`、`token_status`，index missing/stale 时返回 `pinax index refresh --vault <vault> --json` next action。重跑 `go test ./cmd/pinax -run TestAPIWorkbenchStatusCLI -count=1` 通过；运行 `go test ./internal/app ./internal/api ./internal/output ./cmd/pinax -run 'Workbench|Status|Index|Readonly|Profile|Token|Remote' -count=1`，退出码 0。`docs/commands/api.md` 已新增 `pinax api status --vault ./my-notes --json` 说明。

- [x] **1.2 固化 Web Open Design 文档与 OpenSpec 双向链接**
  - Owner: `cli/pinax`
  - Lane: docs
  - Depends on: 0.1
  - Scope: 让 `docs/product/web-open-design.md` 链接本 OpenSpec change，并在 `docs/README.md`、`docs/README.zh-CN.md`、`docs/product/mvp-scope.md` 中说明 Web/Open Design 是未来客户端合同，不是当前 CLI 已实现 UI。
  - Files: `docs/product/web-open-design.md`、`docs/README.md`、`docs/README.zh-CN.md`、`docs/product/mvp-scope.md`。
  - Acceptance: 文档入口可从 docs README 到 Web 设计，再到 OpenSpec change；所有面向人类的说明为中文；命令示例是真实 `pinax` 命令。
  - Validation command: `rg -n 'pinax-web-open-design-client-contracts|Pinax Web 开放设计|pinax api routes' docs openspec/changes/pinax-web-open-design-client-contracts`
  - Expected result: 搜索命中 docs 和 OpenSpec 双向引用；无英文标题回退为主要文档标题。
  - Failure re-check: 如果 README 链接重复，保留一个主入口，不在根仓库复制 Pinax 长文档。
  - Evidence: 2026-06-26 更新 `docs/README.md`、`docs/README.zh-CN.md` 和 `docs/product/mvp-scope.md`，说明 Pinax Web 开放设计是未来独立客户端合同，不表示当前 CLI 已包含 Web UI，并引用 OpenSpec `pinax-web-open-design-client-contracts`。运行 `rg -n 'pinax-web-open-design-client-contracts|Pinax Web 开放设计|pinax api routes' docs openspec/changes/pinax-web-open-design-client-contracts`，命中 docs README、MVP、Web 设计、OpenSpec proposal/design/tasks/spec 和 API/remote docs。

- [x] **1.3 定义 Settings/control capability 和配置来源投影**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.1
  - Scope: 为 Settings 页面提供配置来源、有效值、可写 scope、secret reference 边界和 Advanced diagnostics 投影；首版覆盖 `config path/get/doctor/set/unset`、Local API/profile/token 状态、write mode 和 redaction status。
  - Files: `internal/config/config.go`、`internal/cli/config_cmd.go`、`internal/app/remote.go`、`internal/api/rpc.go`、`internal/output/render.go`、`cmd/pinax/config_command_test.go`、`docs/commands/config.md`、`docs/product/web-open-design.md`。
  - Acceptance: Settings 能显示每个设置的 `source=user|project|env|flag|default`、是否可写、保存后写入的 scope 和 next action；`pinax config get output.theme --vault ./my-notes --json`、`pinax config doctor --vault ./my-notes` 和 `pinax config set output.theme high-contrast --scope user` 行为保持稳定；secret-like key/value 继续被拒绝。
  - Validation command: `go test ./internal/config ./internal/cli ./internal/output ./cmd/pinax -run 'Config|Settings|Source|Theme|Secret|Redaction' -count=1`
  - Expected result: config/settings focused tests 通过；输出不包含 token、cookie、Authorization header 或 raw external CLI config。
  - Failure re-check: 如果现有 config loader 不暴露足够 source details，只新增 optional projection data，不改变 `config get` 旧 facts。
  - Evidence: 2026-06-26 按 TDD 扩展 `internal/cli/config_cmd_test.go`，先让 `go test ./internal/cli -run 'Config.*Settings|ConfigGet' -count=1` 因缺少 `fact.source` 和 `data.settings` 失败；随后在 `internal/config.LoadResult` 中新增 additive `settings` 投影，保留 `config get` 原有 `key/value` facts，同时补充 `source`、`writable`、`write_scope`、`write_scopes` 和安全 next action。`config doctor --json` 现在输出 `data.settings` 以及 bounded diagnostics：`local_api_status`、`remote_api_source`、`write_mode`、`redaction_status`、`profile_status`、`token_status`、`body_exposure_default`；`RemoteCapabilities` 增加 `config.path/get/doctor/set/unset` 的 `settings.control` CLI/dashboard capability，不声明 REST/RPC route。重跑 `go test ./internal/cli -run 'Config.*Settings|ConfigGet' -count=1` 通过；运行 `go test ./internal/config ./internal/cli ./internal/output ./cmd/pinax -run 'Config|Settings|Source|Theme|Secret|Redaction' -count=1`，退出码 0；运行 `go test ./internal/app -run 'Remote|Capability|OpenAPI|Web' -count=1`，退出码 0。`docs/commands/config.md` 和 `docs/product/web-open-design.md` 已说明 Settings projection、source 枚举、写入 scope 与脱敏边界。

- [x] **1.4 补齐 Appearance theme 和 Keymap 设置合同**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.3
  - Scope: 固化 Settings 中 Appearance 和 Keymap 的合同。Appearance 复用现有 `output.theme`、`output.color`、`output.markdown.style`、`themes.custom.*`；Keymap 首版只提供 Web-client preference schema 和冲突检测要求，Pinax CLI 当前只暴露 `editor.command`。
  - Files: `internal/config/config.go`、`internal/cli/config_cmd.go`、`cmd/pinax/config_command_test.go`、`docs/commands/config.md`、`docs/product/web-open-design.md`、`openspec/changes/pinax-web-open-design-client-contracts/specs/pinax-web-client-contracts/spec.md`。
  - Acceptance: Appearance 文档和 tests 覆盖 `pinax config set output.theme high-contrast --scope user`、`pinax config set output.color auto --scope user`、`pinax config set output.markdown.style dark --scope user`、`pinax config set themes.custom.accent cyan --scope user`；Keymap 文档明确不得伪造 `pinax keymap` 命令，并将 future typed keymap config 作为后续 additive task。
  - Validation command: `go test ./internal/config ./cmd/pinax -run 'Config|Theme|CustomTheme|EditorCommand|Keymap' -count=1`
  - Expected result: 现有 config tests 通过；Keymap 无专门 CLI 时不出现假命令示例。
  - Failure re-check: 如果要新增 `ui.keymap.*` config key，必须先扩展 `configKeys()`、`Value()`、`parseConfigValue()`、validation 和 docs，不能让 `config set` 写未知 key。
  - Evidence: 2026-06-26 新增 `cmd/pinax/config_command_test.go`，通过真实 CLI 覆盖 `pinax config set output.theme high-contrast --scope user`、`pinax config set output.color auto --scope user`、`pinax config set output.markdown.style dark --scope user`、`pinax config set themes.custom.accent cyan --scope user` 和 `pinax config set editor.command "code --wait" --scope user`，并断言 `config doctor --json` 暴露 `editor.command` 且不出现伪造的 `pinax keymap` 或 `ui.keymap`。运行 `go test ./cmd/pinax -run 'ConfigAppearanceAndKeymap' -count=1`，退出码 0；运行 `go test ./internal/config ./cmd/pinax -run 'Config|Theme|CustomTheme|EditorCommand|Keymap' -count=1`，退出码 0。`docs/commands/config.md` 和 `docs/product/web-open-design.md` 已说明 Appearance 复用现有 config key，Keymap 首版只展示 `editor.command`，Web 快捷键偏好需等待后续 additive typed config 合同。

- [x] **1.5 补齐 Cloud Sync Settings 合同**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.3
  - Scope: Settings 的 Cloud Sync 分组展示 backend、workspace/device、secret refs、doctor、diff、daemon status/logs、conflicts 和 dangerous actions；所有写操作通过 `pinax cloud ...` 或 `pinax sync ...`。
  - Files: `internal/cli/ops_integration_cmd.go`、`internal/cli/sync_cmd.go`、`internal/app/service.go`、`internal/output/render.go`、`cmd/pinax/sync_cloud_backend_plan_command_test.go`、`cmd/pinax/sync_daemon_command_test.go`、`docs/commands/cloud.md`、`docs/commands/sync.md`、`docs/product/web-open-design.md`。
  - Acceptance: Settings 可展示 `pinax cloud status --vault ./my-notes --json`、`pinax cloud doctor --vault ./my-notes --json`、`pinax sync diff --target cloud --vault ./my-notes --json`、`pinax sync push --target cloud --vault ./my-notes --dry-run --json`、`pinax sync daemon status --vault ./my-notes --json` 和 redacted logs；daemon run/start/push/pull 必须显示确认和 secret-ref 边界。
  - Validation command: `go test ./cmd/pinax ./internal/app ./internal/output -run 'Cloud|SyncDaemon|CloudStatus|CloudDoctor|SyncDiff|DryRun|Secret|Redaction' -count=1`
  - Expected result: Cloud Sync focused tests 通过；输出不包含 raw token、secret、provider stderr、Authorization header、note body 或 plaintext sync payload。
  - Failure re-check: 如果某 backend 不可用，projection 返回 stable error 和 next action，不允许 UI 显示成功或 `remote_write=true`。
  - Evidence: 2026-06-26 复核现有 `cloud`、`sync`、daemon 和 redaction 覆盖；`docs/commands/cloud.md`、`docs/commands/sync.md`、`docs/product/web-open-design.md` 已列出 `pinax cloud status --vault ./my-notes --json`、`pinax cloud doctor --vault ./my-notes --json`、`pinax sync diff --target cloud --vault ./my-notes --json`、`pinax sync push --target cloud --vault ./my-notes --dry-run --json`、`pinax sync daemon status --vault ./my-notes --json`、redacted daemon logs、Cloud Sync 与 Local API 边界、secret-ref 边界和 daemon/push/pull confirmation 要求。运行 `go test ./cmd/pinax ./internal/app ./internal/output -run 'Cloud|SyncDaemon|CloudStatus|CloudDoctor|SyncDiff|DryRun|Secret|Redaction' -count=1`，退出码 0。

- [x] **1.6 补齐 Publish Settings 合同**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.3
  - Scope: Settings 的 Publish 分组展示 profiles、target、renderer、theme、plan/build/serve/deploy、latest receipt 和 secret scan status；deploy 进入 danger zone。
  - Files: `internal/cli/publish_cmd.go`、`internal/app/publish*.go`、`internal/app/publishops/`、`internal/output/render.go`、`cmd/pinax/publish_command_test.go`、`docs/commands/publish.md`、`docs/product/web-open-design.md`。
  - Acceptance: Settings 可展示 `pinax publish profile list --vault ./my-notes --json`、`pinax publish profile validate public --vault ./my-notes --json`、`pinax publish theme list --vault ./my-notes --json`、`pinax publish plan --profile public --target github-pages --vault ./my-notes --json`、`pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json` 和 loopback serve；deploy 必须要求 `--yes`、latest receipt、safe output path 和 target repo/path 检查。
  - Validation command: `go test ./cmd/pinax ./internal/app ./internal/output -run 'Publish|Profile|Theme|Plan|Build|Serve|Deploy|SecretScan|Receipt' -count=1`
  - Expected result: publish focused tests 通过；plan/build/deploy 输出不泄露 private body、absolute vault paths、`.pinax/**` internals、tokens 或 provider payload。
  - Failure re-check: 如果 Hugo/gh 等外部工具缺失，Settings 只能显示 diagnostic 和 next action，不把 deploy 标为 ready。
  - Evidence: 2026-06-26 复核现有 Publish Settings 合同覆盖；`docs/commands/publish.md` 与 `docs/product/web-open-design.md` 已列出 `pinax publish profile list --vault ./my-notes --json`、`pinax publish profile validate public --vault ./my-notes --json`、`pinax publish theme list --vault ./my-notes --json`、`pinax publish plan --profile public --target github-pages --vault ./my-notes --json`、`pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json`、loopback `publish serve` 和 `publish deploy ... --yes`，并说明 receipt、secret scan、safe output path、target repo/path 和 danger-zone 边界。运行 `go test ./cmd/pinax ./internal/app ./internal/output -run 'Publish|Profile|Theme|Plan|Build|Serve|Deploy|SecretScan|Receipt' -count=1`，退出码 0。

## P2: 右侧 Agent 侧栏合同

- [x] **2.1 实现 bounded agent context projection**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: 为 note card/detail、search result、project board item、graph entity、canvas object ref、editor selection 提供统一 bounded context shape；默认不返回完整 body。
  - Files: `internal/domain/types.go`、`internal/app/service.go`、`internal/app/query.go`、`internal/app/project_board.go`、`internal/app/linkgraph.go`、`internal/output/render.go`、`cmd/pinax/*_test.go`。
  - Acceptance: context projection 至少包含 `context_id`、`source_kind`、`display_title`、`refs`、`snippets`、`evidence`、`body_exposure`、`actions`；body mode 只能由显式参数升级；`--agent` 输出保持 low-token key=value。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'AgentContext|NoteDisplay|SearchResult|ProjectBoard|Graph|EditorSelection|Redaction' -count=1`
  - Expected result: context tests 通过；默认输出不含完整 note body、raw prompt 或 provider payload。
  - Failure re-check: 如果 canvas object 还没有 service，先定义引用 shape 和 future capability metadata，不手写 `.pinax/canvases/*.json`。
  - Evidence: 2026-06-26 按 TDD 新增 `cmd/pinax/agent_context_command_test.go`，首轮 `go test ./cmd/pinax -run TestBoundedAgentContextProjectionCLI -count=1` 因 note card 缺少 `agent_context` 失败；随后新增 `domain.AgentContext`/`pinax.agent_context.v1` 统一 shape，并在 note display、search result、project board item、project board 顶层 `data.agent_contexts`、note links/backlinks graph projection 中输出 bounded context。shape 包含 `context_id`、`source_kind`、`display_title`、`refs`、`snippets`、`evidence`、`body_exposure`、`actions`；默认 card/search/board/graph 不输出完整 body。重跑 `go test ./cmd/pinax -run TestBoundedAgentContextProjectionCLI -count=1` 通过；运行 `go test ./internal/app ./internal/output ./cmd/pinax -run 'AgentContext|NoteDisplay|SearchResult|ProjectBoard|Graph|EditorSelection|Redaction' -count=1`，退出码 0。`docs/product/web-open-design.md` 已记录 `pinax.agent_context.v1` 字段和 body exposure 升级边界。

- [x] **2.2 补齐 Agent plan/diff/apply gate projection**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1
  - Scope: 为右侧 Agent 的 `Ask`、`Diagnose`、`Plan`、`Apply after review` 提供统一 action/diff/snapshot requirement/receipt preview 合同；不新增自由执行命令。
  - Files: `internal/domain/types.go`、`internal/app/service.go`、`internal/output/render.go`、`internal/api/rpc.go`、`internal/api/http.go`、`cmd/pinax/*_test.go`、`docs/product/web-open-design.md`。
  - Acceptance: read-only Ask/Diagnose 不写 vault；Plan 返回 reviewable plan 和真实 next command；Apply 在 readonly server、缺 `yes=true` 或缺 snapshot 时返回 `write_disabled`、`approval_required` 或 `snapshot_required`；成功 apply 返回 receipt 和 restore hint。
  - Validation command: `go test ./internal/app ./internal/api ./internal/output ./cmd/pinax -run 'Agent|Plan|Diff|Apply|Snapshot|Receipt|WriteDisabled|ApprovalRequired' -count=1`
  - Expected result: focused tests 通过；失败 projection 包含可执行 next action，不泄露私密 payload。
  - Failure re-check: 如果已有 repair/organize/proof projection 可复用，优先适配到 Agent action shape，不新增平行 plan 类型。
  - Evidence: 2026-06-26 复核现有 plan/diff/apply gate projection，复用 project item plan、repair/organize/proof/version/publish/sync 等既有受控链路，不新增自由执行命令。`docs/product/web-open-design.md` 已说明 Agent 侧栏只能读取 bounded context、调用 registered capability、显示真实 `pinax ...` 命令，并且 apply 必须满足 write mode、snapshot gate 和 `yes=true`。运行 `go test ./internal/app ./internal/api ./internal/output ./cmd/pinax -run 'Agent|Plan|Diff|Apply|Snapshot|Receipt|WriteDisabled|ApprovalRequired' -count=1`，退出码 0。

## P3: BYOK 和 local provider 合同

- [x] **3.1 固化 provider status 面板输出**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 1.1
  - Scope: 把 `pinax kb provider list` 与 `pinax kb provider doctor <provider>` 的 projection 纳入 Web provider status 合同，输出 `configured`、`credential_source`、`local_only`、default model、doctor action 和 rebuild examples。
  - Files: `internal/app/kb.go`、`internal/cli/kb_cmd.go`、`internal/output/render.go`、`cmd/pinax/kb_command_test.go`、`docs/commands/kb.md`、`docs/product/web-open-design.md`。
  - Acceptance: `pinax kb provider list --vault ./my-notes --json` 不显示 key 值；OpenAI/Gemini 缺凭据返回 stable `provider_not_configured`；Ollama 显示 local service reachability；`fake` 标记为 testing/offline provider。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'KBProvider|ProviderDoctor|CredentialSource|Ollama|Fake|Redaction' -count=1`
  - Expected result: provider tests 通过；输出只包含 env var 名、配置源类型或 local endpoint，不包含真实 secret。
  - Failure re-check: 如果现有 provider projection 已覆盖字段，只新增 Web-facing optional facts，保留旧字段。
  - Evidence: 2026-06-26 复核 `kb provider list/doctor` 现有投影和文档，`docs/commands/kb.md` 与 `docs/product/web-open-design.md` 已说明 `configured`、`credential_source`、`local_only`、default model、OpenAI/Gemini 缺凭据的 stable `provider_not_configured`、Ollama local service reachability 和 `fake` testing/offline provider 边界。运行 `go test ./internal/app ./internal/output ./cmd/pinax -run 'KBProvider|ProviderDoctor|CredentialSource|Ollama|Fake|Redaction' -count=1`，退出码 0。

- [x] **3.2 增加 BYOK/local provider UI error next actions**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 3.1
  - Scope: provider missing、local model offline、sidecar missing、embedding backend unavailable 时输出 Web 可直接展示的 next action 命令。
  - Files: `internal/app/kb.go`、`internal/output/render.go`、`cmd/pinax/kb_command_test.go`、`docs/commands/kb.md`。
  - Acceptance: 缺凭据时 next action 是 `pinax kb provider doctor <provider> --vault ./my-notes --json`；重建示例使用真实命令如 `pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json`；不建议在 Web 表单粘贴 key。
  - Validation command: `go test ./cmd/pinax ./internal/app ./internal/output -run 'KBProvider|NextAction|ProviderNotConfigured|Sidecar|Redaction' -count=1`
  - Expected result: next action tests 通过；human/json/agent 输出都不显示明文 key。
  - Failure re-check: 如果 provider doctor 错误来自外部进程 stderr，必须先过 redaction helper 后进入 projection。
  - Evidence: 2026-06-26 复核 provider missing、sidecar unavailable、local/offline provider 和 rebuild command 覆盖；`docs/product/web-open-design.md` 明确缺 credential 时展示 `pinax kb provider doctor <provider> --vault ./my-notes --json`，并禁止 Web 表单收集或保存 raw key；`docs/commands/kb.md` 列出 OpenAI/Gemini/Ollama/fake provider doctor 与 rebuild 示例。运行 `go test ./cmd/pinax ./internal/app ./internal/output -run 'KBProvider|NextAction|ProviderNotConfigured|Sidecar|Redaction' -count=1`，退出码 0。

## P4: Pinax Editor 合同

- [x] **4.1 固化 Editor note read/display/body exposure 合同**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 2.1
  - Scope: 为 Pinax Editor 的 preview/source/split 提供 note read/show projection，明确 card/detail/context/body 的默认与升级语义。
  - Files: `internal/cli/note_cmd.go`、`internal/app/service.go`、`internal/output/render.go`、`cmd/pinax/note_record_command_test.go`、`docs/commands/note.md`、`docs/product/web-open-design.md`。
  - Acceptance: `pinax note read "Research Log" --display card --vault ./my-notes --json` 返回 bounded card；`--display body` 才返回正文；`--agent` 输出不含完整 body；缺 note 时提供 `pinax index refresh --vault ./my-notes --json` next action。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'NoteRead|NoteDisplay|BodyExposure|Editor|MissingNote|Redaction' -count=1`
  - Expected result: note display tests 通过；旧 `note show/read` 行为保持兼容。
  - Failure re-check: 如果旧测试依赖 body 默认输出，保留 CLI human 兼容路径，但 machine projection 必须显式标记 body exposure。
  - Evidence: 2026-06-26 复核 note display/body exposure 覆盖；`note read/show --display card|detail|context` 保持 bounded metadata/excerpt/`agent_context`，`--display body` 才显式返回正文，缺 note 时保留 index refresh next action。运行 `go test ./internal/app ./internal/output ./cmd/pinax -run 'NoteRead|NoteDisplay|BodyExposure|Editor|MissingNote|Redaction' -count=1`，退出码 0。`docs/commands/note.md` 与 `docs/product/web-open-design.md` 已说明 Editor preview/source/split 的 body exposure 升级语义和真实命令。

- [x] **4.2 补齐 Editor diff、managed block 和附件工作流**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 4.1, 2.2
  - Scope: 为 Agent rewrite、managed block refresh、attachment add/list 和 snapshot before apply 提供 Editor 可用的 diff/receipt/status projection。
  - Files: `internal/app/service.go`、`internal/cli/note_cmd.go`、`internal/assets/`、`internal/output/render.go`、`cmd/pinax/note_record_command_test.go`、`cmd/pinax/asset_command_test.go`、`docs/commands/note.md`、`docs/commands/asset.md`。
  - Acceptance: `pinax note refresh "Research Log" --rendered --yes --vault ./my-notes --json` 只刷新 managed block；`pinax note attach "Research Log" ./diagram.png --placement note-folder --embed --vault ./my-notes --json` 返回 bounded attachment receipt；`pinax version snapshot --vault ./my-notes --message "snapshot before editor apply" --json` 可作为 apply 前置证据。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'NoteRefresh|ManagedBlock|Attachment|VersionSnapshot|Editor|Receipt' -count=1`
  - Expected result: focused tests 通过；managed block marker 外正文保持不变；附件命令不泄露本机绝对路径。
  - Failure re-check: 如果 attachment projection 需要路径样式，优先使用 vault-relative 或 note-relative path，不输出用户 home path。
  - Evidence: 2026-06-26 复核 Editor diff、managed block refresh、attachment receipt 和 apply 前 snapshot 工作流；`docs/product/web-open-design.md`、`docs/commands/note.md`、`docs/commands/asset.md` 已记录 `pinax note refresh "Research Log" --rendered --yes --vault ./my-notes --json`、`pinax note attach "Research Log" ./diagram.png --placement note-folder --embed --vault ./my-notes --json` 和 `pinax version snapshot --vault ./my-notes --message "snapshot before editor apply" --json`，并说明 managed block marker 外正文保持不变、附件只输出 vault/note-relative bounded receipt。运行 `go test ./internal/app ./internal/output ./cmd/pinax -run 'NoteRefresh|ManagedBlock|Attachment|VersionSnapshot|Editor|Receipt' -count=1`，退出码 0。

## P5: Kanban、图谱、搜索和画布合同

- [x] **5.1 补齐 Kanban P0 client projection**
  - Owner: `cli/pinax`
  - Lane: E
  - Depends on: 1.1, 2.2
  - Scope: 为 Web Kanban 提供 board show、saved view、card inspector、add/move/archive plan 的 bounded projection，并显示 WIP slot、blocked/overdue、assignee/due/labels/subtasks 等 UI 字段。
  - Files: `internal/app/project_board.go`、`internal/cli/project_cmd.go`、`internal/output/render.go`、`cmd/pinax/project_board_command_test.go`、`docs/commands/project.md`。
  - Acceptance: `pinax project board show research --subproject stock-learning --vault ./my-notes --json` 返回 columns/cards/actions；drag move 走 service；archive 缺 snapshot 返回 stable gate；card detail 不默认包含完整 note body。
  - Validation command: `go test ./internal/app ./internal/output ./cmd/pinax -run 'ProjectBoard|Kanban|BoardView|CardInspector|WIP|Archive|Snapshot' -count=1`
  - Expected result: Kanban focused tests 通过；旧 project board commands 兼容。
  - Failure re-check: 如果 WIP 仍是 P1，只输出 optional `wip.status=not_configured` 或 omitted field，不伪造 limit。
  - Evidence: 2026-06-26 复核 project board P0 client projection；`docs/commands/project.md` 和 `docs/product/web-open-design.md` 已记录 `pinax project board show research --subproject stock-learning --vault ./my-notes --json`、saved view、card inspector、add/move/archive service gate、`pinax version snapshot --vault ./my-notes --message "snapshot before archive"` 和 archive approval。WIP 在 P0 只保留可见位置/可选状态，不伪造 limit；card detail 默认 bounded，不返回完整 note body。运行 `go test ./internal/app ./internal/output ./cmd/pinax -run 'ProjectBoard|Kanban|BoardView|CardInspector|WIP|Archive|Snapshot' -count=1`，退出码 0。

- [x] **5.2 补齐图谱 P0 client projection**
  - Owner: `cli/pinax`
  - Lane: E
  - Depends on: 2.1
  - Scope: 为知识图谱提供 entity search、一跳/二跳展开、关系证据、confidence、link status、table linkage 和 anti-hairball controls 的 projection。
  - Files: `internal/app/linkgraph.go`、`internal/notelinks/`、`internal/cli/collection_graph_cmd.go`、`internal/output/render.go`、`cmd/pinax/*graph*_test.go`、`docs/commands/note.md`。
  - Acceptance: `pinax note links "Research Log" --vault ./my-notes --json`、`pinax note backlinks "Research Log" --vault ./my-notes --json` 和 `pinax graph query --kind technique --match storyboard --vault ./my-notes --json` 输出节点/边/证据 bounded facts；默认不加载全量图。
  - Validation command: `go test ./internal/app ./internal/notelinks ./internal/output ./cmd/pinax -run 'Graph|Link|Backlink|Entity|Evidence|Confidence|Bounded' -count=1`
  - Expected result: graph tests 通过；ambiguous/broken links 不自动修复。
  - Failure re-check: 如果 graph query 仍缺少字段，先补 optional facts，不改变 note links/backlinks 现有 envelope。
  - Evidence: 2026-06-26 复核 graph P0 client projection；`docs/commands/note.md` 和 `docs/product/web-open-design.md` 已记录 `pinax note links "Research Log" --vault ./my-notes --json`、`pinax note backlinks "Research Log" --vault ./my-notes --json`、`pinax graph query --kind technique --match storyboard --vault ./my-notes --json`，并说明节点/边/关系证据、confidence/link status、anti-hairball 一跳/二跳控制和 broken/ambiguous link 不自动修复。运行 `go test ./internal/app ./internal/notelinks ./internal/output ./cmd/pinax -run 'Graph|Link|Backlink|Entity|Evidence|Confidence|Bounded' -count=1`，退出码 0。

- [x] **5.3 补齐搜索 P0 client projection 与 `rg` fallback 诊断**
  - Owner: `cli/pinax`
  - Lane: F
  - Depends on: 1.1
  - Scope: 为搜索侧边栏和全屏搜索页提供按 note 分组、snippet highlight、heading path、filters、index status、recent search 和 raw text scan fallback 诊断合同。
  - Files: `internal/app/search.go`、`internal/index/`、`internal/cli/search_cmd.go`、`internal/output/render.go`、`cmd/pinax/search_database_command_test.go`、`docs/commands/search.md`、`docs/product/web-open-design.md`。
  - Acceptance: `pinax search "authentication" --tag auth --group work --folder architecture --kind reference --status active --vault ./my-notes --json` 返回 grouped bounded results；index stale/missing 给 `pinax index refresh --vault ./my-notes --json`；`rg` fallback 只能作为 service-managed diagnostic，不让浏览器直接扫描 vault。
  - Validation command: `go test ./internal/app ./internal/index ./internal/output ./cmd/pinax -run 'Search|Grouped|Snippet|Heading|IndexStatus|RG|Fallback|Redaction' -count=1`
  - Expected result: search tests 通过；snippet 不突破 body exposure；不输出 `.pinax/**` 或 `.git/**` 匹配。
  - Failure re-check: 如果 `rg` 不可用，projection 返回 fallback unavailable 和 index refresh action，不失败整个搜索页。
  - Evidence: 2026-06-26 复核搜索 P0 client projection；`docs/commands/search.md` 和 `docs/product/web-open-design.md` 已记录 `pinax search "authentication" --tag auth --group work --folder architecture --kind reference --status active --vault ./my-notes --json` 这类过滤入口、grouped bounded results、snippet/heading path/index status、stale/missing index 的 `pinax index refresh --vault ./my-notes --json` next action，以及 `rg` 只能作为 application service managed fallback，不允许浏览器直接扫描 vault，且排除 `.pinax/**` 和 `.git/**`。运行 `go test ./internal/app ./internal/index ./internal/output ./cmd/pinax -run 'Search|Grouped|Snippet|Heading|IndexStatus|RG|Fallback|Redaction' -count=1`，退出码 0。

- [x] **5.4 定义 Canvas layout metadata capability 和 future-client boundary**
  - Owner: `cli/pinax`
  - Lane: F
  - Depends on: 2.1
  - Scope: 为无限画布定义 note/search/graph/project/evidence/frame/connector 的 layout metadata shape、capability discovery 和写入边界；首版可只交付 spec、service interface 和 read-only projection，不实现完整画布编辑器。
  - Files: `internal/domain/types.go`、`internal/app/service.go`、`internal/app/remote.go`、`openspec/changes/pinax-web-open-design-client-contracts/specs/pinax-web-client-contracts/spec.md`、`docs/product/web-open-design.md`。
  - Acceptance: API discovery 能标记 canvas capability 为 `future-client-only` 或 `planned`，并说明 layout metadata 必须由 service 写入；canvas object refs 不保存完整 note body、搜索结果全文或 provider payload。
  - Validation command: `openspec validate pinax-web-open-design-client-contracts --strict && go test ./internal/app ./internal/domain -run 'Canvas|Layout|Capability|Bounded' -count=1`
  - Expected result: OpenSpec validation 通过；如果 Go service 尚未实现，任务输出必须明确后续子任务和 future-client boundary。
  - Failure re-check: 不允许让 Web 或 Agent 手写未来 `.pinax/canvases/*.json`；如果需要 fixture，只能作为 test input，不作为官方 write path。
  - Evidence: 2026-06-26 按 TDD 在 `internal/app/remote_test.go` 增加 `canvas.layout.metadata` discovery 断言，首轮 `go test ./internal/app -run 'Remote|Capability|Canvas|Web' -count=1` 因缺 capability 失败；随后在 `RemoteCapabilities` 新增只读 planned capability：`id=canvas.layout.metadata`、`ui_group=canvas.view`、`body_exposure_default=none`、`write_gate=readonly`、`local_only_reason=future-client-only`，`copy_command` 指向真实 discovery 命令 `pinax api routes --vault <vault> --json`，不注册伪造的 `pinax canvas` CLI/REST/RPC 写入口。重跑 `go test ./internal/app -run 'Remote|Capability|Canvas|Web' -count=1` 通过；运行 `openspec validate pinax-web-open-design-client-contracts --strict && go test ./internal/app ./internal/domain -run 'Canvas|Layout|Capability|Bounded' -count=1`，退出码 0。`docs/commands/api.md` 和 `docs/product/web-open-design.md` 已说明 layout metadata 必须由 service-owned structured assets 写入，未来客户端不得手写 `.pinax/canvases/*.json`，也不得保存完整 note body、搜索全文、raw provider payload 或 raw prompt。

## P6: 收口和 future-client handoff

- [x] **6.1 生成 Web client contract handoff 文档**
  - Owner: `cli/pinax`
  - Lane: docs
  - Depends on: 1.2, 2.2, 3.2, 4.2, 5.1, 5.2, 5.3, 5.4
  - Scope: 形成未来独立客户端子项目可消费的 handoff：capability matrix、API discovery commands、provider safety rules、Editor projection rules、proof gate checklist、mobile constraints 和 deferred items。
  - Files: `docs/product/web-open-design.md`、`docs/interfaces/remote-api-contract.md`、`docs/commands/api.md`、`openspec/changes/pinax-web-open-design-client-contracts/design.md`、`openspec/changes/pinax-web-open-design-client-contracts/tasks.md`。
  - Acceptance: handoff 文档不包含客户端源码路径承诺；明确未来客户端必须是独立 owner；所有命令示例真实可运行；列出暂不做的 hosted multi-user、浏览器直接读本地文件、Web 直接写 `.pinax/**`、全量图谱、实时多人协作、3D 图谱和默认批量替换。
  - Validation command: `rg -n 'future-client|独立客户端|pinax api routes|proof gate|BYOK|Pinax Editor' docs openspec/changes/pinax-web-open-design-client-contracts`
  - Expected result: handoff 入口完整，读者能从 docs 找到 OpenSpec 和 validation commands。
  - Failure re-check: 如果 root docs 需要链接，只更新索引或 handoff，不复制 Pinax 子项目长文档。
  - Evidence: 2026-06-26 在 `docs/product/web-open-design.md` 新增 `Future-client handoff` 章节，集中列出独立客户端 owner 边界、`pinax api routes/status/schema export` discovery commands、capability handoff matrix、proof gate checklist、BYOK/provider safety、Pinax Editor 规则、移动端约束和 deferred items。运行 `rg -n 'future-client|独立客户端|pinax api routes|proof gate|BYOK|Pinax Editor' docs openspec/changes/pinax-web-open-design-client-contracts`，命中 Web 设计、docs README、MVP、remote/API docs、OpenSpec proposal/design/tasks/spec，handoff 入口完整。

- [x] **6.2 最终合同验证和归档准备**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 6.1
  - Scope: 运行 OpenSpec 和 focused tests，整理 evidence，确认没有破坏 CLI/API/output/redaction 合同。
  - Files: `openspec/changes/pinax-web-open-design-client-contracts/*`、`docs/product/web-open-design.md`、相关 Go tests。
  - Acceptance: `openspec validate pinax-web-open-design-client-contracts --strict` 通过；`openspec validate --all --strict` 通过或只存在已记录无关 active change 失败；触及 Go 后 `task check` 通过或记录无关失败；新增集成证据路径脱敏。
  - Validation command: `openspec validate pinax-web-open-design-client-contracts --strict && openspec validate --all --strict`
  - Expected result: OpenSpec 严格校验通过；若 Go 代码被修改，补充 `task check` 结果。
  - Failure re-check: 如果全量 OpenSpec 因其他 active change 失败，运行 focused validation 并记录失败 change 名称、错误摘要和为什么与本变更无关。
  - Evidence: 2026-06-26 运行 `openspec validate pinax-web-open-design-client-contracts --strict && openspec validate --all --strict`，47 项通过、0 失败；运行 focused Go tests `go test ./internal/app ./internal/api ./internal/output ./cmd/pinax -run 'Remote|Capability|OpenAPI|Web|Workbench|Status|AgentContext|Provider|NoteRead|NoteRefresh|ProjectBoard|Graph|Search|Canvas|Layout' -count=1`，退出码 0。首轮 `task check` 暴露本变更新增 helper 的 lint 问题和 MCP bounded context 泄露风险，已从源头修复：删除未接入 future helper，project board agent context 不再包含 note excerpt，graph agent context 只使用 link evidence、不回退 note body。复跑 `go test ./internal/mcpserver -run 'ReadonlyMCPProjectBoardTool|GraphContextBounds' -count=1` 和相关 focused tests 均通过；最终运行 `task check`，OpenSpec 47 项通过、`golangci-lint run` 0 issues、`go test ./...` 通过、sidecar protocol tests 通过、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` 通过。
