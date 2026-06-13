# journal Command

`pinax journal` manages daily, weekly, and monthly journals. It is suitable for fixed-interval notes such as diaries, weekly notes, and monthly retrospectives.

## Subcommands

| Command | Purpose |
| --- | --- |
| `pinax journal daily open|show|append` | Create, read, or append to a daily note. |
| `pinax journal weekly open|show|append` | Create, read, or append to a weekly note. |
| `pinax journal monthly open|show|append` | Create, read, or append to a monthly note. |

## Common Workflows

```bash
pinax journal daily open --vault ./my-notes --editor "$EDITOR"
pinax journal daily append --body "Daily retrospective" --vault ./my-notes
pinax journal weekly show --vault ./my-notes --json
pinax journal monthly append --body "Monthly summary" --vault ./my-notes
```

## Compatible Aliases

The old root commands `daily`, `weekly`, and `monthly` remain compatible with scripts, but the primary path is `pinax journal daily|weekly|monthly`.
