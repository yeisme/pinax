# metadata Command

`pinax metadata` is used to complete note frontmatter metadata. It is narrower than `organize`: it only handles metadata planning and application, and does not organize the file structure.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax metadata plan [query]` | Preview the metadata completion plan. | Does not write. |
| `pinax metadata plan --save` | Save the plan to `.pinax/metadata-plans/<plan_id>.json`. | Writes the saved plan only. |
| `pinax metadata apply --yes` | Apply the metadata completion plan (fresh in-memory scan). | Writes Markdown frontmatter. |
| `pinax metadata apply --plan <id> --yes` | Apply a saved plan exactly as reviewed. | Writes Markdown frontmatter. |

## Common Workflow

```bash
pinax metadata plan --vault ./my-notes --json
pinax metadata plan --vault ./my-notes --save --json
pinax metadata apply --vault ./my-notes --plan metadata-abc123 --yes
pinax metadata apply --vault ./my-notes --yes
```

## Plan Freshness（plan 新鲜度）

已保存的 metadata plan（`pinax.metadata_plan.v1`）记录生成时的 vault 指纹。`apply --plan` 前会做 freshness 守卫：plan 过期或 vault 在 plan 之后发生变化时返回稳定错误 `plan_stale` 并提示重新 `pinax metadata plan --save`，不会产生部分写入。

- `--allow-stale` 是显式逃生门：跳过 freshness 守卫照常 apply，投影附 `plan_stale_overridden` warning；`--yes` 确认语义不变。
- 不带 `--plan` 的单命令流程不受影响，仍在 apply 时重新扫描。
- 拒绝时 `--events` 输出 `stage.failed`（`reason=plan_stale`）。
- 跨管道统一视图见 [pipeline](./pipeline.md)。

## Selection Rules

- To complete only frontmatter such as `kind`, `status`, and tags: use `metadata`.
- To move scattered files or organize structure in bulk: use `organize`.
- To generate maintenance actions from health issues: use `repair`.
