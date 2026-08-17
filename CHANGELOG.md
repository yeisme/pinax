# Changelog

## v0.2.0 (2026-08-17)

First minor release after v0.1.x. Two breaking changes; read the migration notes before upgrading a synced vault.

### Breaking

- **`pinax kb` command tree removed.** Pinax no longer ships a vector/embedding/RAG runtime or the Inferrum/LanceDB sidecar; former `pinax kb *` commands now report unknown command and `/v1/kb/review/*` returns 404. `kb.*` config keys and `PINAX_KB_*` env vars are gone (leftover YAML keys are inert). Export Markdown and use an external RAG owner: `pinax export markdown` + `pinax search`. See `docs/commands/kb.md`.
- **Sync encryption key derivation v2** (PBKDF2-SHA256 600k iterations, per-secret salt). Envelopes written by v0.2.0 cannot be read by older binaries — upgrade every device that syncs the vault before pushing. Legacy envelopes remain readable; migrate with `pinax sync pull --yes && pinax sync push --yes` and verify with `pinax sync keys --json` (`remote_derivation=v2`). Runbook: `docs/commands/sync.md`.

### Security

- Local API: a failed `--token-file` load now refuses all requests instead of silently downgrading to an unrestricted temp token; no-auth mode and the dashboard validate the Host header (DNS rebinding); RPC dry runs are gated by allow-write like the REST surface; `redaction.Cloud` scrubs S3/rclone credential forms.
- Publish: folder/mapping registries are cross-process locked and written atomically; provider results without an object token are rejected.

### Fixed

- Watcher error forwarding can no longer deadlock event delivery; debounce flush no longer hangs shutdown.
- `cloudclient` returns a stable `VAULT_ID_REQUIRED` error instead of panicking on an unconfigured workspace.
- Push receipts count re-encrypted uploads; sync evidence/monitor failures warn on stderr instead of vanishing.
- Concurrent root-command construction (parallel CLI tests, in-process API servers) no longer crashes on cobra template registration.

### Performance

- Full test suite ~2.5× faster (t.Parallel adoption; test-only KDF cost override), race gate ~6× faster.
- `publish doc push --all`: single vault snapshot per run (was O(N²) rescans) and the native-docx second pass only re-pushes notes with newly published cross-doc links; prepared packages are pruned to the newest three per note.
- Monitor run records are pruned to the newest 200.

### Added

- `pinax sync keys` — key derivation status and remote envelope classification for migration verification.
- Agent runtime RPC methods (`Pinax.Agent.Context/Memory.Recall/Continuity/Inbox/TrustCenter`) reachable via `/v1/rpc`.
- PR CI workflow (`task ci` incl. race leg), unified sync wire types (`internal/syncwire`).

## v0.1.11 (2026-07-23)

- Release builds use vendored modules (`-mod=vendor`).

## v0.1.10 (2026-07-21)

- Agent continuity runtime and identity-safe sync.

## v0.1.8 (2026-07-18)

- Repository-encrypted S3/COS credential bootstrap, agent memory runtime, sync hardening waves. See `git log v0.1.7..v0.1.8`.
