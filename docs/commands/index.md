# index Command

`pinax index` manages the local SQLite/GORM index projection. The Markdown vault remains the source of truth; the index is a rebuildable projection.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax index` | Summarize index status and recommend next steps. | Does not write. |
| `index status` | Check index status. | Does not write. |
| `index explain` | Explain projection status and safe next steps. | Does not write. |
| `index doctor` | Diagnose freshness, schema, corruption, or unreadable issues. | Does not write. |
| `index lookup <query>` | Look up note, asset, and adoptable file candidates. | Does not write. |
| `index refresh` | Maintain the index projection at low cost. | Writes the index database. |
| `index rebuild` | Fully reset and rebuild the index. | Rebuilds the index database. |
| `index sync` | Sync external changes to the index. | Writes the index database. |
| `index repair` | Projection-safe repair; can dry-run by default. | Writes the index with `--yes`. |
| `index init` | Initialize the index database. | Writes the index database. |
| `index page preview|create|refresh` | Preview, create, and refresh index pages. | create/refresh write Markdown managed pages. |

## Common Workflows

```bash
pinax index --vault ./my-notes
pinax index refresh --vault ./my-notes
pinax index doctor --vault ./my-notes --json
pinax index rebuild --vault ./my-notes --json
pinax index lookup diagram --scope all --vault ./my-notes --json
```

## SQLite Index and Review Index Pages

The SQLite/GORM index managed by `pinax index` is a structured projection of each note, containing fields such as `status`, `lifecycle_status`, `kind`, and `folder`. `lifecycle_status` is derived from `status`:

- `inbox`, `draft`, `active`, `archived`, and `discarded` map directly
- `status: system` + `kind: index` → `system`
- Other custom status values → `active`

`discarded` notes are excluded from `note list` and `search` by default, and can only be viewed with `--status discarded` or dedicated commands.

Review index pages are system notes with `kind: index` and `status: system`. They use built-in templates and managed blocks to generate refreshable navigation pages:

- `pinax inbox index preview|create|refresh`: inbox index page (uses the `index.inbox` template)
- `pinax draft index preview|create|refresh`: drafts index page (uses the `index.drafts` template)
- `pinax index page preview|create|refresh <name>`: general-purpose index page

Built-in review index templates: `index.inbox`, `index.drafts`, `index.decisions`, `index.learning`, `index.meetings`, `index.research`.

## Selection Rules

- missing or stale: prefer `index refresh`.
- schema incompatible, corrupt, or unreadable: run `index doctor` first, and `index rebuild` if necessary.
- Only checking candidate objects: use `index lookup`; do not inspect `.pinax/index` directly.
