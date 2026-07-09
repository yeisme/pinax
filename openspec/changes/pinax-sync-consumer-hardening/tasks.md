# pinax-sync-consumer-hardening Tasks

## Wave 1 — Non-breaking fixes (parallel, no migration)

### Task 1: profile:// secret resolution

**Files**: `internal/profile/profile.go`, `internal/profile/profile_test.go`

- [ ] Add `profile://` case to `ResolveSecretRef` switch:
  ```go
  case strings.HasPrefix(ref, "profile://"):
      profileName := strings.TrimPrefix(ref, "profile://")
      return resolveAWSProfileSecret(profileName)
  ```
- [ ] Implement `resolveAWSProfileSecret(profileName string) (string, error)`:
  - Path: `os.Getenv("AWS_SHARED_CREDENTIALS_FILE")` or `~/.aws/credentials`.
  - Parse INI: find `[profileName]` section, return `aws_secret_access_key`.
  - Error if profile not found or file unreadable.
  - Read-only, no subprocess.
- [ ] Acceptance: `go test ./internal/profile -run ResolveSecretRef -count=1`. Fixture credentials file with `[tencent-cos-pinax]` section resolves to secret. Missing profile → error. `env://` and `plain:` paths unchanged.

### Task 2: DeriveKey resolves before PBKDF2

**Files**: `internal/remote/crypto.go`, `internal/remote/crypto_test.go`

- [ ] `DeriveKey(secretRef string) (CryptoKey, error)`:
  ```go
  resolved, err := profile.ResolveSecretRef(secretRef)
  if err != nil {
      return CryptoKey{}, fmt.Errorf("resolve encryption secret: %w", err)
  }
  ```
  - PBKDF2 over `resolved` with salt `pinax-cloud-sync-salt-v1` (unchanged).
- [ ] Add `KeyID(secretRef string) string`: resolve, derive, return `keyID` string. Empty on error.
- [ ] Acceptance: `go test ./internal/remote -run DeriveKey -count=1`. `DeriveKey("profile://test-profile")` with fixture produces same key as `DeriveKey("plain:<raw-secret>")`. Unresolvable ref → error.

### Task 3: Daemon env inheritance

**Files**: `internal/app/sync_daemon.go`

- [ ] In `SyncDaemonStart` at line ~147, before `cmd.Start()`:
  ```go
  cmd.Env = os.Environ()
  ```
- [ ] Acceptance: `go test ./internal/app -run DaemonStart -count=1`. Daemon child process env contains `PINAX_SYNC_SECRET` when set in parent.

### Task 4: Push auto-rebase on revision_conflict

**Files**: `internal/app/cloud_sync.go`

- [ ] In the push commit path (`cloudSyncPush` or equivalent), wrap the commit call:
  ```go
  commitResult, commitErr := commitRevision(ctx, ...)
  if commitErr != nil && isRevisionConflict(commitErr) && req.Yes {
      // Auto-rebase
      remoteSnapshot, pullErr := pullRemoteSnapshot(ctx, ...)
      if pullErr == nil {
          rebasedPlan, planErr := rebuildPushPlan(localSnapshot, remoteSnapshot)
          if planErr == nil && !hasConflicts(rebasedPlan) {
              // Upload new blobs if any
              // Retry commit once
              commitResult, commitErr = commitRevision(ctx, ...)
          } else if hasConflicts(rebasedPlan) {
              return conflictProjection(rebasedPlan), nil
          }
      }
  }
  ```
- [ ] Single retry, no loop.
- [ ] Conflicts → `conflict_required` projection with `pinax sync conflicts list` next action.
- [ ] Without `--yes`: return `revision_conflict` error directly (no auto-rebase).
- [ ] Acceptance: `go test ./internal/app -run PushAutoRebase -count=1`. Three cases: auto-rebase success, conflict detection, retry exhausted. Existing push tests unchanged.

## Wave 2 — Daemon service install/uninstall

