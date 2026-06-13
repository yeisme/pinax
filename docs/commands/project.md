# project Command

`pinax project` manages project partitions within a vault. Projects are used to set default organization prefixes, names, and descriptions for notes.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax project create <slug>` | Create a vault project. | Writes project configuration. |
| `pinax project list` | List vault projects. | Does not write. |
| `pinax project switch <slug>` | Switch the current vault project. | Writes current project state. |

## Common Workflow

```bash
pinax project create research --name "Research" --notes-prefix notes/research --vault ./my-notes
pinax project list --vault ./my-notes --json
pinax project switch research --vault ./my-notes
pinax note add "Research Log" --project research --vault ./my-notes
```

## When Not to Use

If you only want to temporarily place a note in a directory, `pinax note add --dir <dir>` or `pinax note move <note> <dir>` is enough.
