# Migration & rollback runbook: declarative sync config

This runbook migrates an existing device from the per-device `.pinax/cloud/config.yaml`
 Capsa runtime config to the declarative `.pinax/pinax-sync.yaml` repository layer.
It does **not** rotate the encryption key, delete remote revisions, or force any device offline.

## Pre-conditions

- Pinax CLI built from the branch that includes `pinax sync repo`.
- Existing device has a working `pinax capsa login` / `pinax sync` flow.
- You can run `pinax sync repo doctor --vault <vault> --json` without error after migration.

## Migration steps

1. **Read the current runtime config.** Capture the current backend, workspace and
   encryption secret reference so the declaration matches the live state:

   ```bash
   pinax sync status --vault ./my-notes --json
   ```

2. **Generate the declaration from the live values.** Use the same workspace, backend
   kind and endpoint; pick a logical `encryption-key-id` (e.g. `personal-sync-key`):

   ```bash
   pinax sync repo init \
     --backend-kind s3-direct \
     --endpoint s3://my-bucket/prefix \
     --workspace personal \
     --credential-id tencent-cos-pinax \
     --encryption-key-id personal-sync-key \
     --remote-delete-policy deny \
     --vault ./my-notes --json
   ```

   The old `.pinax/cloud/config.yaml` is **not** deleted. `pinax sync repo init` only
   writes `.pinax/pinax-sync.yaml` and `.gitignore` device-state protection.

3. **Store the encryption secret in the encrypted asset.** Reuse the same plaintext
   value the device already uses for encryption, so the key identity does not change:

   ```bash
   PINAX_SYNC_FAKE_KEY=<unlock-identity> \
   pinax sync repo secret set \
     --name personal-sync-key --kind encryption_key \
     --value <plaintext-key> --provider fake \
     --vault ./my-notes --json
   ```

   The plaintext is transient; only ciphertext is stored.

4. **Plan the apply.** Confirm there is no drift and no surprise high-risk change:

   ```bash
   pinax sync repo plan --device <this-device> --vault ./my-notes --json
   ```

5. **Apply with explicit approval.** Because the runtime config already exists and the
   workspace matches, no high-risk change is expected; `--yes` confirms the regenerate:

   ```bash
   pinax sync repo apply --device <this-device> --yes --vault ./my-notes --json
   ```

   Apply backs up the prior runtime config to `.pinax/cloud/config.yaml.bak` before
   overwriting, so the previous state is restorable.

6. **Verify convergence.** Doctor must report `healthy`, and a pull/push smoke must work:

   ```bash
   pinax sync repo doctor --vault ./my-notes --json
   pinax sync pull --vault ./my-notes --json
   ```

## Onboarding a second device

```bash
git clone <repo> ./my-notes
PINAX_SYNC_FAKE_KEY=<unlock-identity> \
  pinax sync repo bootstrap --device desktop-2 --vault ./my-notes --json
pinax sync pull --vault ./my-notes --json
```

A bootstrapping device with no local sync receipt defaults to **pull-only**; it does not
upload local deletions or replace remote state. Enable the daemon only after the first
pull succeeds.

## Rollback

If the declarative layer misbehaves, roll back to the per-device runtime config:

1. Stop the daemon if running: `pinax sync daemon stop --vault ./my-notes`.
2. Restore the backed-up runtime config:

   ```bash
   cp .pinax/cloud/config.yaml.bak .pinax/cloud/config.yaml
   ```

3. Continue using the old `pinax capsa login` / `pinax sync` flow. The repository
   declaration and encrypted secrets asset can remain in place; old clients ignore them.

**Do not** delete remote revisions or auto-reset the encryption key during rollback. If
the encryption key identity must change, that is a separate, deliberate key-rotation
process outside this runbook.

## What does NOT change

- The Capsa manifest, revision CAS, conflict resolution and encryption protocol are unchanged.
- Existing `pinax sync push/pull/daemon` commands resolve the generated runtime config
  transparently; no repeated backend flags are required after `apply`/`bootstrap`.
- Server-side tenant authorization, quota, audit and rate limiting remain out of scope for
  direct S3/rclone transport; `doctor` reports the `auth_boundary` fact transparently.
