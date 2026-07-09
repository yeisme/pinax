# sync Command

`pinax sync` generates, records, and executes sync plans. Explicit `diff`/`push`/`pull` commands are short-lived workflows; `pinax sync daemon` is the local long-running process for automatic Capsa Sync.

For `--target capsa`, the protocol is distributed: every device keeps a local vault, and the selected Capsa Sync transport coordinates encrypted blob, manifest, and revision exchange. The content manifest is selected by `.pinaxignore` and can include Markdown, scripts, assets, attachments, and other regular files. The transport can be Capsa Server, S3-compatible direct storage, rclone direct storage, or embedded Go API/local RPC. This differs from `pinax api serve`, which is centralized remote access to one server-side vault.

Project and subproject deletions are represented as explicit encrypted delete markers in the manifest. A deletion is not inferred from a missing file entry. Push reports additive `delete_markers` and `trash_backup_blobs` facts, and `remote_write=true` is still emitted only after the selected transport commits the revision.

When docs or operator notes use the phrase backup mirror, it means a CLI-side direct transport mirror of encrypted Capsa Sync objects under a user-controlled provider boundary. S3 direct and rclone direct do not become Capsa server-side storage: provider credentials own access, and Pinax does not provide server-side auth, audit, object lifecycle, tenant policy, or rate limiting for those writes. Capsa server transport is the path that owns server auth/audit/object lifecycle semantics.

The backup mirror boundary also excludes realtime daemon and conflict policy changes. `pinax sync daemon` remains the local realtime automation layer, and conflict inspection/resolution remains under explicit `pinax sync conflicts` commands. New daemon lifecycle behavior, automatic merge behavior, conflict resolution semantics, or transport-specific push notifications need separate OpenSpec coverage before implementation.

## Subcommands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax sync diff` | Generates a sync diff plan. | Does not write to the remote. |
| `pinax sync push` | Pushes local encrypted manifest/blob changes when the selected transport can commit a durable revision. | Requires `--yes`; `remote_write=true` is allowed only after revision commit succeeds. |
| `pinax sync pull` | Pulls the committed remote revision and applies decrypted local changes. | Requires `--yes`; preserves conflicting local edits as `.conflict.md` copies. |
| `pinax sync daemon run` | Runs the local sync daemon in the foreground. | Requires `--yes`; watches local changes and polls remote head. |
| `pinax sync daemon start` | Starts the local sync daemon in the background. | Requires `--yes`; writes `.pinax/sync-daemon/` runtime state. |
| `pinax sync daemon status` | Reads local daemon state. | Read-only. |
| `pinax sync daemon stop` | Requests graceful daemon shutdown. | Writes a local stop request. |
| `pinax sync daemon logs` | Reads redacted daemon events. | Read-only. |
| `pinax sync conflicts list` | Lists local conflict copies. | Read-only. |
| `pinax sync conflicts diff <file>` | Shows a diff between a conflict copy and its trunk file. | Read-only. |
| `pinax sync conflicts show <file>` | Shows conflict content for manual or agent merge workflows. | Read-only. |
| `pinax sync conflicts resolve <file>` | Resolves a conflict copy by keeping local, keeping remote, or applying a merged file. | Requires explicit resolve flags and write confirmation where supported. |

## Common workflows

Inspect a plan without writing:

```bash
pinax vault ignore status --vault ./my-notes --json
pinax sync diff --target capsa --vault ./my-notes --json
pinax sync push --target capsa --vault ./my-notes --dry-run --json
```

Configure an S3-compatible direct backend and sync two devices:

```bash
export PINAX_SYNC_SECRET="your-encryption-secret"

pinax capsa backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device laptop \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./device-a
pinax sync push --target capsa --vault ./device-a --yes --json

pinax capsa backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device desktop \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./device-b
pinax sync pull --target capsa --vault ./device-b --yes --json
```

### Tencent Cloud COS (real-world verified)

Tencent Cloud Object Storage (COS) is S3-compatible and uses **virtual-hosted** addressing by default. Pinax auto-detects `.myqcloud.com` endpoints and selects virtual-hosted style automatically — you do not need `--addressing-style virtual-hosted`.

AWS profile in `~/.aws/credentials`:

```ini
[profile tencent-cos-pinax]
region = ap-guangzhou

[tencent-cos-pinax]
aws_access_key_id = AKIDxxxxxxxxxxxx
aws_secret_access_key = <your-secret-key>
```

