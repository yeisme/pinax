# Pinax 文档地图

[English](./README.md)

`docs/` 是 Pinax 子项目的产品、设计、运行、协议、实现、QA 和 release 文档入口。命令名、flag、JSON key、错误码和机器协议字段保持英文，以保证脚本、agent 和 SDK 稳定。

Pinax 是本地优先的统一笔记 Agent CLI：Markdown vault 是用户知识资产的真源；SQLite/GORM 是可重建索引投影；version backend 提供版本证据和受保护工作流的 snapshot basis；外部平台通过 CLI-backed Provider adapter 或 Cloud Sync transport 接入。

## 当前状态

- 当前阶段：本地优先 notebook workflow 已可通过 CLI 使用，适合外部开发者评估。
- 已支持：local init、vault validate、daily/inbox/draft、note add/create/list/read/edit/rename/move/archive/delete/tag、共享 `NoteDisplay`、project board、saved views、SQLite/GORM index、search、link/backlink/orphan、attachment、Markdown import/export、template、metadata plan/apply、repair plan/apply、organize plan/list/apply、version snapshot、asset manifest、read-only dashboard、read-only MCP、localhost REST/RPC adapter，以及 server/file/S3/rclone Cloud Sync transport。
- Provider automation 和 briefing delivery 仍处于 experimental。
- 用户可见 note path 使用 vault-relative canonical path，例如 `foo.md` 或 `work/foo.md`；历史 `notes/foo.md` 只作为兼容输入。
- 贡献入口见 [CONTRIBUTING.zh-CN.md](../CONTRIBUTING.zh-CN.md)。

## 关键边界

### Note 和索引

- Markdown vault 是笔记、附件和关系图的真源。
- SQLite/GORM index 只是可重建投影；索引缺失或过期时先运行 `pinax index refresh --vault <vault>`。
- `pinax note links`、`pinax note backlinks`、`pinax note orphans`、`search --link-target`、doctor、repair、organize、dashboard 和 MCP 必须复用 application service 的解析和 resolver。

### Version 和 Asset

- `pinax version` 是用户可见的版本证据入口；Git 只是可选 backend type 和隐藏兼容 alias。
- Asset manifest 是 CLI-authored metadata；asset 文件本身仍是 vault 中的普通可迁移文件。
- Asset payload、raw diff、provider payload、webhook token、secret ref、Authorization/Cookie 不得进入 stdout、stderr、event、record log 或 fixture。

### Project Board、Remote API 和 Cloud Sync

- `pinax project board show|plan|configure|export` 提供本地 project workspace，vault 仍是真源。
- `pinax note read/show --display card|detail|context|body`、project board、dashboard、MCP、REST 和 RPC 共用 `NoteDisplay`；默认 bounded display 不输出完整正文。
- `pinax api serve` 是 localhost REST/RPC projection adapter，不是 public hosted API。
- Cloud Sync 是独立分布式同步设计：每台设备保留本地 vault，Cloud backend 协调加密 revision、blob 和 conflict。详见 [Cloud Sync Architecture](./architecture/cloud-sync-design.md)。

## 文档入口

- [中文 README](../README.zh-CN.md)
- [English README](../README.md)
- [Documentation Design](./overview/documentation-design.md)
- [长期资料源笔记](./overview/durable-source-notes.md)
- [Product Positioning](./overview/product-positioning.md)
- [MVP Scope](./product/mvp-scope.md)
- [Architecture Boundaries](./architecture/architecture-boundaries.md)
- [Cloud Sync Architecture](./architecture/cloud-sync-design.md)
- [Go Development Ecosystem Design](./architecture/go-development-ecosystem.md)
- [CLI Output Contract](./interfaces/cli-output-contract.md)
- [Local REST/RPC Contract](./interfaces/remote-api-contract.md)
- [Command Manual](./commands/README.md)
- [Operations Manual](./operations/local-development.md)
- [贡献指南](../CONTRIBUTING.zh-CN.md)
- [安全策略](../SECURITY.zh-CN.md)

## 常用入口

| 目标 | 命令 |
| --- | --- |
| 创建知识库 | `pinax init ./my-notes --title "My Knowledge Base"` |
| 注册默认 vault | `pinax vault register ./my-notes --name work --default` |
| 快速写 note | `pinax note add "Title" --body "Content" --vault work` |
| 搜索内容 | `pinax search "keyword" --vault work` |
| 查看 vault 健康 | `pinax vault doctor --vault work` |
| 生成修复计划 | `pinax repair plan --vault work --save` |
| 生成整理计划 | `pinax organize plan --vault work --save` |
| 刷新索引 | `pinax index refresh --vault work --json` |
| 查看本地 API routes | `pinax api routes --vault work --json` |

详细命令说明见 [Command Manual](./commands/README.md)。

## 验证入口

只改文档时通常不需要运行 Go 测试；修改 Go 代码后运行：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

如果安装了 Task，也可以运行：

```bash
task check
```
