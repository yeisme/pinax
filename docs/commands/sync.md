# sync Command
`pinax sync` generates, records, and executes sync plans. Explicit `diff`/`push`/`pull` commands are short-lived workflows; `pinax sync daemon` is the local long-running process for automatic Capsa Sync.

For Pinax vault backup and new-device restore, the preferred target architecture is repository-encrypted S3/COS: Git carries Markdown plus `.pinax/pinax-sync.yaml` and encrypted `.pinax/project-secrets.yaml`; Capsa direct S3 stores encrypted blobs, manifests and revisions. Git is a companion/version channel, not a replacement for native S3 commit evidence. rclone is a provider compatibility or external archive fallback, not the first recommendation.

This preferred repository-encrypted path remains **experimental** until the active binary supports remote-aware repository unlock for `diff|push`, durable manifest commit read-back, capability gating, atomic migration and real macOS bidirectional dogfood. A projection containing `real remote writes are not wired yet`, `remote_checked=false`, or `remote_write=false` without `up_to_date=true` is not proof of a completed backup.

For `--target capsa`, the protocol is distributed: every device keeps a local vault, and the selected Capsa Sync transport coordinates encrypted blob, manifest, and revision exchange. The content manifest is selected by `.pinaxignore` and can include Markdown, scripts, assets, attachments, and other regular files. The transport can be Capsa Server, S3-compatible direct storage, rclone direct storage, or embedded Go API/local RPC. This differs from `pinax api serve`, which is centralized remote access to one server-side vault.

Project and subproject deletions are represented as explicit encrypted delete markers in the manifest. A deletion is not inferred from a missing file entry. Push reports additive `delete_markers` and `trash_backup_blobs` facts, and `remote_write=true` is still emitted only after the selected transport commits the revision.

When docs or operator notes use the phrase backup mirror, it means a CLI-side direct transport mirror of encrypted Capsa Sync objects under a user-controlled provider boundary. S3 direct and rclone direct do not become Capsa server-side storage: provider credentials own access, and Pinax does not provide server-side auth, audit, object lifecycle, tenant policy, or rate limiting for those writes. Capsa server transport is the path that owns server auth/audit/object lifecycle semantics.

The backup mirror boundary also excludes realtime daemon and conflict policy changes. `pinax sync daemon` remains the local realtime automation layer, and conflict inspection/resolution remains under explicit `pinax sync conflicts` commands. New daemon lifecycle behavior, automatic merge behavior, conflict resolution semantics, or transport-specific push notifications need separate OpenSpec coverage before implementation.

## Subcommands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax sync diff` | Generates a sync diff plan. | Does not write to the remote. |
| `pinax sync push` | Pushes local encrypted manifest/blob changes when the selected transport can commit a durable revision. | Requires `--yes`; `remote_write=true` is allowed only after revision commit succeeds. |
| `pinax sync pull` | Pulls the committed remote revision and applies decrypted local changes. | Requires `--yes`; preserves conflicting local edits as `.conflict.md` copies. |
| `pinax sync daemon run` | Runs the local sync daemon in the foreground. | Requires `--yes`; watches local changes and polls remote head. |
| `pinax sync daemon start` | Starts the local sync daemon in the background. | Requires `--yes`; writes `.pinax/sync-daemon/` runtime state. |
| `pinax sync daemon status` | Reads local daemon state. | Read-only. |
| `pinax sync daemon stop` | Requests graceful daemon shutdown. | Writes a local stop request. |
| `pinax sync daemon logs` | Reads redacted daemon events. | Read-only. |
| `pinax sync logs list` | Lists recent sync run receipts. | Read-only. |
| `pinax sync logs show <run-id>` | Shows one sync receipt and its redacted operations. | Read-only. |
| `pinax sync logs tail` | Shows run and file-level sync timeline events. | Read-only. |
| `pinax sync logs tail --follow` | Streams newly appended redacted sync events until canceled. | Read-only. |
| `pinax sync conflicts list` | Lists local conflict copies. | Read-only. |
| `pinax sync conflicts diff <file>` | Shows a diff between a conflict copy and its trunk file. | Read-only. |
| `pinax sync conflicts show <file>` | Shows conflict content for manual or agent merge workflows. | Read-only. |
| `pinax sync conflicts resolve <file>` | Resolves a conflict copy by keeping local, keeping remote, or applying a merged file. | Requires explicit resolve flags and write confirmation where supported. |

