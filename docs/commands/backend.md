# backend Command

`pinax backend` manages vault backend provider profiles and backend-specific sync actions. External platforms are providers, not the source of truth for notes.

## Subcommands

| Command | Purpose | Writes/External Effects |
| --- | --- | --- |
| `backend` / `backend list` / `backend ls` | List backend profiles; `ls` is a short alias for `list`. | Does not write. |
| `backend add <kind> <name>` | Add a backend profile, such as local, s3, or rclone. | Writes backend metadata; does not store secrets. |
| `backend show <name>` | View backend status. | Does not write. |
| `backend doctor <name>` | Diagnose backend configuration. | Does not write. |
| `backend capabilities <name>` | View backend capabilities. | Does not write. |
| `backend diff <name>` | Generate a dry-run sync plan. | Does not write to the remote. |
| `backend push <name>` | Execute backend push. | May write to the remote when `--yes` is used. |
| `backend pull <name>` | Execute backend pull. | May write locally when `--yes` is used. |
| `backend remove <name>` | Remove a backend profile. | Writes backend metadata. |
| `backend object list <name> [prefix]` | List objects for the specified backend. | Does not write. |
| `backend object stat <name> <key>` | View the status of the specified backend object. | Does not write. |

## Common Workflow

```bash
pinax backend add s3 work-s3 --bucket notes --region us-east-1 --profile work --vault ./my-notes
pinax backend ls --vault ./my-notes --json
pinax backend show work-s3 --vault ./my-notes
pinax backend doctor work-s3 --vault ./my-notes
pinax backend object list work-s3 pinax/ --vault ./my-notes --json
pinax backend object stat work-s3 pinax/manifest.json --vault ./my-notes --json
pinax backend diff work-s3 --vault ./my-notes --json
pinax backend push work-s3 --dry-run --vault ./my-notes --json
pinax backend push work-s3 --yes --vault ./my-notes
```

## Security Boundary

A backend profile may store a bucket, region, prefix, profile name, or rclone remote, but must not store raw access keys, secrets, cookies, or Authorization headers.
