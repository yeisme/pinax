# pinax-sync-hardening Design

## 1. Encryption key resolution

### Current flow

```
config.secret_ref = "profile://tencent-cos-pinax"
config.encryption_secret_ref = ""
→ EncryptionSecretRef(config) = "profile://tencent-cos-pinax"  (fallback)
→ DeriveKey("profile://tencent-cos-pinax")  ← uses literal string as key material
→ PBKDF2("profile://tencent-cos-pinax", static_salt) → weak CryptoKey
```

### Problem

`DeriveKey` receives the raw reference string, not the resolved secret. `ResolveSecretRef` handles `env://`, `keychain://`, and `plain:`, but has no `profile://` case — it falls through to `default:` which returns the ref as-is.

### Fix

Two-layer fix:

**Layer 1: `DeriveKey` resolves references before key derivation.**

```go
func DeriveKey(secretRef string) (CryptoKey, error) {
    if secretRef == "" {
        return CryptoKey{}, fmt.Errorf("secret ref required")
    }
    resolved, err := profile.ResolveSecretRef(secretRef)
    if err != nil {
        // If resolution fails, don't fall back to the raw ref — fail closed.
        return CryptoKey{}, fmt.Errorf("resolve encryption secret: %w", err)
    }
    if resolved == "" {
        return CryptoKey{}, fmt.Errorf("encryption secret resolved to empty")
    }
    salt := []byte("pinax-cloud-sync-salt-v1")
    key := pbkdf2.Key([]byte(resolved), salt, 100000, 32, sha256.New)
    ...
}
```

**Layer 2: `ResolveSecretRef` gains `profile://` scheme.**

```go
case strings.HasPrefix(ref, "profile://"):
    profileName := strings.TrimPrefix(ref, "profile://")
    return resolveAWSProfileSecret(profileName)
```

`resolveAWSProfileSecret` reads `~/.aws/credentials` for the named profile and returns `aws_secret_access_key`. If the profile is not found, returns an error. This is a read-only parse; no subprocess.

**Layer 3: Weak-key warning in `capsa backend set s3` / `capsa login`.**

When `encryption_secret_ref` is empty and `secret_ref` is not `env://` or `keychain://`, emit projection warning:

```
warnings:
  - code: weak_encryption_key
    message: Encryption key derived from provider credential reference; set --encryption-secret-ref for a dedicated sync encryption key
```

### Migration

- Existing vaults with `secret_ref: profile://...` will produce a **different** encryption key after this fix. Their existing COS blobs are encrypted with the old weak key.
- `capsa doctor` SHALL detect this: compare `key_id` in sync-state against current derived key_id. If they differ, report `encryption_key_mismatch` with hint to re-push.
- Users who never had real encrypted data (or are willing to re-push) simply run `pinax sync push --yes` after upgrade.

## 2. Daemon environment inheritance

### Fix

In `SyncDaemonStart` (`sync_daemon.go`), before `cmd.Start()`:

```go
cmd.Env = os.Environ()
```

This is a one-line fix. The foreground `SyncDaemonRun` already inherits the parent process env.

## 3. Push auto-rebase on revision_conflict

### Current flow

```
sync push → cloudSyncPushPlan → commitRevision → REVISION_CONFLICT → return error
```

### Fix

In `cloudSyncPush` (`cloud_sync.go`), wrap the commit call:

```go
commitResult, commitErr := commitRevision(...)
if commitErr == cloudsync.ErrRevisionConflict && req.Yes {
    // Auto-rebase: pull remote, rebuild plan, retry once
    remoteSnapshot, pullErr := cloudRemoteSnapshot(...)
    if pullErr != nil {
        return ..., pullErr
    }
    // Rebuild plan against new base
    localPlan, planErr := buildPushPlan(localSnapshot, remoteSnapshot)
    if planErr != nil {
        return ..., planErr
    }
    // If conflicts detected, return conflict_required
    if hasConflicts(localPlan) {
        return conflictProjection(...), nil
    }
    // Retry commit
    commitResult, commitErr = commitRevision(...)
}
```

### Constraints

- Only auto-rebase when `--yes` is set (user consented to writes).
- Only retry once — repeated conflicts mean manual intervention needed.
- `sync pull` stays pull-only; no auto-push.
- `pinax sync` (bidirectional) already does pull-then-push; no change needed there.

## 4. `pinax sync daemon install/uninstall`

### Service unit templates

**Linux (systemd user unit):**

```ini
[Unit]
Description=Pinax Sync Daemon for %i
After=network-online.target

[Service]
Type=simple
WorkingDirectory=<vault_abs>
Environment=PINAX_SYNC_SECRET=<from env or config>
ExecStart=<pinax_bin> sync daemon run --target capsa --vault <vault_abs> --yes
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
```

**macOS (launchd plist):**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.yeisme.pinax-sync.<vault></string>
    <key>ProgramArguments</key>
    <array>
        <string><pinax_bin></string>
        <string>sync</string>
        <string>daemon</string>
        <string>run</string>
        <string>--target</string>
        <string>capsa</string>
        <string>--vault</string>
        <string><vault_abs></string>
        <string>--yes</string>
    </array>
    <key>WorkingDirectory</key>
    <string><vault_abs></string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
</dict>
</plist>
```

### Implementation

```go
func (s *Service) SyncDaemonInstall(ctx context.Context, req SyncDaemonRequest) (domain.Projection, error) {
    // 1. Resolve vault absolute path
    // 2. Resolve pinax binary path (os.Executable)
    // 3. Detect platform (runtime.GOOS)
    // 4. Generate unit file content from template
    // 5. Write to platform-specific location
    // 6. Return projection with enable instructions
}
```

### Constraints

- SHALL NOT start the service automatically.
- SHALL NOT store the raw `PINAX_SYNC_SECRET` in the unit file if it comes from `env://`. Instead, use `EnvironmentFile=<vault>/.pinax/cloud/sync-env` (mode 0600) or instruct user to set it in their shell profile.
- `uninstall` SHALL remove the unit file but SHALL NOT stop a running service.
- Windows returns `platform_unsupported` with hint to use Task Scheduler.

## Risks

1. **Encryption key change breaks existing COS data.** Mitigated by `capsa doctor` detection + re-push guidance. Users with no prior real data (test vaults) are unaffected.
2. **`profile://` AWS credentials parsing is fragile.** If the user's `~/.aws/credentials` format is non-standard, resolution fails. Mitigated by clear error messages and the `--encryption-secret-ref env://` escape hatch.
3. **Auto-rebase hides multi-user conflicts.** Mitigated by single retry limit and `conflict_required` return on unresolved conflicts.
