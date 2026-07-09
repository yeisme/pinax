# pinax-sync-consumer-hardening Tasks

## Task 1: profile:// secret resolution

**Files**: `internal/profile/profile.go`, `internal/profile/profile_test.go`

- [ ] Add `profile://` scheme to `ResolveSecretRef`.
- [ ] Implement `resolveAWSProfileSecret(profileName string) (string, error)`: parse `~/.aws/credentials` (INI), return `aws_secret_access_key`. Use `AWS_SHARED_CREDENTIALS_FILE` if set.
- [ ] Tests: fixture credentials file, missing profile (error), `env://` still works, `plain:` still works.

## Task 2: DeriveKey resolves before PBKDF2

**Files**: `internal/remote/crypto.go`, `internal/remote/crypto_test.go`

- [ ] `DeriveKey` calls `profile.ResolveSecretRef` before PBKDF2.
- [ ] Fail closed on resolution error.
- [ ] Add `KeyID(secretRef string) string`.
- [ ] Tests: `profile://` ref produces same key as raw secret. `env://` produces same key as raw value. Unresolvable ref errors.

## Task 3: Weak-key warning

**Files**: `internal/app/cloud.go`, `internal/app/cloud_sync.go`

- [ ] `CloudBackendSetS3` / `CapsaBackendSetS3`: emit `weak_encryption_key` warning when `encryption_secret_ref` is empty and `secret_ref` is not `env://` or `keychain://`.
- [ ] `capsa doctor`: compare stored `key_id` vs current `crypto.KeyID()`. Report `encryption_key_mismatch`.
- [ ] Tests: warning appears for bare `profile://` ref without `--encryption-secret-ref`.

## Task 4: Daemon env inheritance

**Files**: `internal/app/sync_daemon.go`

- [ ] `SyncDaemonStart`: add `cmd.Env = os.Environ()`.
- [ ] Test: child process inherits `PINAX_SYNC_SECRET`.

## Task 5: Push auto-rebase

**Files**: `internal/app/cloud_sync.go`

- [ ] In `cloudSyncPush`, when commit returns `revision_conflict` and `req.Yes`: pull, rebuild plan, retry once.
- [ ] Conflicts → `conflict_required` projection with next actions.
- [ ] Single retry, no loop.
- [ ] Tests: auto-rebase success, conflict detection, retry-fail.

## Task 6: Daemon install/uninstall

**Files**: `internal/app/sync_daemon.go`, `internal/cli/sync_cmd.go`

- [ ] `SyncDaemonInstall`: detect platform, resolve paths, generate unit from template, write.
- [ ] `SyncDaemonUninstall`: remove unit file.
- [ ] Linux: `~/.config/systemd/user/capsa-sync-<slug>.service`.
- [ ] macOS: `~/Library/LaunchAgents/com.yeisme.capsa-sync.<slug>.plist`.
- [ ] Windows: `platform_unsupported`.
- [ ] `EnvironmentFile` for secrets, never inline.
- [ ] CLI subcommands `install`/`uninstall` under `sync daemon`.
- [ ] Projection includes enable command.
- [ ] Tests: Linux unit content, macOS plist XML, Windows error.

## Task 7: Documentation

**Files**: `docs/commands/sync.md`, `docs/commands/capsa.md`

- [ ] All S3/server examples include `--encryption-secret-ref env://PINAX_SYNC_SECRET`.
- [ ] `pinax sync daemon install` section in `sync.md`.
- [ ] Weak-key warning explanation in `capsa.md`.
- [ ] Encryption migration note in troubleshooting.

## Task 8: Verification

- [ ] `go build ./...`
- [ ] `go test ./internal/remote ./internal/profile ./internal/app ./cmd/pinax -count=1`
- [ ] `gofmt -l .` (empty)
- [ ] `golangci-lint run` (0 issues)
- [ ] `openspec validate pinax-sync-consumer-hardening --strict`
- [ ] `task check`
