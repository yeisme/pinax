## 背景

Pinax 已经支持 `pinax note links`、`pinax note backlinks` 和 Obsidian 风格 `[[...]]` 双链，但当前服务层增强解析与索引层投影解析不完全一致。结果是：CLI 扫描路径可以识别 alias、heading、歧义候选，但本地 SQLite/GORM projection、search link-target、增量刷新和 query source 仍可能使用简化规则。

本变更把 wiki link 解析、目标解析、冲突处理和索引写入统一到一个内部能力中，避免用户在出现断链、同名冲突、alias 冲突或附件 embed 时才发现行为不一致。

## 目标

- 统一 `[[Title]]`、`[[Title|Alias]]`、`[[Title#Heading]]`、`[[Title#Heading|Alias]]` 和 Markdown 相对笔记链接的解析行为。
- 让 app 查询、index rebuild、增量 projection、search/query 复用同一套 link graph 规则。
- 对同名标题、frontmatter alias、文件 stem fallback 的冲突返回 `ambiguous` 和候选，不自动猜测。
- 对缺失目标返回 `broken`，并继续通过 `repair plan` / `organize plan` 的 `manual_review` 操作处理。
- 保持 CLI JSON envelope 和既有字段向后兼容，只补齐已有可选字段的稳定语义。

## 非目标

- 不实现完整 Obsidian block reference、heading 存在性校验或 UI 图谱。
- 不自动重写用户 Markdown 正文。
- 不新增外部依赖、云端图数据库或后台 daemon。
- 不删除、改名或重定义既有 CLI 输出字段。

## 兼容性

- CLI 输出为 additive/bugfix：保留 `source_path`、`target`、`kind`、`broken` 等既有字段，继续输出 `status`、`target_raw`、`target_alias`、`target_heading`、`line`、`evidence`、`candidates` 等可选字段。
- 数据库 projection 使用现有 `LinkRecord` 字段，不做 destructive migration。
- 如果旧索引缺字段或 stale，用户可运行 `pinax index rebuild --vault ./my-notes --json` 重建 projection。