## Output and preview

默认人类输出是表格，机器模式仍来自同一 projection：

```bash
pinax sync diff --vault ./my-notes --preview status --limit 10
pinax sync diff --vault ./my-notes --preview diff
pinax sync diff --vault ./my-notes --content-diff --limit 5
pinax sync --vault ./my-notes --progress auto
pinax sync --vault ./my-notes --events
pinax sync --vault ./my-notes --agent
pinax sync --vault ./my-notes --json
```

`--preview status` 显示 Added/Modified/Deleted/Renamed/Conflicts 与最多 N 条 A/M/D/R/C 路径；`--preview diff` 增加 manifest 元数据 unified diff；`--preview none` 只保留统计和 revision。`--limit 0` 不列路径。`--content-diff` 只在显式启用时生成有界 Markdown 正文 diff，且不会进入 receipt、事件日志或 COS/S3 对象。

同步 projection 可选包含 `data.sync_view`（`pinax.sync.output.v1`）。`scope=remote-aware` 表示本次比较读取了远端 head；`scope=cached` 只表示本地缓存比较，不是远端备份成功证据。JSON 保留完整 `data.plan.operations`，agent 只输出有限 `change.N.*`，events 只输出 `start/progress/end/error` NDJSON。

默认进度写 stderr：TTY 使用单行刷新，非 TTY 使用简洁阶段行；`--progress never` 完全关闭实时进度。`--json` 和 `--agent` 不输出实时进度，`--events` 将进度写入 stdout NDJSON。

## Common workflows

Inspect a plan without writing:

```bash
pinax vault ignore status --vault ./my-notes --json
pinax sync diff --target capsa --vault ./my-notes --json
pinax sync push --target capsa --vault ./my-notes --dry-run --json
```

Configure an S3-compatible direct backend and sync two devices:

```bash
export PINAX_SYNC_SECRET="your-encryption-secret"

pinax capsa backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device laptop \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./device-a
pinax sync push --target capsa --vault ./device-a --yes --json

pinax capsa backend set s3 \
  --bucket notes \
  --region us-east-1 \
  --prefix pinax-sync/ \
  --profile work \
  --workspace personal \
  --device desktop \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./device-b
pinax sync pull --target capsa --vault ./device-b --yes --json
```

### Tencent Cloud COS (real-world verified)

Tencent Cloud Object Storage (COS) is S3-compatible and uses **virtual-hosted** addressing by default. Pinax auto-detects `.myqcloud.com` endpoints and selects virtual-hosted style automatically — you do not need `--addressing-style virtual-hosted`.

AWS profile in `~/.aws/credentials`:

```ini
[profile tencent-cos-pinax]
region = ap-guangzhou

[tencent-cos-pinax]
aws_access_key_id = AKIDxxxxxxxxxxxx
aws_secret_access_key = <your-secret-key>
```

Configure two devices and sync:

```bash
export PINAX_SYNC_SECRET="your-encryption-secret"

# Device A (laptop)
pinax capsa backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --profile tencent-cos-pinax \
  --prefix pinax-sync/ \
  --workspace personal \
  --device laptop \
  --secret-ref env://PINAX_SYNC_SECRET \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax sync push --vault ./my-notes --yes --json

# Device B (desktop) — same bucket, different device id
pinax capsa backend set s3 \
  --bucket pinax-note-1322128555 \
  --region ap-guangzhou \
  --endpoint https://cos.ap-guangzhou.myqcloud.com \
  --profile tencent-cos-pinax \
  --prefix pinax-sync/ \
  --workspace personal \
  --device desktop \
  --secret-ref env://PINAX_SYNC_SECRET \
  --encryption-secret-ref env://PINAX_SYNC_SECRET \
  --vault ./my-notes
pinax sync pull --vault ./my-notes --yes --json
```

After initial push, use `pinax sync --vault ./my-notes --yes` for bidirectional sync. If you moved a note locally but haven't pushed yet, `sync pull` returns `LOCAL_UNPUSHED_CHANGES`; `pinax sync` (without subcommand) will push the move and pull remote changes in one step.

