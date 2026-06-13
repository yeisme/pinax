# export Command

`pinax export` exports a local Markdown bundle according to conditions. The current main subcommand is `export markdown`.

## Usage

```bash
pinax export markdown ./out --vault ./my-notes --json
pinax export markdown ./out --tag imported --vault ./my-notes --json
pinax export markdown ./out --group research --kind reference --status active --vault ./my-notes --json
```

## Filters

| Parameter | Purpose |
| --- | --- |
| `--tag` | Filter by tag. |
| `--group` | Filter by group. |
| `--folder` | Filter by folder. |
| `--kind` | Filter by purpose category. |
| `--status` | Filter by status. |

## Write Boundaries

Export writes to the output directory and `.pinax/receipts/export-*.json`; it does not modify the source Markdown note.
