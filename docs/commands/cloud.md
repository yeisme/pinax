# cloud Command

`pinax cloud` manages local state for the distributed Pinax Cloud Sync protocol. It is not the same feature as `pinax api serve`: `api serve` exposes one centralized vault through local REST/RPC, while Cloud Sync keeps a separate local vault on every device and exchanges encrypted revisions, manifests, and blobs through a selected transport.

The word `cloud` names the sync protocol, not necessarily a hosted Pinax Cloud service. Pinax Cloud Sync can use a server transport, S3-compatible direct storage, rclone-backed providers such as OneDrive, or embedded Go API/local RPC entrypoints that call the same app service.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax cloud login` | Shortcut for configuring a server-style Cloud backend endpoint, workspace, device, and secret reference. | Writes cloud state; does not save the raw token. |
| `pinax cloud backend set server` | Configures a Pinax Cloud Server transport. | Writes cloud state; does not save the raw token. |
| `pinax cloud backend set s3` | Configures direct S3/MinIO/R2-compatible object storage transport. | Writes cloud state; does not save access key or secret key. |
| `pinax cloud backend set rclone` | Configures an rclone direct transport such as an existing OneDrive remote. | Writes cloud state; does not save OAuth refresh tokens. |
| `pinax cloud status` | Views cloud state. | Does not write. |
| `pinax cloud logout` | Logs out or clears the local device/backend state. | Writes cloud state. |
| `pinax cloud doctor` | Diagnoses cloud state and transport boundaries. | Does not write. |

## Centralized Local API vs Cloud Sync Protocol

| Pattern | Command surface | Vault ownership | Current status |
| --- | --- | --- | --- |
| Centralized local access | `pinax api serve`, `pinax --api-url ...`, local RPC routes | One running `pinax api serve` process owns one server-side vault. Callers do not keep an independent synchronized vault. | Implemented for registered local API routes. Not a Cloud Sync transport. |
| Cloud Sync server transport | `pinax cloud login` / `pinax cloud backend set server`, then `pinax sync --target cloud` | Every device owns its own local vault. Pinax Cloud Server coordinates encrypted blob/revision exchange. | Implemented through the shared sync engine and `internal/cloudclient.Transport`; `remote_write=true` is emitted only after a durable revision commit and local sync-state receipt. |
| Cloud Sync S3 direct transport | `pinax cloud backend set s3`, then `pinax sync --target cloud` | Every device owns its own local vault. The provider stores encrypted Cloud Sync objects. | Implemented for the direct object-store engine; `remote_write=true` is emitted only after the head/revision commit succeeds. |
| Cloud Sync rclone direct transport | `pinax cloud backend set rclone`, then `pinax sync --target cloud` | Every device owns its own local vault. rclone is the provider credential boundary. | Implemented through the shared object-store sync path; lock-object commit protection covers providers without reliable conditional writes. |
| Embedded Go API / local RPC | `app.Service` methods and `Pinax.Sync.Push` / `Pinax.Sync.Pull` local RPC | Same local app service and vault mutation rules as CLI. | Implemented for local callers. This is not `pinax api serve` centralized remote mode. |

The distributed design is similar to Obsidian Sync: laptop, phone, and desktop all keep local vaults. The transport stores encrypted sync artifacts and revision order; it does not become the plaintext note source of truth.

`pinax sync daemon` is the local automation layer on top of this protocol. It runs on each device, watches local vault changes, polls the remote Cloud Sync head for remote changes, and then invokes the same pull/push engine as explicit CLI commands. It is not a hosted Pinax Cloud service, and it does not give the transport plaintext note access.

## User-runnable setup examples

Server transport configuration:

```bash
pinax cloud login \
  --endpoint https://cloud.example.test \
  --workspace ws_123 \
  --device laptop \
  --secret-ref env://PINAX_CLOUD_TOKEN \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax cloud status --vault ./my-notes --json
pinax cloud doctor --vault ./my-notes
```

### Pinax Cloud Sync MLP server contract

The server transport speaks the Pinax Cloud Sync MLP (minimum lovable product) REST contract. The public protocol uses `vault_id` terminology and never transfers plaintext Markdown:

| Operation | Method & Path | Notes |
| --- | --- | --- |
| Health | `GET /v1/health` | Readiness probe. |
| Bootstrap | `POST /v1/auth/bootstrap` | Self-hosted single-account bootstrap; issues a device session. |
| Current principal | `GET /v1/auth/principal` | Login-state facts after bootstrap. |
| Create vault | `POST /v1/vaults` | Returns `vault_id` and `crypto_mode`. |
| Link device | `POST /v1/vaults/{vault_id}/link` | Binds the current device to the vault. |
| Changes cursor | `GET /v1/vaults/{vault_id}/changes?since=<revision_id>` | Returns revision and object refs after the cursor; object refs use `path_hash`/`blob_hash`, never plaintext paths. |
| Current head | `GET /v1/vaults/{vault_id}/head` | Current revision and manifest blob id. |
| Blob batch check | `POST /v1/vaults/{vault_id}/blobs:batch-check` | Returns the subset of encrypted blobs missing from storage. |
| Sign upload | `POST /v1/vaults/{vault_id}/blobs:sign-upload` | Returns a server-owned object key and upload plan. |
| Upload blob | `PUT /v1/vaults/{vault_id}/blobs/{blob_id}` | Stores an encrypted envelope only. |
| Download blob | `GET /v1/vaults/{vault_id}/blobs/{blob_id}` | Returns an encrypted envelope only. |
| Revision CAS commit | `POST /v1/vaults/{vault_id}/revisions` | Atomic head update gated on `base_revision` matching the current head. |

Stable error codes (uppercase, machine-readable): `UNAUTHENTICATED`, `DEVICE_REVOKED`, `FORBIDDEN_SCOPE`, `REVISION_CONFLICT`, `VALIDATION_FAILED`, `BLOB_MISSING`, `BACKEND_UNAVAILABLE`. `REVISION_CONFLICT` is retryable: the client should pull, rebase, and retry the commit.

`remote_write=true` is emitted by the CLI only after the server CAS commit succeeds **and** the local sync-state receipt is written. Dry-run, blob upload only, failed upload, backend unavailable, and revision conflict all render `remote_write=false`. The `--workspace` flag stays for CLI/local config compatibility; at the MLP REST boundary it maps to `vault_id` so the public contract is vault-terminology only.

S3-compatible direct transport:

```bash
pinax cloud backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device laptop \
  --vault ./my-notes
