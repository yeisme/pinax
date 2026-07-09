# pinax-sync-consumer-hardening Proposal

## Why

Pinax has a parallel sync implementation in `internal/remote/crypto.go` and `internal/app/sync_daemon.go` that does NOT use the shared Capsa SDK yet. Four reliability gaps identified during real-world Tencent COS testing must be fixed in pinax's own code, following the **same contract** as the Capsa SDK change `capsa-sync-reliability`.

When pinax eventually migrates to `import "github.com/yeisme/capsa"`, these fixes will be superseded by the SDK. Until then, both implementations must produce the same user experience.

## What Changes

### 1. Encryption key resolution (`internal/remote/crypto.go`, `internal/profile/profile.go`)

- `ResolveSecretRef` gains `profile://` scheme: parse `~/.aws/credentials`, return `aws_secret_access_key`.
- `DeriveKey` calls `ResolveSecretRef` before PBKDF2. Fail closed on error.
- New `KeyID(secretRef string) string` in `internal/remote/crypto.go`.
- `capsa backend set s3` / `capsa login`: emit `weak_encryption_key` warning when no `--encryption-secret-ref` and `secret_ref` is not `env://` or `keychain://`.
- `capsa doctor`: compare stored key_id vs current. Report `encryption_key_mismatch` on mismatch.

### 2. Daemon env inheritance (`internal/app/sync_daemon.go`)

- `SyncDaemonStart`: add `cmd.Env = os.Environ()` before `cmd.Start()`.

### 3. Push auto-rebase (`internal/app/cloud_sync.go`)

- When `sync push --yes` receives `revision_conflict`: auto-pull, rebuild plan, retry once.
- Conflicts → `conflict_required` projection.
- Single retry, no loop.
- `sync pull` stays pull-only.
- `pinax sync` (bidirectional) unchanged.

### 4. Daemon install/uninstall (`internal/app/sync_daemon.go`, `internal/cli/sync_cmd.go`)

- `pinax sync daemon install`: generate systemd user unit (Linux) or launchd plist (macOS).
- `pinax sync daemon uninstall`: remove unit file.
- Unit uses `capsa-sync-<slug>` naming convention (SDK-level, not `pinax-sync`).
- Uses `EnvironmentFile` for secrets, never inline.
- Does not auto-start the service.
- Windows: `platform_unsupported`.

### 5. UX unification

- All S3/server examples in docs include `--encryption-secret-ref env://PINAX_SYNC_SECRET`.
- `pinax sync` (no subcommand) is the documented default sync entry point.
- `pinax sync daemon start` documented as the foreground-less alternative.
- `pinax sync daemon install` documented as the persistent service option.

## Capabilities

### Modified Capabilities

- `pinax-cloud-sync`: encryption resolution, auto-rebase, daemon env, service install.
- `pinax-cli-tree-modification`: new `install`/`uninstall` subcommands under `sync daemon`.

## Impact

- `internal/remote/crypto.go`: DeriveKey, KeyID.
- `internal/profile/profile.go`: ResolveSecretRef gains `profile://`.
- `internal/app/cloud_sync.go`: push auto-rebase.
- `internal/app/sync_daemon.go`: env fix, Install/Uninstall.
- `internal/cli/sync_cmd.go`: install/uninstall subcommands.
- `docs/commands/sync.md`, `docs/commands/capsa.md`: updated examples.

## Coordination

- SDK contract: `shared/capsa/openspec/changes/capsa-sync-reliability`.
- Auctra consumer: `cli/auctra/openspec/changes/auctra-sync-consumer-parity`.
- Architecture anchor: `docs/architecture/capsa-platform.md`.
