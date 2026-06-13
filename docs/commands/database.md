# database Command

`pinax database` manages local note database views and property schemas. It is intended for scenarios where you need to reuse query results, table columns, or property type constraints.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `database view save <name>` | Save a database view, either from filters or from `--query`. | Writes view metadata. |
| `database view list` | List database views. | Does not write. |
| `database view show <name>` | Show a database view. | Does not write. |
| `database view delete <name> --yes` | Delete a database view. | Writes view metadata. |
| `database schema infer` | Infer property schemas from existing notes. | Does not write. |
| `database schema set <property>` | Set a property type and allowed values. | Writes schema metadata. |

## Common Workflows

```bash
pinax database view save active --query 'SELECT title FROM notes WHERE status = "active"' --vault ./my-notes --json
pinax database view list --vault ./my-notes
pinax database view show active --vault ./my-notes --json
pinax database schema infer --vault ./my-notes --json
pinax database schema set status --type select --values active,done,archived --vault ./my-notes
```

## Relationship with query

Use `pinax query explain` and `pinax query run` to debug SQL first; once it is stable, save it with `pinax database view save`.
