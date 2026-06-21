## Design

Pinax remains the source-of-truth owner for Markdown content. The KB layer writes imported text into the vault through app services, then projects registered notes into a local vector store path under `.pinax/kb/lancedb/`.

The implementation keeps the Pinax Go CLI pure-Go and routes `backend=lancedb` to a Python `pinax-lancedb-sidecar` process over the `pinax.kb.sidecar.v1` stdin/stdout JSON protocol. The sidecar uses the Python `lancedb` package to create and query the local store, while Pinax owns note scanning, chunking, embedding provider calls, bounded projection output, and all CLI contracts.

```mermaid
flowchart LR
  Source[text or Markdown source] --> Import[pinax kb import]
  Import --> Vault[Markdown vault]
  Vault --> Rebuild[pinax kb rebuild]
  Rebuild --> Embed[Pinax embedding provider]
  Embed --> Sidecar[pinax-lancedb-sidecar]
  Sidecar --> LanceDB[(.pinax/kb/lancedb)]
  LanceDB --> Search[pinax kb search/context]
```

## Data Flow

```text
markdown/txt source
  -> pinax kb import
  -> Pinax Markdown vault
  -> pinax kb rebuild/refresh
  -> pinax-lancedb-sidecar
  -> .pinax/kb/lancedb projection
  -> pinax kb search/context
```

## Safety Boundaries

- `kb import --dry-run` is read-only.
- `kb import` requires `--yes` before writing vault files.
- `kb search` and `kb context` read the semantic projection only.
- `backend=lancedb` requires the sidecar; `backend=fake` is the built-in deterministic test backend and must not be described as LanceDB.
- The sidecar receives vectors, metadata, and bounded previews only; it does not receive `chunk_text`, full note bodies, raw provider payloads, or credentials.
- Cloud Sync continues to sync encrypted vault revisions; LanceDB projection files are local rebuildable artifacts.
- Agent output returns bounded chunk previews and never includes full note bodies by default.

## Verification Evidence

- 2026-06-19: `go test ./cmd/pinax -run 'TestKB' -count=1` failed before implementation with `unknown command "kb" for "pinax"`.
- 2026-06-19: `go test ./cmd/pinax -run 'TestKB' -count=1` passed after adding the KB vertical slice.
- 2026-06-19: `python3 -m compileall tools/pinax-lancedb-sidecar` passed after adding the Python sidecar.
- 2026-06-19: `PYTHONPATH=tools/pinax-lancedb-sidecar/src temp/kb-sidecar-venv/bin/python -m unittest discover tools/pinax-lancedb-sidecar/tests` passed with real local LanceDB rebuild/search coverage.
