# sync Command

`pinax sync` generates, records, and executes sync plans. It is intended for short-lived CLI workflows, not as a long-running daemon.

For `--target cloud`, the protocol is distributed: every device keeps a local Markdown vault, and the selected Cloud Sync transport coordinates encrypted blob, manifest, and revision exchange. The transport can be Pinax Cloud Server, S3-compatible direct storage, rclone direct storage, or embedded Go API/local RPC. This differs from `pinax api serve`, which is centralized remote access to one server-side vault.

## Subcommands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax sync diff` | Generates a sync diff plan. | Does not write to the remote. |
| `pinax sync push` | Pushes local encrypted manifest/blob changes when the selected transport can commit a durable revision. | Requires `--yes`; `remote_write=true` is allowed only after revision commit succeeds. |
| `pinax sync pull` | Pulls the committed remote revision and applies decrypted local changes. | Requires `--yes`; preserves conflicting local edits as `.conflict.md` copies. |
| `pinax sync conflicts list` | Lists local conflict copies. | Read-only. |
| `pinax sync conflicts diff <file>` | Shows a diff between a conflict copy and its trunk file. | Read-only. |
| `pinax sync conflicts show <file>` | Shows conflict content for manual or agent merge workflows. | Read-only. |
| `pinax sync conflicts resolve <file>` | Resolves a conflict copy by keeping local, keeping remote, or applying a merged file. | Requires explicit resolve flags and write confirmation where supported. |

## Common workflows

Inspect a plan without writing:

```bash
pinax sync diff --target cloud --vault ./my-notes --json
pinax sync push --target cloud --vault ./my-notes --dry-run --json
```

Configure an S3-compatible direct backend and sync two devices:

```bash
pinax cloud backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device laptop \
  --vault ./device-a
pinax sync push --target cloud --vault ./device-a --yes --json

pinax cloud backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device desktop \
  --vault ./device-b
pinax sync pull --target cloud --vault ./device-b --yes --json
```

Use a local object-store transport for development or local E2E checks:

```bash
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target cloud --vault ./device-a --yes --json
pinax sync pull --target cloud --vault ./device-b --yes --json
```

Configure server and rclone backends; a push claims a completed write only after the selected transport commits a durable revision:

```bash
pinax cloud login --endpoint https://cloud.example.test --workspace ws_123 --device laptop --secret-ref env://PINAX_CLOUD_TOKEN --vault ./my-notes
pinax sync push --target cloud --vault ./my-notes --yes --json

pinax cloud backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes
pinax sync push --target cloud --vault ./my-notes --yes --json
```

These apply commands use the same sync engine as direct object-store transports. If the selected backend is unavailable, the commit fails, or the configured scheme is unsupported, the command must return a structured partial/error such as `transport_unavailable`, `unsupported_scheme`, or `revision_conflict` with `remote_write=false`. It must not silently no-op, produce a dummy revision, or emit `remote_write=true`.

## Cloud Sync execution model

The target execution flow is transport-independent:

1. Scan the local vault and build a client-side manifest.
2. Encrypt note blobs and manifest metadata before upload.
3. Ask the selected transport which encrypted blobs are missing.
4. Upload missing encrypted blobs and the encrypted manifest object.
5. Commit the new revision with compare-and-swap against the known base revision.
6. Write local sync-state / run evidence after the commit result is known.
7. Other devices read the committed head, download missing encrypted blobs, decrypt locally, and apply changes.
8. Conflicting local edits are preserved as local conflict copies instead of being silently overwritten.

`remote_write=true` belongs only to step 5 after a durable revision commit. It is not valid for dry-runs, plan generation, blob uploads, manifest uploads, conflict failures, unsupported transports, or pull operations.

## Conflict workflow

When pull detects a local edit for a path also changed remotely, Pinax writes the remote trunk to the canonical note path and preserves the local edit next to it, for example `alpha.20260612153000.conflict.md`.

Use these commands to inspect and resolve:

```bash
pinax sync conflicts list --vault ./my-notes --json
pinax sync conflicts diff ./my-notes/notes/alpha.20260612153000.conflict.md --vault ./my-notes
pinax sync conflicts show ./my-notes/notes/alpha.20260612153000.conflict.md --vault ./my-notes --json
pinax version snapshot --vault ./my-notes --message "snapshot before sync conflict resolve"
pinax sync conflicts resolve ./my-notes/notes/alpha.20260612153000.conflict.md --merged ./merged-alpha.md --vault ./my-notes --yes
```

Conflict output and next actions must be consumable by humans and agents. Note bodies may appear only when the user explicitly asks for local content, such as `conflicts show`; sync receipts, stdout summaries, event streams, fixtures, object metadata, provider stderr, and backend logs must remain redacted.

## Relationship with backend and Local API

`sync` is the entry point for sync workflows. `cloud` configures the Cloud Sync transport state. `backend` manages provider profiles, capabilities, and object-store diagnostics.

Pinax Cloud Server is one Cloud Sync transport and owns auth/device state, revision CAS, blob persistence, audit, and readiness. S3/rclone direct transports skip the remote Pinax Cloud service and use provider credentials as the access boundary. Embedded Go API/local RPC calls the same app service as the CLI and does not bypass approval, dry-run, snapshot, conflict, event, or redaction rules.

`pinax api serve` is not a Cloud Sync transport. It exposes one centralized vault through local REST/RPC and is useful for dashboards and local agents that intentionally operate against that vault.

See [`docs/architecture/cloud-sync-design.md`](../architecture/cloud-sync-design.md) for the architecture split.
