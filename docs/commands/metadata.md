# metadata Command

`pinax metadata` is used to complete note frontmatter metadata. It is narrower than `organize`: it only handles metadata planning and application, and does not organize the file structure.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax metadata plan [query]` | Preview the metadata completion plan. | Does not write. |
| `pinax metadata plan --trust-fields [--stale-after <rfc3339>]` | Include `trust_fields` backfill operations (missing `generated`, and missing `stale_after` when `--stale-after` is given). | Does not write. |
| `pinax metadata apply --yes` | Apply the metadata completion plan. | Writes Markdown frontmatter. |
| `pinax metadata apply --trust-fields --yes` | Apply trust field backfill with the same approval gate. | Writes Markdown frontmatter. |

## Common Workflow

```bash
pinax metadata plan --vault ./my-notes --json
pinax metadata plan "research" --vault ./my-notes
pinax metadata plan --trust-fields --stale-after 2026-12-01T00:00:00+00:00 --vault ./my-notes --json
pinax metadata apply --vault ./my-notes --yes
pinax metadata apply --trust-fields --stale-after 2026-12-01T00:00:00+00:00 --vault ./my-notes --yes
```

## Trust Field Backfill

`--trust-fields` is an explicit opt-in: the default plan/apply output is unchanged. The backfill fills a missing `generated: {by, at}` (by `agent:pinax/<version>`, at the note `created_at` when valid) and, with `--stale-after`, a missing `stale_after`. Existing trust fields are never overwritten, writes reuse the `--yes` approval gate, record events, receipts, and the atomic YAML-node patcher, and there is no batch injection without these explicit commands.

## Selection Rules

- To complete only frontmatter such as `kind`, `status`, and tags: use `metadata`.
- To move scattered files or organize structure in bulk: use `organize`.
- To generate maintenance actions from health issues: use `repair`.
