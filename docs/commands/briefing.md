# briefing Command

`pinax briefing` manages daily trending notes briefing. It is responsible for recipes, candidate generation, and delivery surfaces, but providers, research, and delivery must all be executed through the CLI/service boundary.

## Subcommands

| Command | Purpose | Writes/External Effects |
| --- | --- | --- |
| `briefing recipe init` | Create the default briefing recipe. | Writes recipe asset. |
| `briefing recipe show` | View the recipe. | No writes. |
| `briefing recipe set` | Update topic, limit, or source. | Writes recipe asset. |
| `briefing run --dry-run` | Generate only a candidate preview. | Does not write to the vault or deliver. |
| `briefing run --yes` | Run briefing and write candidate notes. | Writes to the vault. |
| `briefing deliver feishu --dry-run` | Preview the Feishu delivery receipt. | Does not send an HTTP POST. |
| `briefing deliver feishu --yes` | Deliver to the Feishu webhook. | Has remote writes. |

## Common Workflow

```bash
pinax briefing recipe init --topic "AI" --limit 10 --vault ./my-notes
pinax briefing recipe show --vault ./my-notes --json
pinax briefing run --dry-run --vault ./my-notes --json
pinax briefing run --yes --vault ./my-notes
pinax briefing deliver feishu --webhook "$PINAX_FEISHU_WEBHOOK" --title "Today's briefing" --text "Content" --secret-ref env://PINAX_FEISHU_WEBHOOK --dry-run --vault ./my-notes --json
```

## Security Boundary

Do not write webhook URLs, tokens, cookies, or raw provider payloads to stdout, fixtures, or documentation examples. Real delivery must explicitly use `--yes`.
