# 贡献 Pinax

[English](./CONTRIBUTING.md)

Pinax 是本地优先的 Go CLI。Markdown vault 是真源；`.pinax/` 保存由 CLI 创建的投影、收据、索引和配置。贡献代码或文档时必须保护这个边界。

## 开始之前

1. 阅读 [中文 README](./README.zh-CN.md)、[docs/README.zh-CN.md](./docs/README.zh-CN.md) 和 [architecture boundaries](./docs/architecture/architecture-boundaries.md)。
2. 行为变更需要创建或更新 `openspec/changes/pinax-<slug>/` 下的 OpenSpec change。
3. 面向用户的示例必须是可直接运行的真实 `pinax` 命令。不要在文档中写本地 shell alias 或 agent-only wrapper。

## 开发环境

前置要求：

- Go 1.26.1 或更新版本。
- 可选：[Task](https://taskfile.dev/)，用于项目快捷命令。

常用命令：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

安装 Task 后可运行：

```bash
task check
```

`task check` 覆盖格式检查、lint、测试、构建和 OpenSpec validation。

## 代码边界

- CLI wiring 放在 `cmd/pinax` 和 `internal/cli`。
- 用例编排放在 `internal/app`。
- 稳定领域模型放在 `internal/domain`。
- 输出渲染放在 `internal/output`，并保持默认 human output、`--agent`、`--json`、`--events`、`--explain` 合同。
- 脱敏逻辑放在 `internal/redaction`；不要在 command handler 中散落 token 或 payload 过滤。
- SQLite/GORM index 是可重建投影，不是 Markdown 真源。

## 安全规则

- 不要把 secret、provider payload、原始 Authorization/Cookie header、webhook URL、明文 note body 写入日志、fixture、receipt、文档或测试输出。
- 写 Markdown、`.pinax/`、provider state、Git/version state 或 remote sync state 的命令必须有明确 approval gate，例如 `--yes`、`--dry-run` 或 snapshot requirement。
- Local REST/RPC 和 MCP surface 必须复用 application service，不能绕过 CLI/service write gate。
- Cloud Sync 只有在远端 revision durable commit 且本地 sync-state evidence 写入后，才能输出 `remote_write=true`。

## Pull Request Checklist

- [ ] 为行为变更添加或更新了聚焦测试。
- [ ] 行为变更同步更新了 OpenSpec specs/tasks。
- [ ] 用户可见命令、状态或工作流变化同步更新了 README 或 docs。
- [ ] 运行了 `task check` 或文档中的 fallback commands。
- [ ] 验证新输出不会泄露 secret，也不会把 diagnostics 混入 machine stdout。

## 许可证

当前还没有选择公开开源许可证。在项目 owner 添加 `LICENSE` 文件前，请不要假设代码已授予再分发或复用权利。
