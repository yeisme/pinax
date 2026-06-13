## Why

Pinax 当前默认人类输出、帮助、错误提示和部分文档仍混用中文 CLI chrome；这与仓库级 AI-native CLI 输出合同中“Pinax 等 agent 工具默认使用英文用户可见输出”的要求冲突。现在需要先把迁移范围、兼容边界和任务拆分固化到 Pinax 子项目 OpenSpec，避免后续实现时只翻译局部文案而漏掉帮助、错误、机器输出和测试合同。

## What Changes

- 将 Pinax 默认人类输出改为英文：默认 summary、表格标题、事实标签、下一步、错误说明、help、usage、examples、stderr diagnostics、operator logs、`--explain` 报告和项目内命令文档。
- 保留机器协议稳定：命令名、flag 名、JSON envelope 字段、`--agent` key、event type、schema key、enum、provider id、第三方 payload 字段不因本迁移改名。
- 明确非英文数据不被盲目翻译：用户笔记正文、引用材料、provider 返回内容、fixture 故事文本、第三方字段和值、历史归档内容可以保持原语言。
- 要求所有输出模式继续来自同一个 projection：default human summary、`--json`、`--agent`、`--events`、`--explain` 不得各自手写分叉语义。
- 增加 contract tests：覆盖英文 CLI chrome、stdout/stderr 分离、机器输出 parseability、无 ANSI/日志混入、脱敏和 intentional non-English allowlist。
- 更新 README、docs、Taskfile 描述、帮助示例和 golden/snapshot，使示例命令可直接运行且用户可见说明为英文。
- **BREAKING**：默认人类输出语言从中文切换为英文；脚本和 agent 不应解析默认人类输出，必须使用 `--json`、`--agent` 或 `--events`。

## Capabilities

### New Capabilities

- `english-cli-output-contract`：定义 Pinax 英文默认输出、机器输出稳定性、脱敏和测试合同。

### Modified Capabilities

- `cli-tree-ux`：help、usage、examples、argument errors 和 workflow group heading 从中文默认改为英文默认。
- `configurable-output-rendering`：summary renderer、Markdown metadata summary、preview output、dimension labels 和机器模式隔离要求改为英文 CLI chrome。

## Impact

- Owner：`cli/pinax`。
- 主要代码范围：`internal/output`、`internal/cli`、`internal/app` projection/error/action 文案、`cmd/pinax` contract tests、`internal/api` serve diagnostics、provider/briefing/delivery 输出边界。
- 主要文档范围：`README.md`、`docs/**`、`Taskfile.yml` 任务描述、CLI help examples、测试 golden/snapshot。
- 主要测试范围：`cmd/pinax/main_test.go`、`internal/output/render_test.go`、相关 e2e/testscript/golden 文件和红线扫描测试。
- 不改变 vault 中用户内容语言、不改变 notes/templates 正文、不改变 OpenSpec 产物语言要求（本目录 OpenSpec 仍按配置使用中文撰写）。
- 不引入新的输出框架；优先扩展现有 projection/renderer 和 shared redaction 边界。
