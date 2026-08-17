# Removed: `pinax kb` and the native vector KB

Pinax no longer owns a vector database, embedding provider, LanceDB/Inferrum
sidecar, or semantic RAG runtime, and the `pinax kb` command tree has been
removed entirely. Running any `pinax kb` subcommand now reports an unknown
command. Markdown remains the source of truth; Pinax continues to provide
local SQLite/GORM indexing, deterministic text search, memory, and encrypted
sync.

## Removed surface

- The `pinax kb` command tree (import, rebuild, refresh, doctor, search,
  context, evaluate, activate, rollback, provider, generations).
- The `/v1/kb/review/*` REST routes (previously HTTP 410 `kb_decoupled`).
- The `kb.context` / `kb.review.*` remote capabilities and routes.
- `kb.*` configuration keys (`kb.sidecar.executable`,
  `kb.sidecar.timeout_seconds`, `PINAX_KB_SIDECAR`, `PINAX_KB_SIDECAR_TIMEOUT`).
  Leftover YAML keys are inert; remove them at any time.

## RAG handoff

Export Markdown and let an external RAG system own the semantic pipeline:

```bash
pinax export markdown <output-dir> --vault ./my-notes --json
pinax search "local text query" --vault ./my-notes --json
```

The external RAG project owns ingestion, chunking, embeddings, vector store,
evaluation, and retrieval. Pinax does not store or sync vectors, embedding
payloads, provider credentials, reranker state, or generated context. Do not
write those artifacts into the Pinax vault or `.pinax` directory.

## Local replacements

```bash
pinax import markdown ./source --vault ./my-notes --yes --json
pinax index refresh --vault ./my-notes --json
pinax search "local text query" --vault ./my-notes --json
pinax memory recall "durable fact" --vault ./my-notes --json
```

`pinax memory` is a deterministic, cited ledger and is intentionally not a
replacement vector store.

## Data safety

Historical `.pinax/kb/**` artifacts were removed as part of the decoupling
cleanup. Pinax does not delete vault notes, sync revisions, or S3/COS objects.
If an external RAG projection is needed, rebuild it from
`pinax export markdown`.
