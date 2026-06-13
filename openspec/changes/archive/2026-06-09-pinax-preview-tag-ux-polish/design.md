## Context

Pinax 的 CLI 输出遵循同一 projection 多 renderer 的合同。当前 `note.show`、`template.render` 等命令在默认人类模式会渲染 Markdown 正文，但 `note.preview` 和 `template.preview` 没有进入正文渲染分支，用户只能看到摘要指标。维度列表已有计数表，但 tag 分布需要用户自己比较数字。

本变更只优化 CLI 默认人类输出，不引入 TUI、pager、dashboard 页面或新存储结构。

## Goals / Non-Goals

**Goals:**

- 让 `template preview` 和 `note preview` 在默认人类输出中直接展示预览正文。
- 让预览 projection 暴露标签事实，便于人类摘要和机器模式共享同一上下文。
- 让 tag 维度列表提供计数、占比和热度条，提升扫描效率。
- 保持 `--json`、`--agent` 和 `--events` 不包含人类专用表格可视化或 ANSI 样式。

**Non-Goals:**

- 不新增图形界面、交互式 tag 云或长期 daemon。
- 不改变 tag 存储格式、索引 schema 或 GORM repository 行为。
- 不改变现有命令名、flag、JSON envelope 必填字段或 agent key 命名。

## Decisions

1. 在 `internal/output` 扩展现有 summary renderer，而不是在 Cobra 命令层拼输出。

   这样 preview、JSON、agent 和 events 仍来自同一个 projection，符合 Pinax 输出合同。替代方案是在命令层针对 `note preview`/`template preview` 手写正文输出，但会分裂机器输出和人类输出来源。

2. 在 `internal/app` 给 preview projection 补充 `tags` fact，而不是从 renderer 解析 note/template body。

   renderer 只负责展示 projection，不应解析 Markdown 或重新计算业务上下文。标签事实由 application service 根据请求或解析后的 note metadata 提供。

3. 维度可视化使用纯文本百分比和 `#` 热度条。

   纯文本在非终端、`NO_COLOR=1`、CI 和 agent 终端中稳定可读；不依赖颜色表达意义。`占比` 表示所有维度计数中的比例，`热度` 是按当前结果最大计数归一化的扫描条。

## Risks / Trade-offs

- 默认输出更长 -> 仅对 preview 和维度列表增加内容，机器输出不变；长正文仍走既有 Markdown 渲染路径。
- tag 计数总和可能大于 note 数 -> `占比` 按 tag assignment 总数计算，并保留原始 `数量`，避免误解。
- 视觉条可能被脚本误用 -> 只出现在 summary mode，测试覆盖 agent/JSON 不包含人类可视化文本。

## Migration Plan

无需迁移。该变更只影响默认人类 stdout 渲染；依赖 `--json` 或 `--agent` 的脚本保持兼容。
