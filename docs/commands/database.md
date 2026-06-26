# database Command

`pinax database` manages local note database views and property schemas. It is intended for scenarios where you need to reuse query results, table columns, or property type constraints.

Status: saved view render and property schema contracts are mature for local CLI/API use. Obsidian-style dataview compatibility remains preview because it intentionally supports a safe subset, not the full plugin language.

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
pinax database view save active-board --display board --query 'SELECT title, status FROM notes LIMIT 20' --board-column status --vault ./my-notes --json
pinax database view save due-calendar --display calendar --query 'SELECT title, due FROM notes LIMIT 20' --calendar-field due --vault ./my-notes --json
pinax database view list --vault ./my-notes
pinax database view show active --vault ./my-notes --json
pinax database view render active-dv --vault ./my-notes --json
pinax database schema infer --vault ./my-notes --json
pinax database schema set status --type select --values active,done,archived --vault ./my-notes
```

## Relationship with query

Use `pinax query explain`, `pinax query run`, `pinax dataview explain`, and `pinax dataview run` to debug queries first; once stable, save them with `pinax database view save`. New query-backed views are written with `schema_version: pinax.views.v3`; old v1/v2 view files remain readable.

## Render Displays And Tabs

`database view render` supports `table`, `board`, `list`, and `calendar` displays. Render output is bounded and includes optional `data.database_view`, `data.database_tab`, `fact.database.*`, and `fact.database_tab.*` fields while preserving the older `view`, `render`, `rows`, and `display` fields.

Saved views can also be embedded in Markdown as tabs:

````markdown
```pinax-database-view active-dv
```

```pinax-database-view due-calendar
```
````

Render the note through the shared projection service:

```bash
pinax note show "Dashboard" --view rendered --vault ./my-notes --json
```

The saved view registry stores view configuration only. It must not store result rows, dashboard layout state, or client active-tab selection.

## Remote Surfaces

The same database render projection is available through local REST/RPC, read-only MCP, dashboard database tabs, and remote CLI mode:

```bash
pinax api routes --vault ./my-notes --json
pinax --api-url http://127.0.0.1:8787 database view render active-dv --json
```

Remote handlers call the application service; they do not parse Markdown fences or `.pinax/views.json` directly.
