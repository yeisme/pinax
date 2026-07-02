# asset Command

`pinax asset` manages images, documents, and binary assets in a vault. The asset manifest is written by the CLI/service; do not hand-write `.pinax/assets/manifest.json`.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `asset add <file>` | Add a file to the vault asset manifest. | Writes the asset file and manifest. |
| `asset list` | List assets. | Does not write. |
| `asset show <asset>` | View asset details and path display. | Does not write. |
| `asset link <asset>` | Generate a link plan from an asset to a note. | Does not write. |
| `asset backlinks <asset>` | List notes that reference this asset. | Does not write. |
| `asset move <asset> <target> --plan` | Generate a move and reference rewrite plan. | Does not write. |
| `asset remove <asset> --plan` | Generate a deletion or reference review plan. | Does not write. |
| `asset orphans` | List assets not referenced by any note. | Does not write. |
| `asset missing` | List asset references that point to missing files. | Does not write. |
| `asset repair --plan` | Generate an asset repair plan. | Does not write. |
| `asset preview <asset>` | Read-only preview of a single asset. | Does not write. |
| `asset verify` | Verify file hashes in the manifest. | Does not write. |

## Common Workflows

```bash
pinax asset add ./diagram.png --vault ./my-notes --json
pinax asset list --vault ./my-notes --agent
pinax asset show diagram.png --path-style markdown --vault ./my-notes --json
pinax asset link diagram.png --note "authentication plan" --vault ./my-notes --json
pinax note attach "authentication plan" ./diagram.png --placement note-folder --embed --vault ./my-notes --json
pinax note attachments "authentication plan" --include-paths --vault ./my-notes --json
pinax asset missing --vault ./my-notes --json
pinax asset repair --plan --vault ./my-notes --json
pinax asset verify --vault ./my-notes --json
```

## Obsidian Compatibility

Pinax recognizes common Markdown and Obsidian attachment references, including Markdown image links and wiki embeds. `asset backlinks`, `asset missing`, `asset orphans`, and `asset repair --plan` are read-only inspection or plan commands; they do not rewrite note bodies, move files, or delete files.

The asset manifest remains Pinax-owned under `.pinax/assets/manifest.json`. Use `pinax asset add` or `pinax note attach` to register assets instead of editing the manifest by hand. Assets and attachments that already exist in `assets/`, `attachments/`, or note-local asset folders can still be discovered by index-backed commands and reviewed through `asset missing` or `asset orphans`.

Repair is intentionally review-first for Obsidian vaults. Missing attachment references and orphan assets produce a plan with evidence; applying destructive or body-rewriting changes requires an explicit reviewed flow instead of an automatic rewrite.

## Boundaries

Currently, move, remove, and repair are all plan-first and do not provide a direct write path without a plan.

Use [`note attach`](./note.md) when the workflow needs both an asset copy/register operation and a note reference update in one command. Use [`prompt`](./prompt.md), [`collection`](./collection.md), and [`graph`](./graph.md) for prompt assets and graph projections.
