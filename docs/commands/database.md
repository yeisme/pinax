# database Command

`pinax database` manages local note database views and property schemas. It is intended for scenarios where you need to reuse query results, table columns, or property type constraints.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `database view save <name>` | Save a database view, either from filters or from `--query`. | Writes view metadata. |
| `database view list` | List database views. | Does not write. |
| `database view show <name>` | Show a database view. | Does not write. |
| `database view render <name>` | Render a saved database view. | Does not write. |
| `database view delete <name> --yes` | Delete a database view. | Writes view metadata. |
| `database schema infer` | Infer property schemas from existing notes. | Does not write. |
| `database schema set <property>` | Set a property type and allowed values. | Writes schema metadata. |

## Common Workflows

```bash
pinax database view save active --query 'SELECT title FROM notes WHERE status = "active"' --language sql --vault ./my-notes --json
pinax database view save active-dv --language dataview --query 'TABLE title, status FROM #pinax LIMIT 20' --group-by status --vault ./my-notes --json
pinax database view list --vault ./my-notes
pinax database view show active --vault ./my-notes --json
pinax database view render active-dv --vault ./my-notes --json
pinax database schema infer --vault ./my-notes --json
pinax database schema set status --type select --values active,done,archived --vault ./my-notes
```

## Relationship with query

Use `pinax query explain`, `pinax query run`, `pinax dataview explain`, and `pinax dataview run` to debug queries first; once stable, save them with `pinax database view save`. New query-backed views are written with `schema_version: pinax.views.v3`; old v1/v2 view files remain readable.
