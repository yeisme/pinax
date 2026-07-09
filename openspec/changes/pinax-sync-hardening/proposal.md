# pinax-sync-hardening Proposal

## Why

Real-world testing against Tencent Cloud COS (S3 direct) exposed four gaps that prevent Pinax Sync from being a trustworthy daily backup tool:

1. **Encryption key uses reference string instead of real secret.** When `secret_ref` is `profile://tencent-cos-pinax` and `encryption_secret_ref` is unset, `EncryptionSecretRef()` falls back to `secret_ref`, and `DeriveKey()` uses the literal string `profile://tencent-cos-pinax` as PBKDF2 input — not the actual `aws_secret_access_key`. All COS blobs are encrypted with a publicly-known weak key.
2. **Daemon child process does not inherit environment variables.** `SyncDaemonStart` spawns `exec.Command` without setting `cmd.Env`, so `env://PINAX_SYNC_SECRET` is invisible to the daemon child. The daemon crashes on first encryption attempt.
3. **`sync push` returns `revision_conflict` instead of auto-rebasing.** First-time backup against a vault with existing remote state fails immediately. Users must know to use `pinax sync` (bidirectional) instead.
4. **No `pinax sync daemon install` for OS service registration.** Users must hand-craft systemd units or launchd plists. The daemon `start` command only fork-execs a bare child that dies when the terminal closes.

## What Changes

### 1. Encryption key resolution (`internal/remote/crypto.go`, `internal/remote/state.go`, `internal/profile/profile.go`)

- `DeriveKey` SHALL resolve `profile://`, `env://`, and `keychain://` references through `ResolveSecretRef` before PBKDF2 key derivation.
- If `secret_ref` is a `profile://` reference and no `encryption_secret_ref` is configured, `capa backend set s3` and `capsa login` SHALL emit a `weak_encryption_key` warning in the projection.
- New `--encryption-secret-ref` flag SHALL be recommended in all S3/server setup examples.

### 2. Daemon environment inheritance (`internal/app/sync_daemon.go`)

- `SyncDaemonStart` SHALL set `cmd.Env = os.Environ()` so the child process inherits `PINAX_SYNC_SECRET` and all other environment variables.
- `SyncDaemonRun` (foreground) already inherits the parent env; no change needed there.

### 3. `sync push` revision_conflict auto-rebase (`internal/app/cloud_sync.go`)

- When `sync push` receives `revision_conflict` and `--yes` is set, it SHALL automatically pull the remote revision, rebase the local plan, and retry the push once.
- If the rebase produces conflicts, it SHALL return `conflict_required` with conflict next actions.
- `sync pull` SHALL NOT auto-push (remains pull-only).
- The bidirectional `pinax sync` command SHALL keep its existing pull-then-push behavior.

### 4. `pinax sync daemon install/uninstall` (`internal/app/sync_daemon.go`, `internal/cli/sync_cmd.go`)

- New `pinax sync daemon install` SHALL generate a platform-appropriate service unit:
  - **Linux**: systemd user unit at `~/.config/systemd/user/pinax-sync-<vault>.service`
  - **macOS**: launchd plist at `~/Library/LaunchAgents/com.yeisme.pinax-sync.<vault>.plist`
  - **Windows**: not implemented in this change (returns `platform_unsupported`)
- The unit SHALL run `pinax sync daemon run --target capsa --vault <vault> --yes` with `Restart=on-failure`.
- `pinax sync daemon uninstall` SHALL remove the generated unit file.
- Both commands SHALL NOT start/stop the service automatically; user runs `systemctl --user enable --now pinax-sync-<vault>` or `launchctl load`.

## Capabilities

### New Capabilities

- `pinax-sync-daemon-service`: OS service registration for the sync daemon.

### Modified Capabilities

- `pinax-cloud-sync`: encryption key resolution, push auto-rebase, daemon env inheritance.

## Impact

- `internal/remote/crypto.go`: `DeriveKey` calls `ResolveSecretRef`.
- `internal/remote/state.go`: `EncryptionSecretRef` adds weak-key detection.
- `internal/app/cloud_sync.go`: push auto-rebase on `revision_conflict`.
- `internal/app/sync_daemon.go`: `cmd.Env = os.Environ()`, new `SyncDaemonInstall`/`SyncDaemonUninstall`.
- `internal/cli/sync_cmd.go`: new `install`/`uninstall` subcommands.
- `internal/profile/profile.go`: `ResolveSecretRef` gains `profile://` scheme.
- Tests: encryption key resolution, daemon env inheritance, push auto-rebase, service unit generation.
- Docs: `sync.md`, `capsa.md` updated with `--encryption-secret-ref` and daemon install.
