# record Command

`pinax record` manages the vault record ledger. It is used to register notes as trackable records and to view the history summary of a single record.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax record init` | Initialize the record ledger. | Writes `.pinax/` ledger assets. |
| `pinax record status` | View ledger status. | Does not write. |
| `pinax record adopt [query]` | Register existing Markdown notes into the ledger. | Writes ledger records. |
| `pinax record history <query>` | View a record history summary by note ref. | Does not write. |

## Common Workflow

```bash
pinax record init --vault ./my-notes
pinax record status --vault ./my-notes
pinax record adopt "Research Log" --vault ./my-notes --json
pinax record history "Research Log" --vault ./my-notes
```

## Notes

The record ledger is a structured asset managed by the CLI/service. Do not hand-write ledger metadata under `.pinax/`.