COS region endpoints follow `https://cos.<region>.myqcloud.com` (e.g. `cos.ap-beijing.myqcloud.com`, `cos.ap-shanghai.myqcloud.com`). The `--profile` flag reads credentials from the named AWS shared profile; Pinax never stores the raw secret key.

Use a local object-store transport for development or local E2E checks:

```bash
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target capsa --vault ./device-a --yes --json
pinax sync pull --target capsa --vault ./device-b --yes --json
```

Delete markers are produced by CLI-authored trash commands before sync:

```bash
pinax project delete history --vault ./device-a --yes --json
pinax trash list --vault ./device-a --json
pinax sync push --target capsa --vault ./device-a --yes --json
```

Configure server and rclone backends; a push claims a completed write only after the selected transport commits a durable revision:

```bash
pinax capsa login --endpoint https://capsa.example.test --workspace ws_123 --device laptop --secret-ref env://PINAX_CAPSA_TOKEN --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes
pinax sync push --target capsa --vault ./my-notes --yes --json

pinax capsa backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes
pinax sync push --target capsa --vault ./my-notes --yes --json
```

Run local automatic sync after Capsa Sync is configured:

```bash
pinax sync daemon run --target capsa --vault ./my-notes --yes
pinax sync daemon status --vault ./my-notes --json
pinax sync daemon logs --vault ./my-notes --limit 20 --json
pinax sync daemon stop --vault ./my-notes
```

`run` stays in the foreground and handles terminal/process shutdown. It performs one startup pull-before-push sync cycle immediately, then continues watching local changes and polling the remote head. Default human output prints live progress lines; use `--events` for NDJSON streaming automation while `--json` remains a final single-envelope output.

`start` launches the same runner in the background where supported:

```bash
pinax sync daemon start --target capsa --vault ./my-notes --yes
```

The first daemon release detects local file changes with a local watcher and detects remote changes by polling the Capsa Sync head. Redacted daemon state and event logs live under `.pinax/sync-daemon/` and can be inspected with `pinax sync daemon logs --vault ./my-notes --limit 20 --json`. It is a local process, not a hosted service, and it does not change the Capsa Sync plaintext boundary: transports still coordinate encrypted blobs, encrypted manifests, and revision metadata only.

Inspect which files a completed sync planned and follow later sync activity in another terminal:

```bash
pinax sync logs tail --vault ./my-notes --limit 50
pinax sync logs tail --vault ./my-notes --follow
pinax sync logs tail --vault ./my-notes --follow --events
```

The timeline contains a `sync.file` item for each non-manifest operation and a final `sync.run` summary. File items include the run ID, direction, operation kind, final run status, and the path allowed by the run's `path_policy`. `--path-policy hash` emits `path_sha256:...`; `--path-policy omitted` keeps the operation but removes the path. `--follow` supports default human output, `--agent`, and `--events`; use non-follow `--json` when a single JSON envelope is required.

These apply commands use the same sync engine as direct object-store transports. If the selected backend is unavailable, the commit fails, or the configured scheme is unsupported, the command must return a structured partial/error such as `transport_unavailable`, `unsupported_scheme`, or `revision_conflict` with `remote_write=false`. It must not silently no-op, produce a dummy revision, or emit `remote_write=true`.

## Daemon service installation

For long-running persistent sync, install the daemon as an OS service:

### Linux (systemd user unit)

```bash
pinax sync daemon install --vault ./my-notes --yes
systemctl --user enable --now capsa-sync-my-notes
```

To remove:

```bash
systemctl --user stop capsa-sync-my-notes
pinax sync daemon uninstall --vault ./my-notes --yes
```

### macOS (launchd)

```bash
pinax sync daemon install --vault ./my-notes --yes
launchctl load ~/Library/LaunchAgents/com.yeisme.capsa-sync.my-notes.plist
```

To remove:

```bash
launchctl unload ~/Library/LaunchAgents/com.yeisme.capsa-sync.my-notes.plist
pinax sync daemon uninstall --vault ./my-notes --yes
```

## Capsa Sync execution model

The target execution flow is transport-independent:

