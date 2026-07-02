# 任务

## Lane A：发布版定位与文档收敛

- [x] **A1. 收敛 README 与 quickstart 首屏**
  - Owner：`cli/pinax` 文档实现者。
  - Scope：修改 `README.md`、`README.zh-CN.md`、`docs/quickstart.md`，把第一屏主路径收敛为 local proof loop、bounded context、safe write/restore。
  - Dependencies：无，可并行。
  - Acceptance：首屏不把 Cloud Sync、publish、plugin、Workbench、daemon、provider-backed synthesis 当作必需路径；所有示例命令真实可运行。
  - Validation：`rg -n "Cloud Sync|plugin|publish|daemon|Workbench" README.md docs/quickstart.md` 检查高级路径是否处在 secondary/advanced 语境；`openspec validate pinax-release-agent-interface-convergence --strict` 通过。
  - Expected result：文档先展示 `pinax init`、`pinax note add`、`pinax proof loop run`、`pinax repair plan`、`pinax version snapshot`、`pinax repair apply`、`pinax version restore`。
  - Failure re-check：若示例命令引用不存在 flag，运行 `go run ./cmd/pinax <command> --help` 修正文档。

- [x] **A2. 对齐产品定位和 MVP 范围**
  - Owner：`cli/pinax` 产品/实现负责人。
  - Scope：修改 `docs/overview/product-positioning.md`、`docs/product/mvp-scope.md`，加入发布版能力矩阵和 maturity 标签，删除或降级未实现能力的“当前可用”表述。
  - Dependencies：A1 可并行。
  - Acceptance：文档明确 CLI-first、API/MCP derived surface、MCP readonly、provider-backed answer synthesis preview/experimental。
  - Validation：`rg -n "当前|Supported|Preview|实验|mature|first-support" docs/overview/product-positioning.md docs/product/mvp-scope.md` 人工复核标签一致性。
  - Expected result：用户能区分 mature、first-support、preview、experimental。
  - Failure re-check：若 docs 和 `pinax api routes --json` 能力发现冲突，以实际 route registry 为准修正文档。

## Lane B：Capability registry 与 CLI-first contract

- [x] **B1. 定义发布版核心 capability 清单**
  - Owner：CLI/API contract 实现者。
  - Scope：在现有 route/capability registry 中为 release core 标记 `release_core=true` 或等价稳定字段；覆盖 vault bootstrap、capture、retrieve、diagnose、plan、apply safely、discover。
  - Dependencies：A2 输出能力矩阵。
  - Acceptance：`pinax api routes --vault ./my-notes --json` 能列出每个 release core capability 的 `command`、`capability_id`、`readonly`、`body_allowed`、`approval_required`、`snapshot_required`、`copy_command`、`local_only_reason`。
  - Validation：新增 registry 单元测试，命令示例：`go test ./internal/api ./internal/cli -run 'ReleaseCore|RemoteCapabilities|RoutesMatchRegistry' -count=1`。
  - Expected result：发现面可被 agent 稳定消费。
  - Failure re-check：若 registry 字段属于稳定输出新增，确认 `cli-output-contract` spec delta 和 golden test 已覆盖。

- [x] **B2. 补齐 CLI JSON/agent 输出契约测试**
  - Owner：CLI 输出实现者。
  - Scope：为 release core 命令补 command-level/testscript 或 golden tests，验证 `--json` 输出为单一 projection、`--agent` 输出为 key=value、stderr 承载诊断。
  - Dependencies：B1 可并行部分执行。
  - Acceptance：覆盖至少 `vault validate`、`note add`、`search`、`memory context`、`vault doctor`、`repair plan --save`、`version snapshot`、`repair apply --yes`、`version restore --plan`、`api routes`。
  - Validation：`go test ./cmd/pinax ./internal/output ./internal/cli -run 'Output|Agent|JSON|ReleaseCore' -count=1`。
  - Expected result：release core 命令有稳定机器输出证据。
  - Failure re-check：若命令还没有能力实现，任务必须显式记录 `local_only_reason=planned`，不能伪造成功输出。

## Lane C：Local API/RPC 派生面

- [x] **C1. 验证 API route discovery 与 OpenAPI 派生**
  - Owner：Local API 实现者。
  - Scope：补齐 `pinax api routes` 和 `pinax api schema export --format openapi` 对 release core routes 的覆盖；OpenAPI 只导出真实 REST paths。
  - Dependencies：B1。
  - Acceptance：OpenAPI operation 带 `x-pinax-command`、`x-pinax-capability`、`x-pinax-readonly`、`x-pinax-body-allowed`、`x-pinax-approval-required`、`x-pinax-snapshot-required`。
  - Validation：`go test ./internal/api -run 'OpenAPI|RoutesMatchRegistry|ReleaseCore' -count=1`。
  - Expected result：HTTP schema 不维护第二套手写路径。
  - Failure re-check：若发现 planned capability 被导出为 REST path，删除虚构 path，保留 discovery metadata。

- [x] **C2. 覆盖 readonly 与 allow-write gate**
  - Owner：Local API 安全实现者。
  - Scope：增加 component/e2e 测试覆盖 readonly server 写入返回 `write_disabled`，allow-write 缺少 `yes=true` 返回 `approval_required`，需要 snapshot 时返回 `snapshot_required`。
  - Dependencies：C1。
  - Acceptance：测试证明 API handler 不直接写 Markdown、`.pinax/**`、Git、provider 或 remote state。
  - Validation：`go test ./internal/api ./internal/app -run 'WriteDisabled|ApprovalRequired|SnapshotRequired|ReleaseCore' -count=1`。
  - Expected result：API 派生面无法绕过 CLI proof loop gate。
  - Failure re-check：失败时检查 handler 是否绕过 `internal/app` 或缺少 route group write classification。

