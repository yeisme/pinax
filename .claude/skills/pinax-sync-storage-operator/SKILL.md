---
name: pinax-sync-storage-operator
description: Use when an agent needs to inspect, configure, or safely operate Pinax Cloud Sync, sync daemon/logs/conflicts, S3-compatible or rclone transports, storage backend, backend profiles, backend object diagnostics, or remote/local storage checks without exposing credentials.
---

# Pinax Sync And Storage Operator

Operate Pinax storage, backend profiles, Cloud Sync, sync daemon, logs, conflicts, and object diagnostics without exposing credentials or confusing local storage, Remote API Mode, and sync transports.

## Use When

- The task mentions Cloud Sync, `pinax sync`, sync daemon, sync logs, conflicts, server/file/S3/rclone transports, Tencent COS, AWS profile, backend profiles, backend object listings, storage status, push/pull, conflict checks, or device/workspace setup.
- The user wants to configure object storage by profile, rclone remote, or secret ref.
- A sync operation may write remote state and needs a plan or diagnostic first.

## Command Patterns

```bash
pinax storage status --json
pinax storage set s3 --bucket pinax-note-1322128555 --region ap-guangzhou --endpoint https://cos.ap-guangzhou.myqcloud.com --prefix pinax-storage/ --profile tencent-cos-pinax --json
pinax backend list --json
pinax backend add s3 tencent-cos --bucket pinax-note-1322128555 --region ap-guangzhou --endpoint https://cos.ap-guangzhou.myqcloud.com --prefix pinax-storage/ --profile tencent-cos-pinax --json
pinax backend doctor tencent-cos --json
pinax backend object list tencent-cos pinax/ --json
pinax backend object stat tencent-cos pinax/manifest.json --json
pinax cloud status --json
pinax cloud backend set s3 --bucket pinax-note-1322128555 --region ap-guangzhou --endpoint https://cos.ap-guangzhou.myqcloud.com --prefix pinax-sync/ --profile tencent-cos-pinax --secret-ref profile://tencent-cos-pinax --workspace yeisme-notes --device laptop --json
pinax cloud backend set rclone --remote pinax-remote:notes --prefix pinax-sync/ --workspace yeisme-notes --device laptop --json
pinax vault ignore status --json
pinax vault ignore apply --yes --json
pinax sync diff --target cloud --json
pinax sync push --target cloud --dry-run --json
pinax sync daemon status --json
pinax sync daemon logs --limit 20 --json
pinax sync conflicts list --json
```

## Tencent COS Profile Without AWS CLI

Pinax only needs an AWS-compatible profile that the S3 SDK can read. Do not tell users they must install AWS CLI just to run `aws configure`. On a new machine, show direct profile creation with placeholders only:

```bash
mkdir -p ~/.aws
chmod 700 ~/.aws

cat >> ~/.aws/credentials <<'EOF'
[tencent-cos-pinax]
aws_access_key_id = <Tencent COS SecretId>
aws_secret_access_key = <Tencent COS SecretKey>
EOF

cat >> ~/.aws/config <<'EOF'
[profile tencent-cos-pinax]
region = ap-guangzhou
EOF

chmod 600 ~/.aws/credentials ~/.aws/config
```

Then configure the Pinax Cloud Sync S3 direct backend:

```bash
pinax cloud backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --prefix pinax-sync/ \
  --profile tencent-cos-pinax \
  --secret-ref profile://tencent-cos-pinax \
  --workspace yeisme-notes \
  --device <device-name> \
  --json
```

Keep `~/.aws/credentials` and `~/.aws/config` outside the repository. If a user placed AWS files inside a vault or project by mistake, stop and ask them to move the files to `~/.aws/`; do not add real credentials to `.gitignore` as a substitute for moving them out of the repo.

## Backup Mirror Workflow

For Tencent COS, S3-compatible storage, or rclone, call the workflow a backup mirror only when Pinax Cloud Sync writes encrypted sync objects through the CLI direct transport. It is not Pinax Cloud server storage and does not provide server-side auth, audit, lifecycle policy, tenant policy, realtime merge, or automatic conflict resolution.

Use this order:

```bash
pinax vault ignore status --json
pinax vault ignore plan --json
pinax vault ignore apply --yes --json
pinax cloud status --json
pinax cloud doctor --json
pinax sync diff --target cloud --json
pinax sync push --target cloud --dry-run --json
pinax sync push --target cloud --yes --json
```

