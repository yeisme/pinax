# `kb` compatibility and RAG handoff

Pinax no longer owns a vector database, embedding provider, LanceDB/Inferrum
sidecar, or semantic RAG runtime. Markdown remains the source of truth; Pinax
continues to provide local SQLite/GORM indexing, deterministic text search,
memory, and encrypted sync.

## Existing command names

The released `pinax kb` command names remain parseable for one migration window.
Every old subcommand returns `error.code=kb_decoupled`, performs no vector I/O,
and does not modify `.pinax/kb/**`:

```bash
pinax kb doctor --vault ./my-notes --json
pinax kb search "query" --vault ./my-notes --json
pinax kb context "task" --vault ./my-notes --json
```

The response points to the supported handoff:

```bash
pinax export markdown <output-dir> --vault ./my-notes --json
pinax search "query" --vault ./my-notes --json
```

`pinax kb` compatibility can be removed in the next major release after external
RAG consumers have migrated.

## External RAG ownership

The external RAG project owns the complete semantic pipeline:

1. ingest exported Markdown and preserve source paths/identities;
2. chunk and normalize content;
3. select embedding, vector store, and optional reranker;
4. run evaluation and citation checks;
5. serve context or answer synthesis through its own API.

Pinax does not store or sync vectors, embedding payloads, provider credentials,
reranker state, or generated context. Do not write those artifacts into the
Pinax vault or `.pinax` directory.

## Import and local search replacement

Use the normal Markdown import and text index commands:

```bash
pinax import markdown ./source --vault ./my-notes --dry-run --json
pinax import markdown ./source --vault ./my-notes --yes --json
pinax index refresh --vault ./my-notes --json
pinax search "local text query" --vault ./my-notes --json
pinax memory recall "durable fact" --vault ./my-notes --json
```

`pinax memory` is a deterministic, cited ledger and is intentionally not a
replacement vector store.

## Configuration migration

The following are removed and no longer read:

- `kb.sidecar.executable`
- `kb.sidecar.timeout_seconds`
- `PINAX_KB_SIDECAR`
- `PINAX_KB_SIDECAR_TIMEOUT`

`pinax config set kb.* ...` returns `config_key_deprecated`. Existing YAML keys
are ignored by the new runtime; remove them after confirming the external RAG
pipeline is ready.

## Data safety and rollback

The repository and test-workspace historical `.pinax/kb/**` artifacts were
removed as part of the decoupling cleanup. Pinax does not delete vault notes,
sync revisions, or S3/COS objects. If an external RAG projection is needed
again, rebuild it from `pinax export markdown`; restoring an older binary does
not restore deleted vector artifacts.
