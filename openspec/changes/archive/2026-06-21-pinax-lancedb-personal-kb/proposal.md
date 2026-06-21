## Why

Pinax needs a practical personal knowledge-base layer for Markdown and plain-text material without turning Cloud Sync into a plaintext hosted notes backend. The user-selected direction is local-first multi-device usage, MinIO/S3-compatible encrypted sync, and per-device local semantic projections that can be rebuilt from the vault.

This change adds a first vertical slice for `pinax kb`: import text/Markdown into the vault, rebuild a local LanceDB semantic projection through a Python sidecar, and expose bounded semantic search/context for agents.

## What Changes

- Add `pinax kb import/rebuild/refresh/search/context` commands.
- Add a semantic projection adapter boundary with `backend=lancedb`, provider metadata, and `pinax.kb.sidecar.v1` stdin/stdout protocol.
- Add Gemini as the default embedding provider boundary and `fake` as the test/local validation provider.
- Store semantic projection under `.pinax/kb/lancedb/` through `pinax-lancedb-sidecar` and keep it rebuildable, not synchronized as Cloud Sync data.
- Keep the Pinax Go CLI pure-Go; the Python sidecar owns the LanceDB runtime dependency.
- Keep Cloud Sync behavior unchanged: MinIO/S3 transports synchronize encrypted vault revisions only.

## Compatibility

This change is additive. It does not remove or rename existing CLI commands, output fields, config keys, database columns, or sync protocol fields.

## Non-Goals

- No direct LAN peer discovery or device-to-device transfer in v1.
- No synchronization of LanceDB files between devices.
- No hosted plaintext Pinax Cloud notes backend.
- No Web Dashboard or mobile client priority in this slice.
