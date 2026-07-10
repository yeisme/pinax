# pinax-sync-consumer-hardening Tasks

## 1. SDK-backed encryption

- [x] Add public Capsa SDK crypto helpers: `DeriveKey`, `KeyID`, `EncryptBlob`, `DecryptBlob`, `EncryptManifest`, and `DecryptManifest`.
- [x] Replace Pinax `internal/remote/crypto.go` implementation with a compatibility wrapper over `github.com/yeisme/capsa`.
- [x] Add Pinax module dependency and local development replace for `../../shared/capsa`.
- [x] Preserve `pinax.cloud.envelope.v1` and `pinax.cloud.manifest.v1` wire schema while adopting the Capsa SDK salt/key-id namespace.

## 2. Secret resolution and key warnings

- [x] Support `profile://` secret references through AWS shared credentials file parsing.
- [x] Fail closed when a secret reference cannot be resolved.
- [x] Emit `weak_encryption_key` warning when the backend credential reference is reused as the sync encryption secret.
- [x] Record sync key id in current sync state and report `encryption_key_mismatch` in `capsa doctor`.

## 3. Push and daemon reliability

- [x] Auto-rebase `sync push --yes` once on revision conflict.
- [x] Preserve `sync pull` as pull-only and return `conflict_required` for content conflicts.
- [x] Ensure `sync daemon start` inherits the parent process environment.
- [x] Add `sync daemon install` and `sync daemon uninstall` with Capsa-named systemd/launchd units.

## 4. Documentation cleanup

- [x] Update sync examples to use `pinax capsa` and `--target capsa`.
- [x] Document the COS/S3 re-push requirement when migrating from `pinax-cloud-sync-salt-v1` to `capsa-sync-salt-v1`.
- [x] Update backend references from `backend-server/pinax-cloud` to `backend-server/capsa` where the doc discusses current sync validation.

## 5. Verification

- [x] `go test ./...` in `shared/capsa` passed.
- [x] `go test ./internal/remote ./internal/profile ./internal/app ./cmd/pinax -count=1` in `cli/pinax` passed.
- [x] `openspec validate --all --strict` in `cli/pinax` passed before archive preparation.
