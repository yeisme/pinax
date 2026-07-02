# task Command

`pinax task` manages inferred and adopted tasks found in Markdown project boards. Pinax does not become an external task tracker; it writes only local task adoption evidence when explicitly confirmed.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax task adopt <item> --plan` | Preview adoption for an inferred checklist task. | No |
| `pinax task adopt <item> --yes` | Write a local task adoption ledger. | `.pinax/task-adoptions/<item>.json` |

## Examples

```bash
pinax project board show research --vault ./my-notes --json
pinax task adopt task_abc123 --plan --vault ./my-notes --json
pinax task adopt task_abc123 --yes --vault ./my-notes --json
```

Use `--plan` first when an item comes from a Markdown checklist line. The confirmed command writes the ledger through the application service; agents must not hand-write `.pinax/task-adoptions/**`.
