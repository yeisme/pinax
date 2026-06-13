# storage Command

`pinax storage` configures the vault storage backend. The storage configuration describes a local or S3 backend profile and does not store provider secrets.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `storage set local` | Configure a local storage backend. | Writes the storage profile. |
| `storage set s3` | Configure an S3 storage backend. | Writes the storage profile; does not connect to S3. |
| `storage status` | View storage backend status. | Does not write. |
| `storage doctor` | Diagnose storage backend configuration. | Does not write. |

## Common Workflows

```bash
pinax storage set local --root ./my-notes --vault ./my-notes
pinax storage set s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json
pinax storage status --vault ./my-notes
pinax storage doctor --vault ./my-notes --json
```

## Compatibility Aliases

The old `storage set-local` and `storage set-s3` remain compatible, but the primary path uses `storage set local|s3`.