Configure two devices and sync:

```bash
export PINAX_SYNC_SECRET="your-encryption-secret"

# Device A (laptop)
pinax capsa backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --profile tencent-cos-pinax \
  --prefix pinax-sync/ \
  --workspace personal \
  --device laptop \
  --secret-ref env://PINAX_SYNC_SECRET \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax sync push --vault ./my-notes --yes --json

# Device B (desktop) — same bucket, different device id
pinax capsa backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --profile tencent-cos-pinax \
  --prefix pinax-sync/ \
  --workspace personal \
  --device desktop \
  --secret-ref env://PINAX_SYNC_SECRET \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax sync pull --vault ./my-notes --yes --json
```

After initial push, use `pinax sync --vault ./my-notes --yes` for bidirectional sync. If you moved a note locally but haven't pushed yet, `sync pull` returns `LOCAL_UNPUSHED_CHANGES`; `pinax sync` (without subcommand) will push the move and pull remote changes in one step.

COS region endpoints follow `https://cos.<region>.myqcloud.com` (e.g. `cos.ap-beijing.myqcloud.com`, `cos.ap-shanghai.myqcloud.com`). The `--profile` flag reads credentials from the named AWS shared profile; Pinax never stores the raw secret key.

Use a local object-store transport for development or local E2E checks:

```bash
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target capsa --vault ./device-a --yes --json
pinax sync pull --target capsa --vault ./device-b --yes --json
```

Delete markers are produced by CLI-authored trash commands before sync:

```bash
pinax project delete history --vault ./device-a --yes --json
pinax trash list --vault ./device-a --json
pinax sync push --target capsa --vault ./device-a --yes --json
```

Configure server and rclone backends; a push claims a completed write only after the selected transport commits a durable revision:

```bash
pinax capsa login --endpoint https://capsa.example.test --workspace ws_123 --device laptop --secret-ref env://PINAX_CAPSA_TOKEN --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes
pinax sync push --target capsa --vault ./my-notes --yes --json

pinax capsa backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes
pinax sync push --target capsa --vault ./my-notes --yes --json
```

Run local automatic sync after Capsa Sync is configured:

```bash
pinax sync daemon run --target capsa --vault ./my-notes --yes
pinax sync daemon status --vault ./my-notes --json
pinax sync daemon logs --vault ./my-notes --limit 20 --json
pinax sync daemon stop --vault ./my-notes
```

`run` stays in the foreground and handles terminal/process shutdown. It performs one startup pull-before-push sync cycle immediately, then continues watching local changes and polling the remote head. Default human output prints live progress lines; use `--events` for NDJSON streaming automation while `--json` remains a final single-envelope output.

`start` launches the same runner in the background where supported:

```bash
pinax sync daemon start --target capsa --vault ./my-notes --yes
```

The first daemon release detects local file changes with a local watcher and detects remote changes by polling the Capsa Sync head. Redacted daemon state and event logs live under `.pinax/sync-daemon/` and can be inspected with `pinax sync daemon logs --vault ./my-notes --limit 20 --json`. It is a local process, not a hosted service, and it does not change the Capsa Sync plaintext boundary: transports still coordinate encrypted blobs, encrypted manifests, and revision metadata only.

These apply commands use the same sync engine as direct object-store transports. If the selected backend is unavailable, the commit fails, or the configured scheme is unsupported, the command must return a structured partial/error such as `transport_unavailable`, `unsupported_scheme`, or `revision_conflict` with `remote_write=false`. It must not silently no-op, produce a dummy revision, or emit `remote_write=true`.

## Daemon service installation

For long-running persistent sync, install the daemon as an OS service:

### Linux (systemd user unit)

```bash
pinax sync daemon install --vault ./my-notes --yes
systemctl --user enable --now capsa-sync-my-notes
```

To remove:

```bash
systemctl --user stop capsa-sync-my-notes
pinax sync daemon uninstall --vault ./my-notes --yes
```

### macOS (launchd)

```bash
pinax sync daemon install --vault ./my-notes --yes
launchctl load ~/Library/LaunchAgents/com.yeisme.capsa-sync.my-notes.plist
```

To remove:

```bash
launchctl unload ~/Library/LaunchAgents/com.yeisme.capsa-sync.my-notes.plist
pinax sync daemon uninstall --vault ./my-notes --yes
```

## Capsa Sync execution model

The target execution flow is transport-independent:

