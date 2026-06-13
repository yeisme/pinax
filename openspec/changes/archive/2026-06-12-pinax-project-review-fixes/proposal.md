## Why

Pinax review found a small set of release-blocking contract drifts after Cloud Sync landed: remote note reads could accidentally expose too much by default, REST errors need stable HTTP status mapping without losing Projection bodies, server Cloud Sync commits need enough blob/device metadata to be auditable, direct transports must fail conservatively when CAS cannot be proven, an unused sync executor path should be removed, and Cloud Sync docs/spec text still contains stale “unimplemented/no-op” wording.

This change records the intended fixes without changing Go code or repository-wide docs in this planning pass.

## What Changes

- Tighten Local API/RPC contracts so `Pinax.Note.Read` defaults to bounded `NoteDisplay` output and requires an explicit body-capable display/request before note bodies are returned.
- Require REST handlers to map known Projection error classes to stable HTTP status codes while preserving the failed Projection envelope in the body.
- Align Cloud Sync server commit requirements with the current transport truth: server sync is wired through `internal/cloudclient.Transport`, direct file/S3/rclone paths share `internal/cloudsync.ObjectStoreTransport`, and `remote_write=true` is valid only after durable revision commit plus local sync-state evidence.
- Require server commit metadata to include non-secret workspace/device/request/revision/manifest/blob evidence for audit and retry diagnosis.
- Require direct transports to prefer provider conditional writes, fall back to lock-object CAS only when safe, and otherwise return structured `remote_write=false` errors instead of no-op success.
- Record removal of the dead sync executor path so Cloud Sync execution has one app-service/protocol owner.
- Record follow-up documentation/spec alignment so stale server/rclone “unimplemented” text is removed from Cloud Sync docs and live OpenSpec deltas.

## Capabilities

### Modified Capabilities

- `pinax-cli-remote-api-mode`: bounded RPC note reads and REST/RPC error transport semantics.
- `pinax-cloud-sync`: server/direct transport readiness, commit metadata, conservative fallback, `remote_write` gate, executor ownership, and docs/spec alignment.

## Impact

- OpenSpec only in this task: `openspec/changes/pinax-project-review-fixes/**`.
- Planned implementation/docs follow-up: API/RPC handlers, REST error mapper, Cloud Sync commit metadata, direct transport fallback handling, removal of obsolete sync executor code, Cloud Sync docs/spec wording.
- Non-goals for this task: no Go implementation edits, no repository-wide docs edits, no gates/formatters/validators.