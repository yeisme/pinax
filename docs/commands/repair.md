# repair Command

`pinax repair` generates maintenance plans from vault doctor issues and applies only low-risk fixes. It is suitable for turning health problems into reviewable, savable actions protected by snapshots.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax repair plan` | Generate a plan from doctor issues. | Does not write by default. |
| `pinax repair plan --save` | Save a repair plan. | Writes `.pinax/repair-plans/<plan_id>.json`. |
| `pinax repair list` | List saved repair plans. | No. |
| `pinax repair apply --plan <id> --yes` | Apply saved low-risk fixes. | Writes to the vault; requires snapshot protection. |
| `pinax repair apply --plan <id> --yes --allow-stale` | Apply a saved plan even when vault facts drifted after planning. | Writes to the vault; requires snapshot protection. |

## Common Workflow

```bash
pinax vault doctor --vault ./my-notes
pinax repair plan --vault ./my-notes --save --json
pinax repair list --vault ./my-notes --json
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes --snapshot-message "pre-repair snapshot"
```

## What Is Applied Automatically

`repair apply` only performs low-risk metadata, tags, index rebuild, and archive status fixes. Duplicate titles, broken links, ambiguous links, empty notes, and orphan notes only generate manual review items; it does not automatically delete, merge, or rewrite body content.

## Plan Freshness 与 --allow-stale（plan 新鲜度）

`repair apply --plan` 前会做 freshness 守卫：plan 记录的 facts 摘要与当前 vault 扫描指纹不一致（或 plan 已过期）时返回稳定错误 `plan_stale` 并提示重新 `pinax repair plan --save`，不会产生部分写入；拒绝时 `--events` 输出 `stage.failed`（`reason=plan_stale`）。

- `--allow-stale` 是显式逃生门：跳过 freshness 守卫照常 apply，投影附 `plan_stale_overridden` warning；`--yes` 与 snapshot 要求不变。
- 跨管道统一视图与 freshness 徽标见 [pipeline](./pipeline.md)。

## Difference from organize

`repair` starts from health issues; `organize` starts from structural organization suggestions. Both follow plan first, `--yes`, and snapshot protection.
