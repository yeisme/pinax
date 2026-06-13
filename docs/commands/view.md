# view Command

`pinax view` manages saved note retrieval views. It is suitable for saving commonly used filter criteria under a name, such as active work, a specific project, or a certain type of reference.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax view save <name>` | Save a set of note filter criteria. | Writes saved view metadata. |
| `pinax view list` | List saved views. | Does not write. |
| `pinax view show <name>` | Retrieve notes using a saved view. | Does not write. |
| `pinax view delete <name>` | Delete a saved view. | Writes saved view metadata. |

## Common Workflow

```bash
pinax view save active-work --group work --status active --kind reference --sort updated --vault ./my-notes --json
pinax view list --vault ./my-notes
pinax view show active-work --vault ./my-notes --json
pinax view delete active-work --vault ./my-notes --yes
```

## Difference from database view

`pinax view` saves note filter criteria; `pinax database view` can save Pinax SQL queries and display columns.
