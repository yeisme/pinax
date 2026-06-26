# Pinax Cloud Sync Architecture

Pinax remote behavior has three different patterns. They must not be described as the same feature.

## Pattern A: Local API centralized access

`pinax api serve` exposes one local vault through a loopback REST/RPC projection adapter.

```text
client CLI / agent
  -> pinax --api-url http://127.0.0.1:8787 ...
  -> pinax api serve
  -> one server-side Markdown vault
```

Characteristics:

- Source of truth: the single vault attached to the running `pinax api serve` process.
- Client state: the caller does not maintain an independent synchronized vault for these remote commands.
- Transport: `POST /v1/rpc` and selected REST routes returning Pinax Projection envelopes.
- Writes: controlled by `--allow-write`, `yes=true`, and command-level approval/snapshot gates.
- Deployment boundary: local/loopback by default; cross-machine use should go through an explicit tunnel or trusted local network wrapper.
- Current status: implemented for supported read and controlled mutation routes.

This pattern is useful for dashboards, local agents, and a central always-on workstation. It is not multi-device file synchronization and must not be documented as a Cloud Sync transport.

## Pattern B: Cloud Sync protocol with pluggable transport

Pinax Cloud Sync is the intended multi-device design. Each device keeps its own local Markdown vault and syncs encrypted manifests/blobs through a transport.

```text
laptop vault        phone vault         desktop vault
    |                   |                    |
    | encrypted blobs + encrypted manifest   |
    v                   v                    v
        Cloud Sync Protocol + Transport
          server | s3-direct | rclone-direct | embedded
```

The word `cloud` names the synchronization protocol, not necessarily a hosted Pinax Cloud service.

### Transport modes

| Transport | Endpoint example | Needs remote Pinax Cloud service | Implemented status | Trade-off |
| --- | --- | --- | --- | --- |
| `server` | `https://cloud.example.test` | Yes | Implemented through the shared sync engine and `internal/cloudclient.Transport` for current revision, blob batch-check/upload/download, and revision commit. | Server gives auth, audit, policy, and multi-tenant control, but requires backend deployment. |
| `s3-direct` | `s3://notes/pinax-sync` | No | Implemented through the direct object-store engine over `remote.BlobStore`; local/file and S3-compatible backends commit with CAS semantics where supported. | Provider credentials define access; no Pinax server-side auth/audit. |
| `rclone-direct` | `rclone://onedrive/PinaxSync` | No | Implemented through the shared object-store transport and the rclone-backed `remote.BlobStore`; lock-object commit protection is used where provider conditional writes are unavailable. | Broad provider reach, but weaker conditional-write semantics. |
| `embedded` | Go API / local RPC | No | Implemented through `app.Service` and local RPC methods such as `Pinax.Sync.Push` / `Pinax.Sync.Pull`. | Same-process trust boundary; not a network service. |

All transports must expose the same logical operations:

- `CurrentHead`: read current revision and manifest blob id.
- `BatchCheck`: return missing encrypted blob ids.
- `PutBlob` / `GetBlob`: store and retrieve encrypted envelopes.
- `PutManifest` / `GetManifest`: store and retrieve encrypted manifest envelopes.
- `CommitRevision`: atomically move head from `base_revision` to `new_revision`, or return `revision_conflict`.

Unsupported schemes, unavailable backends, and failed commit paths must return stable partial/error codes such as `unsupported_scheme`, `transport_unavailable`, `unauthorized`, `backend_unavailable`, or `revision_conflict`. They must not silently no-op, write dummy revisions, or emit `remote_write=true`.

### Local daemon layer

`pinax sync daemon` is a client-side process that reuses the same Cloud Sync transport operations. It does not add a new remote service role.

```text
local watcher + remote head poller
  -> serial daemon queue
  -> sync pull when remote head is newer
  -> sync push when local manifest is dirty
  -> local state/events under .pinax/sync-daemon/
```

The daemon runs one startup pull-before-push cycle before waiting for the next timer or file event. Remote changes are detected by polling `CurrentHead` in the first release. Local changes are detected by a vault watcher with scan fallback. `.git/`, `.pinax/`, LanceDB projections, provider caches, and daemon runtime files are ignored so generated state does not trigger sync loops.

