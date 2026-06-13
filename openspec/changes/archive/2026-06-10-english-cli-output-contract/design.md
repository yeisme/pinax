## Context

Pinax 是 Go/Cobra CLI，命令层负责参数、help 和渲染接线；业务 projection、facts、actions、error code 和 data 由 `internal/app` 等 service 产生；默认 human/`--json`/`--agent`/`--events`/`--explain` 渲染集中在 `internal/output` 及命令 helper。当前仓库级 CLI 输出合同要求 Pinax 这类 agent 工具默认使用英文用户可见输出，但 Pinax 子项目历史上按照中文默认输出实现，导致 help、summary、错误、下一步、docs 和测试断言混用中文。

本变更只设计迁移任务，不在本步骤直接改实现。实现阶段必须在 `cli/pinax` owner 内完成，遵守现有 projection/renderer 边界和 OpenSpec 任务清单。

## Goals / Non-Goals

**Goals:**

- 将默认人类输出、help、usage、examples、错误说明、stderr diagnostics、operator logs、docs 命令说明和 `--explain` 报告统一为英文。
- 保留机器输出合同稳定：`--json`、`--agent`、`--events` 的字段、key、command id、event type 和 schema version 不因为语言迁移破坏脚本。
- 保留同源 projection：命令只产生结构化 projection，由 renderer 选择英文 human output 或机器模式。
- 用测试保护迁移结果：英文 CLI chrome、机器 stdout 纯净、stdout/stderr 分离、redaction、intentional non-English data allowlist。
- 更新帮助、文档、示例和 golden，使用户看到的 Pinax 操作面是英文且可直接运行。

**Non-Goals:**

- 不翻译用户笔记正文、模板正文、引用内容、provider 原始返回文本、第三方 payload 字段和值、fixture 中刻意作为领域数据的非英文内容。
- 不重命名命令、flags、JSON 字段、agent keys、event type、schema key、provider id 或 model id。
- 不新增 `--lang`、i18n framework、本地化资源包或双语切换；本变更是英文 clean cutover。
- 不改 Pinax OpenSpec 产物语言要求；OpenSpec 仍按 `openspec/config.yaml` 使用简体中文。
- 不把 raw prompt、hidden prompt、chain-of-thought、provider payload、token、cookie、Authorization header 或私有工具参数写入任何输出或测试 fixture。

## Decisions

### D1：英文 clean cutover，不做运行时多语言开关

选择一次性把 Pinax CLI chrome 切到英文，而不是加 `--lang` 或配置项。

理由：当前需求是“改为英文输出”，仓库级输出合同也要求 Pinax 默认英文。多语言开关会扩大范围，要求双份 golden、双份 help、双份错误维护，并让 agent/脚本更难判断默认行为。

替代方案：保留中文默认并加英文模式。拒绝，因为它无法解决默认输出合同冲突，还会让旧中文 chrome 继续出现在 smoke 和 docs 中。

### D2：机器输出只验证 parseability 和稳定字段，不依赖 human summary

`--json` 必须是一份 JSON envelope；`--agent` 必须是稳定 ASCII key=value；`--events` 必须是 NDJSON；`--explain` 是英文审查摘要。测试不得通过解析默认 summary 来驱动脚本。

理由：语言迁移会改变默认 prose；脚本依赖默认 prose 本身就是合同错误。机器模式应继续承担自动化入口。

### D3：区分 CLI chrome 和 domain data

扫描非英文字符时必须分类：

- 必须翻译：section label、table heading、fact label、summary prose、error message、hint、next action label、help/usage/example prose、Taskfile task description、docs command explanation。
- 必须保留：用户 note/template body、quoted source、provider payload、third-party field/value、fixture story text、历史归档、OpenSpec 中文产物。

理由：盲目替换中文会破坏用户数据和测试语义；只替换 CLI chrome 才是正确边界。

### D4：优先改 projection/renderer 边界，避免命令层散落英文拼接

如果多个命令共享“重点/指标/下一步/错误”等中文标签，应在 `internal/output` 统一换成英文；如果 service projection 的 `Summary`、`Action.Command`、`CommandError.Message/Hint` 是用户可见 CLI chrome，则在对应 service 或 helper 中改成英文。

理由：Pinax 已有多模式输出合同。把翻译散落在 Cobra command 会让 `--json`、`--agent`、human output 漂移，也容易漏掉 error path。

### D5：测试先行，先让代表性输出用例失败

实现阶段先添加失败测试，再改输出。测试覆盖：root help、命令 help、成功 summary、失败 error、`--json`、`--agent`、`--events`、`--explain`、redaction 和 intentional non-English allowlist。

理由：当前中文输出分布很广，没有测试先行会导致局部翻译看似完成但边缘命令继续输出中文。

## Risks / Trade-offs

- Risk：测试过拟合英文长句，导致小文案调整频繁破坏测试。Mitigation：断言 section label、必要事实、可执行 action、无中文 CLI chrome、parseability 和 redaction，不断言大段 prose。
- Risk：扫描中文字符返回大量用户内容和历史归档。Mitigation：先分类，再为 intentional data 建 allowlist；不要用全局替换。
- Risk：机器输出被顺手改字段。Mitigation：机器模式测试解析 envelope/key/event，并保留稳定字段；字段删除或重命名必须提升 contract major version。
- Risk：long-running command 把英文 progress 或日志混入 machine stdout。Mitigation：stdout 只给所选输出模式；diagnostics/progress 写 stderr 或结构化 event。
- Risk：帮助文档和实际命令不一致。Mitigation：所有 docs/examples 使用真实 `pinax ...` 命令，不写 agent wrapper 或不存在命令。

## Verification Strategy

- Focused RED/GREEN：`go test ./cmd/pinax ./internal/output ./internal/cli ./internal/api -run 'Output|English|Agent|JSON|Help|Serve|API' -count=1`。
- 非英文扫描：使用项目允许的内容搜索工具或测试 helper 检查 CLI chrome 中的 Han 字符；剩余匹配必须被分类为 domain data、OpenSpec 中文产物、历史归档或第三方 payload。
- 机器合同：解析 JSON、agent key=value、events NDJSON；检查 stdout 无 ANSI、无日志、无 localized prose。
- 全项目门禁：`task check`，包含 fmt、lint、test、build、`openspec validate --all`。
- 本变更验证：`openspec validate english-cli-output-contract --strict`。
