# knowledge Command

`pinax knowledge` exports an allowlisted, provider-neutral knowledge projection for Inferrum ingestion. The Pinax vault remains the source of truth. Export writes only the output package; it does not write vault Markdown, Inferrum/txtai databases, or retrieval indexes.

## Usage

```bash
pinax knowledge export-projection --output ./projection.json --vault ./my-notes --json
pinax knowledge export-projection --output ./projection.json --allowlist notes/public.md --vault ./my-notes --json
pinax knowledge export-projection --output ./projection.json --allowlist-file ./allowlist.txt --from-package ./prior.json --vault ./my-notes --json
```

## Dual-condition allowlist

A note enters the projection only when both conditions are true:

1. Its vault-relative path matches `--allowlist` or `--allowlist-file`.
2. Its frontmatter includes an allow marker (`knowledge_export: allow`).

The default allowlist is empty. An empty allowlist produces an empty package with `empty_reason=allowlist_empty` and does not read non-allowlisted note bodies.

## Incremental export

`--from-package` compares content digests against a prior package. Unchanged notes are omitted. Deleted allowlisted notes become tombstone revocation entries.

## Write Boundaries

Export writes only `--output`. The path must be outside the vault. Vault notes, `.pinax/index.sqlite`, and any Inferrum/txtai database paths stay unchanged.
