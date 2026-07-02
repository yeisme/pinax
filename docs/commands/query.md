# query Command

`pinax query` runs controlled Pinax SQL against the local notes database. It is used for more precise tabular queries, debugging template query-backed rendering, and designing database views.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax query explain <sql>` | Explain the query plan and safety boundaries. | Does not write. |
| `pinax query run <sql>` | Run a query. | Does not write, unless explicit `--lazy-index` triggers index preparation. |

## Common Workflows

```bash
pinax index status --vault ./my-notes
pinax query explain 'SELECT title FROM notes LIMIT 20' --vault ./my-notes
pinax query run 'SELECT title, status FROM notes WHERE status = "active" LIMIT 20' --vault ./my-notes --json
pinax query run 'SELECT status, COUNT(*) AS total FROM notes GROUP BY status LIMIT 20' --lazy-index --vault ./my-notes --json
pinax query run 'SELECT title FROM notes LIMIT 20' --lazy-index --vault ./my-notes --json
```

Supported sources include `notes`, `tasks`, `links`, `backlinks`, and `assets`. The query layer supports bounded comparison operators, `IN`, `EXISTS`, `IS EMPTY`, `COUNT`, `MIN`, `MAX`, `GROUP BY`, sorting, limits, and cursor pagination.

## Parameters

| Parameter | Subcommand | Purpose |
| --- | --- | --- |
| `--sort` | `run` | Sort by property. |
| `--limit` | `run` | Limit the number of returned results. |
| `--cursor` | `run` | Pagination cursor. |
| `--lazy-index` | `run` | Allow explicit lazy index loading. |

## Boundaries

Pinax SQL goes through the controlled query layer. Business logic or documentation should not encourage directly hand-writing SQLite files or bypassing the repository.
