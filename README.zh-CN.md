# Pinax（中文）

[English README](./README.md)

Pinax 是面向 Markdown vault 的 **agent-safe 知识控制平面**——它让 AI 安全地读取、诊断、修复和同步你真实的本地知识库，同时让每一次 agent 写入都可审计、可预览、可回滚。你的 Markdown vault 始终是真源；agent 看不到不该看的明文，云端也没有明文笔记。

> 三个可复述概念：**Local Vault 是真源 / Proof Loop 保护每一次 agent 写入 / Cloud Sync 只协调密文。**

## The aha moment

一条命令跑完整个 agent-safe 流程——先预览，再 plan、snapshot、apply，出问题就 restore。每一步都有界：agent 读到的是 projection，不是原始正文；写入只通过显式的 plan → snapshot → apply 链发生。

```bash
pinax proof loop run --vault ./my-notes --json            # 预览：一个带 proof_loop_run_id 的 projection
pinax repair plan --vault ./my-notes --save                # 把 vault 健康问题变成可审阅的 plan
pinax version snapshot --vault ./my-notes --message "before repair"   # 任何写入前的保护快照
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes      # 只 apply 已批准的低风险修复
pinax version restore notes/example.md --revision HEAD --plan --vault ./my-notes          # 出问题了？
pinax version restore apply --vault ./my-notes --plan restore-<id> --yes                 # 通过 CLI 受控路径回滚
```

## 为什么用 Pinax

| 差异化点 | 含义 |
| --- | --- |
| **Proof loop 安全写入** | 每次 agent 驱动的变更都是 plan → snapshot → apply → receipt → restore。没有直接文件手术，没有静默写入，每次 apply 都可逆。 |
| **Plaintext boundary** | 读取命令默认 `--display card`，不输出完整正文。Agent、MCP、dashboard、project board 共用一个有界 projection；只有显式 `--display body` 才会在本地 JSON projection 中包含正文。 |
| **自托管加密同步** | Pinax Cloud 只协调加密 revision——AES-256-GCM 客户端加密，服务端永远看不到明文笔记，也永远不执行本地工具。 |

Pinax **互补** Obsidian 和 Logseq，作为你 vault 的 agent-safe 维护层；**避开** Notion 的云锁定；比 Reflect **更可编程、更可验证**。它不是另一个笔记 App——它是让你的已有 Markdown vault 对 AI 安全的控制平面。

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

- Go 1.26.1 或更新版本，或下载下方预编译 archive。
- 可选：[Task](https://taskfile.dev/)，用于 `task check` 等开发快捷命令。

从源码安装：

```bash
go install github.com/yeisme/pinax/cmd/pinax@latest
```

从 GitHub Release 下载预编译 archive（当前系列为 **pre-release**，`pinax/v0.1.0-preview`）：

```bash
# linux x86_64（请按你的平台调整 os/arch：darwin、windows；x86_64、aarch64）
curl -L -o pinax.tar.gz https://github.com/yeisme/pinax/releases/download/pinax/v0.1.0-preview.1/pinax_0.1.0-preview.1_linux_x86_64.tar.gz
tar xzf pinax.tar.gz
./pinax version

# 校验 checksum（从同一 release 页面下载 checksums.txt）
sha256sum -c checksums.txt --ignore-missing
```

Windows 使用 `.zip` archive 而非 `.tar.gz`。完整 asset 列表见 release 页面（`darwin`、`linux`、`windows` × `x86_64`、`aarch64`）。

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

## 五大核心工作流

Pinax 围绕一条 agent-safe proof loop 构建。用户或 agent 驱动一个真实 Markdown vault 经过五个阶段，每个阶段都保持有界——projection 永不输出完整正文，写入只通过 plan、snapshot、receipt 和显式 apply 发生。

| 路径 | 作用 | 入口命令 |
| --- | --- | --- |
| **Capture** | 向 vault 添加 note、inbox item 和 journal entry。 | `pinax init`、`pinax note add`、`pinax inbox capture`、`pinax journal daily append` |
| **Retrieve** | 构建 index projection 并读取有界上下文。 | `pinax index sync`、`pinax search`、`pinax note links`、`pinax note backlinks`、`pinax note orphans` |
| **Diagnose** | 检查 vault 健康并暴露低风险和需审阅项。 | `pinax vault doctor`、`pinax vault stats` |
| **Plan** | 把问题变成可审阅、可保存的 repair 和 organize plan。 | `pinax repair plan --save`、`pinax organize plan --save` |
| **Apply safely** | 先 snapshot，再以显式确认 apply 低风险变更。 | `pinax version snapshot`、`pinax repair apply --yes`、`pinax organize apply --yes` |

Agent 可以用一条命令跑完整个 loop。Preview 是只读的；加 `--apply --yes` 来创建新 snapshot 并 apply 已批准的操作：

```bash
pinax proof loop run --vault ./my-notes --json            # 预览：一个带 proof_loop_run_id 的 projection
pinax proof loop run --vault ./my-notes --apply --yes     # 新 snapshot + 已批准的 repair/organize apply
```

如果 apply 出错，通过 CLI 受控的 restore 路径从最近 snapshot 回退单个文件（绝不是直接文件手术）：

```bash
pinax version restore notes/example.md --revision HEAD --plan --vault ./my-notes
pinax version restore apply --vault ./my-notes --plan restore-<id> --yes   # local_write=true, remote_write=false
```

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax inbox capture "an idea" --vault ./my-notes
pinax note add "Research Log" --body "First note" --vault ./my-notes
pinax index sync --vault ./my-notes --json
pinax search "First note" --vault ./my-notes --json
pinax vault doctor --vault ./my-notes --json
pinax repair plan --vault ./my-notes --save --json
pinax version snapshot --vault ./my-notes --message "checkpoint"
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes
```

每条命令都支持 `--json`、`--agent`、`--events` 和 `--explain` 输出模式，共用一个 projection 边界：有界事实和下一步动作，永不输出原始正文、token 或 provider payload。Cloud Sync、daily briefing、provider 扩展和托管平台能力是独立的高级工作流，不属于这条本地 proof loop。

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
- [Agent-safe boundary（安全边界）](./docs/overview/agent-safe-boundary.md)
- [中文文档地图](./docs/README.zh-CN.md)
- [英文文档地图](./docs/README.md)
- [产品定位](./docs/overview/product-positioning.md)
- [命令手册](./docs/commands/README.md)
- [贡献指南（中文）](./CONTRIBUTING.zh-CN.md)
- [安全策略（中文）](./SECURITY.zh-CN.md)

## 许可证

当前还没有选择公开开源许可证。在项目 owner 添加 `LICENSE` 文件前，请不要假设代码已授予再分发或复用权利。