### Task 5: Daemon install/uninstall service methods

**Files**: `internal/app/sync_daemon.go`, `internal/cli/sync_cmd.go`

- [ ] Add `SyncDaemonInstall(ctx context.Context, req SyncDaemonInstallRequest) (domain.Projection, error)`:
  - Resolve vault abs path via `filepath.Abs(req.VaultPath)`.
  - Resolve pinax binary via `os.Executable()`.
  - Detect platform via `runtime.GOOS`.
  - Slug = sanitized vault basename (lowercase, non-alnum → `-`).
  - Linux: generate systemd user unit string, write to `~/.config/systemd/user/capsa-sync-<slug>.service`.
  - macOS: generate launchd plist XML, write to `~/Library/LaunchAgents/com.yeisme.capsa-sync.<slug>.plist`.
  - Windows: return `platform_unsupported`.
  - Write env file to `<vault>/.pinax/cloud/sync-env` (mode 0600) if `PINAX_SYNC_SECRET` is in env.
  - Projection facts: `unit_path`, `enable_command` (e.g. `systemctl --user enable --now capsa-sync-<slug>`).
  - Do NOT start the service.
- [ ] Add `SyncDaemonUninstall(ctx context.Context, req SyncDaemonInstallRequest) (domain.Projection, error)`:
  - Remove unit file.
  - Do NOT stop running service.
  - Projection facts: `unit_path`, `removed=true`.
- [ ] Add CLI subcommands:
  - `pinax sync daemon install [--vault <vault>] [--yes]`
  - `pinax sync daemon uninstall [--vault <vault>] [--yes]`
- [ ] Acceptance: `go test ./internal/app -run DaemonInstall -count=1`. Linux template has `Restart=on-failure`, `EnvironmentFile`, `capsa-sync-` prefix. macOS plist is valid XML. Windows returns `platform_unsupported`. `uninstall` removes file.

## Wave 3 — UX hardening

### Task 6: Weak-key warning

**Files**: `internal/app/cloud.go`

- [ ] In `CloudBackendSetS3` / `CapsaBackendSetS3`, after writing config:
  - If `EncryptionSecretRef` is empty AND `SecretRef` is not `env://` or `keychain://`:
    - Add projection warning: `{code: "weak_encryption_key", message: "...", hint: "Set --encryption-secret-ref env://PINAX_SYNC_SECRET"}`
- [ ] In `CapsaDoctor`:
  - Compute current `crypto.KeyID(EncryptionSecretRef(config))`.
  - Compare against stored `sync-state.json` key_id (if present).
  - Mismatch → report `encryption_key_mismatch`.
- [ ] Acceptance: `go test ./internal/app -run WeakKey -count=1`. `capsa backend set s3 --profile x` without `--encryption-secret-ref` → warning in projection.

### Task 7: Documentation

**Files**: `docs/commands/sync.md`, `docs/commands/capsa.md`

- [ ] All S3/server examples add `--encryption-secret-ref env://PINAX_SYNC_SECRET`.
- [ ] `sync.md`: new section "Daemon service installation" with Linux/macOS examples.
- [ ] `capsa.md`: weak-key warning explanation + `--encryption-secret-ref` guidance.
- [ ] `sync.md` troubleshooting: add `encryption_key_mismatch` entry.
- [ ] Acceptance: no doc references to removed `cloud` command. All examples use `capsa` target.

## Wave 4 — Verification

### Task 8: Full verification

- [ ] `go build ./...`
- [ ] `go test ./internal/remote ./internal/profile ./internal/app ./cmd/pinax -count=1`
- [ ] `gofmt -l .` (empty)
- [ ] `golangci-lint run` (0 issues)
- [ ] `openspec validate pinax-sync-consumer-hardening --strict`
- [ ] `task check`
- [ ] Real-world smoke: `pinax sync push --yes` against COS with `--encryption-secret-ref` succeeds.
