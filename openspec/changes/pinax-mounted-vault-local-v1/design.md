# Design

## Context

根合同要求 Pinax 零对象存储客户端。DriveBridge 把内容树以符号链接放进 `--vault-root`，`.pinax` 是本机实目录。

## Decisions

### D1 doctor 只看文件系统

- `control_plane_local`：`.pinax` 存在且不是符号链接。
- `content_mount`：`none`（普通目录）、`drivebridge_path`（notes 为符号链接或存在 `.drivebridge-pinax-vault.json`）、`unmounted`（缺失或悬空链接）、`unknown_fuse`（其它非常规）。
- `content_writable`：能在 notes 下创建临时文件则为 true。
- `drivebridge_preset`：标记文件为 `pinax-vault`，否则 `none`。
- 未装 DriveBridge 不导致 `storage set local` 失败。`.pinax` 若是符号链接则 issue `control_plane_on_remote`。

### D2 init 不破坏已有 notes

`MkdirAll(notes)` 前 `Lstat`：已存在的目录或指向目录的符号链接保留。悬空链接失败为可诊断错误，不 `os.Remove` 用户挂载点。

### D3 Pattern D 文档

cloud-sync-design 增加挂载 vault；强调与 Capsa、attach/hydrate 分家。

## 验证

```bash
go test ./internal/app -run 'InitVault|StorageDoctor|MountedVault' -count=1
openspec validate pinax-mounted-vault-local-v1 --strict --no-interactive
```