For restore or second-device verification, initialize/select the second vault, configure the same backend with a different `--device`, then pull:

```bash
pinax cloud backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --prefix pinax-sync/ \
  --profile tencent-cos-pinax \
  --secret-ref profile://tencent-cos-pinax \
  --workspace yeisme-notes \
  --device <second-device-name> \
  --json
pinax sync pull --target cloud --yes --json
pinax vault validate --json
```

Treat `remote_write=true` from `pinax sync push` as the success signal for the backup write. A successful `cloud backend set`, object list, dry-run, or blob upload alone is not backup completion.

## Ignore Handling

Use Pinax commands for vault ignore policy instead of editing metadata by hand:

```bash
pinax vault ignore status --json
pinax vault ignore plan --json
pinax vault ignore apply --yes --json
```

`.pinaxignore` controls which vault content enters the Pinax Cloud Sync manifest. `.gitignore` only controls Git. New or existing vaults should have the Pinax-managed metadata-only `.gitignore` block so Git does not track runtime `.pinax` state, receipts, local sync state, daemon logs, indexes, or caches. Do not hand-edit `.pinax/**`, `.pinaxignore`, or the managed `.gitignore` block unless the Pinax command is unavailable and the user explicitly approves a manual recovery.

## Workflow

1. Inspect current state first: `pinax storage status --json`, `pinax backend list --json`, `pinax cloud status --json`, and `pinax sync status --target cloud --json` when available.
2. Store only profile names, endpoint URLs, bucket names, rclone remote names, prefixes, workspace IDs, device IDs, and secret refs in Pinax config.
3. Do not paste or save raw access keys in Pinax config, docs, logs, receipts, or repository files. Use AWS-compatible profile names such as `tencent-cos-pinax`, rclone remotes such as `onedrive:PinaxSync`, or secret refs such as `profile://tencent-cos-pinax` and `env://PINAX_CLOUD_TOKEN`.
4. If the S3-compatible provider is Tencent COS and AWS CLI is unavailable, create `~/.aws/credentials` and `~/.aws/config` directly with placeholders; do not require `aws configure`.
5. Before backup or sync writes, check `pinax vault ignore status --json` and run `pinax vault ignore apply --yes --json` when the Pinax-managed ignore files are missing.
6. Use separate prefixes for local storage backend data and Cloud Sync data when possible, for example `pinax-storage/` and `pinax-sync/`.
7. Before remote writes, run `pinax sync diff --target cloud --json` and use `--dry-run` when available.
8. For local automatic sync, inspect daemon status/logs before start/stop, and only run `pinax sync daemon run --target cloud --yes` or `pinax sync daemon start --target cloud --yes` after the user approves automatic writes.
9. On conflicts, inspect `pinax sync conflicts list`, `show`, or `diff`, then stop before choosing local, remote, or merged resolution without user approval.
10. Keep Cloud Sync transports (`server`, `file`, `s3`, `rclone`) separate from localhost Remote API Mode (`pinax api serve` and `--api-url`).

## Safety Boundaries

- Never echo full access keys, secret keys, Authorization headers, cookies, provider config contents, or token values.
- Do not hand-edit `.pinax/cloud/config.yaml`, `.pinax/sync-state.json`, `.pinax/backends.json`, `.pinax/sync-daemon/**`, conflict receipts, or backend receipts.
- Do not commit or copy `~/.aws/credentials`, `~/.aws/config`, `.env`, `.pinax/sync-state.json`, `.pinax/sync-daemon/**`, generated indexes, caches, receipts, or local vault runtime state into the repository.
- Do not claim a remote write succeeded unless the Pinax command returns `remote_write=true` or an equivalent success fact.
- Remote API Mode is not Cloud Sync; keep `--api-url` workflows separate from `pinax cloud` and `pinax sync`.
- Do not treat a direct object store list or upload as a successful sync unless the Pinax sync command committed the revision and reported the success fact.

## Validation

- After storage/backend configuration: `pinax storage doctor --json` and `pinax backend doctor <name> --json`.
- For Cloud Sync: `pinax cloud doctor --json` and `pinax sync diff --target cloud --json`.
- For backup mirror: `pinax vault ignore status --json`, `pinax sync push --target cloud --dry-run --json`, then a confirmed `pinax sync push --target cloud --yes --json` with `remote_write=true`.
- For daemon work: `pinax sync daemon status --json` and `pinax sync daemon logs --limit 20 --json`.
- Verify output reports configured profile/secret-ref status without raw secrets.