1. Scan the local vault and build a client-side manifest from files allowed by `.pinaxignore`.
2. Encrypt content blobs and manifest metadata before upload.
3. Ask the selected transport which encrypted blobs are missing.
4. Upload missing encrypted blobs and the encrypted manifest object.
5. Commit the new revision with compare-and-swap against the known base revision.
6. Write local sync-state / run evidence after the commit result is known.
7. Other devices read the committed head, download missing encrypted blobs, decrypt locally, and apply changes.
8. Conflicting local edits are preserved as local conflict copies instead of being silently overwritten.

`remote_write=true` belongs only to step 5 after a durable revision commit. It is not valid for dry-runs, plan generation, blob uploads, manifest uploads, conflict failures, unsupported transports, or pull operations.

The daemon uses the same rule. It may call `sync pull` before `sync push` when the remote head is newer, and it stops automatic writes with `conflict_required` if pull creates conflict copies that need user review.

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

`sync` is the entry point for sync workflows. `capsa` configures the Capsa Sync transport state. `backend` manages provider profiles, capabilities, and object-store diagnostics.

Capsa Server is one Capsa Sync transport and owns auth/device state, revision CAS, blob persistence, audit, and readiness. S3/rclone direct transports skip the remote Capsa service and use provider credentials as the access boundary. Embedded Go API/local RPC calls the same app service as the CLI and does not bypass approval, dry-run, snapshot, conflict, event, or redaction rules.

S3 direct can serve as a backup mirror for encrypted Capsa Sync objects, but it remains a direct object-store transport. It must not be documented as Capsa server-side storage, and backup mirror wording must not promise daemon convergence or conflict resolution without a separate OpenSpec change.

`pinax api serve` is not a Capsa Sync transport. It exposes one centralized vault through local REST/RPC and is useful for dashboards and local agents that intentionally operate against that vault.

Client CLI parity does not replace the daemon. Remote API Mode can let a client trigger supported explicit sync operations through registered RPC capabilities, but realtime multi-device convergence should run `pinax sync daemon` on each device that owns a local vault.

See [`docs/architecture/cloud-sync-design.md`](../architecture/cloud-sync-design.md) for the architecture split. See [Client CLI Parity and Realtime Sync](../interfaces/client-cli-parity-and-sync.md) for the client coverage boundary. See also [`api`](./api.md), [`token`](./token.md), and [`profile`](./profile.md) for Remote API Mode.

## S3 direct sync troubleshooting

### `NoSuchBucket`

The bucket name is wrong or the credentials lack access. Verify the bucket exists and the profile has read/write permission:

```bash
pinax capsa doctor --vault ./my-notes --json
```

### `PathStyleDomainForbidden` (Tencent COS)

COS requires **virtual-hosted** addressing. Pinax auto-detects `.myqcloud.com` endpoints and sets `path_style=false`. If you overrode with `--addressing-style path`, remove it and reconfigure:

```bash
pinax capsa backend set s3 --bucket <bucket> --region ap-guangzhou --endpoint https://cos.ap-guangzhou.myqcloud.com --profile <profile> --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes
```

### `revision_conflict`

Another device pushed a newer revision since your last pull. Run `pinax sync --vault ./my-notes --yes` (bidirectional) to pull remote changes and re-push in one step. If conflicts arise, Pinax preserves local edits as `.conflict.md` files — use `pinax sync conflicts list` to inspect.

### `encryption_key_mismatch`

The encryption key has changed since the last sync. This happens after upgrading Pinax (the DeriveKey fix changes key derivation). Run `pinax capsa doctor --vault ./my-notes` to confirm, then re-push:

```bash
pinax sync push --vault ./my-notes --yes
```

### `LOCAL_UNPUSHED_CHANGES`

You moved or deleted a note locally without pushing. `sync pull` refuses to run to avoid restoring old paths. Run `pinax sync --vault ./my-notes --yes` to push the local move/delete and pull remote changes together.

### `cloud_not_configured`

No Capsa backend is configured for this vault. Run `pinax capsa backend set s3 ...` (S3 direct) or `pinax capsa login ...` (Capsa Server) first. Check with `pinax capsa status --vault ./my-notes --json`.

### Checksum warnings in logs

Pinax sets `RequestChecksumCalculationWhenRequired` and `ResponseChecksumValidationWhenRequired` for S3-compatible providers. This suppresses per-object `x-amz-checksum-*` WARN messages from services like Tencent COS that don't return checksum headers. No action needed.
