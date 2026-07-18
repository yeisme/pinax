# pinax-sync-consumer-hardening Proposal

## Why

Tencent COS/S3 dogfooding exposed reliability gaps in Pinax Capsa Sync: weak key sourcing when provider credentials doubled as encryption material, daemon environment loss, stale-base push failures, missing OS service unit generation, and drift between Pinax-local crypto and the shared Capsa SDK.

Pinax now imports `github.com/yeisme/capsa` for the encryption API. The sync orchestration remains in Pinax for this change, but key derivation, envelope encryption, and manifest encryption are delegated to the Capsa SDK so Pinax uses `capsa-sync-salt-v1` instead of the historical `pinax-cloud-sync-salt-v1`.

## What Changes

- `internal/remote/crypto.go` becomes a compatibility wrapper over `github.com/yeisme/capsa` crypto APIs.
- `github.com/yeisme/capsa` exposes public encryption helpers for consumers that still own their sync orchestration.
- `ResolveSecretRef` supports `profile://` AWS shared credentials and fails closed when a secret cannot be resolved.
- `capsa backend set s3` and `capsa doctor` report weak-key and key-mismatch warnings.
- `sync push --yes` auto-rebases once on revision conflict.
- `sync daemon start` inherits environment variables; `sync daemon install/uninstall` generate Capsa-named systemd/launchd units.
- Docs now point sync operations and backend validation at Capsa, not the historical `pinax cloud` surface.

## Migration

Existing COS/S3 objects pushed by the old Pinax-local crypto path were encrypted with `pinax-cloud-sync-salt-v1`. After this change Pinax derives keys through the Capsa SDK with `capsa-sync-salt-v1`, so existing remote data must be re-pushed from a complete local vault with the current encryption secret.

Recommended operator sequence:

```bash
pinax capsa doctor --vault ./my-notes --json
pinax sync push --target capsa --vault ./my-notes --yes --json
```

## Capabilities

### Modified Capabilities

- `pinax-cloud-sync`: SDK-backed crypto, key mismatch migration warning, auto-rebase, daemon env, service install.
- `pinax-cli-tree-modification`: `sync daemon install` and `sync daemon uninstall` commands.

## Impact

- Pinax now depends on the local Capsa SDK module during development through `replace github.com/yeisme/capsa => ../../shared/capsa`.
- Wire schema remains frozen as `pinax.cloud.*`; only key derivation salt/key-id namespace changes to the Capsa SDK path.
- Remote COS/S3 objects from the old key derivation path are not readable with the new key and require re-push.
