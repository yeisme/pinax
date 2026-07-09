# capsa Command

`pinax capsa` manages local state for the distributed Pinax Capsa Sync protocol. It is not the same feature as `pinax api serve`: `api serve` exposes one centralized vault through local REST/RPC, while Capsa Sync keeps a separate local vault on every device and exchanges encrypted revisions, manifests, and blobs through a selected transport.

The word `capsa` names the sync protocol, not necessarily a hosted Capsa service. Pinax Capsa Sync can use the current server-style `capsa login` transport, S3-compatible direct storage, rclone-backed providers such as OneDrive, or embedded Go API/local RPC entrypoints that call the same app service.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax capsa login` | Shortcut for configuring a server-style Capsa backend endpoint, workspace, device, and secret reference. | Writes Capsa sync state; does not save the raw token. |
| `pinax capsa backend set s3` | Configures direct S3/MinIO/R2-compatible object storage transport. | Writes Capsa sync state; does not save access key or secret key. |
| `pinax capsa backend set rclone` | Configures an rclone direct transport such as an existing OneDrive remote. | Writes Capsa sync state; does not save OAuth refresh tokens. |
| `pinax capsa status` | Views Capsa sync state. | Does not write. |
| `pinax capsa logout` | Logs out or clears the local device/backend state. | Writes Capsa sync state. |
| `pinax capsa doctor` | Diagnoses Capsa sync state, transport boundaries, and encryption key health (`encryption_key_mismatch`, `weak_encryption_key`). | Does not write. |

## Centralized Local API vs Capsa Sync Protocol

| Pattern | Command surface | Vault ownership | Current status |
| --- | --- | --- | --- |
| Centralized local access | `pinax api serve`, `pinax --api-url ...`, local RPC routes | One running `pinax api serve` process owns one server-side vault. Callers do not keep an independent synchronized vault. | Implemented for registered local API routes. Not a Capsa Sync transport. |
| Capsa Sync server transport | `pinax capsa login`, then `pinax sync --target capsa` | Every device owns its own local vault. Capsa Server coordinates encrypted blob/revision exchange. | Implemented through the shared sync engine and `internal/cloudclient.Transport`; `remote_write=true` is emitted only after a durable revision commit and local sync-state receipt. |
| Capsa Sync S3 direct transport | `pinax capsa backend set s3`, then `pinax sync --target capsa` | Every device owns its own local vault. The provider stores encrypted Capsa Sync objects. | Implemented for the direct object-store engine; `remote_write=true` is emitted only after the head/revision commit succeeds. |
| Capsa Sync rclone direct transport | `pinax capsa backend set rclone`, then `pinax sync --target capsa` | Every device owns its own local vault. rclone is the provider credential boundary. | Implemented through the shared object-store sync path; lock-object commit protection covers providers without reliable conditional writes. |
| Embedded Go API / local RPC | `app.Service` methods and `Pinax.Sync.Push` / `Pinax.Sync.Pull` local RPC | Same local app service and vault mutation rules as CLI. | Implemented for local callers. This is not `pinax api serve` centralized remote mode. |

The distributed design is similar to Obsidian Sync: laptop, phone, and desktop all keep local vaults. The transport stores encrypted sync artifacts and revision order; it does not become the plaintext note source of truth.

`pinax sync daemon` is the local automation layer on top of this protocol. It runs on each device, watches local vault changes, polls the remote Capsa Sync head for remote changes, and then invokes the same pull/push engine as explicit CLI commands. It is not a hosted Capsa service, and it does not give the transport plaintext note access.

## User-runnable setup examples

Server transport configuration:

```bash
pinax capsa login \
  --endpoint https://capsa.example.test \
  --workspace ws_123 \
  --device laptop \
  --secret-ref env://PINAX_CAPSA_TOKEN \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax capsa status --vault ./my-notes --json
pinax capsa doctor --vault ./my-notes
```

### Pinax Capsa Sync MLP server contract

The server transport speaks the Pinax Capsa Sync MLP (minimum lovable product) REST contract. The public protocol uses `vault_id` terminology and never transfers plaintext Markdown:

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
export PINAX_SYNC_SECRET="your-encryption-secret"
pinax capsa backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device laptop \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax capsa doctor --vault ./my-notes --json
```

### S3 addressing style and provider auto-detection

Pinax auto-detects the S3 addressing style from the endpoint URL:

| Provider | Endpoint pattern | Default style | Reason |
| --- | --- | --- | --- |
| Tencent COS | `*.myqcloud.com` / `*.myqcloud.com.cn` | **virtual-hosted** | COS rejects path-style requests with `PathStyleDomainForbidden` |
| MinIO / custom | Any other endpoint with a custom `--endpoint` | **path-style** | Most S3-compatible appliances require path-style |
| AWS S3 | No `--endpoint` (default) | virtual-hosted | AWS SDK default |

Override with `--addressing-style path` or `--addressing-style virtual-hosted` when needed. For Tencent COS, do not set `--addressing-style path`.