The daemon must acquire a per-vault runner lock and the shared sync operation lock. It must pause with `conflict_required` when pull creates conflict copies, and it must not emit `remote_write=true` unless the underlying push path completed the durable revision commit and local sync-state receipt.

## Object-store layout for direct transports

Direct S3/rclone transports store Cloud Sync objects under a configured prefix:

```text
{prefix}/
  protocol.json
  workspaces/{workspace_id}/vaults/{vault_id}/
    head.json
    locks/commit.lock
    revisions/{revision_id}.json
    manifests/sha256/{first2}/{next2}/{manifest_blob_id}.json
    blobs/sha256/{first2}/{next2}/{blob_id}.json
    devices/{device_id}.json
    audit/YYYY/MM/DD/{event_id}.json
```

`head.json` is the trunk pointer:

```json
{
  "schema_version": "pinax.cloud.head.v1",
  "vault_id": "vault_abc",
  "current_revision": "rev_20260612_001",
  "manifest_blob_id": "sha256:abc",
  "updated_at": "2026-06-12T12:00:00Z",
  "updated_by_device": "laptop"
}
```

`revisions/{revision_id}.json` stores revision metadata only:

```json
{
  "schema_version": "pinax.cloud.revision.v1",
  "revision_id": "rev_20260612_001",
  "parent_revision_id": "rev_20260611_009",
  "manifest_blob_id": "sha256:manifest",
  "blob_ids": ["sha256:blob1", "sha256:blob2"],
  "created_at": "2026-06-12T12:00:00Z",
  "created_by_device": "laptop"
}
```

Blob and manifest objects are encrypted envelopes. They must not include plaintext note bodies, plaintext paths, raw tokens, Authorization headers, cookies, raw secret refs, provider stderr, or provider payloads.

```json
{
  "schema_version": "pinax.cloud.envelope.v1",
  "alg": "AES-256-GCM",
  "key_id": "local-key-v1",
  "nonce": "base64",
  "ciphertext": "base64",
  "plain_sha256": "sha256"
}
```

The plaintext manifest exists only on the client after decryption. It may contain paths such as `notes/a.md`; direct transports and remote services store only the encrypted manifest envelope.

## CAS and locking

`remote_write=true` requires a durable revision commit in every transport.

### Pinax Cloud server transport

The server uses database transaction semantics:

```text
transaction:
  load vault head
  if head != base_revision: return revision_conflict
  verify referenced encrypted blobs exist
  create revision
  update vault.head_revision_id
  append redacted audit fact
commit
```

A successful server commit must be idempotent for repeated request ids / idempotency keys. Unauthorized, insufficient-scope, backend-unavailable, and revision-conflict responses must preserve local Markdown and return stable error codes with redacted diagnostics.

### S3 direct transport

S3 direct should prefer provider conditional write when available:

```text
read head.json -> etag
upload blobs/manifests/revision object
PUT head.json If-Match: <etag>
```

If the provider does not support usable `If-Match`, the transport must use a short-lived lock object:

```text
locks/commit.lock
  device_id
  request_id
  expires_at
```

The lock must expire. A client crash must not permanently block other devices.

### Rclone / OneDrive direct transport

OneDrive should be supported first through rclone, not native Microsoft Graph. Rclone gives one adapter for OneDrive, Dropbox, Google Drive, WebDAV, and similar remotes. Because many rclone providers lack strong conditional writes, this transport must use lock objects and must treat uncertain write state as retryable/diagnosable instead of silently claiming success.

Native OneDrive through Microsoft Graph is non-MVP. It may be added later as a separate transport because OAuth/device-code/token-refresh, keychain storage, Graph eTags, `If-Match`, quota errors, and Graph throttling need their own design and tests.

## Current CLI behavior

Server-style configuration:

```bash
pinax cloud login \
  --endpoint https://cloud.example.test \
  --workspace ws_123 \
  --device laptop \
  --secret-ref env://PINAX_CLOUD_TOKEN \
  --vault ./my-notes
```

Explicit backend selection:

```bash
pinax cloud backend set server --endpoint https://cloud.example.test --workspace ws_123 --device laptop --secret-ref env://PINAX_CLOUD_TOKEN --vault ./my-notes
pinax cloud backend set s3 --bucket notes --prefix pinax-sync/ --region us-east-1 --profile work --workspace personal --device laptop --vault ./my-notes
pinax cloud backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes
```

