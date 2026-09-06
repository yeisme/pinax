# browse Command

`pinax browse [path]` composes a read-only synthesized directory view over registered vault notes: subfolders with recursive note counts, the notes in the current layer sorted by update time, and trust/freshness badges derived from note frontmatter. It is the progressive-disclosure counterpart to `pinax search` for directory-first navigation.

## Usage

```bash
pinax browse --vault ./my-notes
pinax browse notes/architecture --vault ./my-notes
pinax browse notes/architecture --lazy-index off --vault ./my-notes --json
```

## Output

- `path`: the vault-relative folder being viewed (empty means the vault root).
- `subfolders`: immediate child folders that contain notes, each with its recursive note count.
- `items`: notes directly in this layer, sorted by `updated_at` descending, each carrying derived `trust` (`human`/`machine`/`unverified`) and `fresh` (`fresh`/`stale`) badges plus an optional `description`/`summary` excerpt.

Machine output mirrors these fields: `--agent` emits `detail.path=`, `subfolder.N.*`, and `note.N.trust=`/`note.N.fresh=` lines; `--json` carries the full view in the envelope `data`.

## Boundaries

- `browse` is strictly read-only: it never creates or modifies `index.md`, note files, or any `.pinax/**` asset, and it never writes `.pinax/index.sqlite` regardless of the `--lazy-index` value (`--lazy-index off` is accepted for symmetry with `search` and is asserted read-only in tests).
- Missing folders fail closed with `browse_path_not_found` and a bounded list of available folders; browse never guesses the nearest directory.
- Browse synthesizes from the same note scan that feeds the index projection, so results stay consistent even when no index file exists yet.
