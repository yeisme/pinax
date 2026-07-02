# 任务

- [x] 1. Owner: `cli/pinax`; Lane: spec; Depends on: none; Scope: OpenSpec proposal/design/tasks/spec delta。Acceptance: `openspec validate pinax-publish-preview-logging --strict` 通过；失败时先修 requirement/scenario 格式和 Mermaid。
  - Evidence: 2026-06-29 创建 proposal/design/tasks/spec delta；最终验证见任务 5。
- [x] 2. Owner: `cli/pinax`; Lane: test; Depends on: 1; Scope: 新增 publish preview logging RED tests。Acceptance: focused tests 先因缺少 `plan_checked`、`renderer_started`、`serve_ready` 等事件或 stderr 阶段日志失败；Validation: `go test ./cmd/pinax -run 'Publish.*Log|Publish.*Events' -count=1`。
  - Evidence: 2026-06-29 先运行 `go test ./cmd/pinax -run 'Publish.*Log|Publish.*Events' -count=1`，观察到 RED：`profile_ready` / `plan_checked` 阶段事件和 stderr 日志缺失。
- [x] 3. Owner: `cli/pinax`; Lane: app/cli; Depends on: 2; Scope: 实现 publish live event sink 和 CLI output mode 路由。Acceptance: `--events` 输出 NDJSON 阶段事件，summary mode 阶段日志进入 stderr，`--json`/`--agent` 不混入进度；Validation: `go test ./cmd/pinax -run 'Publish.*Log|Publish.*Events|Publish.*OutputModes' -count=1`。
  - Evidence: 2026-06-29 `go test ./cmd/pinax -run 'Publish.*Log|Publish.*Events' -count=1` 通过。
- [x] 4. Owner: `cli/pinax`; Lane: docs; Depends on: 3; Scope: 更新 publish 命令文档。Acceptance: 文档展示真实 `pinax publish ... --events` 命令并说明 stdout/stderr 边界；Validation: `rg -n "publish build .*--events|publish dev .*--events|stderr|NDJSON" docs/commands/publish.md`。
  - Evidence: 2026-06-29 `rg -n "publish build .*--events|publish dev .*--events|stderr|NDJSON" docs/commands/publish.md` 命中 Preview Logs And Events 小节。
- [x] 5. Owner: `cli/pinax`; Lane: verify; Depends on: 3,4; Scope: focused verification and OpenSpec validation。Acceptance: focused Go tests、OpenSpec validate 通过；Validation: `go test ./cmd/pinax -run 'Publish.*Log|Publish.*Events|Publish.*OutputModes|PublishBuild|PublishDev|PublishServe' -count=1` and `openspec validate pinax-publish-preview-logging --strict`。
  - Evidence: 2026-06-29 `go test ./cmd/pinax -run 'Publish.*Log|Publish.*Events|Publish.*OutputModes|PublishBuild|PublishDev|PublishServe' -count=1` 通过；`openspec validate pinax-publish-preview-logging --strict` 通过；`openspec validate --all --strict` 通过，54 passed / 0 failed。
