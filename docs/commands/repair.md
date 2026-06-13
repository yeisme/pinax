# repair Command

`pinax repair` generates maintenance plans from vault doctor issues and applies only low-risk fixes. It is suitable for turning health problems into reviewable, savable actions protected by snapshots.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax repair plan` | Generate a plan from doctor issues. | Does not write by default. |
| `pinax repair plan --save` | Save a repair plan. | Writes `.pinax/repair-plans/<plan_id>.json`. |
| `pinax repair apply --plan <id> --yes` | Apply saved low-risk fixes. | Writes to the vault; requires snapshot protection. |

## Common Workflow

```bash
pinax vault doctor --vault ./my-notes
pinax repair plan --vault ./my-notes --save --json
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes --snapshot-message "pre-repair snapshot"
```

## What Is Applied Automatically

`repair apply` only performs low-risk metadata, tags, index rebuild, and archive status fixes. Duplicate titles, broken links, ambiguous links, empty notes, and orphan notes only generate manual review items; it does not automatically delete, merge, or rewrite body content.

## Difference from organize

`repair` starts from health issues; `organize` starts from structural organization suggestions. Both follow plan first, `--yes`, and snapshot protection.