1. Scan the local vault and build a client-side manifest from files allowed by `.pinaxignore`.
2. Encrypt content blobs and manifest metadata before upload.
3. Ask the selected transport which encrypted blobs are missing.
4. Upload missing encrypted blobs and the encrypted manifest object.
5. Commit the new revision with compare-and-swap against the known base revision.
6. Write local sync-state / run evidence after the commit result is known.
7. Other devices read the committed head, download missing encrypted blobs, decrypt locally, and apply changes.
8. Conflicting local edits are preserved as local conflict copies instead of being silently overwritten. A conflict copy is only preserved when local content actually diverged from the common base revision (or divergence cannot be proven, e.g. first sync of the object): a pull onto an untouched local copy fast-forwards the remote blob silently, and a file edited after the plan was built still preserves a copy as a TOCTOU guard. On manifest v2 vaults conflict copies are local-only snapshots: they keep the note's canonical `note_id`, so they are excluded from the v2 remote manifest and the identity audit (syncing them would duplicate object identity and block pushes). Resolve them with `pinax sync conflicts resolve`.

`remote_write=true` belongs only to step 5 after a durable revision commit. It is not valid for dry-runs, plan generation, blob uploads, manifest uploads, conflict failures, unsupported transports, or pull operations.

The daemon uses the same rule. It may call `sync pull` before `sync push` when the remote head is newer, and it stops automatic writes with `conflict_required` if pull creates conflict copies that need user review.

## Key derivation and re-encryption

Sync content is encrypted with an AES-256-GCM envelope whose key is derived
from the vault's sync secret. Since 2026-08-16 the derivation is v2: PBKDF2
at 600k iterations with a salt derived from the secret itself. Envelopes
written by older builds (100k iterations, static salt) remain readable —
decryption picks the key by each envelope's `key_id` — but every push
re-encrypts any remote object still under the legacy derivation, even when
the content is unchanged.

### Checking and re-encrypting a vault

```bash
pinax sync keys --vault ./my-notes --json
```

Reports the active v2 key id, the legacy key id, and which derivation the
remote manifest is encrypted under (`remote_derivation`: `v2`, `legacy`,
`empty`, or `unknown` for a foreign secret). `reencryption_required=true`
means the remote still holds legacy-encrypted objects.

To migrate a vault to the v2 derivation:

1. Upgrade `pinax` on this device (older binaries cannot read v2 envelopes).
2. `pinax sync keys --vault ./my-notes --json` — expect `remote_derivation=legacy`.
3. `pinax sync pull --target capsa --vault ./my-notes --yes --json` — verifies
   legacy data is still readable and up to date locally.
4. `pinax sync push --target capsa --vault ./my-notes --yes --json` — re-encrypts
   every legacy-keyed object and commits a fresh v2 manifest. Content-equal
   vaults are NOT skipped: the up-to-date fast path only fires when every
   remote object already carries the active key id.
5. `pinax sync keys --vault ./my-notes --json` — expect `remote_derivation=v2`
   and `reencryption_required=false`.
6. On every other device: upgrade `pinax`, then repeat steps 3–5. Devices on
   the old derivation keep working until they upgrade, because new envelopes
   are v2-only; do not push from an old build once migration started.

Rollback: restore the previous `pinax` binary — it still reads legacy
envelopes; a v2-encrypted remote requires re-pushing from the upgraded build
after rollback.

## Conflict workflow

When pull detects a local edit for a path also changed remotely, Pinax writes the remote trunk to the canonical note path and preserves the local edit next to it, for example `alpha.20260612153000.conflict.md`.

Use these commands to inspect and resolve:

```bash
pinax sync conflicts list --vault ./my-notes --json
pinax sync conflicts diff ./my-notes/notes/alpha.20260612153000.conflict.md --vault ./my-notes
pinax sync conflicts show ./my-notes/notes/alpha.20260612153000.conflict.md --vault ./my-notes --json
pinax version snapshot --vault ./my-notes --message "snapshot before sync conflict resolve"
pinax sync conflicts resolve ./my-notes/notes/alpha.20260612153000.conflict.md --merged ./merged-alpha.md --vault ./my-notes --yes
```

Conflict output and next actions must be consumable by humans and agents. Note bodies may appear only when the user explicitly asks for local content, such as `conflicts show`; sync receipts, stdout summaries, event streams, fixtures, object metadata, provider stderr, and backend logs must remain redacted.

## Relationship with backend and Local API

