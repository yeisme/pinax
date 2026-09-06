# Tasks

## 1. 合同冻结

- [x] 1.1 冻结 `pinax.plan.v1` 读模型（归一字段、SourceSchema 透传、不迁移存储红线）与 `pinax.pipeline.stage.v1` 事件合同（stage.started/completed/failed、counts、additive 原则）。Validation: `openspec validate pinax-pipeline-unified-ux-v1 --strict --no-interactive`。

## 2. 读模型与聚合

- [x] 2.1 实现四管道 reader adapter + freshness 指纹判定（复用 facts 扫描 helper；损坏 plan fail-closed 条目）；单测覆盖归一与 stale 判定（改 note ⇒ stale）。Validation: `go test ./internal/app -run Pipeline -count=1`。
- [x] 2.2 实现 `pinax pipeline status`（pending plans + recent receipts 聚合、human/json/agent 三模式、只读断言）。Validation: `go test ./cmd/pinax -run PipelineStatus -count=1`。
- [x] 2.3 实现 `pinax pipeline show <id>`（plan/receipt 双形态、风险分组、path redaction、未知 id 稳定错误）。Validation: `go test ./cmd/pinax -run PipelineShow -count=1`。

## 3. 安全基线与事件

- [x] 3.1 metadata/repair/restore apply 补 freshness 守卫（plan_stale 拒绝 + `--allow-stale` 逃生门 + stage.failed 事件；organize 对齐既有判定）；无保存 plan 的单命令流程回归零变更。Validation: `go test ./internal/app -run Apply -count=1`。
- [x] 3.2 共享 `emitPipelineStage` helper 接入全部 apply 型命令（含 sync push/pull、publish build/deploy、proof loop run）；事件 golden NDJSON。Validation: `go test ./internal/app -run Stage -count=1`。

## 4. e2e 与收口

- [x] 4.1 testscript e2e：plan 保存 → 改 vault → apply 拒绝 → --allow-stale 通过 → receipt 入 status → show 双形态；命令树/completion 覆盖。Validation: `task test:integration`。
- [x] 4.2 docs（新 `docs/commands/pipeline.md` + 各 apply 命令文档补 freshness/--allow-stale）+ `task check` 全绿。Validation: `task check`。
