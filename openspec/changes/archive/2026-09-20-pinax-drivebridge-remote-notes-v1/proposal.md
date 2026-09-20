## Why

根包 `openspec/changes/drivebridge-consumer-binding-v1` 已冻结：各项目原 local/S3 继续完善，DriveBridge **adopt** 同一位置做文件面，不替换 owner 存储。Pinax 现有三条远程路径（`storage set local|s3`、Capsa Cloud Sync、Remote API Mode）仍然有效，但不能把 DriveBridge 当成第四套笔记核或静默新开桶。本 change 把根合同落成 Pinax 可实施规格与任务，使后续 Wave 3 实现（节点 PX-3-01）有明确命令、doctor 事实和依赖门，而本切片不改运行时代码。

## What Changes

- 冻结 Pinax 原存储入口为 canonical，并继续完善：`pinax storage set local|s3`、`pinax storage status`、`pinax storage doctor`、`pinax capsa backend set`、`pinax backend add`。本包 **不删除、不重命名、不强制迁移** 这些命令。
- 新增加法命令，第一路径是零拷贝 attach **已经存在的** local 根或同一 `storage set s3` 前缀，不要求用户再配一个 DriveBridge 桶。
- 把工作副本绑到 OneDrive / Google Drive 仍是显式 `visible-working-copy` opt-in；doctor 必须报告 `provider-plaintext`，不得报成 Capsa encrypted。
- 冻结职责：DriveBridge 只做文件身份、版本、传输与浏览；Pinax 仍拥有笔记身份、Markdown 真源、proof loop、索引与 bounded projection。列出 `.md` 不是笔记投影。
- 写入围栏：DriveBridge 不得写 `.pinax/**`；Pinax 不得把「网盘里多了一个 Markdown」报告成笔记已保存。
- 三种远程模式分开报告：Capsa Cloud Sync、Remote API Mode、DriveBridge attach。Capsa 的 `remote_write=true` 不得因 DriveBridge 上传成功而发出。
- 给出可实施任务：attach 已有存储、doctor/status 诚实声明、换机 hydrate、附件引用。实现任务等待 DriveBridge adopt（节点 DB-2-01）；本 change 仍写清规格。
- 本切片只写 OpenSpec 与可选文档索引，不改 `cmd/**` 或 `internal/**`。

## Capabilities

### New Capabilities

- `pinax-drivebridge-remote-notes`: Pinax 侧 attach/adopt 已有 local/S3、显式网盘工作副本、hydrate、doctor 诚实声明、写入围栏、文件面与笔记核分离，以及附件 DriveBridge 引用。

### Modified Capabilities

- `pinax-cloud-sync`: 增加第三种远程模式隔离；Capsa `remote_write=true` 不得因 DriveBridge 操作成功而发出；DriveBridge attach 不是 Capsa transport。
- `pinax-cli-remote-api-mode`: DriveBridge attach 不得把 `--api-url` Remote API Mode 变成多设备文件同步；真源仍是服务端附着的那一个 vault。
- `asset-management`: 已钉住的附件 DriveBridge 文件版本可被其他项目绑定；跨项目不得共用笔记身份。

## Impact

- 根来源（只读）：根仓库 `openspec/changes/drivebridge-consumer-binding-v1/` 的 `design.md`、`specs/pinax-remote-notes/spec.md`、`specs/drivebridge-owner-storage-adopt/spec.md`、`handoffs.md` Wave 3。
- 本 change：`cli/pinax/openspec/changes/pinax-drivebridge-remote-notes-v1/**`。实现期（PX-3-01，不在本切片）预计触及 `internal/cli/storage_cmd.go`、`internal/app` storage/vault doctor、CLI-authored `.pinax/drivebridge-attach.yaml`、`docs/commands/storage.md` 与 `docs/architecture/cloud-sync-design.md` 的 Pattern C 说明。
- 硬依赖：DriveBridge `storage adopt` 零拷贝（`mcp/drivebridge` 节点 DB-2-01）。本规格可先写；Go 实现不得在 adopt 合同可测前宣称完成。
- 不改：Capsa 密文协议、Remote API、`pinax storage set local|s3` 现网语义、`pinax capsa backend set`、`pinax backend add`。
- 文档：面向人的说明用中文；命令名、flag、JSON/YAML key、错误码保持英文。
