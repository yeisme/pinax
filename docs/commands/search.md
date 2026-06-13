# search Command

`pinax search <query>` searches local notes. It prefers a fresh SQLite/GORM index; when the index is unavailable, it falls back to scanning Markdown and provides next steps.

## Usage

```bash
pinax search "authentication" --vault ./my-notes
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
| `--at`, `--revision`, `--changed-since`, `--include-dirty` | Version-aware search. |

## Boundaries

`search` is read-only and does not write Markdown, `.pinax/`, Git, providers, or remotes. When the target is ambiguous, it returns an error and does not automatically choose a candidate.
