# yeisme-notes: device-local AWS profile → repository-encrypted credentials

Migration runbook for moving a private Pinax vault from a per-device `~/.aws/credentials` profile to a repository-encrypted S3/COS credential bundle. The migration does **not** rotate the Capsa content encryption key and does **not** delete remote revisions; it only changes how the S3 transport authenticates.

> Scope: single-user, repository-scoped. Multi-user key sharing, age recipients and cloud KMS are out of scope for the first version.

## Prerequisites

- Pinax with `pinax sync repo credential` and `credentialctl project` support.
- A private Git vault already syncing to an S3/COS remote via a device-local AWS profile.
- The existing Capsa content encryption key reference (unchanged by this migration).

## 1. On the existing device — capture the credential (no plaintext in Git)

Create the repository-encrypted envelope and store the S3/COS credential bundle. The plaintext flows only through secure stdin; it is never written to argv, stdout, logs or Git.

```bash
cd yeisme-notes

# Initialize the repository credential envelope (passphrase prompted from TTY,
# or via --passphrase-file / --env-var for scripted setups).
pinax sync repo credential init \
  --vault . \
  --project pinax \
  --repository yeisme-notes \
  --passphrase-file ~/.pinax.pass \
  --json

# Store the S3/COS bundle. Pipe the JSON via --stdin; do NOT use --value.
printf '%s' '{"access_key_id":"<YOUR_AKID>","secret_access_key":"<YOUR_SK>"}' | \
  pinax sync repo credential set \
    --vault . \
    --name tencent-cos-pinax \
    --kind credential \
    --format s3_credentials.v1 \
    --version 1 \
    --stdin \
    --passphrase-file ~/.pinax.pass \
    --json
```

Verify only metadata is returned (no plaintext):

```bash
pinax sync repo credential list --vault . --json
pinax sync repo doctor --vault . --json   # look for credential_mode + repository_encrypted_ready
```

## 2. Switch the declaration to repository-encrypted

Edit `.pinax/pinax-sync.yaml` (or re-run `pinax sync repo init`) and set:

```yaml
backend:
  kind: s3-direct
  s3:
    bucket: yeisme-notes
    endpoint: https://cos.ap-shanghai.myqcloud.com
    region: ap-shanghai
    credential_mode: repository-encrypted
```

Plan and apply so the local runtime config is regenerated:

```bash
pinax sync repo plan   --vault . --json
pinax sync repo apply  --vault . --yes --json
```

The Capsa content encryption key, remote namespace and remote revisions are **unchanged** — only the S3 credential resolution path changes.

## 3. Commit the ciphertext

Commit `.pinax/pinax-sync.yaml` and `.pinax/project-secrets.yaml` (the latter is ciphertext + identity only). Never commit `~/.pinax.pass` or any plaintext credential file — keep the passphrase in a user-level secret store or macOS Keychain.

```bash
git add .pinax/pinax-sync.yaml .pinax/project-secrets.yaml
git commit -m "chore(sync): switch S3 credential to repository-encrypted bundle"
git push
```

## 4. On the second device — clone and bootstrap

```bash
git clone https://github.com/yeisme/yeisme-notes.git yeisme-notes
cd yeisme-notes

# Provide the passphrase via file/env (or TTY prompt on macOS).
PINAX_REPO_PASS='<the passphrase>' pinax sync repo bootstrap \
  --vault . \
  --device "$(scutil --get LocalHostName 2>/dev/null || hostname)" \
  --unlock env \
  --env-var PINAX_REPO_PASS \
  --pull \
  --yes \
  --json
```

Validate that the second device resolved the same notes, revision and key identity:

```bash
pinax sync repo doctor --vault . --json   # credential_mode=repository-encrypted, repository_encrypted_ready=true
pinax vault validate --vault . --json
```

`remote_write=false` must hold on the new device until a deliberate push is approved.

## Rollback

If repository-encrypted mode misbehaves, roll back to the device-local AWS profile without touching the remote:

1. Set `credential_mode: device-profile` in `.pinax/pinax-sync.yaml`.
2. `pinax sync repo apply --vault . --yes --json` to regenerate the runtime config.
3. Re-create `~/.aws/credentials` (or the shared profile) if it was removed.
4. The Capsa encryption key and remote revisions are untouched.

The committed ciphertext bundle can be left in place (unreferenced) or removed with `pinax sync repo credential remove`.

## Operational cautions

- **Git history retains old ciphertext.** Rekey (`pinax sync repo credential` rekey via `credentialctl project rekey`) changes the passphrase but old commits remain decryptable by the old passphrase. For high-risk rotation, revoke the old COS SecretId/SecretKey in the provider console.
- **Real macOS Keychain smoke** (vs the Linux fake-executable tests) and a **real second-Mac COS restore** are dogfood gates tracked in the OpenSpec change; complete them before declaring stable-from.
- **No daemon TTY prompt.** The daemon must resolve credentials from a non-interactive source (Keychain/file/env); it must not open an interactive prompt.
