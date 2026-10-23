## Why

根包 `pinax-drivebridge-mounted-vault-v1` 把对象存储 vault 的主路径定为 DriveBridge 路径挂载 + 本地 Pinax。Pinax 侧只需把该目录当普通 vault：doctor 报告文件系统事实，`init` 容忍已有内容挂载点，文档增加 Pattern D。禁止把 `storage set s3` 做成 live vault 文件系统。

## What Changes

- `storage doctor` / `vault doctor` 加法 facts：`control_plane_local`、`content_mount`、`content_writable`、`drivebridge_preset`。
- `pinax init` 在 `notes/` 已存在（目录或指向目录的符号链接）时幂等成功，不拆挂载、不清空远程。
- `docs/architecture/cloud-sync-design.md` 增加 Pattern D；`docs/commands/storage.md` 标明对象存储 vault 不走 `storage set s3` 正文读写。
- 不实现 S3/rclone vault 引擎，不删除 attach/hydrate。

## Capabilities

### New Capabilities

- `pinax-mounted-vault`：挂载目录当本地 vault 的 doctor/init/文档合同。

### Modified Capabilities

无删除。既有 `pinax-drivebridge-remote-notes` 的 attach/hydrate 保持。

## Impact

Owner：`cli/pinax`。触及 `internal/app` doctor/init、docs。DriveBridge 命令归 `mcp/drivebridge` 的 `drivebridge-pinax-vault-mount-v1`。
