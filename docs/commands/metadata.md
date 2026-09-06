# metadata Command

`pinax metadata` is used to complete note frontmatter metadata. It is narrower than `organize`: it only handles metadata planning and application, and does not organize the file structure.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax metadata plan [query]` | Preview the metadata completion plan. | Does not write. |
| `pinax metadata plan --save` | Save the plan to `.pinax/metadata-plans/<plan_id>.json`. | Writes the saved plan only. |
| `pinax metadata plan --trust-fields [--stale-after <rfc3339>]` | Include `trust_fields` backfill operations (missing `generated`, and missing `stale_after` when `--stale-after` is given). | Does not write. |
| `pinax metadata apply --yes` | Apply the metadata completion plan (fresh in-memory scan). | Writes Markdown frontmatter. |
| `pinax metadata apply --plan <id> --yes` | Apply a saved plan exactly as reviewed; rejected as `plan_stale` when the vault changed since planning (pass `--allow-stale` to apply anyway). | Writes Markdown frontmatter. |
| `pinax metadata apply --trust-fields --yes` | Apply trust field backfill with the same approval gate. | Writes Markdown frontmatter. |

## Common Workflow

```bash
pinax metadata plan --vault ./my-notes --json
pinax metadata plan "research" --vault ./my-notes
pinax metadata plan --trust-fields --stale-after 2026-12-01T00:00:00+00:00 --vault ./my-notes --json
pinax metadata plan --vault ./my-notes --save --json
pinax metadata apply --vault ./my-notes --plan metadata-abc123 --yes
pinax metadata apply --vault ./my-notes --yes
pinax metadata apply --trust-fields --stale-after 2026-12-01T00:00:00+00:00 --vault ./my-notes --yes
```

<<<<<<< HEAD
## Trust Field Backfill

`--trust-fields` is an explicit opt-in: the default plan/apply output is unchanged. The backfill fills a missing `generated: {by, at}` (by `agent:pinax/<version>`, at the note `created_at` when valid) and, with `--stale-after`, a missing `stale_after`. Existing trust fields are never overwritten, writes reuse the `--yes` approval gate, record events, receipts, and the atomic YAML-node patcher, and there is no batch injection without these explicit commands.

||||||| a62af0b
=======
## Plan Freshness（plan 新鲜度）

已保存的 metadata plan（`pinax.metadata_plan.v1`）记录生成时的 vault 指纹。`apply --plan` 前会做 freshness 守卫：plan 过期或 vault 在 plan 之后发生变化时返回稳定错误 `plan_stale` 并提示重新 `pinax metadata plan --save`，不会产生部分写入。

- `--allow-stale` 是显式逃生门：跳过 freshness 守卫照常 apply，投影附 `plan_stale_overridden` warning；`--yes` 确认语义不变。
- 不带 `--plan` 的单命令流程不受影响，仍在 apply 时重新扫描。
- 拒绝时 `--events` 输出 `stage.failed`（`reason=plan_stale`）。
- 跨管道统一视图见 [pipeline](./pipeline.md)。

>>>>>>> wt/pipeline-unified-ux
## Selection Rules

- To complete only frontmatter such as `kind`, `status`, and tags: use `metadata`.
- To move scattered files or organize structure in bulk: use `organize`.
- To generate maintenance actions from health issues: use `repair`.
