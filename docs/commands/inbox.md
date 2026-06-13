# inbox Command

`pinax inbox` is used to capture first, then organize later. It is suitable for temporary ideas, links, meeting fragments, and pending materials when you are not sure where they belong.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax inbox capture <title>` | Quickly create an inbox note. | Writes a Markdown note. |
| `pinax inbox list` | List inbox notes. | Does not write. |
| `pinax inbox triage <note>` | Organize an inbox note into a project, folder, kind, and status. | Writes note metadata/path. |
| `pinax inbox show <note>` | View inbox note content. | Does not write. |
| `pinax inbox promote <note> --to <draft\|active>` | Promote an inbox note to draft or active. | Writes note metadata, optionally moves path. |
| `pinax inbox discard <note> --yes` | Discard an inbox note (sets status=discarded, does not delete the file). | Writes note metadata. |
| `pinax inbox index preview` | Preview the inbox index page (using the index.inbox template). | Does not write. |
| `pinax inbox index create` | Create the inbox index page (kind: index, status: system). | Writes the system index page. |
| `pinax inbox index refresh` | Refresh the managed block of the inbox index page. | Writes the system index page. |

## Common Workflow

```bash
pinax inbox capture "Temporary idea" --body "Write it down first" --tags idea --vault ./my-notes
pinax inbox list --vault ./my-notes
pinax inbox triage "Temporary idea" --group work --folder ideas --kind reference --status active --vault ./my-notes

# Promote to drafts
pinax inbox promote "Temporary idea" --to draft --vault ./my-notes

# Promote directly to active
pinax inbox promote "Temporary idea" --to active --folder ideas --kind reference --vault ./my-notes

# Discard an inbox note that is no longer needed
pinax inbox discard "Temporary idea" --yes --vault ./my-notes

# View the inbox index
pinax inbox index preview --vault ./my-notes
pinax inbox index create --vault ./my-notes
```

## Status Transition Rules

Inbox notes can be promoted to the following statuses:
- `draft`: via `inbox promote --to draft`
- `active`: via `inbox promote --to active`
- `archived`: not directly supported; promote first, then archive
- `discarded`: via `inbox discard --yes`

## dry-run and Confirmation

- `inbox promote --dry-run`: only previews the status transition plan and does not modify files.
- `inbox discard --yes`: explicit confirmation is required before discarding.
- Remote API write operations are constrained by `--allow-write` and `yes=true`.

## Relationship with organize

`inbox triage` is explicit organization for a single note; use `pinax organize plan --save` for batch structural organization.
