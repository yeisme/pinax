# monitor Command

`pinax monitor` reads local command performance monitor traces. It helps inspect command duration, status, evidence paths, and maintenance state without rebuilding indexes or mutating the vault.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax monitor runs` | List monitor runs. | No |
| `pinax monitor show <run-id>` | Show one monitor run. | No |
| `pinax monitor tail` | Read the latest monitor runs. | No |
| `pinax monitor summary` | Summarize monitor runs. | No |
| `pinax monitor manage` | Summarize monitor log maintenance. | No |

## Examples

```bash
pinax monitor runs --command note.search --status success --limit 20 --vault ./my-notes --json
pinax monitor show run_abc123 --vault ./my-notes --json
pinax monitor tail --vault ./my-notes --agent
pinax monitor summary --vault ./my-notes --json
pinax monitor manage --vault ./my-notes --json
```

Monitor traces are operational evidence, not user notes. They are read through the CLI/service boundary and rendered through the standard Pinax projection modes.
