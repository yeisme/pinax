# plan Command

`pinax plan` manages the personal planning workflow, including daily, weekly, and monthly plans, TaskBridge action drafts, and plan snapshots.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax plan daily` | Generate a daily plan. | Writes plan assets when `--save` or `--yes` is used. |
| `pinax plan weekly` | Generate a weekly plan. | Writes plan assets when `--save` or `--yes` is used. |
| `pinax plan monthly` | Generate a monthly plan. | Writes plan assets when `--save` or `--yes` is used. |
| `pinax plan actions` | Generate TaskBridge action drafts. | Writes drafts when `--save` is used. |
| `pinax plan snapshot` | Generate a plan snapshot. | Writes a plan snapshot. |

## Common Workflows

```bash
pinax plan daily --dry-run --vault ./my-notes --json
pinax plan daily --taskbridge --save --vault ./my-notes
pinax plan weekly --save --vault ./my-notes
pinax plan actions --from daily --save --vault ./my-notes
pinax plan snapshot --vault ./my-notes --json
```

## Boundaries

Plan commands generate local plan assets or action drafts, and should not bypass the CLI to hand-write TaskBridge metadata.
