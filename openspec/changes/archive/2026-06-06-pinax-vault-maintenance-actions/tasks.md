## 1. Test Fixtures and RED Tests

- [x] 1.1 新增 fixture vault，覆盖 missing metadata、missing tags、duplicate title、empty note、stale note、orphan note、index stale 和 plan stale 场景。
- [x] 1.2 先写 `repair plan --json` contract test，验证 JSON envelope、plan id、operations、risk、skipped issues、next actions 和默认只读行为，并确认测试先失败。
- [x] 1.3 先写 `repair plan --save --json` test，验证 `.pinax/repair-plans/<plan_id>.json` 由 service 写入且 schema version 正确，并确认测试先失败。
- [x] 1.4 先写 `repair apply` approval/snapshot protection tests，验证无 `--yes` 返回 `approval_required`、无 snapshot 返回 `snapshot_required`，并确认测试先失败。
- [x] 1.5 先写 plan stale test，验证 note 变化后 apply 返回 `plan_stale`，并确认测试先失败。
- [x] 1.6 先写 dashboard readonly repair endpoint tests，验证 GET 可读、非 GET 拒绝、敏感信息脱敏，并确认测试先失败。

## 2. Domain Model and Plan Repository

- [x] 2.1 新增 repair plan domain model：plan id、schema version、created/expiry、source facts、issue snapshot、operations、risk、status 和 evidence。
- [x] 2.2 新增 repair operation model，区分 `automatic`、`manual_review`、`skipped`，并定义 metadata、tag、archive status、index rebuild、duplicate/manual review 等 operation kind。
- [x] 2.3 新增 `.pinax/repair-plans/` repository，只通过 application service 写入和读取 JSON 资产。
- [x] 2.4 为 plan id、path boundary、schema validation、expiry 和 redaction 添加单元测试。

## 3. Repair Planner Service

- [x] 3.1 新增 `VaultRepairPlanner`，复用 `VaultHealthService` issue 输出，不重复扫描规则。
- [x] 3.2 实现 missing metadata、missing tags、index stale 的 automatic operation 生成。
- [x] 3.3 实现 duplicate title、empty note、orphan note 的 manual-review operation 生成，禁止自动删除、合并或正文改写。
- [x] 3.4 实现 `--save` 写入计划资产并返回 saved path、plan id、operation counts 和 next actions。
- [x] 3.5 实现计划过期和 source fact 记录，为 apply drift check 提供依据。

## 4. Repair Apply Service

- [x] 4.1 新增 `VaultRepairApplier`，加载 saved plan 并验证 schema、vault boundary、expiry、status 和 source fact drift。
- [x] 4.2 实现 approval guard：无 `--yes` 返回 `approval_required`。
- [x] 4.3 实现 Git snapshot guard：无最近 Pinax snapshot 返回 `snapshot_required` 和 runnable next action。
- [x] 4.4 实现 automatic metadata/tag 修复，复用现有 frontmatter helper，不改正文。
- [x] 4.5 实现 automatic index rebuild，复用 `RebuildIndex` service。
- [x] 4.6 实现 archive status 修复，MVP 只写 frontmatter `status: archived`，不移动文件。
- [x] 4.7 对 manual-review/skipped operations 只记录结果，不执行危险写入。
- [x] 4.8 append redacted event evidence，记录 plan id、operation kind、result 和 stable error code，不写 raw payload 或 secrets。

## 5. CLI and Output Contract

- [x] 5.1 新增 `pinax repair` command group 和 `repair plan`、`repair apply` Cobra 命令。
- [x] 5.2 `repair plan` 支持 `--vault`、`--save`、默认 human、`--json`、`--agent`。
- [x] 5.3 `repair apply` 支持 `--vault`、`--plan`、`--yes`、`--snapshot-message`、默认 human、`--json`、`--agent`。
- [x] 5.4 新增 repair projection renderer/agent output，确保 operation counts、risk、plan id、saved path 和 next actions 从同一 projection 渲染。
- [x] 5.5 更新 `pinax repair --help` 和 root help，文案保持本地 Markdown vault 维护语义。

## 6. Dashboard Readonly Repair Views

- [x] 6.1 新增 dashboard repair plan summary service 或 endpoint，读取 saved plans 但不写入。
- [x] 6.2 新增 issue drilldown/plan summary HTML，展示 plan id、risk distribution、expiry、operation list 和 CLI apply command。
- [x] 6.3 验证 dashboard 没有 POST/PUT/DELETE 写入路由，非 GET 返回 method not allowed。
- [x] 6.4 验证 dashboard repair 输出经过 redaction，不展示 token、webhook URL、cookies、Authorization header、raw payload 或未脱敏 trace。

## 7. Documentation and Verification

- [x] 7.1 更新 README 或本子项目 docs，加入 `pinax repair plan`、`pinax repair apply` 和 dashboard repair drilldown 示例。
- [x] 7.2 运行 `gofmt -w` 覆盖变更 Go 文件。
- [x] 7.3 运行聚焦测试：`go test ./internal/app ./cmd/pinax ./internal/dashboard`。
- [x] 7.4 运行全量测试：`go test ./...`。
- [x] 7.5 运行构建：`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [x] 7.6 运行 OpenSpec 校验：`openspec validate --all`。
- [x] 7.7 如果本机安装 `task`，运行 `task check`；否则记录 fallback 命令结果。

## Verification Evidence

- RED confirmed: `go test ./cmd/pinax -run TestRepairPlanJSONIsReadonlyAndSaveWritesPlanAsset -count=1` failed with `unknown command "repair" for "pinax"` before implementation.
- RED confirmed: `go test ./cmd/pinax -run TestRepairApplyRequiresApprovalAndSnapshot -count=1` failed at missing `repair apply --plan` support before implementation.
- RED confirmed: `go test ./cmd/pinax -run TestRepairApplyLowRiskOperationsAndRejectsStalePlan -count=1` failed because apply did not patch note frontmatter before implementation.
- RED confirmed: `go test ./internal/dashboard -run TestReadonlyDashboardServesRepairPlans -count=1` failed because `/api/repair-plans` returned dashboard HTML before endpoint implementation.
- Evidence: `go test ./cmd/pinax -run 'TestRepair' -count=1` exited 0 after implementation.
- Evidence: `go test ./internal/dashboard -run 'TestReadonlyDashboardServes(StatsDoctorAndRedacts|RepairPlans)' -count=1` exited 0 after implementation.
- Evidence: `go test ./internal/app ./cmd/pinax ./internal/dashboard -count=1` exited 0.
- Evidence: `go test ./...` exited 0.
- Evidence: `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` exited 0.
- Evidence: `openspec validate --all` exited 0 with 5 passed, 0 failed.
- Evidence: `task check` exited 0 and ran OpenSpec validation, `go test ./...`, gofmt check, and build.
