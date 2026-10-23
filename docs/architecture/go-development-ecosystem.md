# Go 开发生态

Pinax 以本地优先、可分发、可测试的 Go CLI 为核心。人类和 Agent 共用开发入口，不为每次任务重新拼装命令。

<a id="entry-commands"></a>
## 开发入口

Taskfile 负责聚合，具体工作仍由 Go 工具链完成：

```bash
task build
task test
task check
task openspec
task clean
```

没有 Task 时可以直接执行对应命令：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

<a id="go-module-boundaries"></a>
## 模块边界

| 路径 | 职责 |
| --- | --- |
| `cmd/pinax/`、`internal/cli/` | Cobra 入口、命令工厂、参数与依赖装配 |
| `internal/app/` | 应用服务与用例编排 |
| `internal/domain/` | 稳定领域模型、状态机、命令投影 |
| `internal/config/` | Viper 默认值、环境与配置校验 |
| `internal/output/` | summary、agent、JSON、events、explain 渲染 |
| `internal/redaction/` | token、webhook、payload 与 trace 脱敏 |
| `internal/runtime/` | 时钟、文件系统、进程、取消 |
| `internal/vault/` | Markdown vault repository |
| `internal/index/` | SQLite/GORM 可重建索引 |
| `internal/git/` | Git adapter 与 snapshot plan |
| `internal/provider/` | CLI-backed Provider adapter |
| `internal/sync/` | diff/pull/push/conflict 状态机 |
| `internal/briefing/` | 调研、证据、评分与审阅队列 |
| `internal/mcpserver/` | stdio MCP 表面 |
| `tests/e2e/`、`testdata/script/` | testscript、fixture 与 golden 输出 |

<a id="dependency-defaults"></a>
## 依赖选择

CLI 使用 Cobra/pflag；Viper 只在 config 层读取配置；关系持久化通过 GORM，本地索引默认 SQLite。Markdown/frontmatter 库的选择理由和 fixture 在实现 change 中记录。命令 e2e 复用 `github.com/rogpeppe/go-internal/testscript`，外部系统使用 fake executable/process adapter，不依赖真实 token、公网或用户 vault。

<a id="output-contract"></a>
## 输出合同

所有入口从同一 command projection 渲染：默认英文人类摘要，`--agent` 为 key=value，`--json` 为单 envelope，`--events` 为 NDJSON，`--explain` 为英文评审摘要。人类开发文档使用中文。

机器输出 stdout 只放对应格式；诊断、进度与日志走 stderr，并遵守脱敏要求。此处修正旧文档“默认中文 CLI 输出”的过时说明，以当前 AGENTS.md 为准。

<a id="layered-testing"></a>
## 测试分层

| 层次 | 范围 | 工具 |
| --- | --- | --- |
| unit | 领域规则、投影、脱敏、slug、评分 | Go testing 表驱动 |
| integration | service + repository + 临时 vault/SQLite | testing、testing/fstest、临时目录 |
| component | CLI + fake provider + 临时 Git | testscript |
| e2e | 完整用户流程 | testscript |
| performance | 索引、检索、评分、进程开销 | Go benchmark；按需新增具体入口 |

<a id="task-slice-order"></a>
## 能力建设顺序

开发基础与输出合同 → 本地 vault → 索引检索 → 版本证据 → CLI-backed Provider → 同步状态机 → Agent 输出 → 只读 MCP → 日报与调研。每个切片复用同一服务、状态和证据，不复制产品栈；这个顺序不等于全部已完成。

<a id="quality-gates"></a>
## 按改动验证

普通实现运行相关包测试，默认使用 Go 缓存；文档只检查相关链接和内容。`task ci:quick` 是针对性入口，`task check` 用于稳定改动后的完整本地检查。完整检查不在每次提交或每轮修复后重复执行。race 不属于默认门禁。
