## Why

Pinax 已有五条 plan/apply 型管道（organize、metadata、repair、version restore、publish）加 sync 与 proof loop，各自有独立的 plan 存储（`.pinax/organize-plans`、`.pinax/repair-plans` 等）、独立 receipt、独立 preview/--yes 语义。用户视角的痛点：

1. **没有一处能看全"哪些管道有 pending plan、哪些刚跑完"**——要记五棵命令树；`pinax activity` 覆盖事件但不覆盖 plan 状态。
2. **plan 新鲜度语义不统一**——proof loop 内部有 organize plan freshness 判定（`ensureOrganizePlanFresh`），但独立调用 `organize apply` 等管道时用户无法知道 plan 是否已落后于 vault 事实。
3. **计划检视体验割裂**——各管道 plan 文件格式不同，没有统一"这个 plan 会改哪些文件、风险如何"的视图；`--events` 阶段事件各管道自成一体。

对照 OKF 的 Attested Computation 流程心智（discover → load → parameterize → execute → attest → gate，每步有显式状态），本 change 把既有管道统一到一层**只读聚合 + 统一事件合同**上，不迁移存储、不改各管道既有合同。

## What Changes

- 新增统一 plan 读模型 `pinax.plan.v1`：跨 organize/metadata/repair/restore 归一化 plan 头（plan_id、kind、schema_version、created_at、facts 摘要、operations 计数、**freshness** 派生）；各管道存储原地不动，读取端归一。
- 新增 `pinax pipeline` 命令组：
  - `pipeline status`：pending saved plans（含 freshness 徽标）+ 最近 N 条 apply 型 receipt（organize/metadata/repair/restore/sync/publish/proof loop）的单一聚合视图。
  - `pipeline show <plan-id|receipt-id>`：统一详情（操作按 vault 写入 vs 元数据改动分组、changed paths、关联 snapshot、下一步命令提示）。
- 统一 apply 安全基线：所有 plan 型 apply 在 plan 落后于当前 vault 事实时 MUST 拒绝执行并提示重新 plan，除非显式 `--allow-stale`；`--yes` 非交互确认语义对齐。
- 统一阶段事件合同 `pinax.pipeline.stage.v1`（NDJSON）：apply 型命令发出 `stage.started`/`stage.completed` 事件（携带 pipeline kind、plan_id、阶段名、计数），由共享 app-service helper 统一发出。
- 非目标：不迁移/重命名既有 plan 存储与 receipt schema（`pinax.apply_receipt.v1` 原样兼容）、不改 sync/publish 专有 preview 合同、不引入常驻 daemon、不做交互式 TUI。

## Capabilities

### New Capabilities
- `spec:pipeline-unified-ux` — 统一 plan 读模型、pipeline status/show 聚合面、apply freshness 基线与阶段事件合同。

## Impact

- 代码：`internal/domain`（plan 读模型类型）、`internal/app`（聚合服务、共享事件 helper、各 apply 的 freshness 门）、`internal/cli`（pipeline 命令组）、`cmd/pinax`（测试）。
- 兼容：既有 plan 文件、receipt、命令输出零破坏；freshness 拒绝路径为新增守卫（默认开启，`--allow-stale` 逃生门）；未保存的 plan（内存态 preview）不受影响。
- 下游：proof loop 复用统一事件 helper；DSH pane/Workbench 后续可消费 `pipeline status` 投影。
