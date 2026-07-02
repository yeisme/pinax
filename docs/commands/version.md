# version Command

`pinax version` manages vault version evidence. Git is only an optional backend; the user's primary path is `version status/snapshot/history/diff/show/restore/changed/backends`.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `version` | Outputs the Pinax version. | No writes. |
| `version status` | Checks the current vault version backend status. | No writes. |
| `version snapshot --message <msg>` | Creates local version snapshot evidence and content objects. | Writes `.pinax/version` evidence. |
| `version history` | Lists snapshot history. | No writes. |
| `version diff --base <rev> --target <rev>` | Reads a version difference summary. | No writes. |
| `version show <path> --revision <rev>` | Reads file content evidence by revision. | No writes. |
| `version restore <path> --revision <rev> --plan` | Generates a restore plan. | Does not write to the vault. |
| `version changed --since <rev>` | Reads candidate changed paths after a revision. | No writes. |
| `version backends` | Lists available version backends. | No writes. |

## Common Workflow

```bash
pinax version status --vault ./my-notes
SNAPSHOT_ID=$(pinax version snapshot --vault ./my-notes --message "Pre-organization snapshot" --json | jq -r '.facts.snapshot_id')
pinax version history --vault ./my-notes --json
pinax version diff --base rev_1 --target rev_2 --vault ./my-notes --json
pinax version restore notes/a.md --revision "$SNAPSHOT_ID" --plan --vault ./my-notes --json
```

The local backend stores snapshot content under `.pinax/version/objects/` and advertises `read_at_revision_supported=true`. Use the `snapshot_id` returned by `version snapshot` or `version history` as the restore revision.

## Compatibility Alias

The old `pinax git snapshot` still exists as a hidden compatibility alias; new documentation uses `pinax version snapshot`.
