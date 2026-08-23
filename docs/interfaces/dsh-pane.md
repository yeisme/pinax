# DSH Pane Interface Contract

Pinax is the domain owner of the DSH Pinax Pane: the DSH Host (implemented in `agent/harness-plugins`, change `dsh-pinax-pane-v1`) consumes Pinax-owned projections and must never copy vault state or hand-write structured metadata. This document defines the Pinax-side contract implemented by `internal/app/pane.go`.

## Snapshot envelope (`pane.event.v1alpha1`)

`AssemblePaneSnapshot(projection, context)` maps an in-process `note.list` projection onto a snapshot event:

- Entities are bounded: `ref` (note ID), `version`, and `value` with only `title`, `kind`, `status`, `tags`. Note paths, bodies, and frontmatter never enter the envelope.
- Unsafe refs are skipped: empty, absolute POSIX/Windows paths, URL schemes, and `token`/`authorization`/`cookie` substrings.
- Status mapping: `success` → `ready`; `failed`/`error`/`offline` → `offline`; `permission_denied` → `permission_denied`. A failed projection is never presented as `ready` with empty entities.
- `occurredAt`/`observedAt` are the real UTC assembly time (RFC3339). Snapshot `freshness` is `fresh`; negative snapshots report `unknown`.
- `cursor`/`sequence` are placeholders (`c-1`/`-1`) until an event stream lands; after the first snapshot the Host must not poll to fake realtime — without a stream the Pane shows `offline`.

Context (`PaneContext`) requires `workspaceRef` and `revision`; assembly fails closed without them.

## Backlinks surface

`AssemblePaneBacklinks(projection, context)` maps a `note.backlinks` projection onto the pane envelope:

- Entities are the linking notes: `ref` = source note id, bounded `value` with `title` (≤160 chars), `link_kind`, `status` (`ok`/`broken`).
- Bounded to 50 entities (`truncated: true` beyond); unsafe source refs are skipped and counted in `dropped_unsafe` (scanned across the whole input, not just the shown page).
- An empty backlinks result is a legal `ready` snapshot; a failed upstream projection maps to `offline`; a round-tripped (non in-process) shape fails closed with an error.

## Graph summary surface

`AssemblePaneGraphSummary(notes, links, context)` derives a path-free link-graph summary:

- `payload.totals`: `nodes`, `edges`, `components`, `truncated`.
- Entities are the top-20 nodes by total degree (`value`: `degree`, `in`, `out`), deterministic ordering (degree desc, then ref asc).
- Node identity is the note id only; no filesystem paths, no bodies, no titles on this surface.

## History timeline surface

`AssemblePaneHistory(projection, context)` maps record-ledger revision events onto `payload.timeline`:

- Events carry only `op`, `ref`, `revision` (content hash), `time`; bounded to 100 with `truncated` and `dropped_unsafe`.
- Round-tripped shapes fail closed.

All three surfaces share the snapshot envelope, the `paneUnsafe` ref red line, and the same status mapping as the note-list snapshot.

## Artifact references (`pane.artifact.v1alpha1`)

`PaneArtifactFromNote(note)` emits a path-free artifact ref: `owner=pinax`, `mediaType=text/markdown`, `capabilities=[open, link, attach_context]`. A ref that hits the unsafe list returns an error instead of a reference.

## Gated actions

`PaneGatedActions(revision)` declares the only mutations a Pane may submit:

| Action | Owner command | Gate |
| --- | --- | --- |
| `inbox.capture` | `pinax inbox capture` | expected revision, idempotency, receipt |
| `sync.run` | `pinax sync run` | expected revision, idempotency, receipt |

Agents must not assemble JSON/YAML as canonical changes by hand. `RejectHandwrittenMetadata(blob)` fails closed on any client-submitted metadata blob that did not go through the Pinax parser, and the vault stays unchanged.

## Negative states

`AssemblePaneNegative(kind, context)`:

- `offline` / `permission_denied` → empty snapshot with that status, `freshness=unknown`.
- `handwritten_metadata` → rejection error (fail closed).
- anything else → error.

## Evidence

The `dsh-pane` integration-evidence profile (component layer) runs the pane contract tests:

```bash
go run ./tools/testkit/integrationevidence -profile dsh-pane
```

Extra checks: `pane_snapshot_redaction`, `handwritten_metadata_rejected`, `backlinks_bounded`, `graph_summary_no_paths`, `history_timeline_bounded`.
