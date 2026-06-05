# Go 开发生态设计

Pinax 以 Go CLI 为主线，目标是本地优先、可分发、可测试、可由 agent 稳定驱动。开发生态要让人和 agent 使用同一套入口，避免每个任务临时拼命令。

## 入口命令

Pinax 采用 Taskfile 作为开发任务聚合层，参考 Cohors 的 `task build` 使用体验，但任务实现落到 Go 工具链。

```bash
task build
task test
task check
task openspec
task clean
```

没有安装 `task` 时，可以直接运行等价命令：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

## Go 模块边界

默认目录结构：

```text
cmd/pinax/              Cobra 入口和命令接线
internal/cli/           后续可迁移的 Cobra command factory 和 dependency wiring
internal/app/           application service / use case 编排
internal/domain/        稳定领域模型、状态机和 command projection
internal/config/        Viper defaults、env、project config、validate
internal/output/        summary、--agent、--json、--events、--explain renderer
internal/redaction/     token、webhook、raw payload、trace 脱敏
internal/runtime/       clock、filesystem、process runner、context/cancellation
internal/vault/         Markdown vault repository
internal/index/         SQLite/GORM 索引投影 repository
internal/git/           Git adapter 和 snapshot plan
internal/provider/      CLI-backed Provider interface
internal/sync/          diff/pull/push/conflict state machine
internal/briefing/      daily-hot-notes workflow、evidence、scoring、review queue
internal/mcpserver/     pinax mcp serve stdio surface
tests/e2e/              testscript command e2e
testdata/script/        fixture 文件树和 golden stdout/stderr
```

## 依赖默认值

- CLI：Cobra / pflag。
- 配置：Viper，只在 `internal/config` 引入，命令层不直接读配置文件。
- 持久化：GORM，SQLite 作为本地索引和投影默认存储。
- Markdown/frontmatter：优先选稳定库，进入实现 change 时记录选择理由和 fixture。
- 命令 e2e：`github.com/rogpeppe/go-internal/testscript`。
- 外部系统：优先 fake executable 和 process adapter，不在测试中依赖真实 token、真实公网或用户 vault。

## 输出合同

每个用户入口都必须从同一个 command projection 渲染：

- 默认中文摘要。
- `--agent` 低 token `key=value`。
- `--json` 单一 JSON envelope。
- `--events` NDJSON。
- `--explain` 决策解释。

机器输出 stdout 只能包含机器格式；诊断、progress、provider stderr 和日志写 stderr。

## 分层测试

| 层级 | 范围 | 默认工具 |
| --- | --- | --- |
| unit | domain rule、projection、redaction、slug、score | Go `testing` table-driven tests |
| integration | app service + repository + temp vault / SQLite | Go `testing`、`testing/fstest`、临时目录 |
| component | CLI command + fake provider + temp Git repo | `testscript` |
| e2e | `pinax` 二进制完整用户流程 | `testscript` |
| performance | 索引、搜索、briefing scoring、provider process overhead | Go benchmark、`task perf-*` 后续补充 |

## 任务切片顺序

1. Go dev ecosystem：Taskfile、CI 基线、testscript harness、output projection skeleton。
2. Local Vault Workbench：`init`、`doctor`、`note new/list/show`、frontmatter 和 validate。
3. Index and Search：GORM repository、tag/link/backlink/search。
4. Git Safety：status、snapshot plan、changed paths、rollback hint。
5. CLI-backed Provider：`ntn`、`lark-cli` capability probe 和 fake executable。
6. Sync Engine：diff/pull/push/conflict queue、dry-run/yes gate。
7. Agent Surface：`--agent`、`--json`、`--events`、`--explain` contract tests。
8. MCP Read/Plan：stdio MCP resources/tools，默认只读和 dry-run。
9. Daily Hot Notes：Hermes/internet-access evidence ledger、scoring、review queue、Feishu delivery。

## 质量门禁

提交前至少运行：

```bash
task check
```

如果没有安装 `task`，运行：

```bash
gofmt -w cmd internal
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```
