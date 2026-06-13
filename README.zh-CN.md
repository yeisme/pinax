# Pinax（中文）

[English README](./README.md)

Pinax 是一个本地优先的 Markdown 笔记 CLI，面向希望保留可迁移知识库的人和 Agent。你的 Markdown vault 始终是真源；`.pinax/` 只保存由 CLI 创建的配置、索引、收据、事件和审计投影，这些内容可以被重建、审查和迁移。

Pinax 关注安全的本地工作流：捕获笔记、建立索引和搜索、检查链接和反向链接、生成修复和整理计划、在高风险写入前创建版本快照、提供 bounded JSON/agent 输出，并通过显式 Cloud Sync transport 同步加密 revision。

## 状态

| 能力 | 状态 |
| --- | --- |
| 本地 Markdown vault、note、journal、inbox/draft、template、search、link/backlink、asset、project board、repair/organize plan | 已支持 |
| CLI 输出模式：默认 summary、`--agent`、`--json`、`--events`、`--explain` | 已支持 |
| 本地 dashboard、只读 MCP、localhost REST/RPC adapter | 已支持 |
| 基于 server、file/S3-compatible object store、rclone transport 的 Cloud Sync | Preview |
| Provider automation 和 briefing delivery | Experimental |

## 安装

前置要求：

- Go 1.26.1 或更新版本。
- 可选：[Task](https://taskfile.dev/)，用于 `task check` 等开发快捷命令。

从源码安装：

```bash
go install github.com/yeisme/pinax/cmd/pinax@latest
```

从本地 checkout 构建：

```bash
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
./dist/pinax version
```

## 快速开始

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax vault validate --vault ./my-notes --json
pinax note add "Research Log" --body "First note" --tags research --vault ./my-notes
pinax index refresh --vault ./my-notes --json
pinax search "First note" --vault ./my-notes --json
```

更多命令入口见 [Command Manual](./docs/commands/README.md)。详细命令文档保持英文，以保证 flag、schema key、错误码和机器输出字段稳定一致。

## 核心概念

### Markdown vault 是真源

普通笔记、附件和用户正文都保存在本地 vault 中。SQLite/GORM index、asset manifest、sync state、repair plan、render receipt 等 `.pinax/` 内容是可审查的机器投影，不应该被手写维护。

### 显式写入边界

多数查看命令默认只读。写入 Markdown、`.pinax/`、version backend、provider state 或 remote sync state 的命令需要显式确认，例如 `--yes`、`--dry-run` 或版本快照要求。

### 面向 Agent 的 bounded 输出

`note read/show --display card|detail|context`、project board、dashboard、MCP、REST 和 RPC 共用 `NoteDisplay` 投影。默认 bounded display 不输出完整正文；只有显式 `--display body` 才会在本地 JSON 投影中包含正文。

### Cloud Sync 不是 Local API

`pinax api serve` 是本机 localhost REST/RPC 投影适配器。Cloud Sync 是分布式同步协议：每台设备保留自己的本地 vault，通过选定 transport 交换加密 blob、manifest 和 revision metadata。`remote_write=true` 只在远端 revision 被 durable commit 且本地写入 sync-state evidence 后出现。

## 常用命令

初始化和注册 vault：

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax vault register ./my-notes --name work --default
pinax vault validate --vault work --json
```

写入、搜索和查看关系：

```bash
pinax note add "Research Log" --body "Today's observations" --tags research --vault work
pinax note list --tag research --recent --limit 20 --vault work
pinax search "observations" --vault work --json
pinax note links "Research Log" --vault work --json
pinax note backlinks "Research Log" --include-broken --vault work --json
```

维护和整理：

```bash
pinax vault doctor --vault work --agent
pinax repair plan --vault work --save --json
pinax version snapshot --vault work --message "snapshot before repair"
pinax repair apply --vault work --plan repair-abc123 --yes
pinax organize plan --vault work --save --agent
```

本地 API / MCP：

```bash
pinax api routes --vault work --json
pinax api schema export --format openapi --vault work --json
pinax api serve --readonly --no-auth --port 8787 --vault work
pinax mcp serve --vault work
```

Cloud Sync preview：

```bash
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target cloud --vault ./device-a --yes --json
pinax sync pull --target cloud --vault ./device-b --yes --json
```

## 本地验证

```bash
task check
```

没有安装 Task 时使用：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

## 文档入口

- [英文 README](./README.md)
- [中文文档地图](./docs/README.zh-CN.md)
- [英文文档地图](./docs/README.md)
- [命令手册](./docs/commands/README.md)
- [贡献指南（中文）](./CONTRIBUTING.zh-CN.md)
- [安全策略（中文）](./SECURITY.zh-CN.md)

## 许可证

当前还没有选择公开开源许可证。在项目 owner 添加 `LICENSE` 文件前，请不要假设代码已授予再分发或复用权利。