pinax cloud doctor --vault ./my-notes --json
```

OneDrive through rclone direct transport:

```bash
rclone config
pinax cloud backend set rclone \
  --remote onedrive:PinaxSync \
  --workspace personal \
  --device laptop \
  --vault ./my-notes
pinax cloud doctor --vault ./my-notes --json
```

Native Microsoft Graph / OneDrive OAuth is intentionally not part of the MVP. OneDrive examples should use rclone until a separate native Graph adapter design owns device-code login, token refresh, keychain storage, eTag conditional writes, and Graph-specific failure handling.

`cloud login` requires the server configuration fields `--endpoint`, `--workspace`, `--device`, and `--secret-ref`. `--secret-ref` points to the cloud auth token. `--encryption-secret-ref` points to the shared client-side sync encryption secret and falls back to `--secret-ref` only for older configs. For direct S3/rclone backends, Pinax stores provider references such as AWS profile or rclone remote name, not raw secrets.

Cloud Sync state is CLI-authored. The primary human-readable config is `.pinax/cloud/config.yaml`. For S3 direct backends, Pinax stores structured fields instead of an escaped endpoint URI:

```yaml
schema_version: pinax.cloud.config.v1
backend_kind: s3-direct
workspace_id: personal
device_id: laptop
secret_ref: profile://work
s3:
  bucket: notes
  prefix: pinax-sync/
  endpoint: http://127.0.0.1:9010
  region: us-east-1
  profile: work
  path_style: true
```

Older `.pinax/cloud/config.json` files are read for compatibility, but new `pinax cloud backend set ...` writes YAML and removes the legacy JSON config.

## `remote_write=true` rule

A Cloud Sync push may output `remote_write=true` only after the selected transport has durably committed a new revision and Pinax has written the local sync-state receipt. A dry-run, plan, blob upload, manifest upload, failed commit, unsupported backend capability, or unsupported scheme is not a remote write.

Direct local/object-store example:

```bash
pinax init ./device-a --title "Device A"
pinax init ./device-b --title "Device B"
mkdir -p ./device-a/notes
printf '# Alpha\n\nfrom device A\n' > ./device-a/notes/alpha.md
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target cloud --vault ./device-a --yes --json
pinax sync pull --target cloud --vault ./device-b --yes --json
```

Local daemon preview:

```bash
pinax sync daemon run --target cloud --vault ./device-a --yes
pinax sync daemon status --vault ./device-a --json
pinax sync daemon stop --vault ./device-a
```

The first daemon release runs an immediate startup sync cycle, then uses remote-head polling for remote changes and a local watcher for vault file changes. Local runtime state and redacted daemon events are stored under `.pinax/sync-daemon/` and must not be synced as vault content.

The push may report `"remote_write":true` only after the direct transport commits the head revision. The pull reports `"remote_write":false` because it writes the local vault from the committed remote revision.

Unavailable backends, unsupported schemes, and failed commit paths must return a structured partial or error such as `transport_unavailable`, `unsupported_scheme`, `revision_conflict`, or another stable code. They must not silently no-op, create dummy revisions, or emit `remote_write=true`.

## Boundaries

- Server transport: Pinax Cloud Server owns auth/device scope, idempotency, revision CAS, audit, readiness, and encrypted object persistence.
- S3 direct transport: provider credentials are the access boundary; there is no Pinax server-side auth, audit, multi-tenant policy, or rate limiting.
- Rclone direct transport: rclone config is the credential boundary; lock-object commit protection is required before the transport can claim successful remote writes when provider conditional writes are unavailable.
- Embedded Go API/local RPC: local integrations call the same app service; they do not bypass approval, dry-run, conflict, event, or redaction rules.
- Local API: `pinax api serve` is centralized access to one vault and must not be documented as a Cloud Sync transport.

Do not include real endpoint tokens, Authorization headers, cookies, plaintext note bodies, encrypted secret values, raw secret refs, provider stderr, or provider payloads in stdout, stderr, events, fixtures, receipts, object metadata, docs, or examples.

For the full architecture distinction, see [`docs/architecture/cloud-sync-design.md`](../architecture/cloud-sync-design.md).
