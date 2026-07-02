# Pinax 子项目指令

本文件只适用于 `cli/pinax` 子项目。进入本目录后，按这里的边界执行实现、测试、构建和 Pinax 子项目文档维护；跨项目治理、skills 分配和仓库级索引回到根仓库处理。

## 工作语言

- 默认使用中文和用户沟通。
- 面向人的开发文档、计划、审查意见和运行说明优先使用中文；CLI help、CLI output、日志、错误提示、自动化示例和 `--explain` 报告保持英文或既有稳定术语。
- 代码标识符、命令名、协议字段、JSON/YAML key、第三方工具名和标准技术术语可以保留英文。

## 项目定位

Pinax 是 Go 编写的本地优先统一笔记 Agent CLI。它把用户知识资产保存在可迁移 Markdown vault 中，通过 SQLite/GORM 建立本地索引投影，通过 Git 管理版本和回滚，通过 CLI-backed Provider adapter 与 `ntn`、`lark-cli`、Hermes/internet-access 等外部能力协作。

Pinax 不是云笔记后端、新闻爬虫、飞书知识库或长期 daemon。外部平台是 provider 或 delivery surface，Pinax vault 才是笔记真源。

## 技术栈

- Go CLI，入口在 `cmd/pinax`。
- Pinax 是 CLI-only 项目，命令是短生命周期进程；不新增 `.air.toml` 或 Air 热加载入口，本地迭代使用 `task run ARGS="..."` 或 `go run ./cmd/pinax ...`。
- 命令框架使用 Cobra / pflag；后续配置可引入 Viper。
- 关系型持久化、索引投影和 repository 默认使用 GORM；业务层禁止硬编码 SQL 字符串。
- 命令级、process e2e、golden stdout/stderr、fixture 文件树和完整用户流程测试默认使用 `github.com/rogpeppe/go-internal/testscript`。

## 必用本域 Skills

代码修改、调试、测试或审查本子项目时优先触发：

- 任意代码实现、调试、测试、重构：`yeisme-coding-execution-driver`。
- CLI 输出、`--agent`、`--json`、`--events`、`--explain` 或脱敏合同：`ai-native-cli-output-contract`。
- Go/Cobra/Viper CLI 架构：`golang-cobra-viper-cli-architecture`。
- 后端、repository、provider adapter、事件和索引设计：`backend-system-workflow`。
- 行为变更或 bugfix：`test-driven-development`。
- 失败命令、provider 异常、Git 状态异常、索引或 sync 异常：`systematic-debugging`。
- 完成前确认：`verification-before-completion`。
- PR/代码审查、质量扫描：`review`、`health`。
- 性能专项：`performance-profiler`。

如果任务涉及网页研究证据采集路线，按需读取 `internet-access`；如果任务涉及 MCP Gateway 接入，回到根仓库或 `mcp/gateway` owner 使用对应 skill。

Pinax 运行操作不是代码实现时，先走 Pinax agent 路由：

- 用户说“写一篇 Pinax 笔记”“保存到 Pinax”“写入 vault”“收进 inbox”时，先用 `pinax-agent-router`，再路由到 `pinax-vault-operator`。
- 直接写入前先确认 vault：`pinax vault list --json`。
- 长正文用 `pinax note add "<title>" --stdin --json` 或 `pinax inbox capture "<title>" --stdin --json`，不要手写 `.pinax/**` 元数据或索引。
- 如果用户只是要普通文章、社媒文案或临时草稿，没有提 Pinax/vault/storage，则按普通内容生成处理，不默认写入 vault。

## 架构边界

- CLI 参数、补全、命令入口和用户输出放在 `cmd/pinax`。
- 用例编排放在 `internal/app`；命令层只做参数校验、调用 service、选择输出模式。
- 稳定领域模型放在 `internal/domain`。
- 输出 projection、默认 human summary、`--agent`、`--json`、`--events` 和 `--explain` 渲染放在 `internal/output`；这些 CLI/自动化输出面保持英文或既有稳定术语。
- 脱敏策略放在 `internal/redaction`，不得在命令层散落 token 处理。
- 外部 provider 必须隔离到 adapter 包；优先调用外部 CLI，例如 `ntn`、`lark-cli`，不要默认直接接 native API。
- Git 行为必须通过 adapter/service 聚合，不要在命令层拼复杂 Git porcelain 解析。
- `.pinax/config.yaml`、provider profile、mapping、sync-state、event JSONL、briefing receipt、delivery receipt 和 feedback 都是 CLI-authored structured assets，必须由 CLI/service 创建和修改。
- 用户或 agent 可以编辑 `notes/**/*.md` 正文；机器可读 metadata 必须通过 `pinax` 命令规范化或修复。

## 文档归属

- 产品、设计、运行、协议、实现、QA 和 release 文档放本子项目 `docs/`，作为 Pinax 文档真源。
- 子项目实现计划、任务状态、验证证据和 closeout 放本子项目 `openspec/`。
- 不要把 Pinax 的长期产品文档复制到根 `docs/**`；根目录只保留跨项目 handoff、治理规则和索引。

## 禁止事项

- 不把 Pinax 源码直接维护在根仓库普通目录；本项目必须作为独立 Git 仓库并由根仓库 submodule 引用。
- 不让 agent 手写 `.pinax/*.yaml`、`.pinax/*.json`、`.pinax/events/*.jsonl`、briefing receipt、delivery receipt 或 feedback metadata。
- 不在 handler、command、application service 或普通业务逻辑中硬编码 SQL；Go 持久化默认通过 GORM repository。
- 本地 provider token、webhook URL、cookies、Authorization header 等真实凭据只能进入用户级本地配置或用户级 secret store；不把这些值、外部 CLI 配置内容、raw payload 或未脱敏 trace 写入 stdout、stderr、事件、fixture、运行证据、项目资产或 Git。
- 不让 `--dry-run` 写 vault、投递飞书、更新反馈或执行远端写入。
- 不绕过 app service 让 MCP/tool/provider 直接写 vault 或调用远端写入。
- 不提交构建产物、coverage、本地 vault、provider 缓存、测试报告或运行 secrets。
- 不回滚已有未提交改动，除非用户明确要求。

## 测试和质量门禁

修改 Go 代码后至少运行：

```bash
task check
```

`task check` 覆盖 `task fmt-check`、`task lint`、`task test`、`task build`、`task kb:sidecar:protocol` 和 `openspec validate --all`；提交前或 CI 对齐时也可以运行 `task ci`。常用单项命令：

```bash
task fmt-check
task lint
task test
task build
```

没有安装 `task` 时运行：

```bash
golangci-lint fmt --diff
golangci-lint run
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

修改 CLI 输出、结构化输出或脱敏规则时，补充 contract tests，并验证 stdout/stderr 分离。

涉及 provider、Hermes、internet-access、Feishu 或 Git 的测试必须使用 fake executable、fake server、fixture vault、临时 Git 仓库和 testscript，不依赖真实公网、真实 token 或用户 vault。

## OpenSpec 开发入口

实现相关计划、任务状态、验证证据和 closeout 必须放在本子项目 `openspec/`：

```bash
openspec list
openspec validate --all
```

新建实现变更时使用 `pinax-<slug>` 命名：

```bash
openspec new change pinax-<slug>
```

OpenSpec workflow 以 skills 和 `openspec` CLI 为入口。根目录 `openspec/` 只负责设计 handoff，不记录 Pinax 代码实现进度。
