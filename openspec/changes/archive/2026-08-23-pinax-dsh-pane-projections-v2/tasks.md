## 任务

- [x] 1. `AssemblePaneBacklinks` + 单测（有界、红线、空输入合法 ready、`dropped_unsafe` 计数）。
- [x] 2. `AssemblePaneGraphSummary` + 单测（计数、top-k≤20、无路径、红线扫描）。
- [x] 3. `AssemblePaneHistory` + 单测（≤100 事件、truncated 标记、revision 事件有界字段、红线扫描）。
- [x] 4. `newPaneSnapshotEnvelope` 支持 timeline 槽位填充；三面状态映射与 note list 面一致。
- [x] 5. `dsh-pane` 证据 profile 扩展 extra checks 并归档运行证据。
- [x] 6. `docs/interfaces/dsh-pane.md` 补三面接口文档。
- [x] 7. 归档门：harness `dsh-pinax-pane-v1` Host 消费落地后复核一致性。（2026-08-23：harness 已归档落地；其 pinax.spec.ts 覆盖 note/backlink/graph/history 投影与封闭 ref allowlist，与本仓库 `internal/app/pane_projections.go` 三面的有界/红线语义一致——backlink/graph/history 经 harness 侧共享 domain snapshot normalization 消费，Go 侧为 canonical owner 合同。复核通过。）

验证要求：`go test ./internal/app -run Pane -count=1`、`golangci-lint run`、`openspec validate --all` 全绿。

证据（2026-08-23）：`internal/app/pane_projections.go` 三组装器 + `pane_projections_test.go` 8 个用例（有界截断、dropped_unsafe 全量计数、红线递归扫描、空输入合法 ready、failed→offline、round-trip 形状 fail closed、graph 连通分量与确定性 top-k）；实现中修正了截断语义——超限后继续全量扫描计数 unsafe 丢弃，而非提前 break。`dsh-pane` 证据 profile 扩展 3 项 extra checks，运行证据 `temp/integration-test-runs/20260823T150153Z-2265383`。`docs/interfaces/dsh-pane.md` 补三面接口文档。验证：`go test ./...` 0 失败、`golangci-lint` 0 issues、`openspec validate --all` 73/73。
