## Context

Pinax 原本通过 Git 作为核心的多端同步方案。但因为 Git 在移动端适配性极差，现有的移动客户端实现面临着不必要的性能和复杂度开销。因此我们需要构建一个基于“盲存储”架构（加密存储至云端对象存储系统，且服务器不感知数据具体内容）的自定义同步层。

## Goals / Non-Goals

**Goals:**
- 实现端到端加密的远程同步协议（如 AES-256-GCM）。
- 引入 JSON 格式的同步清单（Manifest），记录远程和本地文件的 hash 与版本。
- 采用简单的“原目录退化”策略。出现并发写入冲突时，服务端文件保持为主干版本（原路径），本地产生冲突的版本在原目录下就地另存，通过添加时间戳和 `.conflict.md` 后缀（如 `name.20260609.conflict.md`）的方式进行保留。
- 提供对应 CLI 命令来全局搜索这些冲突文件并比对差异。

**Non-Goals:**
- 不支持基于算法的自动化增量 Diff 合并，依赖人工通过搜索后缀来手动解决冲突。
- 不引入 WebSocket 实时推送，使用客户端驱动的显式拉取和推送（Sync）。

## Decisions

- **存储后端抽象 (Storage Backend)**: 为了应对不同的部署需求，在 `internal/cloud` 中设计一个核心的 `StorageBackend` 接口（封装 `Get`, `Put`, `Stat` 等操作）。
  - **S3 / API 接入**: 支持通过 HTTP 请求和 `If-Match` 乐观锁来保证并发行为了。
  - **FUSE / 文件系统挂载**: 支持诸如 `JuiceFS`、`SMB` 或者本地绝对路径的 `file://` 后端协议。它直接使用 Go 的文件 I/O，并利用基于内容 hash 校验和原子重命名来实现并发锁和更新。
- **同步凭证**: 客户端需要通过配置文件加载加密的 Sync Secret。
- **多设备并发控制**: 依赖服务端的乐观并发控制（例如 Manifest 的 Base Revision 或 HTTP ETag 匹配）。当多台设备同时进行双向同步或推送时，服务端强制保证清单版本的线性增长；后提交的设备会被拒绝（409 Conflict），强制其先拉取更新（Pull），从而在本地无损解决多设备同时写入的安全问题。
- **无感冲突解决与辅助排查**: 同步中产生冲突时不应当直接中断报错，而是将本地变动在原目录打包为带有 `.conflict.md` 后缀的冲突文件。这让同步进程可以顺利结束，同时避免了因移动文件导致的目录状态和上下文丢失。为了帮助用户在事后处理这些文件，设计 `pinax sync conflicts` 工具集。

## CLI Surface

为实现完整的云端同步体验，规划以下子命令群：

- `pinax sync init`：初始化配置，引导用户绑定后端存储凭证并设置/输入端到端加密密码。
- `pinax sync status`：检测同步健康度，包含与服务端的连通性、当前清单版本对比、以及本地待推送/待拉取的笔记大体数量。
- `pinax sync diff`：仅执行同步计算，不修改本地或远端数据（即 dry-run 模式），供用户在正式 push 前确认。
- `pinax sync`：执行一键双向同步（先获取拉取计划 -> 下载更新及标记冲突 -> 执行推送计划 -> 更新远端清单）。
- `pinax sync pull`：单向拉取。仅获取远端更新并应用到本地，不对远端进行任何写入。
- `pinax sync push`：单向推送。仅推送本地更新到远端；若远端有更新（清单不匹配），则中断并提示需要先 Pull。
- `pinax sync conflicts list [--json]`：扫描并列举本地未处理的 `.conflict.md` 文件。支持 JSON 格式便于 Agent 提取冲突列表。
- `pinax sync conflicts diff <file>`：面向人的命令，打印冲突文件与主干版本的内容差异。
- `pinax sync conflicts show <file> --json`：面向 AI 的命令，直接在 JSON 结构中返回原主干内容（`original_content`）和冲突版本内容（`conflict_content`），避免 AI 解析复杂的 diff 格式。
- `pinax sync conflicts resolve <file>`：冲突解决。
  - `--keep-local`：使用冲突版本覆盖主干并清除冲突文件。
  - `--keep-remote`：丢弃冲突版本。
  - `--merged <path/to/merged>`：AI 完成智能合并后，读取合并后文件的内容覆盖主干并清除冲突文件。

## Risks / Trade-offs

- **Risk**: 加密密钥丢失导致数据无法解密。
  - **Mitigation**: 提供明确的提示，建议用户备份 Sync Secret。
- **Risk**: 并发控制若实现有误会导致相互覆盖。
  - **Mitigation**: 在 `internal/sync` 层实现详尽的单元测试，涵盖模拟服务端的 409 Conflict 回复。