`sync` is the entry point for sync workflows. `capsa` configures the Capsa Sync transport state. `backend` manages provider profiles, capabilities, and object-store diagnostics.

Capsa Server is one Capsa Sync transport and owns auth/device state, revision CAS, blob persistence, audit, and readiness. S3/rclone direct transports skip the remote Capsa service and use provider credentials as the access boundary. Embedded Go API/local RPC calls the same app service as the CLI and does not bypass approval, dry-run, snapshot, conflict, event, or redaction rules.

S3 direct can serve as a backup mirror for encrypted Capsa Sync objects, but it remains a direct object-store transport. It must not be documented as Capsa server-side storage, and backup mirror wording must not promise daemon convergence or conflict resolution without a separate OpenSpec change.

`pinax api serve` is not a Capsa Sync transport. It exposes one centralized vault through local REST/RPC and is useful for dashboards and local agents that intentionally operate against that vault.

Client CLI parity does not replace the daemon. Remote API Mode can let a client trigger supported explicit sync operations through registered RPC capabilities, but realtime multi-device convergence should run `pinax sync daemon` on each device that owns a local vault.

See [`docs/architecture/cloud-sync-design.md`](../architecture/cloud-sync-design.md) for the architecture split. See [Client CLI Parity and Realtime Sync](../interfaces/client-cli-parity-and-sync.md) for the client coverage boundary. See also [`api`](./api.md), [`token`](./token.md), and [`profile`](./profile.md) for Remote API Mode.

## S3 direct sync troubleshooting

### `NoSuchBucket`

The bucket name is wrong or the credentials lack access. Verify the bucket exists and the profile has read/write permission:

```bash
pinax capsa doctor --vault ./my-notes --json
```

### `PathStyleDomainForbidden` (Tencent COS)

COS requires **virtual-hosted** addressing. Pinax auto-detects `.myqcloud.com` endpoints and sets `path_style=false`. If you overrode with `--addressing-style path`, remove it and reconfigure:

```bash
pinax capsa backend set s3 --bucket <bucket> --region ap-guangzhou --endpoint https://cos.ap-guangzhou.myqcloud.com --profile <profile> --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes
```

### `revision_conflict`

Another device pushed a newer revision since your last pull. Run `pinax sync --vault ./my-notes --yes` (bidirectional) to pull remote changes and re-push in one step. If conflicts arise, Pinax preserves local edits as `.conflict.md` files — use `pinax sync conflicts list` to inspect.

### `encryption_key_mismatch`

The encryption key reference differs from the key used by the remote manifest. Pinax persists a generated key as a user-level `stored://capsa-sync-<workspace>` secret when no explicit reference is supplied, so normal subsequent commands do not prompt for a new key. On another device, configure the same shared secret reference; do not generate a new local secret with the same workspace name. Restore the original secret before retrying. Do not push with an unknown key, because that can make existing remote blobs unreadable.

```bash
pinax capsa doctor --vault ./my-notes --json
pinax sync status --vault ./my-notes --json
```

### `LOCAL_UNPUSHED_CHANGES`

You moved or deleted a note locally without pushing. `sync pull` refuses to run to avoid restoring old paths. Run `pinax sync --vault ./my-notes --yes` to push the local move/delete and pull remote changes together.

### `cloud_not_configured`

No Capsa backend is configured for this vault. Run `pinax capsa backend set s3 ...` (S3 direct) or `pinax capsa login ...` (Capsa Server) first. Check with `pinax capsa status --vault ./my-notes --json`.

### Checksum warnings in logs

Pinax sets `RequestChecksumCalculationWhenRequired` and `ResponseChecksumValidationWhenRequired` for S3-compatible providers. This suppresses per-object `x-amz-checksum-*` WARN messages from services like Tencent COS that don't return checksum headers. No action needed.

## Manifest v2 migration

manifest v1 继续作为兼容 decoder；object-first 多端同步需要显式 promotion 到 v2。

```bash
pinax sync manifest audit --vault ./my-notes --json
pinax sync manifest plan --save --vault ./my-notes --json
pinax sync manifest promote --plan manifest-plan-<id> --remote-capability v2 --vault ./my-notes --yes --json
pinax sync manifest rollback --vault ./my-notes --yes --json
```

