# Agent-Safe Boundary

Pinax is designed so that AI agents can operate on a real Markdown vault without leaking plaintext they should not see, without silently writing files, and without trusting the cloud with plaintext. This document describes the concrete boundaries that make that safe, and points to the code and contracts that enforce them.

## Plaintext boundary: CLI default does not leak full note bodies

Every read surface shares one `NoteDisplay` projection. The default display modes — `card`, `detail`, `context` — return bounded facts (title, path, tags, status, summary, link targets, next actions) and never the full note body. Only the explicit `--display body` flag puts the body into a local JSON projection.

```bash
pinax note read "Research Log" --display card --vault ./my-notes --json    # bounded: no body
pinax note read "Research Log" --display detail --vault ./my-notes --json  # bounded: no body
pinax note read "Research Log" --display context --vault ./my-notes --json # bounded: no body
pinax note read "Research Log" --display body --vault ./my-notes --json    # explicit: body included
```

This boundary is shared across `note read/show`, the project board, the local dashboard, MCP tools, REST, and RPC. A contract test (`TestProofLoopJSONProjections`) asserts that no nested field named `body`/`Body`/`note_body`/`raw_body` ever holds a non-empty string in a bounded projection, and that a body sentinel marker never leaks into any envelope at any nesting depth.

## MCP bounded context: agents read projections, not raw files

MCP tools are read-only and reuse the CLI relationship and note projections. The MCP server forces bounded display even when a caller requests the body:

- `pinax.note.links` — outgoing links, supporting `--broken-only`, `--kind`, `--limit`.
- `pinax.note.backlinks` — backlinks, supporting `--include-broken`, `--limit`.
- `pinax.note.context` — bounded graph context around a note (links + backlinks), no note body.
- `pinax.vault.graph_summary` — vault link-graph health summary.

When a tool call omits the display mode or requests `body`, the MCP server downgrades it to `card` automatically. MCP tools do not return full note bodies and do not write Markdown, `.pinax/`, Git, providers, or remote state.

## Cloud no-exec / no-plaintext invariant

Pinax Cloud Sync is a distributed sync coordinator, not a hosted vault. Two invariants hold:

1. **No plaintext** — the server only ever stores encrypted envelopes. Note bodies are encrypted on the client before upload; the server never receives or stores plaintext note content.
2. **No exec** — the cloud backend never executes local tools, provider calls, or CLI commands. It only coordinates encrypted blobs, manifests, revisions, and conflict metadata.

The encrypted envelope schema is `pinax.cloud.envelope.v1`:

| Field | Meaning |
| --- | --- |
| `alg` | `AES-256-GCM` (authenticated encryption) |
| `key_id` | Derived key identifier (PBKDF2, 100000 iterations) |
| `nonce` | Per-blob random nonce |
| `ciphertext` | Base64-encoded ciphertext |
| `plain_sha256` | Integrity hash of the plaintext (not the plaintext itself) |

Envelope validation rejects any envelope missing `ciphertext`, `key_id`, `nonce`, or `plain_sha256`, and rejects metadata that contains plaintext tokens. The manifest and revision metadata are also encrypted envelopes; the server only sees opaque ciphertext and hashes.

`remote_write=true` is valid only after the selected transport durably commits a revision (CAS commit on the server, or durable object-store write) and Pinax writes local sync-state evidence. Dry-runs, plans, blob uploads, failed or unsupported transport operations, and pull operations all keep `remote_write=false`.

## Proof loop write control chain

Every agent-driven write goes through the same control chain. No step performs direct file surgery on the vault.

```
plan → snapshot → apply → receipt → restore
```

1. **Plan** — `pinax repair plan --save` / `pinax organize plan --save` turn vault health issues into a reviewable, savable plan file under `.pinax/`. The plan is read-only evidence until explicitly applied.
2. **Snapshot** — `pinax version snapshot` creates a protective version snapshot before any apply. Every apply can require or auto-create a snapshot via `--snapshot-message`.
3. **Apply** — `pinax repair apply --yes` / `pinax organize apply --yes` execute only low-risk operations (metadata, tags, index rebuild, archive status, low-risk moves). High-risk operations (duplicate titles, broken links, ambiguous links, body rewrites, deletions) only generate manual review items — they are never auto-applied.
4. **Receipt** — apply writes an auditable receipt so the change is traceable and reviewable.
5. **Restore** — `pinax version restore <path> --revision <rev> --plan` generates a restore plan, then `pinax version restore apply --plan <id> --yes` writes the restored content through the CLI service (`local_write=true`, `remote_write=false`). This is the revert path if an apply goes wrong.

`pinax proof loop run` orchestrates the whole chain in one command. Preview is read-only; `--apply --yes` takes a fresh snapshot and applies approved operations.

## Related

- [Product Positioning](./product-positioning.md)
- [CLI Output Contract](../interfaces/cli-output-contract.md)
- [Cloud Sync Architecture](../architecture/cloud-sync-design.md)
- [Architecture Boundaries](../architecture/architecture-boundaries.md)
