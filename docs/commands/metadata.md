# metadata Command

`pinax metadata` is used to complete note frontmatter metadata. It is narrower than `organize`: it only handles metadata planning and application, and does not organize the file structure.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax metadata plan [query]` | Preview the metadata completion plan. | Does not write. |
| `pinax metadata apply --yes` | Apply the metadata completion plan. | Writes Markdown frontmatter. |

## Common Workflow

```bash
pinax metadata plan --vault ./my-notes --json
pinax metadata plan "research" --vault ./my-notes
pinax metadata apply --vault ./my-notes --yes
```

## Selection Rules

- To complete only frontmatter such as `kind`, `status`, and tags: use `metadata`.
- To move scattered files or organize structure in bulk: use `organize`.
- To generate maintenance actions from health issues: use `repair`.
