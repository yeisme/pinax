# KB local MVP migration note

The former Ollama → Inferrum → LanceDB local KB MVP has been retired from
Pinax. Its canary, sidecar, provider, generation, and evaluation code is no
longer part of the Pinax build or release gate.

Pinax now exposes a clean ownership boundary:

```text
Markdown vault ──> Pinax local index / text search / sync
       └─────────> Markdown export ──> external RAG ingest/embed/vector/rerank
```

## Migration steps

```bash
pinax export markdown ./temp/rag-export --vault ./my-notes --json
pinax index refresh --vault ./my-notes --json
pinax search "sanity check" --vault ./my-notes --json
```

Configure the external RAG project to read `./temp/rag-export`, retain the
relative source path and content digest, and own all vector artifacts outside
the Pinax repository and vault. Re-run the export after a sync pull or local
note changes according to that project's ingestion policy.

## Removed integration surface

- `internal/semantic` and all Inferrum/LanceDB Go/vendor dependencies;
- Ollama/embedding provider configuration and credential fallback;
- sidecar canary and M4 preflight tasks;
- KB candidate generations, activation, rollback, review, and retrieval tests;
- `kb.sidecar.*` and `PINAX_KB_*` settings.

Old `pinax kb ...` and `/v1/kb/review/*` calls remain as one-release compatibility
stubs that fail closed with `kb_decoupled` (HTTP 410 for the API). They never
start a process or read/write vector data.

## Rollback and cleanup

Keep the previous Pinax binary available until the external RAG pipeline has
passed its own ingestion and citation checks. The repository and test-workspace
`.pinax/kb/**` artifacts have been removed; vault notes, sync state, and
object-storage data were not changed. Rebuild external vectors from Markdown
export when needed.
