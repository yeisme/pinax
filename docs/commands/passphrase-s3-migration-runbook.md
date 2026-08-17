# yeisme-notes: device-local AWS profile → repository-encrypted credentials

Migration runbook for moving a private Pinax vault from a per-device `~/.aws/credentials` profile to a repository-encrypted S3/COS credential bundle. The migration does **not** rotate the Capsa content encryption key and does **not** delete remote revisions; it only changes how the S3 transport authenticates.

This is the preferred Pinax backup/bootstrap design. Git distributes the vault, declaration and ciphertext envelope; direct S3/COS carries encrypted Capsa blobs, manifests and revisions. rclone is an external fallback only when the current Pinax binary or provider cannot complete the native path.

The capability is still experimental until repository-encrypted remote-aware push/read-back and real macOS bidirectional dogfood are complete. Migration or bootstrap success alone does not prove a durable S3 backup write.

> Scope: single-user, repository-scoped. Multi-user key sharing, age recipients and cloud KMS are out of scope for the first version.

## 0. macOS release-candidate evidence gate

An operator report that one Mac works is an **observed** result. Before a real Mac clone or COS/S3 action, validate the exact release artifact on that Mac; do not substitute the development `go run ./cmd/pinax` binary.

From the Pinax release-candidate checkout that contains the testkit, point the task at the separately downloaded release binary. Use an opaque release provenance value and never put credentials, a vault path, or a Keychain value in these variables:

```bash
PINAX_MACOS_SUPPORT_BINARY=/absolute/path/to/pinax \
PINAX_MACOS_SUPPORT_PROVENANCE=vX.Y.Z@sha256:<64-lowercase-hex-digest> \
PINAX_MACOS_SUPPORT_INSTALL_CHANNEL=archive \
task integration:sync-macos-candidate
```

The task invokes only `version --json` and command help. It does not open a vault or connect to COS/S3. Its redacted evidence directory contains `artifacts/platform-support.json`; a passing result is only `candidate` for the recorded `darwin/<arch>`, exact macOS version, artifact provenance, and install channel. Its `bootstrap_pull`, `outbound_round_trip`, and `recovery_matrix` stages must remain `not_run` until the operator completes the separate workflow below.

## Prerequisites

- Pinax with `pinax sync repo credential` and `credentialctl project` support.
- The active build must support `pinax sync repo migrate device-profile`, clone-time `bootstrap --pull`, and repository unlock for the remote operation being attempted. If help/output lacks the required flags or reports `real remote writes are not wired yet`, stop and use the documented Git/rclone fallback instead of claiming success.
- A private Git vault already syncing to an S3/COS remote via a device-local AWS profile.
- The existing Capsa content encryption key reference (unchanged by this migration).

## 1. On the existing device — migrate through Pinax

The migration command reads the active `s3-direct` runtime, the configured AWS shared profile and the current Capsa content encryption key. It creates `.pinax/pinax-sync.yaml` and one `.pinax/project-secrets.yaml` envelope containing both `s3_credentials.v1` and `capsa_encryption_key.v1`. It does not rotate the content key and does not contact the remote.

```bash
cd yeisme-notes

pinax sync repo migrate device-profile \
  --vault . \
  --unlock prompt \
  --remember-keychain \
  --yes \
  --json
```

Verify only metadata is returned (no plaintext):

```bash
pinax sync repo credential list --vault . --json
pinax sync repo doctor --vault . --json   # look for credential_mode + repository_encrypted_ready
```

## 2. Review the generated declaration

The command authors `credential_mode: repository-encrypted`, removes the device-local AWS profile from the portable declaration, and preserves the workspace, bucket, prefix, endpoint, region and content key value. Review without exposing plaintext:

```bash
pinax sync repo plan --vault . --json
pinax sync repo credential list --vault . --json
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

pinax sync repo bootstrap \
  --vault . \
  --device "$(scutil --get LocalHostName 2>/dev/null || hostname)" \
  --unlock prompt \
  --remember-keychain \
  --pull \
  --yes \
  --json
```

Validate that the second device resolved the same notes, revision and key identity:

```bash
pinax sync repo doctor --vault . --json   # credential_mode=repository-encrypted, repository_encrypted_ready=true
pinax vault validate --vault . --json
pinax sync pull --target capsa --unlock keychain --yes --vault . --json
```

`remote_write=false` must hold on the new device until a deliberate push is approved.

## 5. Durable backup confirmation

The final stable workflow must run a remote-aware diff and deliberate push using the repository-scoped Keychain. Run the first two commands as preflight only. The third command is a remote write and requires explicit operator authorization for the named canary vault and COS/S3 target; do not treat approval for bootstrap or pull as approval for this command.

Target command sequence after the capability lands:

```bash
pinax sync diff --target capsa --unlock keychain --vault . --json
pinax sync push --target capsa --unlock keychain --dry-run --vault . --json
pinax sync push --target capsa --unlock keychain --yes --vault . --json
```

Before the authorized write, create one non-sensitive, uniquely identifiable canary note on the Mac (for example with `pinax note create "Mac COS canary" --slug mac-cos-canary --body "canary" --vault . --json`). After a changed push, pull on the existing device and validate both the vault and the same canary note:

```bash
pinax sync pull --target capsa --unlock keychain --yes --vault . --json
pinax vault validate --vault . --json
pinax note show mac-cos-canary --vault . --json
```

A changed backup completes only with `remote_write=true`, a committed `revision_id`, and remote head/manifest read-back. The existing device must observe the same revision and the canary note. A no-change result completes only with `up_to_date=true` and `remote_checked=true`; it cannot prove the Mac-to-existing-device write path by itself.

## 6. macOS recovery evidence

Run recovery checks in an isolated clone or canary vault, never by deleting the only working Keychain item or by corrupting the production remote. Keep the evidence redacted and record only the stable error code, selected recovery action, `remote_write` fact, and safe revision identity.

| Case | Expected result | Safe recovery action |
| --- | --- | --- |
| Missing or unavailable Keychain item | `sync_repo_unlock_required`; no prompt in a non-interactive invocation and no remote write | Restore/unlock the repository-scoped Keychain item, then rerun `sync repo doctor` and a pull-only bootstrap or pull. |
| Incorrect bootstrap passphrase | `sync_repo_unlock_failed`; runtime compilation and remote mutation do not proceed | Retry from the clean clone with the verified passphrase; do not generate a replacement content key. |
| Interrupted pull or provider/network failure | Bootstrap is not reported successful and no partial remote write is accepted | Run `sync repo doctor`, retry the pull with the verified Keychain source, then run `pinax vault validate --vault . --json`. |
| Remote conflict or failed durable read-back | Do not claim `remote_write=true` or cross-device success | Stop further writes, preserve the redacted failure evidence, recover from the last trusted revision or use the device-profile rollback below. |

Only after the candidate evidence, successful bootstrap pull, changed canary round trip, and these recovery checks cover the same platform tuple may the OpenSpec support matrix mark that tuple `supported`. All other macOS architectures, operating-system versions, and installation channels remain `unverified`.

## Fallback order

If the active Pinax build cannot complete the preferred path:

1. Use private Git to preserve Markdown and the reviewed repository state.
2. Use rclone crypt or another encrypted archive only as an external backup, restoring into staging first.
3. Do not call Git push, rclone copy, object listing, blob upload or local receipt a successful Pinax Capsa S3 commit.

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
