## 1. Local API / RPC review fixes

- [x] 1.1 Add focused coverage proving RPC `Pinax.Note.Read` defaults to bounded `NoteDisplay` facts and does not include full note body unless the request explicitly asks for body exposure.
- [x] 1.2 Ensure the RPC note read path reuses the same application service/display profile as local CLI note read/show and does not construct a separate response shape.
- [x] 1.3 Add REST error mapping coverage for validation, auth, write gate, not found, conflict, unsupported, backend unavailable, and unexpected errors.
- [x] 1.4 Ensure REST responses preserve the failed Projection envelope body with stable `error.code`, English `error.message`, optional `error.hint`, and next actions where useful.

## 2. Cloud Sync server commit evidence

- [x] 2.1 Ensure server transport commits go through `internal/cloudclient.Transport` for current revision, blob batch-check/upload/download, and revision commit.
- [x] 2.2 Propagate non-secret commit evidence across the server transport contract: workspace id in path, device id, request/idempotency id, base revision, requested/committed revision id, manifest id, and blob id set.
- [x] 2.3 Add redaction coverage proving commit metadata, audit, logs, receipts, stdout/stderr, fixtures, and object metadata never include plaintext note paths, note bodies, raw tokens, Authorization/Cookie headers, raw secret refs, provider stderr, or provider payloads.

## 3. Conservative direct transport fallback

- [x] 3.1 Confirm file/S3/rclone direct transports share `internal/cloudsync.ObjectStoreTransport` for object layout, commit CAS, and lock fallback behavior.
- [x] 3.2 Prefer provider conditional writes when reliable; otherwise use `locks/commit.lock` with device id, request id, and expiry before claiming commit success.
- [x] 3.3 Return structured retryable/diagnosable errors with `remote_write=false` when CAS/lock safety cannot be proven.
- [x] 3.4 Add coverage proving unsupported or uncertain direct transports do not create dummy revisions, do not silently no-op, and do not emit `remote_write=true`.

## 4. Sync execution cleanup

- [x] 4.1 Remove the dead sync executor path after verifying all push/pull/diff execution flows are owned by the app service plus `internal/cloudsync` protocol.
- [x] 4.2 Migrate or delete any tests/fixtures tied only to the removed executor path; keep behavior tests on the live service path.
- [x] 4.3 Search callsites to prove no compatibility shim, alias, or stale import keeps the old executor reachable.

## 5. Cloud Sync docs/spec alignment

- [x] 5.1 Update Cloud Sync docs to state current truth: server transport is wired via `internal/cloudclient.Transport`; file/S3/rclone direct transports share `internal/cloudsync.ObjectStoreTransport`.
- [x] 5.2 Remove stale wording that describes server or rclone apply paths as unimplemented, guarded no-ops, or forced `remote_write=false` once commit succeeds.
- [x] 5.3 Keep the durable write gate explicit: dry-run, plan generation, pull, blob-only upload, unsupported path, and uncertain direct fallback remain `remote_write=false`.
- [x] 5.4 Update base OpenSpec specs after implementation so archived and live Cloud Sync wording matches the corrected docs.

## 6. Verification to run after implementation

- [x] 6.1 Run focused API/RPC tests covering RPC note read bounded default and REST error status mapping.
- [x] 6.2 Run focused Cloud Sync tests covering server commit metadata, direct fallback safety, `remote_write` gating, and sync-state evidence.
- [x] 6.3 Run OpenSpec validation for this change and the relevant modified capabilities.