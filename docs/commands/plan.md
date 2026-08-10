# plan Command

`pinax plan` manages local daily, weekly, and monthly planning, project-board context, snapshots, and reviewable action drafts. It does not require an external task runtime or provider account.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax plan daily` | Generate a local daily plan. | Writes plan outputs when `--yes` is used; `--save --yes` also stores a snapshot. |
| `pinax plan daily --task-review` | Refresh the local daily task-review managed block. | Writes only with `--yes`. |
| `pinax plan weekly` | Generate a local weekly plan. | Writes plan outputs when `--yes` is used; `--save --yes` also stores a snapshot. |
| `pinax plan monthly` | Generate a local monthly plan. | Writes plan outputs when `--yes` is used; `--save --yes` also stores a snapshot. |
| `pinax plan actions` | Generate Pinax-owned planning action drafts. | Writes drafts when `--save` is used. |
| `pinax plan snapshot` | Generate a local plan snapshot. | Writes a plan snapshot. |

## Common Workflows

```bash
pinax plan daily --dry-run --vault ./my-notes --json
pinax plan daily --yes --vault ./my-notes
pinax plan daily --task-review --yes --vault ./my-notes --json
pinax plan daily --save --yes --vault ./my-notes --json
pinax plan weekly --save --yes --vault ./my-notes
pinax plan actions --from daily --save --vault ./my-notes --json
pinax plan snapshot --vault ./my-notes --json
```

## Local task review

`pinax plan daily --task-review` summarizes tasks from the local project board into the `daily-task-review` managed block. Without `--yes`, it previews the replacement and leaves the vault unchanged. With `--yes`, it updates only that managed block and preserves user-authored text around it.

## Action drafts

`pinax plan actions --from daily --save --vault <vault>` writes a Pinax-owned `pinax.planning.actions.v1` draft under `.pinax/planning/actions/`. The draft is review evidence only; Pinax does not execute it or delegate it to an external task service.

## Boundaries

Planning commands read local vault and project-board projections, write through Pinax application services, and keep the existing `--dry-run`, `--yes`, and `--save` approval boundaries.
