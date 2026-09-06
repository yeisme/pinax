# Design — 统一管道交互面（pinax-pipeline-unified-ux-v1）

## 1. 体验旅程

### 1.1 `pinax pipeline status` —— 一处看全

```
$ pinax pipeline status
Pipeline status · vault ./my-notes

Pending plans (2)
  Plan      Kind      Ops   Freshness   Saved        Next
  plan-7f3a organize  12    fresh       2h ago       pinax pipeline show plan-7f3a
  plan-9c1d metadata   4    STALE       3d ago       re-plan: pinax metadata plan

Recent runs (6)
  Receipt   Pipeline         Status   Changed  When
  r-a1b2    organize apply   applied  12       2h ago
  r-c3d4    sync push        applied  3        5h ago
  r-e5f6    proof loop       applied  4        1d ago
  …
Next: pinax pipeline show <id> · pinax organize apply --plan plan-7f3a --yes
```

- STALE 徽标：plan 生成后 vault 事实已变化（复用 proof loop 的 freshness 判定思路，推广到 metadata/repair/restore）。
- 只读命令；`--json/--agent/--events` 输出合同照常。

### 1.2 `pinax pipeline show <id>` —— 统一检视

```
$ pinax pipeline show plan-7f3a
plan-7f3a · organize · pinax.organize_plan.v1 · fresh (facts digest ab12cd)

Vault writes (5)
  move   notes/inbox/x.md        → notes/projects/a/x.md
  move   notes/inbox/y.md        → notes/reference/y.md
Metadata writes (7)
  tags_patch  notes/inbox/z.md   +auth
  …

Snapshot   snap-… available (proof loop apply path)
Receipts   none yet (plan not applied)
Next       pinax organize apply --plan plan-7f3a --yes
```

- 操作按风险分组（vault 结构写入 vs 元数据写入）；changed paths 走既有 path redaction 策略。
- 同一命令也接受 receipt id：展示 applied 事实、changed paths、snapshot/ledger 引用。

### 1.3 freshness 守卫 —— 统一 apply 安全基线

```
$ pinax organize apply --plan plan-9c1d --yes
Error: plan_stale — vault changed after this plan was generated (3 notes updated).
Re-run `pinax organize plan --save` or pass --allow-stale to apply anyway.
```

- 判定：plan 内记录的 facts 摘要（note 集合/修订指纹）≠ 当前扫描指纹 ⇒ stale。organize 已有该判定（proof loop 修复波引入），本 change 将同型判定补到 metadata/repair/restore apply。
- `--allow-stale` 显式逃生门；`--events` 下发出 `stage.failed{reason=plan_stale}`。

## 2. 统一读模型 `pinax.plan.v1`（读取端归一，不迁移存储）

```go
type PipelinePlanView struct {
    SchemaVersion string // "pinax.plan.v1"
    PlanID, Kind  string // organize|metadata|repair|restore
    SourceSchema  string // 原管道 plan schema_version
    CreatedAt     time.Time
    OpCounts      map[string]int
    Fresh         bool; FreshReason string
    FactsDigest   string
}
```

- 各管道新增一个 reader adapter（从各自存储反序列化 → 投影到 view）；存储、schema、既有命令不动。
- receipt 侧复用既有 `pinax.apply_receipt.v1`（含 Command/PlanID/SnapshotID/ChangedPaths），聚合视图只读 `.pinax/receipts/` + sync logs。

## 3. 阶段事件合同 `pinax.pipeline.stage.v1`

- 共享 helper `emitPipelineStage(ctx, kind, planID, stage, state, counts)`；apply 型命令（organize/metadata/repair/restore apply、sync push/pull、publish build/deploy、proof loop run）统一发出：

```json
{"type":"stage.started","pipeline":"organize","plan_id":"plan-7f3a","stage":"apply"}
{"type":"stage.completed","pipeline":"organize","plan_id":"plan-7f3a","stage":"apply","counts":{"applied":12}}
{"type":"stage.failed","pipeline":"organize","plan_id":"plan-9c1d","stage":"apply","reason":"plan_stale"}
```

- 既有各命令事件不删除、不改名（additive）；新事件是超集内新增 type，消费者可忽略未知 type。

## 4. 命令组

```
pinax pipeline status [--limit N] [--kind organize,metadata,…]
pinax pipeline show <plan-id|receipt-id>
```

- 归入现有命令树 "Configuration and maintenance" 或 "Organization and search" 组（实现时按 cli-tree 分组规范选定并补 `commands` 目录测试）。
- completion：plan/receipt id 补全。

## 5. 边界与红线

- `pipeline status/show` 严格只读，不写 vault/`.pinax/**`/远端。
- freshness 守卫只作用于**已保存 plan 的 apply**；内存态 preview → apply 单命令流程不受影响。
- 不聚合、不迁移存储；reader adapter 失败（损坏 plan 文件）fail-closed 报告该 plan unreadable，不影响其余条目。

## 6. 测试策略

- 单测：各管道 reader adapter 归一正确性；freshness 指纹判定（改一个 note ⇒ stale）。
- testscript e2e：plan 保存 → 改 vault → apply 被 `plan_stale` 拒 → `--allow-stale` 通过 → receipt 出现在 status；`pipeline show` 两种 id 形态。
- 事件合同：golden NDJSON（含 stage.failed）。
- 回归：无保存 plan 时 apply 流程与现状 golden 一致。

## 7. 风险与取舍

- **freshness 指纹成本**：与 organize 现行判定同型（扫描 note 事实摘要），对 metadata/repair 复用同一 facts 扫描 helper；大 vault 代价可接受（毫秒级指纹 vs apply 风险）。
- **归一层漂移风险**：view 只读投影，主 schema 变更靠 SourceSchema 透传 + 既有 plan schema 测试兜底。
- **publish/sync 深度整合推迟**：v1 只把它们的 receipt 纳入 status 聚合与事件合同，不改其专有 preview（sync 已有成熟 sync_view）。
