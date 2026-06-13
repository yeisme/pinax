# import Command

`pinax import` imports external Markdown files or directories into a Pinax vault. The current main subcommand is `import markdown`.

## Usage

```bash
pinax import markdown ./source --dry-run --vault ./my-notes --json
pinax import markdown ./source --group research --kind reference --status active --conflict rename --yes --vault ./my-notes --json
pinax import markdown ./source/beta.md --conflict overwrite --yes --vault ./my-notes --json
```

## Key Parameters

| Parameter | Purpose |
| --- | --- |
| `--group`, `--folder`, `--kind`, `--status`, `--tags` | Add organizational metadata to imported notes. |
| `--conflict skip|rename|overwrite` | Controls target conflicts. Default is `skip`. |
| `--dry-run` | Only outputs the import plan; does not write to the vault. |
| `--yes` | Confirms execution of import writes. |

## Write Boundaries

`--dry-run` does not write notes, receipts, Git state, or provider state. A real import must explicitly use `--yes`, and writes `.pinax/receipts/import-*.json` through the service.
