## Context

Pinax 的笔记真源是 Markdown frontmatter + body，本地索引只是投影。当前命令面已经支持单笔 tag add/remove/set、note move/rename/archive/delete 和 property 查询展示，但缺少直接管理普通 frontmatter 属性的命令，也缺少对 tag taxonomy 的批量 rename/delete。

本变更保持 CLI-only 和短生命周期进程边界：Cobra 只负责参数接线，metadata 写入和索引刷新都放在 application service。

## Goals / Non-Goals

**Goals:**

- 支持通过 CLI 设置和移除单条笔记的普通属性。
- 支持 tag rename/delete 的 dry-run 和显式确认写入。
- 让自定义 frontmatter 属性进入 typed property projection，并可被 strict property 查询。
- 保持 JSON、agent、events 和默认人类输出由同一 projection 渲染。

**Non-Goals:**

- 不实现批量文件夹移动、目录重命名或任意文件重组。
- 不允许 property 命令覆盖 `schema_version`、`note_id`、`tags`、`title`、`created_at`、`updated_at` 等保留字段。
- 不引入 YAML AST 重写器、TUI、dashboard 页面、后台 daemon 或 provider 远端写入。

## Decisions

1. 属性写入复用现有 frontmatter patch 路径，并扩展为支持字段删除。

   这样可以保留未知 frontmatter 字段和正文内容，且不在命令层手写 Markdown。属性值限制为单行安全 scalar，避免把复杂 YAML 结构作为隐式 schema 入口。

2. `domain.Note` 增加内部 `Frontmatter` map，并用 `json:"-"` 避免扩大机器输出表面。

   索引需要看到普通 frontmatter key；CLI/JSON 输出仍应由明确的 projection 控制，不能把整块 metadata 原样暴露。

3. tag taxonomy 写入要求 `--yes`，`--dry-run` 只返回匹配和变更计划。

   批量改 tag 会触达多条 Markdown 文件；显式确认可以降低误操作风险，同时保持脚本可控。

4. 批量 tag 变更逐条更新 note frontmatter 和 record metadata event，最后刷新一次索引。

   这比每条笔记单独重建索引更便宜，也让输出 facts 能同时报告 `matched`、`changed`、`record_events` 和 `index_updated`。

5. 文件夹/文件管理先不扩展到批量重组。

   现有 `note move`、`note attach` 和 asset 管理覆盖单笔维护。批量文件夹/文件操作需要 snapshot、冲突计划、回滚路径和 dry-run/apply 分离，应作为独立变更设计。

## Risks / Trade-offs

- frontmatter 解析仍是现有轻量实现，不支持复杂 YAML 嵌套属性的精确 round-trip；本次只承诺单行 scalar 属性。
- tag rename/delete 会修改多条 note 文件；通过 `--dry-run`/`--yes`、record 事件和索引刷新事实降低风险。
- body 中 inline property 与 frontmatter property 若同名，查询层仍按现有 property 合并规则处理；本变更重点是让 frontmatter 自定义 key 不再丢失。

## Migration Plan

无需迁移。已有 Markdown frontmatter 在下一次索引刷新时会进入 property projection；新命令写入后会刷新本地索引。
