# Architecture Boundaries

```mermaid
flowchart TD
  U[User / Agent] --> CLI[pinax Cobra CLI]
  CLI --> APP[Application Services]
  APP --> VAULT[Vault Repository\nMarkdown + frontmatter]
  APP --> ASSET[Asset Manifest\nCLI-authored metadata]
  APP --> INDEX[Index Repository\nSQLite + GORM]
  APP --> VERSION[VersionBackend\nlocal / none / optional Git]
  APP --> PROVIDER[CLI-backed Provider Adapters]
  APP --> OUT[Command Projection]
  OUT --> HUMAN[Default English summary]
  OUT --> AGENT[--agent]
  OUT --> JSON[--json]
  OUT --> EVENTS[--events]
```

Boundary rules:

- `cmd/pinax` only handles CLI wiring, flags, argument validation, and output mode selection.
- `internal/app` is responsible for orchestrating use cases.
- `internal/domain` holds stable domain models and projections.
- `internal/output` renders human and machine output from the same projection.
- `internal/redaction` centrally handles redaction of secrets, tokens, raw payloads, and traces.
- Repositories, indexes, and persistence must be implemented through adapter/repository packages. Relational access in Go uses GORM by default.
- `internal/version` only provides capability-driven version evidence; the user-visible entry point is `pinax version`, and Git is only an optional backend or hidden compatibility alias.
- `internal/assets` manages the asset manifest and vault-local file facts; the manifest is CLI-authored metadata, not the source of truth for binary payloads.


Relationship graph boundaries:

- The Markdown vault is the source of truth for bidirectional relationships; SQLite/GORM only stores reconstructable projections such as `LinkRecord`.
- Asset files in the vault are the source of truth for files; the asset manifest stores vault-relative paths, hashes, media facts, and link facts, while SQLite/GORM only stores reconstructable projections.
- The version backend is the evidence source for snapshots, revisions, changed paths, and restore plans. It does not own Markdown bodies, asset bytes, the record ledger, or index projections.
- `pinax note links`, `pinax note backlinks`, `pinax note orphans`, `search --link-target`, doctor, repair, organize, dashboard, and MCP must reuse the application service's relationship parsing/target resolution logic and must not maintain a parallel parser.
- broken, ambiguous, orphan, asset_missing, asset_hash_changed, link_resolution, and link_rewrite may only enter the manual review plan; `--dry-run` and read-only MCP do not write Markdown, `.pinax/`, version backend, provider, or remote state.
- The MCP relationship surface only returns bounded facts, candidate summaries, and runnable next steps, such as `pinax note links <ref> --vault <vault> --json`; it does not return the full note body.
