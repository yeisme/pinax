# 快速开始（5 分钟）

本指南带你在 5 分钟内完成个人本地知识工具的最小闭环：创建 Markdown vault、记录内容、搜索、写 daily journal，并创建本地版本快照。

本指南只覆盖本地核心流程，不涉及 Cloud Sync、MCP server、Templates、Project Boards 等高级能力。需要从空 vault 串到 sync/API/publish/plugin 的完整路线时，读 [完整使用样例](./usage/full-example.md)；命令索引见 [命令手册](./commands/README.md)。

## 前置条件

- Go 1.26.1 或更新版本（用于 `go install`；下载预编译 archive 则不需要）。
- Pinax 是 CLI-only 短生命周期进程，无需后台 daemon。

## 1. 安装 Pinax

任选一种方式：

**方式 A：从源码安装（需要 Go）**

```bash
go install github.com/yeisme/pinax/cmd/pinax@latest
```

**方式 B：公开镜像安装当前稳定版 `v0.2.0`（无需 Go、无需 GitHub token）**

```bash
curl -fsSL https://raw.githubusercontent.com/yeisme/yeisme-dist/main/install.sh | bash -s -- pinax v0.2.0
export PATH="$HOME/.yeisme/bin:$PATH"
```

或从 [yeisme-dist pinax/v0.2.0](https://github.com/yeisme/yeisme-dist/releases/tag/pinax/v0.2.0) 下载对应平台 archive，校验 checksum 后放入 `PATH`：

```bash
# 示例：Linux x86_64
curl -fsSL -o pinax.tar.gz https://github.com/yeisme/yeisme-dist/releases/download/pinax/v0.2.0/pinax_0.2.0_linux_x86_64.tar.gz
curl -fsSL -o checksums.txt https://github.com/yeisme/yeisme-dist/releases/download/pinax/v0.2.0/checksums.txt
sha256sum -c checksums.txt --ignore-missing
tar xzf pinax.tar.gz pinax
chmod +x pinax
sudo mv pinax /usr/local/bin/
```

验证安装：

```bash
pinax version
```

## 2. 初始化 Vault

```bash
pinax init ./my-notes --title "My Knowledge Base"
```

`pinax init` 创建 vault 目录结构和 `.pinax/` 下的 CLI 管理资产（config、index、events）。不会连接云端或写入 provider token。

## 3. 写入第一条笔记

```bash
pinax note add "First Note" --body "My first Pinax note." --vault ./my-notes
```

`note add` 是推荐的笔记创建入口；`note new` 和 `note create` 是兼容别名。

## 4. 记录与找回

```bash
pinax inbox capture "Read the local-first paper" --vault ./my-notes
pinax journal daily append --body "Reviewed local-first tools" --vault ./my-notes
pinax search "local-first" --vault ./my-notes
```

这些命令只依赖本地 vault。同步、远程 API 和 Agent runtime 都不是个人主路径的前置条件。

查看默认入口和完整命令目录：

```bash
pinax --help
pinax commands
```

## 5. 创建本地安全点

在整理或批量修改前创建本地 version snapshot：

```bash
pinax backup create --vault ./my-notes --message "snapshot before repair"
pinax backup history --vault ./my-notes --json
```

高级的 repair、organize、proof loop 和 restore 流程仍然保留，但不要求新用户先理解。

## 6. 证明可回滚

万一 apply 出错，可以通过 CLI 管理的 restore 路径把单个文件回滚到指定 revision（绝不直接做文件手术）：

```bash
# 生成只读 restore plan（使用 version snapshot 输出的 snapshot_id）
SNAPSHOT_ID=$(pinax backup history --vault ./my-notes --json | jq -r '.data.snapshots[0].snapshot_id')
pinax backup restore first-note.md --revision "$SNAPSHOT_ID" --plan --vault ./my-notes --json

# 应用 restore plan 写回本地 Markdown
# <restore_id> 来自上一步 version restore --plan 的输出
pinax backup restore apply --vault ./my-notes --plan <restore_id> --yes --json
```

应用成功即证明：Pinax 的每一次 write 都有 snapshot 保护，可审计、可回滚。

## 下一步

- 浏览 [命令手册](./commands/README.md) 了解每个 workflow 的推荐入口。
- 按 [完整使用样例](./usage/full-example.md) 继续验证项目、API、sync、backend、publish 和 plugin dry-run。
- 阅读 [本地开发](./operations/local-development.md) 了解 `task check`、`task release:local` 等开发任务。
- 高级能力（Cloud Sync、MCP server、Templates、Project Boards）不在本快速开始范围，见对应命令文档。
