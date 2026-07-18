# activity Command

`pinax activity` reads unified vault activity across safe local logs. It is read-only and is meant for operators and agents that need one place to inspect recent vault events, monitor runs, sync runs, API audit entries, and record ledger activity.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax activity sources` | List supported activity sources. | No |
| `pinax activity list` | List recent activity entries. | No |
| `pinax activity show <event-id>` | Show one activity event. | No |
| `pinax activity tail` | Read the latest activity snapshot. | No |
| `pinax activity manage` | Summarize activity log maintenance. | No |

## Examples

```bash
pinax activity sources --vault ./my-notes --json
pinax activity list --source monitor_runs --status success --limit 20 --vault ./my-notes --json
pinax activity show event_abc123 --vault ./my-notes --json
pinax activity tail --vault ./my-notes --agent
pinax activity manage --vault ./my-notes --json
```

Activity output is redacted and bounded. It must not include secrets, raw provider payloads, hidden prompts, or full chain-of-thought.