### Tencent Cloud COS example

```bash
# AWS shared profile (~/.aws/credentials)
# [tencent-cos-pinax]
# aws_access_key_id = AKIDxxxxxxxxxxxx
# aws_secret_access_key = <secret>

export PINAX_SYNC_SECRET="your-encryption-secret"
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
pinax capsa doctor --vault ./my-notes --json
```

COS region endpoints: `cos.ap-guangzhou.myqcloud.com`, `cos.ap-beijing.myqcloud.com`, `cos.ap-shanghai.myqcloud.com`, `cos.ap-chengdu.myqcloud.com`, etc. Pinax suppresses checksum validation warnings for S3-compatible providers (commit `495978f`).

### Weak encryption key warning

If you configure S3 direct or server transport without `--encryption-secret-ref`, Pinax warns `weak_encryption_key`. This means the encryption key is derived from the same credential reference used for transport auth (e.g., `profile://tencent-cos-pinax`).

For production use, always set a dedicated encryption key:

```bash
export PINAX_SYNC_SECRET="your-dedicated-encryption-secret"
pinax capsa backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --profile tencent-cos-pinax \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
```

OneDrive through rclone direct transport:

```bash
rclone config
pinax capsa backend set rclone \
  --remote onedrive:PinaxSync \
  --workspace personal \
  --device laptop \
  --vault ./my-notes
pinax capsa doctor --vault ./my-notes --json
```

Native Microsoft Graph / OneDrive OAuth is intentionally not part of the MVP. OneDrive examples should use rclone until a separate native Graph adapter design owns device-code login, token refresh, keychain storage, eTag conditional writes, and Graph-specific failure handling.

`capsa login` requires the server configuration fields `--endpoint`, `--workspace`, `--device`, and `--secret-ref`. `--secret-ref` points to the Capsa auth token. `--encryption-secret-ref` points to the shared client-side sync encryption secret and falls back to `--secret-ref` only for older configs. For direct S3/rclone backends, Pinax stores provider references such as AWS profile or rclone remote name, not raw secrets.

Capsa Sync state is CLI-authored. The primary human-readable config is `.pinax/cloud/config.yaml`. For S3 direct backends, Pinax stores structured fields instead of an escaped endpoint URI:

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

Older `.pinax/cloud/config.json` files are read for compatibility, but new `pinax capsa backend set ...` writes YAML and removes the legacy JSON config.

## `remote_write=true` rule

A Capsa Sync push may output `remote_write=true` only after the selected transport has durably committed a new revision and Pinax has written the local sync-state receipt. A dry-run, plan, blob upload, manifest upload, failed commit, unsupported backend capability, or unsupported scheme is not a remote write.

Direct local/object-store example:

```bash
pinax init ./device-a --title "Device A"
pinax init ./device-b --title "Device B"
mkdir -p ./device-a/notes
printf '# Alpha\n\nfrom device A\n' > ./device-a/notes/alpha.md
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target capsa --vault ./device-a --yes --json
pinax sync pull --target capsa --vault ./device-b --yes --json
```

Local daemon preview:

```bash
pinax sync daemon run --target capsa --vault ./device-a --yes
pinax sync daemon status --vault ./device-a --json
pinax sync daemon stop --vault ./device-a
```

The first daemon release runs an immediate startup sync cycle, then uses remote-head polling for remote changes and a local watcher for vault file changes. Local runtime state and redacted daemon events are stored under `.pinax/sync-daemon/` and must not be synced as vault content.

The push may report `"remote_write":true` only after the direct transport commits the head revision. The pull reports `"remote_write":false` because it writes the local vault from the committed remote revision.

Unavailable backends, unsupported schemes, and failed commit paths must return a structured partial or error such as `transport_unavailable`, `unsupported_scheme`, `revision_conflict`, or another stable code. They must not silently no-op, create dummy revisions, or emit `remote_write=true`.

## Boundaries

- Server transport: Capsa Server owns auth/device scope, idempotency, revision CAS, audit, readiness, and encrypted object persistence.
- S3 direct transport: provider credentials are the access boundary; there is no Pinax server-side auth, audit, multi-tenant policy, or rate limiting.
- Rclone direct transport: rclone config is the credential boundary; lock-object commit protection is required before the transport can claim successful remote writes when provider conditional writes are unavailable.
- Embedded Go API/local RPC: local integrations call the same app service; they do not bypass approval, dry-run, conflict, event, or redaction rules.
- Local API: `pinax api serve` is centralized access to one vault and must not be documented as a Capsa Sync transport.

Do not include real endpoint tokens, Authorization headers, cookies, plaintext note bodies, encrypted secret values, raw secret refs, provider stderr, or provider payloads in stdout, stderr, events, fixtures, receipts, object metadata, docs, or examples.

For the full architecture distinction, see [`docs/architecture/cloud-sync-design.md`](../architecture/cloud-sync-design.md).
