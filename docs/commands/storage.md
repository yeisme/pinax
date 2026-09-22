# storage Command

`pinax storage` configures the vault storage backend. The storage configuration describes a local or S3 backend profile and does not store provider secrets. `storage set s3` writes the profile only and does not connect to object storage; a plaintext vault whose bytes live in S3/MinIO/COS is DriveBridge `preset pinax-vault` plus `storage set local` on that `--vault-root` (see Pattern D in `docs/architecture/cloud-sync-design.md`).

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `storage set local` | Configure a local storage backend. | Writes the storage profile. |
| `storage set s3` | Configure an S3 storage backend. | Writes the storage profile; does not connect to S3. |
| `storage status` | View storage backend status. | Does not write. |
| `storage doctor` | Diagnose storage backend configuration. | Does not write. |
| `storage attach-drivebridge --space <space>` | Attach DriveBridge to the SAME local root or S3 bucket/prefix（零拷贝，不新开桶、不复制笔记字节）。 | Writes `.pinax/drivebridge-attach.yaml` only. |
| `storage detach-drivebridge` | Remove the DriveBridge attach record; storage profile and notes stay untouched. | Removes the attach record. |
| `storage bind-working-copy --provider onedrive\|gdrive --space <space>` | Explicit opt-in for a plaintext provider working copy. | Writes the attach record with `content_mode=provider-plaintext`. |
| `storage hydrate --space <space>` | Rebuild the working copy from the attached DriveBridge space on this device. | Writes working-copy files and refreshes the index. |

## Common Workflows

```bash
pinax storage set local --root ./my-notes --vault ./my-notes
pinax storage set s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json
pinax storage status --vault ./my-notes
pinax storage doctor --vault ./my-notes --json
```

## DriveBridge attach 边界

- `storage set local|s3` 仍是 canonical 写入路径；attach 只是把 DriveBridge 文件面挂到同一位置，不替代 owner 存储。
- attach 要求 location 与当前 storage profile 完全一致；不一致返回 `drivebridge_location_mismatch`，且不写任何部分 attach 记录。
- 未安装 DriveBridge 时，只有 `attach-drivebridge`、`bind-working-copy`、`hydrate` 失败为 `drivebridge_not_installed`；`storage set local|s3`、`status`、`doctor`（未 attach）、`capsa backend set`、`backend add` 全部照常可用。
- 网盘工作副本必须显式 `bind-working-copy` opt-in；`init`、`note add`、`storage set` 永远不会静默上传 vault。未 opt-in 时把 OneDrive/GDrive 空间当默认 attach 目标返回 `drivebridge_working_copy_opt_in_required`。
- `storage doctor` / `vault doctor` 报告英文 facts：`drivebridge_attached`、`drivebridge_space`、`drivebridge_kind`、`drivebridge_content_mode`（`none` / `adopt_local` / `adopt_s3` / `provider-plaintext` / `opaque_encrypted`）、`drivebridge_location_match`、`capsa_sync_configured`、`remote_api_configured`，以及挂载 vault 加法 `control_plane_local`、`content_mount`、`content_writable`、`drivebridge_preset`。
- Capsa Cloud Sync、Remote API Mode 与 DriveBridge attach 是三种独立模式：DriveBridge 传输成功不会发出 Capsa `remote_write=true`，也不会推进 Capsa sync-state。
- hydrate 钉住观察到的 file id/version/sha256；远端中途变化返回 `file_version_changed`，本机 local 不可见返回 `drivebridge_local_unreachable`；冲突交给 `pinax repair` / `pinax sync conflicts`，DriveBridge 不自动合并正文。
- hydrate 钉住观察到的 file id/version/sha256；远端中途变化返回 `file_version_changed`，本机 local 不可见返回 `drivebridge_local_unreachable`；冲突交给 `pinax repair` / `pinax sync conflicts`，DriveBridge 不自动合并正文。
- hydrate 只落地可校验字节：DriveBridge 清单条目缺 `sha256` 时跳过并给出 `hydrate_unverifiable_file` 警告与 `unverified_skipped` fact；越界路径（绝对路径、`..`）计入 `unsafe_paths_skipped`，绝不写根外。
- attach/bind 拒绝静默改绑：已 attach 其他 space 时返回 `drivebridge_already_attached`，必须先 `pinax storage detach-drivebridge`；同 space 重挂保持幂等。
- attach 记录损坏（读不了 `.pinax/drivebridge-attach.yaml`）时 consume 报 `drivebridge_attach_unreadable`、doctor 报 `drivebridge_attach_unreadable=true`，不会伪装成未 attach；用 `pinax storage detach-drivebridge` 删除坏记录后重新 attach。
- 已知限制：若 owner 侧 adopt 落在与当前 storage profile 不一致的位置（owner 侧行为异常），该 space 会持续返回 `drivebridge_location_mismatch`，而 DriveBridge 当前无 `unadopt` 命令，只能换用新的 space 名重新 attach。

## Compatibility Aliases

The old `storage set-local` and `storage set-s3` remain compatible, but the primary path uses `storage set local|s3`.
