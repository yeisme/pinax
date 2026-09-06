# pipeline 命令

`pinax pipeline` 是统一管道交互面：一处看全所有 plan/apply 型管道的 pending 计划与最近 apply 记录。它覆盖 organize、metadata、repair、restore 四条计划管道与 sync、publish、proof loop 的 apply 记录，把原本分散在多棵命令树里的状态聚合成一个只读视图。

两个子命令都严格只读：不写 vault、不写 `.pinax/**`、不触远端，也不迁移或重命名任何既有 plan 存储与 receipt schema。

## 子命令

| 命令 | 用途 | 写入 |
| --- | --- | --- |
| `pinax pipeline status` | 聚合 pending saved plans（含 freshness 徽标）与最近 N 条 apply 型 receipt。 | 无写入。 |
| `pinax pipeline status --kind <csv>` | 只看指定管道（`organize,metadata,repair,restore,sync,publish,proof_loop`）。 | 无写入。 |
| `pinax pipeline status --limit <N>` | 限制最近 receipt 条数（默认 10，上限 50）。 | 无写入。 |
| `pinax pipeline show <plan-id\|receipt-id>` | 统一检视单条 plan 或 receipt 的详情。 | 无写入。 |

## 常用流程

```bash
pinax pipeline status --vault ./my-notes
pinax pipeline status --vault ./my-notes --kind organize,repair --json
pinax pipeline status --vault ./my-notes --agent
pinax pipeline show organize-abc123 --vault ./my-notes
pinax pipeline show apply-9f2c1d --vault ./my-notes --json
```

输出合同照常支持 `--json` / `--agent` / `--events` / `--explain`；`--events` 模式下 status/show 只有 start/end 两个 envelope 事件（阶段事件属于 apply 型命令）。

## status 视图

- **Pending plans**：四条管道各自存储（`.pinax/organize-plans`、`.pinax/metadata-plans`、`.pinax/repair-plans`、`.pinax/restore-plans`）里的已保存计划，经 `pinax.plan.v1` 读模型归一（plan_id、kind、原 schema_version、created_at、操作计数、facts 摘要、freshness 派生），每条附下一步 apply 命令提示。
- **Recent runs**：最近 N 条 apply 型 receipt，来源包括 `.pinax/receipts/`（`pinax.apply_receipt.v1` 的 organize/metadata/repair apply，`pinax.receipt.v1` 的 restore 与 proof loop）、`.pinax/sync-runs/`（sync push/pull）与 `.pinax/publish/runs/`（publish build）。
- **Freshness 徽标**：plan 生成后 vault 事实已变化（或已过期）即标记 `STALE` 并给出原因（如 `plan_stale:vault_changed`）；判定与各管道 apply 的守卫是同一套逻辑，不会出现"status 显示 fresh 但 apply 拒绝"的漂移。
- **损坏 plan 文件**：单条 plan 不可解析时以 `unreadable` 条目 fail-closed 呈现（附修复提示），命令状态转为 `partial`，其余条目照常列出。

## show 双形态

- **plan 形态**：操作按风险分组（vault 结构写入 `vault_write`、元数据写入 `metadata_write`、人工复核 `manual_review`），附 changed paths（走既有 path redaction）、freshness、facts 摘要与下一步 apply 命令。
- **receipt 形态**：展示 applied 事实（命令、状态、changed paths、ledger 序号、snapshot/ledger 引用），receipt id 可用 `apply-*`、sync run id、publish run id 或 restore/proof loop 收据 id。
- **未知 id**：返回稳定错误 `pipeline_id_not_found` 并提示用 `pipeline status` 列出可用 id，不做就近猜测。

## 统一 freshness 守卫与 --allow-stale

对已保存 plan 的 apply（`organize apply --plan`、`metadata apply --plan`、`repair apply --plan`、`version restore apply --plan`），当 plan 记录的 facts 摘要与当前 vault 扫描指纹不一致时拒绝执行，返回稳定错误 `plan_stale`（restore 为 `restore_plan_stale`）并提示重新 plan：

```text
Error: plan_stale — vault changed after this plan was generated.
Re-run `pinax metadata plan --save` or pass --allow-stale to apply anyway.
```

- `--allow-stale` 是显式逃生门：跳过 freshness 守卫照常 apply，投影附 `plan_stale_overridden` warning 与 `allow_stale=true` fact；其余守卫（`--yes`、snapshot 要求）不受影响。
- 内存态 preview → apply 单命令流程（不带 `--plan`）不受影响，行为与既往一致。
- 拒绝时 `--events` 输出 `stage.failed` 事件（`reason=plan_stale`）。

## 阶段事件合同（pinax.pipeline.stage.v1）

apply 型命令（organize/metadata/repair/restore apply、sync push/pull、publish build/deploy、proof loop run）通过共享 helper 在 `--events` NDJSON 流中按序输出：

```json
{"type":"stage.started","pipeline":"organize","plan_id":"organize-7f3a","stage":"apply"}
{"type":"stage.completed","pipeline":"organize","plan_id":"organize-7f3a","stage":"apply","counts":{"applied":12}}
{"type":"stage.failed","pipeline":"metadata","plan_id":"metadata-9c1d","stage":"apply","reason":"plan_stale"}
```

每条事件带 `schema_version=pinax.pipeline.stage.v1`、pipeline kind、plan_id/run id、stage 名与计数。既有事件一律保留原名原义；新事件为 additive，消费者可忽略未知 type。

## 与其他命令的关系

- 各管道的 `plan`/`apply`/`list` 命令仍是唯一写入面；`pipeline` 只读聚合。
- 需要看事件时间线（含非 apply 事件）时用 `pinax activity`；需要看 sync run 明细时用 `pinax sync logs show <run_id>`。
