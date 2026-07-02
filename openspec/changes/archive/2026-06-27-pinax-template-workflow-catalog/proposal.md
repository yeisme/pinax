# pinax-template-workflow-catalog 提案

## Why

Pinax 已经有 `template list`、`template recommend`、`template inspect`、`template preview`、`template render`、`note add --template`、index page、journal、project workspace 和 proof loop，但模板仍容易被理解成“更多 Markdown 文件”。继续堆模板会提高选择成本，也无法证明用户是否真的从 intent 走到了可检索、可复盘、可继续推进的知识工作流。

更好的产品形态是把模板定义为 Pinax 的工作流入口层：用户先表达 intent，Pinax 通过本地 metadata 推荐合适模板或模板包，允许只读预览，写入统一 index intake 或显式路径，记录使用证据，再把下一步交给 index/search/project workspace/proof loop。这样模板不只是正文骨架，而是“可审查的 workflow starter”。

## What Changes

- 将 template catalog 正式定义为本地、metadata-driven、intent-driven 的 workflow starter catalog。
- 扩展现有模板 metadata，新增可选字段：`scenario_id`、`template_kind`、`intent`、`variable_schema`、`output_policy`、`after_create_actions`、`maturity`、`proof_gate`、`pack`、`lifecycle`、`metrics`。
- 让 `pinax template recommend --intent ...` 从“按关键词挑模板”升级为“返回 workflow recommendation”，包含推荐理由、适用场景、预览命令、创建命令、证据路径、proof handoff 和最多三个替代项。
- 让 `pinax template inspect <name>` 暴露 workflow starter metadata，继续保持英文稳定字段、中文 human summary 和 read-only 行为。
- 让 `pinax template preview <name>` 明确返回 read-only preview projection，并在输出中说明写入影响、目标路径策略、需要的变量、proof gate 和推荐 next command。
- 让 `pinax note add <title> --template <name>` 或 journal/index/project workspace 消费模板后产生 template use evidence：JSON/agent 输出包含 `template_use_id`、template/pack/scenario、effective path、next actions；需要持久 receipt 时由 app service 写入 CLI-authored receipt。
- 引入本地 template pack 概念，但只支持内置包和 vault-local 包；不做远程 marketplace、评分、云同步或 GUI 模板中心。
- 明确 project workspace 关系：template catalog 只负责启动结构、路径策略和 next actions；project workspace 消费模板输出，不反向拥有模板模型。

## Out of Scope

- 不实现远程模板 marketplace、远程模板同步、评分系统或团队模板商店。
- 不允许 AI 自动生成模板后直接发布为 executable template；AI 生成的草稿必须先进入 design/draft lifecycle，再通过 validate/preview/publish 门禁。
- 不新增 GUI-first 模板中心；CLI/API/MCP 可消费合同先稳定。
- 不让模板执行脚本、读取环境变量、访问网络、调用 provider 或绕过 Pinax app service 写 vault。
- 不把模板使用证据写成 agent 手工 Markdown metadata；`.pinax/**`、receipts、events、catalog registry 仍由 CLI/application service 写入。

## Impact

- OpenSpec owner：`cli/pinax`。
- 主要影响：`internal/app/builtin_templates.go`、template metadata/parser、template service、`internal/cli/template_cmd.go`、`internal/output`、`cmd/pinax/template_command_test.go`、template/note/project workspace e2e、`docs/commands/template.md` 和 `docs/operations/local-development.md`。
- 稳定合同面：CLI command behavior、JSON envelope `facts/data/actions`、`--agent` key、template metadata schema、receipt/event schema、MCP/API capability discovery。所有变更必须 additive；不得删除、重命名、重定义既有字段或命令。
- 数据库影响：无默认 DB schema requirement；若后续增加 catalog index projection，只能通过 GORM expand-first 新增 nullable 表/字段/index。

## Compatibility

- `pinax template recommend --intent ...`、`template inspect`、`template preview`、`note add --template` 的既有输出字段必须保留。
- 新字段只作为 optional fields/keys 添加；旧消费者忽略新字段后仍能工作。
- `schema_version` 如需变化只能 minor/additive；不能把既有 `template_kind`、`source`、`path_pattern`、`actions` 改义。
- 旧内置模板、vault-local templates 和 legacy simple templates 继续可用；新增 lifecycle 只能把不可执行 design draft 排除在 create/render 推荐外，不能删除用户模板。

## Validation

```bash
openspec validate pinax-template-workflow-catalog --strict
openspec validate --all --strict
```

后续实现触及 Go 代码后运行：

```bash
task check
```
