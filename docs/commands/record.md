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

## Identity audit and migration

`record identity` 是旧 vault 进入 identity-first 内核的显式迁移控制面。audit 只读；plan 由 CLI 保存；apply 需要 `--yes`，会先创建 snapshot，并支持 resume/restore handoff。

```bash
pinax record identity audit --vault ./my-notes --json
pinax record identity plan --save --vault ./my-notes --json
pinax record identity apply --plan identity-plan-<id> --vault ./my-notes --yes --json
pinax record identity apply --plan identity-plan-<id> --vault ./my-notes --yes --resume --json
```

ledger `object_id` 是权威身份，frontmatter `note_id` 是镜像。missing、legacy、duplicate、frontmatter mismatch、one-ID-many-paths 和 path collision 都必须先进入 audit/plan，不能由读命令静默修复。
