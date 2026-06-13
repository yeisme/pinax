# draft Command

`pinax draft` manages draft box notes. A draft is a Markdown note with status=draft, stored in the `drafts/` directory by default.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax draft create <title>` | Create a draft note (status=draft). | Writes a Markdown note. |
| `pinax draft list` | List draft notes. | Does not write. |
| `pinax draft show <note>` | View draft note content. | Does not write. |
| `pinax draft promote <note>` | Promote a draft to active (default) or a specified status. | Writes note metadata; optionally moves the path. |
| `pinax draft archive <note> --yes` | Archive a draft (sets status=archived). | Writes note metadata. |
| `pinax draft discard <note> --yes` | Discard a draft (sets status=discarded; does not delete the file). | Writes note metadata. |
| `pinax draft index preview` | Preview the draft box index page (using the index.drafts template). | Does not write. |
| `pinax draft index create` | Create the draft box index page (kind: index, status: system). | Writes a system index page. |
| `pinax draft index refresh` | Refresh the managed block of the draft box index page. | Writes a system index page. |

## Common Workflows

```bash
# Create a draft
pinax draft create "Article Ideas" --body "Initial thoughts" --tags writing --vault ./my-notes

# List all drafts
pinax draft list --vault ./my-notes

# View draft content
pinax draft show "Article Ideas" --vault ./my-notes

# Promote to active, specifying folder and kind
pinax draft promote "Article Ideas" --status active --folder articles --kind article --yes --vault ./my-notes

# Preview only the promotion plan
pinax draft promote "Article Ideas" --dry-run --vault ./my-notes

# Archive a draft
pinax draft archive "Article Ideas" --yes --vault ./my-notes

# Discard an unneeded draft
pinax draft discard "Article Ideas" --yes --vault ./my-notes

# View the draft box index
pinax draft index preview --vault ./my-notes
pinax draft index create --vault ./my-notes
```

## Status Transition Rules

Draft notes can be promoted to the following statuses:
- `active`: via `draft promote --status active` (default)
- `archived`: via `draft archive --yes`
- `discarded`: via `draft discard --yes`

Returning from draft to inbox is not supported. Restoring archived and discarded notes is not supported in the initial version.

## dry-run and Confirmation

- `draft promote --dry-run`: Preview only the status transition plan; does not modify files.
- `draft archive --yes` and `draft discard --yes`: Explicit confirmation is required.
- Remote API write operations are constrained by `--allow-write` and `yes=true`.

## Relationship to the note Command

- `pinax draft create <title>` is equivalent to `pinax note add <title> --status draft --folder drafts`.
- `pinax draft list` is equivalent to `pinax note list --status draft --sort updated`.
- The legacy `note add --status draft` and `note list --status draft` remain available.