- [x] **C3. 固化 Remote API Mode 不 fallback**
  - Owner：Remote CLI 实现者。
  - Scope：覆盖 `--api-url`、`PINAX_API_URL`、user config remote URL 的支持命令转发，以及不支持命令返回 `remote_command_unsupported`。
  - Dependencies：B1、C1。
  - Acceptance：Remote mode 不会在 unsupported command 上静默执行本地 vault。
  - Validation：`go test ./internal/cli ./cmd/pinax -run 'RemoteMode|Unsupported|RemoteVaultConflict' -count=1`。
  - Expected result：agent 可以信任 remote mode 是 registry-limited transport。
  - Failure re-check：若本地 control commands 被 remote hijack，按 `config/api/token/profile/vault/cloud/sync` local-only 规则修复。

## Lane D：MCP Agent 体验

- [x] **D1. 发布版 MCP tool/resource 清单对齐**
  - Owner：MCP adapter 实现者。
  - Scope：确认 `pinax mcp serve` 的 `tools/list`、`resources/list` 只暴露 readonly release core read/plan capability，包含 brain context/answer/sources/maintenance_plan 的真实实现状态。
  - Dependencies：B1。
  - Acceptance：MCP 不暴露直接写 vault tool；full-body 请求默认降级或拒绝；plan-only tool 返回 next command。
  - Validation：`go test ./internal/mcp ./cmd/pinax -run 'MCP|ToolsList|ResourcesList|Readonly|Brain' -count=1`。
  - Expected result：agent 可以通过 MCP 安全读取和规划，但不能直接写。
  - Failure re-check：若 MCP tool 绕过 app service，重构到 adapter -> app service -> projection。

- [x] **D2. MCP frame/e2e 证据**
  - Owner：E2E 测试实现者。
  - Scope：增加 stdio MCP frame 测试，覆盖 initialize、tools/list、bounded read tool、write rejection 或 maintenance plan preview。
  - Dependencies：D1。
  - Acceptance：测试不依赖真实 MCP client、网络、token 或用户 vault。
  - Validation：`go test ./internal/mcp ./tests/e2e -run 'MCPReleaseCore|MCPFrame' -count=1`，或项目现有 testscript 等价命令。
  - Expected result：发布版 MCP 体验有自动化证据。
  - Failure re-check：若 stdout/stderr 混入日志导致 frame 解析失败，修复 serve lifecycle 输出分离。

## Lane E：发布版 proof loop 与证据

- [x] **E1. 五分钟 proof loop e2e**
  - Owner：E2E 测试实现者。
  - Scope：新增 installed-binary/process-level proof loop 测试，覆盖 init、note add、proof loop preview、repair plan save、snapshot、apply、restore plan/apply。
  - Dependencies：B2。
  - Acceptance：测试使用 fixture/temp vault，不需要 provider credentials、Cloud Sync、daemon、MCP、dashboard 或源码外部服务。
  - Validation：`go test ./tests/e2e ./cmd/pinax -run 'ProofLoopRelease|FiveMinute' -count=1` 或现有 testscript 命令。
  - Expected result：发布版主路径可复制、可恢复。
  - Failure re-check：若 fixture 不能产生 repair/apply 候选，补确定性 broken link、missing metadata、manual review item 和 restore file。

- [x] **E2. 集成证据与脱敏扫描**
  - Owner：QA/测试基础设施实现者。
  - Scope：确保 release core integration/component/e2e 入口写 `temp/integration-test-runs/<run-id>/summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`，失败仍保留证据。
  - Dependencies：C2、D2、E1。
  - Acceptance：证据不包含 token、Authorization、Cookie、provider payload、hidden system prompt、private tool args、full chain-of-thought 或未经批准的完整 note body。
  - Validation：`task test:integration`；若没有聚合入口，新增并运行等价 project task，然后运行脱敏扫描测试。
  - Expected result：发布门禁可审计。
  - Failure re-check：若测试失败但没写证据，先修 runner cleanup/defer；若泄露敏感模式，补 redaction 并回归。

## Lane F：最终发布门禁

- [x] **F1. 收敛 spec 与文档引用**
  - Owner：变更负责人。
  - Scope：更新 `openspec/changes/pinax-release-agent-interface-convergence/specs/**/spec.md`，确保需求和任务一致；必要时更新命令文档 `docs/commands/api.md`、`docs/commands/mcp.md`、`docs/interfaces/remote-api-contract.md`。
  - Dependencies：A-E。
  - Acceptance：所有新增或变更稳定字段都有 spec delta 和 contract test；所有文档命令真实可运行或清楚标注 planned/preview。
  - Validation：`openspec validate pinax-release-agent-interface-convergence --strict`。
  - Expected result：OpenSpec 可作为实现 handoff。
  - Failure re-check：若 strict validate 报 scenario 缺失，补 `#### Scenario:`；若 capability 和 docs 不一致，以 tests 和 registry 为准。

- [x] **F2. 运行发布版质量门禁**
  - Owner：release owner。
  - Scope：运行项目级质量命令并记录验证证据。
  - Dependencies：F1。
  - Acceptance：`task check` 通过；如本机缺 `task`，运行 fallback 命令并记录缺失原因。
  - Validation：`task check`。
  - Expected result：格式、lint、tests、build、sidecar protocol、OpenSpec 全部通过。
  - Failure re-check：按失败模块回到对应 lane 修复，不允许只更新文档绕过失败。