v2 entry 必须包含 `object_id`、`object_kind`、当前 `path`、`revision_id` 和 `device_id`。同 ID 路径变化产生 move；不同 ID 占用同一路径产生 `path_collision`；共同 base 上的双向内容变化产生 `revision_conflict`。daemon 不会自动 promotion，migration pending 或 unresolved identity 会阻止 remote write。

## Real transport evidence

真实 S3-compatible/server transport smoke 不从命令行接收明文凭据。先通过用户级配置或环境变量提供 endpoint/secret reference，再运行证据入口：

```bash
PINAX_SYNC_REAL_ENDPOINT=https://sync.example.invalid \
PINAX_SYNC_REAL_WORKSPACE=personal-smoke \
PINAX_SYNC_REAL_SECRET_REF=env:PINAX_SYNC_SECRET \
task integration:sync-real
```

每次运行写入 `temp/integration-test-runs/<run-id>/`；stdout、stderr、env 和 summary 会脱敏。没有配置 endpoint 时命令明确失败，不把 skipped test 伪装成真实同步证据。

## Declarative sync repository configuration (experimental)

`pinax sync repo` is an **experimental** declarative configuration layer that lets you commit sync topology to the repository as a structured asset, so a new device or remote dev container can bootstrap by clone + unlock + one command instead of re-entering backend flags. It is additive: existing `pinax sync`/`pinax capsa` commands keep working and are unaffected.

### Three-layer model

| Layer | File | Committed? | Contents |
| --- | --- | --- | --- |
| Repository declaration | `.pinax/pinax-sync.yaml` | Yes | Backend topology, workspace/namespace, logical credential & encryption-key identities, sync policy. No plaintext credentials. |
| Encrypted secrets | `.pinax/project-secrets.yaml` | Yes | Ciphertext only; plaintext exists only during an authenticated runtime unlock. |
| Device runtime state | `.pinax/cloud/` | No (gitignored) | Generated `config.yaml`, source marker, session, blob cache, receipts. |

Logical identities (`credential_id`, `encryption_key_id`) are resolved per-device to local profiles, keychain or a secret manager. The actual credential value is never written to the repository. All assets under `.pinax/` are protected from content-manifest upload by default `.pinaxignore` rules.

### Commands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax sync repo init` | Creates/updates the repository declaration. | Writes `.pinax/pinax-sync.yaml` and updates `.gitignore` device-state protection. Idempotent: re-running preserves the encryption key identity. |
| `pinax sync repo secret set` | Stores an encrypted secret value under a logical identity. | Writes `.pinax/project-secrets.yaml` (ciphertext only). Plaintext is transient. |
| `pinax sync repo secret list` | Lists secret metadata (name, identity, provider). | Read-only; never shows plaintext. |
| `pinax sync repo secret remove` | Removes an encrypted secret. | Mutates the encrypted asset. |
| `pinax sync repo bootstrap` | First device run: unlocks the repository credential and content key, compiles the runtime config, and optionally performs the initial pull. | `--pull --yes` executes a pull-only sync; `remote_write=false`. |
| `pinax sync repo migrate device-profile` | Converts the current AWS shared profile and Capsa content key into one repository-encrypted envelope. | Writes declaration, ciphertext envelope and managed ignore entries; never contacts the remote. |
| `pinax sync repo plan` | Reports intended config changes and drift. | No writes. |
| `pinax sync repo apply` | Regenerates the runtime config from the declaration on an initialized device. | Requires `--yes` for high-risk changes (workspace, backend namespace, encryption key identity, remote-delete policy). Backs up the prior runtime config for rollback. |
| `pinax sync repo doctor` | Diagnoses declaration/runtime drift, key identity and device state. | Read-only. |

### Security defaults

- Plaintext credentials, tokens, passwords and encryption keys are rejected at declaration validation time (`plaintext_sensitive_field`).
- A new device with no local sync receipt is pull-only; it does not upload local deletions or replace remote state.
- Bootstrap fails closed with `sync_repo_unlock_required` when no unlock identity is available, and never guesses or auto-generates a replacement encryption key.
- `plan` never writes; `apply` is the only normal path that regenerates runtime state.
- All sync-repo assets are protected from content-manifest upload (see `.pinaxignore` defaults).

### First-version unlock providers

