# plan Command

`pinax plan` manages the personal planning workflow, including daily, weekly, and monthly plans, TaskBridge action drafts, and plan snapshots.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax plan daily` | Generate a daily plan. | Writes plan outputs when `--yes` is used; `--save --yes` also stores a snapshot. |
| `pinax plan weekly` | Generate a weekly plan. | Writes plan outputs when `--yes` is used; `--save --yes` also stores a snapshot. |
| `pinax plan monthly` | Generate a monthly plan. | Writes plan outputs when `--yes` is used; `--save --yes` also stores a snapshot. |
| `pinax plan actions` | Generate TaskBridge action drafts. | Writes drafts when `--save` is used. |
| `pinax plan snapshot` | Generate a plan snapshot. | Writes a plan snapshot. |

## Common Workflows

```bash
pinax plan daily --dry-run --vault ./my-notes --json
pinax plan daily --taskbridge --dry-run --vault ./my-notes --json
pinax plan daily --taskbridge --yes --vault ./my-notes
pinax plan daily --taskbridge --save --yes --vault ./my-notes --json
pinax plan weekly --save --yes --vault ./my-notes
pinax plan actions --from daily --taskbridge --save --vault ./my-notes --json
pinax plan snapshot --vault ./my-notes --json
```

## TaskBridge Daily Markdown

`pinax plan daily --taskbridge --dry-run --json` reads `taskbridge agent today` and returns the selected commitments, `captured_at`, target daily note, and `planning-daily` managed block facts without writing the vault.

`pinax plan daily --taskbridge --yes --vault <vault>` creates or updates `daily/YYYY-MM-DD.md` and writes only the `planning-daily` managed block. `--save` additionally stores the planning snapshot under `.pinax/planning/snapshots/`.

`pinax plan actions --from daily --taskbridge --save --vault <vault>` generates a `taskbridge.actions.v1` draft from TaskBridge deferred candidates. Pinax does not execute the action; the next step remains a TaskBridge dry-run command.

TaskBridge remains the task source of truth. Pinax owns the Markdown vault write and does not require TaskBridge to export Markdown files for this workflow.

## Boundaries

Plan commands generate local plan assets or action drafts, and should not bypass the CLI to hand-write TaskBridge metadata.
