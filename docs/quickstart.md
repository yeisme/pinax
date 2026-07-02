# 快速开始（5 分钟）

本指南带你在 5 分钟内完成从安装到验证 Proof Loop 的最小流程。你将创建一个本地 Markdown vault、写入笔记、运行只读 proof loop 预览、生成并应用修复计划、并验证可回滚。

本指南只覆盖本地核心流程，不涉及 Cloud Sync、MCP server、Templates、Project Boards 等高级能力（见 [命令手册](./commands/README.md)）。

## 前置条件

- Go 1.26.1 或更新版本（用于 `go install`；下载预编译 archive 则不需要）。
- Pinax 是 CLI-only 短生命周期进程，无需后台 daemon。

## 1. 安装 Pinax

任选一种方式：

**方式 A：从源码安装（需要 Go）**

```bash
go install github.com/yeisme/pinax/cmd/pinax@latest
```

**方式 B：下载 GitHub Release archive（无需 Go）**

从 [Pinax Releases](https://github.com/yeisme/pinax/releases) 下载对应平台的 archive（例如 `pinax_0.1.2_linux_x86_64.tar.gz`），解压并把 `pinax` 放到 `PATH`：

```bash
# 示例：Linux x86_64
curl -L -o pinax.tar.gz https://github.com/yeisme/pinax/releases/download/v0.1.2/pinax_0.1.2_linux_x86_64.tar.gz
curl -L -o checksums.txt https://github.com/yeisme/pinax/releases/download/v0.1.2/checksums.txt
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

## 4. 运行 Proof Loop 预览（只读）

```bash
pinax proof loop run --vault ./my-notes --json
```

`proof loop run` 把 Capture → Retrieve → Diagnose → Plan → Snapshot → Apply 串成一条可调用、可审计的工作流。默认是只读预览，返回一个带 `proof_loop_run_id` 的 bounded projection，列出诊断结果和下一步动作，不会写 vault。

加上 `--apply --yes` 才会在新鲜 snapshot 之后执行已批准的低风险修复。

## 5. 计划、快照、应用修复

把诊断出的健康问题转成可审阅、可保存、受 snapshot 保护的修复动作：

```bash
# 生成修复计划并保存到 .pinax/repair-plans/<plan_id>.json
pinax repair plan --vault ./my-notes --save --json

# 在应用前创建本地 version snapshot
pinax version snapshot --vault ./my-notes --message "snapshot before repair"

# 应用已保存计划中的低风险修复（metadata、tags、index rebuild、archive status）
# <plan_id> 来自上一步 repair plan --save 的输出
pinax repair apply --vault ./my-notes --plan <plan_id> --yes
```

`repair apply` 只执行低风险修复（metadata、tags、index rebuild、archive status）；重复标题、断链、歧义链接、空笔记、孤儿笔记只生成人工审阅项，不会自动删除、合并或改写正文。

## 6. 证明可回滚

万一 apply 出错，可以通过 CLI 管理的 restore 路径把单个文件回滚到指定 revision（绝不直接做文件手术）：

```bash
# 生成只读 restore plan（使用 version snapshot 输出的 snapshot_id）
SNAPSHOT_ID=$(pinax version history --vault ./my-notes --json | jq -r '.data.snapshots[0].snapshot_id')
pinax version restore first-note.md --revision "$SNAPSHOT_ID" --plan --vault ./my-notes --json

# 应用 restore plan 写回本地 Markdown
# <restore_id> 来自上一步 version restore --plan 的输出
pinax version restore apply --vault ./my-notes --plan <restore_id> --yes --json
```

应用成功即证明：Pinax 的每一次 write 都有 snapshot 保护，可审计、可回滚。

## 下一步

- 浏览 [命令手册](./commands/README.md) 了解每个 workflow 的推荐入口。
- 阅读 [本地开发](./operations/local-development.md) 了解 `task check`、`task release:local` 等开发任务。
- 高级能力（Cloud Sync、MCP server、Templates、Project Boards）不在本快速开始范围，见对应命令文档。
