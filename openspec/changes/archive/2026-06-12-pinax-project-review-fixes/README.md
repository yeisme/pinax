# pinax-project-review-fixes

OpenSpec change for the Pinax review fixes that must land before release messaging:

- RPC `Pinax.Note.Read` defaults to bounded note-display output.
- REST errors map to stable HTTP status classes while preserving Projection envelopes.
- Cloud Sync server commits carry redacted blob/device metadata.
- Direct transports fail conservatively when CAS/lock safety cannot be proven.
- Dead sync executor code is removed instead of kept as a parallel path.
- Cloud Sync docs/specs stop describing wired server/rclone transports as unimplemented or no-op paths.

Files in this change:

- `proposal.md` — scope, impact, and capability list.
- `design.md` — decisions and risk checks.
- `tasks.md` — planned implementation/doc/spec checklist.
- `specs/pinax-cli-remote-api-mode/spec.md` — RPC note read and REST error semantics deltas.
- `specs/pinax-cloud-sync/spec.md` — Cloud Sync transport, commit metadata, fallback, executor, and docs/spec alignment deltas.