The first version ships a deterministic `fake` AES-GCM provider (keyed by `PINAX_SYNC_FAKE_KEY`) and an `env` provider (reads `PINAX_SYNC_SECRET_<IDENTITY>`). These make the layer testable end-to-end and support CI/ephemeral bootstrap. Production deployments should register a reviewed provider (age/keychain); `doctor` flags the fake provider. The provider is an abstraction boundary, so swapping implementations does not change the CLI contract.

### Namespace facts (not server RBAC)

`tenant_id` and `app_id` produce a deterministic, collision-resistant remote namespace (`EffectiveNamespace`), but direct S3/rclone transport does **not** provide server-side tenant authorization, quota, audit or rate limiting. Doctor and plan expose these facts transparently. Full multi-tenant authorization is a separate server control-plane capability.

## Encrypted runtime dotenv loader (experimental)

`pinax sync env` is an **experimental** runtime env layer that lets a repository carry an encrypted dotenv asset, so a clone-and-unlock can self-describe the provider/environment variables a sync run needs — without requiring the calling shell to pre-set them and without committing plaintext. It is additive and coexists with `.pinax/project-secrets.yaml` (use dotenv for groups of provider/environment keys; use the logical secret API for single values).

### Asset model

| Layer | File | Committed? | Contents |
| --- | --- | --- | --- |
| Encrypted dotenv asset | `.pinax/pinax-sync.env.age` | Yes | Ciphertext + redacted metadata (key names, digest) only. Plaintext never required to be committed. |
| Materialized plaintext (optional) | `.pinax/runtime/pinax-sync.env` | No (gitignored, `0600`) | Only written by explicit `unlock --materialize`; removed by `clean`. |

The encrypted asset path is **fixed** (not user-selectable) to prevent path-escape and protected-path bypass. Plaintext lives only in an immutable in-memory `EnvSnapshot` resolved at command or daemon run boundaries.

### Commands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax sync env init` | Initializes the encrypted env asset and the managed `.gitignore` block. | Writes `.pinax/pinax-sync.env.age` (empty ciphertext) and updates `.gitignore`. Never creates plaintext. |
| `pinax sync env set KEY` | Stores an encrypted env value. | Re-encrypts the whole document. Plaintext is transient; never echoed in stdout/stderr/receipts. |
| `pinax sync env list` | Lists declared key names (metadata only). | Read-only; never shows values. |
| `pinax sync env unlock` | Resolves the snapshot in memory. | No writes by default. `--materialize` writes `.pinax/runtime/pinax-sync.env` at `0600`. |
| `pinax sync env clean` | Removes only the managed materialized file. | Never removes arbitrary user files; refuses symlinks. |
| `pinax sync env doctor` | Diagnoses asset, permissions, Git tracked-secret and reload state. | Read-only; reports `tracked_secret_env` with a `git rm --cached` remediation when plaintext env is already tracked. |

### Security defaults

- **Plaintext is never committed.** `init` and `set` write ciphertext only; `unlock` stays in memory by default; `--materialize` is the explicit compatibility exit at a fixed `0600` path.
- **Strict dotenv subset.** The parser accepts `KEY=value`, `KEY="quoted"`, `KEY='quoted'` only and rejects shell execution, `$(...)`, backticks, `${...}`, include/source directives, NUL/control characters, duplicate and empty keys. Errors report line number and key name only — never the rejected value.
- **Runtime injection precedence.** Explicit CLI flags > explicit process environment > decrypted env snapshot > project/user config > defaults. Child processes receive only allowlisted keys; the full decrypted environment is never copied to subprocesses.
- **Plaintext env is Git-ignored and Capsa-protected.** A managed `.gitignore` block ignores `.env`, `.env.*`, `*.env`, `.pinax/runtime/` and re-includes the encrypted asset plus `.env.example` templates. The content manifest hard-denies plaintext env paths so a user re-include can never upload secrets as ordinary content.
- **Daemon reload is run-boundary and fail-safe.** The daemon checks the encrypted asset digest between sync runs; on change it unlocks+parses a new snapshot for the next run. The current run retains its original snapshot. On failure it keeps the last successful snapshot and reports `sync_env_reload_failed` without leaking plaintext.

### First-version unlock providers

