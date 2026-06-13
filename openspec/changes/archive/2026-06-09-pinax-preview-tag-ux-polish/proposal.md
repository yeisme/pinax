## Why

`template preview` 和 `note preview` 是用户在写入前检查内容的入口，但默认人类输出只展示摘要指标，正文和标签上下文不够直观。`note tags` 已经能列出计数，但缺少快速判断标签热度和占比的可视化线索。

这次变更优化 CLI-only 的默认阅读体验，让用户不用切到 JSON 或打开文件也能快速确认预览内容和标签分布，同时保持 `--json`、`--agent`、`--events` 等机器输出合同稳定。

## What Changes

- 默认 `template preview` 输出 SHALL 在摘要后展示渲染后的 Markdown 正文，并在提供 `--tags` 时展示标签事实。
- 默认 `note preview` 输出 SHALL 在摘要后展示预览正文，并展示笔记标签事实。
- 默认维度列表输出，特别是 `pinax note tags` / `pinax tag list`，SHALL 在计数外展示标签占比和热度条。
- 机器输出 SHALL 继续由同一 projection 渲染，且不得包含人类表格可视化文本或 ANSI 样式。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `note-command-ux`: 补充 note preview 和 tag 列表的默认人类输出体验要求。
- `configurable-output-rendering`: 补充 template preview 和维度列表在默认人类输出中的 Markdown/表格渲染要求。

## Impact

- 影响代码：`internal/app/service.go`、`internal/output/render.go`、`cmd/pinax/main_test.go`、`internal/output/render_test.go`。
- 影响命令：`pinax template preview`、`pinax note preview`、`pinax note tags`、兼容路径 `pinax tag list`，以及同一 renderer 覆盖的 folder/kind/group 维度列表。
- 不新增依赖、不新增后台服务、不改变 JSON/agent/events envelope 的必填字段。
