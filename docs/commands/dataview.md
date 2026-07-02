# dataview Command

`pinax dataview` runs a safe Dataview-compatible subset by lowering it to the Pinax query engine. It supports `TABLE`, `LIST`, and `TASK` forms with `FROM`, `WHERE`, `SORT`, `GROUP BY`, and `LIMIT`.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax dataview explain <query>` | Explain the lowered query plan. | Does not write. |
| `pinax dataview run <query>` | Run a Dataview-compatible query. | Does not write, unless explicit `--lazy-index` prepares the index. |

## Common Workflows

```bash
pinax dataview explain 'TABLE title, status FROM #pinax LIMIT 5' --vault ./my-notes --json
pinax dataview run 'TABLE title, status FROM #pinax WHERE status = "active" LIMIT 5' --lazy-index --vault ./my-notes --json
pinax dataview run 'TASK FROM "projects" WHERE completed = false LIMIT 10' --lazy-index --vault ./my-notes --json
```

## Boundaries

DataviewJS, dynamic functions, file reads, environment reads, network calls, and raw SQLite are not supported. Use `pinax query` for Pinax SQL and `pinax database view save --language dataview` to persist a Dataview query.