The env loader reuses the same unlock providers as `pinax sync repo secret`. The deterministic `fake` AES-GCM provider (keyed by `PINAX_SYNC_FAKE_KEY`) makes the layer testable end-to-end; the `env` provider resolves values from `PINAX_SYNC_SECRET_*`. Production deployments should register a reviewed provider (age/keychain).

## Repository-encrypted S3/COS credentials (experimental)

`pinax sync repo` can keep both the typed S3/COS credential bundle (`s3_credentials.v1`) and the Capsa content encryption key (`capsa_encryption_key.v1`) in one `credentialctl` project-secrets envelope. The ciphertext lives in `.pinax/project-secrets.yaml` and can be committed. Prompt, macOS Keychain, `0600` file and `PINAX_REPO_PASS` env unlock sources are supported; plaintext never enters the repository runtime YAML or structured output.

Recommendation order:

1. Use this repository-encrypted S3/COS workflow when the active binary reports all required capabilities.
2. Keep Git for Markdown history, declaration/ciphertext review, clone and rollback.
3. Use rclone only when native Pinax S3 cannot support the selected provider or as a separately labeled archive fallback.

The initial clone-time bootstrap is pull-only. Do not describe the full S3 backup lifecycle as stable until ordinary repository-encrypted push can perform a remote-aware dry-run and return either durable `remote_write=true` plus revision read-back, or `up_to_date=true` plus `remote_checked=true`.

### Commands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax sync repo credential init` | Creates the repository-encrypted credential envelope. | Writes `.pinax/project-secrets.yaml` (ciphertext + identity only, `0600`). Never writes plaintext. |
| `pinax sync repo credential set --stdin` | Encrypts one `s3_credentials.v1` bundle (payload via `--stdin`). | Re-writes the envelope atomically. Field-level validation runs before encryption; errors never echo values. |
| `pinax sync repo credential list` | Lists credential bundle metadata (name/identity/kind/format/version). | Read-only; no plaintext. |
| `pinax sync repo credential remove` | Removes one credential bundle. | Requires the unlock passphrase. |
| `pinax sync repo migrate device-profile --unlock prompt --remember-keychain --yes` | Imports the active device AWS profile and existing content key into the portable envelope. | Local files only; `remote_write=false`, `key_rotated=false`. |
| `pinax sync repo bootstrap --unlock prompt --remember-keychain --pull --yes` | Clone-time one-command restore. | Restores the content key to a device-level `stored://` reference and executes pull-only sync. |

Bootstrap/migration unlock source: `--unlock prompt|keychain|file|env`; file mode uses `--passphrase-file`, env mode reads `PINAX_REPO_PASS`, and Keychain references use `keychain://<service>/<account>`. All structured output carries metadata only — never the access key, secret key, session token, content key, passphrase, or ciphertext value.

### Declaration: `credential_mode`

The sync declaration gains an optional `backend.s3.credential_mode`:

- `device-profile` (default): existing behavior — endpoint/profile or the AWS default credential chain.
- `repository-encrypted`: the S3 transport resolves the typed bundle from the repository envelope via `credentialctl` and injects it as an explicit AWS SDK credentials provider. It must NOT silently fall back to a device-local AWS profile.

```yaml
backend:
  kind: s3-direct
  s3:
    bucket: yeisme-notes
    endpoint: https://cos.ap-shanghai.myqcloud.com
    region: ap-shanghai
    credential_mode: repository-encrypted
```

### Deprecation: `sync repo secret set --value`

`pinax sync repo secret set --value <plaintext>` is deprecated; prefer `--stdin`, or `sync repo credential set` for S3/COS bundles. The `--value` flag is retained for at least two minor releases (earliest removal `v0.4.0`); it emits a one-line redacted warning to stderr and never pollutes machine-readable stdout.

### Scope and rollback

- Pinax does NOT own KDF/AEAD/Keychain — those live in `credentialctl` (`pkg/projectsecrets`). Pinax owns only the `s3_credentials.v1` payload type and the projection.
- Rollback: set `credential_mode: device-profile` and regenerate the runtime config; the Capsa content encryption key and remote revisions are untouched.
- First-version validation is on Linux CI (credentialctl Keychain adapter is verified via a fake `/usr/bin/security` executable); real macOS Keychain smoke and the second-device COS restore are tracked as dogfood gates.
