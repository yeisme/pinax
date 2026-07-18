# search Command

`pinax search <query>` searches local notes. It prefers a fresh SQLite/GORM token index; when the index is unavailable, it can fall back to Pinax's built-in native Markdown search and provide next steps. Native search is implemented in the Pinax binary and does not require `rg`, `fzf`, or `bat`.

## Usage

```bash
pinax search "authentication" --vault ./my-notes
pinax search "authentication" --engine native --lazy-index off --vault ./my-notes --json
pinax search "authentication" --engine index --vault ./my-notes --json
pinax search "authentication" --tag auth --group work --folder architecture --kind reference --status active --vault ./my-notes --json
pinax search "authentication" --link-target notes/design/auth.md --vault ./my-notes --json
pinax search "diagram" --has-attachment --vault ./my-notes --json
```

## Filter Parameters

| Parameter | Purpose |
| --- | --- |
| `--tag`, `--group`, `--folder`, `--kind`, `--status` | Filter by note metadata. |
| `--created-after`, `--updated-after` | Filter by date. |
| `--link-target` | Filter by link target; supports note id, path, title, or raw target. |
| `--has-attachment` | Return only notes containing attachment references. |
| `--sort relevance|updated|created|title|path` | Sort order. |
| `--limit` | Limit the number of returned results. |
| `--allow-stale` | Allow a stale index to return partial results. |
| `--engine auto|index|native` | Select the search engine. `auto` prefers a fresh SQLite index, `index` only reads the SQLite/GORM token index, and `native` scans registered Markdown notes inside Pinax. |
| `--lazy-index auto|off|sync` | Control search-time index loading. `off` guarantees search will not write `.pinax/index.sqlite`. |
| `--at`, `--revision`, `--changed-since`, `--include-dirty` | Version-aware search. |

## Boundaries

`search --lazy-index off` is read-only and does not write Markdown, `.pinax/`, Git, providers, or remotes. The default `--lazy-index auto` may maintain the rebuildable local index projection when the vault is small enough for safe lazy loading. When the target is ambiguous, search returns an error and does not automatically choose a candidate.

The index engine uses the `search_token_records` projection to select candidate notes, then loads text only for result snippets. It is the normal fast path for automation. Use `--engine native` only when you explicitly want an in-process Markdown scan fallback.

## Agent Brain Role

`pinax search` contributes lexical candidates and bounded snippets to staged Agent Brain context. The SQLite/GORM token index is a local rebuildable projection; Markdown notes remain the source of truth. Search output should be used as evidence discovery, not as final answer synthesis, and planned `pinax brain answer ...` must cite resulting note refs or snippets rather than dumping full bodies.
