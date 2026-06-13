## Context

The review findings are contract-level issues around already-related Pinax surfaces:

1. Local API/RPC is a thin adapter over application service Projections. It must preserve bounded note-display defaults and return machine-stable errors.
2. Cloud Sync now has real server and direct transports. Server uses `internal/cloudclient.Transport`; file/S3/rclone direct transports share `internal/cloudsync.ObjectStoreTransport`; dry-run/plan/pull remain `remote_write=false`; push may emit `remote_write=true` only after durable revision commit and local sync-state evidence.
3. Some docs/spec wording still describes server/rclone paths as unimplemented or guarded no-ops, which is no longer true and can mislead release messaging.

## Decisions

### 1. RPC note read defaults to bounded display

`Pinax.Note.Read` defaults to the same bounded `NoteDisplay` profile as local note read/show surfaces. Note body exposure is opt-in through an explicit body-capable display/request and remains local Projection output, not an accidental remote default.

### 2. REST status is transport metadata, not the source of truth

REST handlers map stable Projection error classes to HTTP statuses for normal clients, but the response body remains the failed Projection envelope. Suggested mapping:

- request/validation/reference errors: `400`
- auth/session errors: `401`
- write gates, approval-required, insufficient-scope: `403`
- missing vault object or route target: `404`
- revision/conflict/concurrent-write errors: `409`
- unsupported route/capability: `404` or `405` according to the registered route/method
- transport/backend unavailable: `503`
- unexpected internal failure: `500`

Machine clients must continue to key off `error.code`; HTTP status only helps generic REST tooling.

### 3. Cloud Sync commit evidence is required and redacted

Server revision commits need enough metadata for idempotency, audit, and diagnosis: workspace id, vault id, device id, request/idempotency id, base revision, committed revision id, manifest id, blob count or blob digest set reference, status, and timestamp. This metadata must not include plaintext note paths, note bodies, raw tokens, Authorization/Cookie headers, raw secret refs, provider stderr, or provider payloads.

### 4. Direct fallback is conservative

Direct transports may claim success only when the head update is protected by provider CAS/conditional write or by the shared lock-object fallback. If a provider cannot prove either path, the operation returns a structured retryable/diagnosable error with `remote_write=false` after preserving local vault state.

### 5. One Cloud Sync execution owner

The application service and `internal/cloudsync` protocol own sync execution. Any obsolete `internal/sync` executor code that no longer participates in push/pull/diff should be removed rather than kept as a parallel path or compatibility shim.

### 6. Docs/spec alignment follows implementation truth

Cloud Sync docs and specs should stop saying server/rclone apply paths are unimplemented once they are wired. The aligned wording should distinguish current behavior from future improvements without reintroducing no-op language.

## Risks / Checks

- Body leakage: cover RPC `Pinax.Note.Read` default and explicit body request separately.
- Error drift: table-test REST error codes while asserting the Projection body is preserved.
- False sync success: assert `remote_write=true` only after commit metadata and sync-state receipt exist.
- Provider uncertainty: test unsupported/uncertain direct fallback returns `remote_write=false` and does not create dummy revisions.
- Dead path regression: removal should be paired with callsite search proving the old executor is unused or migrated.