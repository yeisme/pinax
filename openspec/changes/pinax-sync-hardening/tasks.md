# pinax-sync-hardening Tasks

## Task 1: Fix encryption key resolution

**Files**: `internal/remote/crypto.go`, `internal/profile/profile.go`

- [ ] Add `profile://` scheme to `ResolveSecretRef` in `internal/profile/profile.go`: parse `~/.aws/credentials` for the named profile, return `aws_secret_access_key`.
- [ ] Add `resolveAWSProfileSecret` helper in `internal/profile/profile.go` (read-only file parse, no subprocess).
- [ ] Modify `DeriveKey` in `internal/remote/crypto.go` to call `profile.ResolveSecretRef` before PBKDF2. Fail closed on resolution error.
- [ ] Add unit tests: `profile://` resolution from fixture credentials file, `env://` still works, `plain:` still works, unknown profile returns error.
- [ ] Add unit test: `DeriveKey` with `profile://` ref produces same key as `DeriveKey` with the raw secret value.

## Task 2: Weak-key warning

**Files**: `internal/app/cloud.go`, `internal/remote/state.go`

- [ ] In `CloudBackendSetS3` / `CapsaBackendSetS3`, when `encryption_secret_ref` is empty and `secret_ref` is not `env://` or `keychain://`, add `weak_encryption_key` warning to projection.
- [ ] Add `capsa doctor` check: compare `key_id` in sync-state against current derived key_id. If mismatch, report `encryption_key_mismatch`.
- [ ] Add test: `capsa backend set s3 --profile x` without `--encryption-secret-ref` produces warning.

## Task 3: Daemon environment inheritance

**Files**: `internal/app/sync_daemon.go`

- [ ] In `SyncDaemonStart`, add `cmd.Env = os.Environ()` before `cmd.Start()`.
- [ ] Add test: daemon child process inherits `PINAX_SYNC_SECRET` env var.

## Task 4: Push auto-rebase on revision_conflict

**Files**: `internal/app/cloud_sync.go`

- [ ] In `cloudSyncPush`, when commit returns `revision_conflict` and `req.Yes` is true: pull remote snapshot, rebuild push plan, retry commit once.
- [ ] If rebuilt plan has conflicts, return `conflict_required` projection with conflict next actions.
- [ ] If retry also fails with `revision_conflict`, return `revision_conflict` error (no infinite retry).
- [ ] `sync pull` stays unchanged (pull-only, no auto-push).
- [ ] Add test: push against stale base with `--yes` auto-rebases and succeeds.
- [ ] Add test: push against stale base with conflicting changes returns `conflict_required`.

## Task 5: `pinax sync daemon install/uninstall`

**Files**: `internal/app/sync_daemon.go`, `internal/cli/sync_cmd.go`, `internal/domain/sync_daemon.go`

- [ ] Add `SyncDaemonInstall` service method: detect platform, resolve vault abs path + binary path, generate unit file from template, write to platform location.
- [ ] Add `SyncDaemonUninstall` service method: remove the generated unit file.
- [ ] Linux template: systemd user unit at `~/.config/systemd/user/pinax-sync-<vault-slug>.service`.
- [ ] macOS template: launchd plist at `~/Library/LaunchAgents/com.yeisme.pinax-sync.<vault-slug>.plist`.
- [ ] Windows: return `platform_unsupported` with Task Scheduler hint.
- [ ] Do NOT store raw `PINAX_SYNC_SECRET` in unit file; use `EnvironmentFile` (Linux) or instruct user to set env (macOS).
- [ ] Add `install`/`uninstall` subcommands to `sync daemon` in `internal/cli/sync_cmd.go`.
- [ ] Projection output SHALL include platform-specific enable command (`systemctl --user enable --now ...` or `launchctl load ...`).
- [ ] Add tests: unit file generation for Linux (template check), macOS (plist XML check), Windows (platform_unsupported).

## Task 6: Documentation

**Files**: `docs/commands/sync.md`, `docs/commands/capsa.md`

- [ ] Update all S3/server examples to include `--encryption-secret-ref env://PINAX_SYNC_SECRET`.
- [ ] Add `pinax sync daemon install` section to `sync.md` with Linux/macOS examples.
- [ ] Add weak-key warning explanation to `capsa.md`.
- [ ] Add encryption migration note to `sync.md` troubleshooting.

## Task 7: Verification

- [ ] `go build ./...`
- [ ] `go test ./internal/remote ./internal/profile ./internal/app ./cmd/pinax -count=1`
- [ ] `gofmt -l .` (empty)
- [ ] `golangci-lint run` (0 issues)
- [ ] `openspec validate pinax-sync-hardening --strict`
- [ ] `task check`