`cloud login` remains a shortcut for `cloud backend set server`.

Cloud state is stored as CLI-authored YAML at `.pinax/cloud/config.yaml`. S3-compatible transports store `s3.bucket`, `s3.prefix`, `s3.endpoint`, `s3.region`, `s3.profile`, and `s3.path_style` as structured fields so operators do not have to read or hand-edit URL-escaped query parameters. Legacy `.pinax/cloud/config.json` remains read-compatible only.

A push against any transport has one write-success point:

```bash
pinax sync push --target cloud --vault ./my-notes --yes --json
```

The command may report `remote_write=true` only after the selected transport durably commits the head revision and Pinax writes local sync-state evidence. A dry-run, plan generation, object upload without successful head/revision commit, failed commit, unsupported backend capability, or unsupported scheme is not a remote write.

## Implementation boundary

Pinax CLI owns the protocol engine and local vault mutation:

- cloud backend profile/state commands;
- vault scan and manifest construction;
- client-side encryption/decryption;
- sync plan and conflict application;
- `cloudsync.Transport` interface;
- HTTP server transport adapter through `internal/cloudclient`;
- object-store direct transport through `internal/cloudsync.ObjectStoreTransport` and `internal/remote` stores;
- rclone direct transport through the shared object-store path and rclone-backed `remote.BlobStore`;
- local RPC/Go API entrypoints that call the same app service;
- output contract, sync run receipt, event, and redaction tests.

Pinax Cloud backend service owns only server transport responsibilities:

- HTTP API routing;
- auth/device state;
- revision CAS and concurrency;
- encrypted object persistence;
- redacted audit logs and operational health;
- idempotency and retry-safe mutation semantics.

Direct transports do not provide Pinax server-side auth, server audit, multi-tenant policy, or rate limiting. Their security boundary is the provider credential reference and the local Pinax approval flow.

## Redaction and evidence surfaces

The redaction boundary covers stdout, stderr, `--events`, sync-state, sync run receipts, `.pinax/events.jsonl`, test fixtures, object keys, object metadata, fake backend logs, provider stderr, and archive evidence. These surfaces may include stable ids, counts, timestamps, backend kind, transport kind, revision ids, and redacted next actions. They must not include plaintext note bodies, plaintext object keys derived from note paths, raw tokens, Authorization headers, cookies, raw secret refs, provider stderr, or provider payloads.

When local conflict inspection intentionally shows note content, it must be an explicit local command such as `pinax sync conflicts show <file> --json`; that local content view is not Cloud transport evidence and must not be copied into backend fixtures or archive receipts.

## Non-Cloud CLI drift exclusion

The following CLI contract drift items are not part of the `pinax-cloud-distributed-sync` completion definition: `note links --broken-only`, `note backlinks --include-broken`, `note orphans --mode`, root help `Other` grouping, `docs/commands/README.md` command-map drift, and `cloud backend set server` documentation polish that is not needed to explain Cloud Sync transport boundaries. A follow-up owner change should be created separately, recommended name: `pinax-cli-contract-drift`.

This Cloud Sync change may mention those items only as explicit exclusions. It must not mix unrelated CLI repairs into Cloud Sync readiness.

## Release verification notes

Before release, the main integrator should run and record at least:

```bash
go test ./cmd/pinax ./internal/app ./internal/cloudsync ./internal/cloudclient -run 'Cloud|Sync|Transport|ObjectStore|Direct|Conflict|Redaction' -count=1
go test ./tests/e2e -run 'TestCloud|TestSyncOfflineAndRedaction' -count=1
openspec validate pinax-cloud-distributed-sync
openspec validate --all
```

Expected release evidence must prove: server, S3/file, rclone, and embedded/local API paths share the sync engine; unsupported schemes and failed commits do not no-op; durable revision commit plus local sync-state evidence is the only source of `remote_write=true`; conflicts are lossless; redaction scans cover stdout/stderr/events/receipts/fixtures/object metadata; and Local API remains documented separately from Cloud Sync Protocol.

The final integration is complete only when two independent local vaults can sync through at least one fake/local transport and one direct or server transport, and both devices converge without plaintext leaking to server logs, stdout, events, fixtures, object metadata, receipts, or diagnostics.
