## Context

Pinax 的稳定边界是本地优先 Markdown 笔记 CLI：Markdown 文件是真源，`.pinax/` 只保存 CLI/service 生成的配置、索引、事件和投影。现有 spec 已经包含 vault 初始化、metadata plan/apply、organize plan/apply、模板、搜索、SQLite/GORM 索引和输出合同，但用户还缺少一个能长期管理 vault 的视角：笔记库规模如何变化、哪些笔记质量差、哪些标签或目录失控、索引是否过期、哪些内容长期没有维护。

本 change 将 dashboard 和数据分析明确放在 note CLI 内部：它们服务于本地 vault 管理，不把 Pinax 改造成 agent 平台或云服务。

## Goals / Non-Goals

**Goals:**

- 提供 `pinax stats`，从本地 Markdown vault 和现有索引投影计算可脚本化统计数据。
- 提供 `pinax doctor`，输出可执行的健康问题清单、严重级别、稳定错误码和下一步命令。
- 提供 `pinax dashboard`，启动只读、本机绑定的 Web dashboard，展示 stats、doctor、recent activity 和 index freshness。
- 复用 `internal/app` service、`internal/output` projection、`internal/redaction` 和 vault path boundary，避免命令层直接拼装业务逻辑。
- 默认不写 vault；如果后续引入 cache/receipt，必须由 CLI/service 写入 `.pinax/`，并受 schema version 和 redaction 约束。

**Non-Goals:**

- 不实现 LLM 自动总结、自动分类、自动修复或 token 成本追踪。
- 不接入 firecrawl、agent-browser、Lark、Notion、Pinax Cloud 或其它 provider。
- 不实现长期后台 daemon；`pinax dashboard` 只在用户显式启动的进程中运行。
- 不提供远端写入、多人协作或云端 dashboard。

## Decisions

### 1. 统计和健康检查作为 application service，而不是 dashboard 专属逻辑

`pinax stats`、`pinax doctor` 和 dashboard 数据接口都调用同一组 service：`VaultAnalyticsService` 和 `VaultHealthService`。这样 CLI human 输出、`--json`、`--agent` 和 dashboard UI 使用相同 projection，避免三套指标定义漂移。

替代方案是让 dashboard server 自己扫描 vault。该方案短期快，但会绕过输出合同、脱敏规则和路径边界，也会让 CLI 与 dashboard 指标不一致，因此不采用。

### 2. MVP 默认计算型只读，不先持久化统计快照

MVP 每次运行从 Markdown 文件、frontmatter、文件 stat、`.pinax/events.jsonl` 和 `.pinax/index.sqlite` 读取事实并计算结果。只有当大型 vault 性能成为真实问题时，后续 change 再引入 `.pinax/analytics-cache.json` 或 SQLite analytics projection。

替代方案是先落地 cache。该方案会增加 schema、失效策略和写入测试负担，不适合当前验证产品差异化。

### 3. Dashboard 使用本机只读 HTTP server

`pinax dashboard --vault <vault>` 启动 HTTP server，默认绑定 `127.0.0.1` 随机端口或显式 `--port`。server 只暴露读取 stats、doctor、recent activity 和静态资源的路由，不提供 note 修改 API。

替代方案是生成静态 HTML 文件。静态文件更简单，但后续交互筛选、刷新和大 vault 分页会受限；本机 HTTP server 更适合作为 CLI 启动的工具界面。

### 4. 健康问题用稳定 issue code 和 next action 表达

`doctor` 输出不只展示文字，还包含稳定 `issue_code`、`severity`、`note_id/path`、`evidence` 和 `next_actions`。例如 `missing_title`、`missing_tags`、`missing_pinax_metadata`、`duplicate_title`、`stale_note`、`empty_note`、`orphan_note`、`index_stale`、`path_escape_rejected`。

这样 agent 或脚本可以消费 `--agent`/`--json`，用户仍能看到中文摘要。复杂边界判断和 issue 分类实现需要中文注释说明判定意图。

### 5. Dashboard 不读取 secrets，不展示 raw provider payload

虽然本 change 不接 provider，但 dashboard 可能读取 `.pinax/` 资产和 events。实现必须通过 redaction 后的 projection 输出，不展示 provider token、webhook URL、Authorization header、cookies、raw payload 或未脱敏 trace。

## Risks / Trade-offs

- 大型 vault 扫描较慢 -> MVP 在输出中暴露 scan duration、note count 和 index freshness；后续基于实测再加 cache 或 GORM analytics projection。
- 健康分数容易变成主观玩具指标 -> MVP 优先输出具体 issue 和 next action；总体 score 只能作为派生摘要，不作为唯一结果。
- Dashboard server 增加 UI 和 HTTP 测试成本 -> MVP 保持只读、少路由、无外部网络依赖，并用 handler 单元测试加 process e2e 覆盖。
- 与已有 `validate` 命令边界重叠 -> `validate` 负责 vault 结构和 schema 正确性；`doctor` 负责笔记管理质量、可维护性和可执行建议。
- 统计读取 `.pinax/index.sqlite` 可能遇到索引缺失或过期 -> stats/doctor 必须降级为 Markdown scan，并明确报告 index status，而不是失败退出。

## Migration Plan

1. 新增 service 和 projection，不改变已有命令行为。
2. 新增 `stats` 和 `doctor` 命令，先覆盖 JSON/agent/human 输出和 testscript fixture。
3. 新增 dashboard server，只读调用同一 projection。
4. 更新 README 或本子项目 docs 的命令说明，但执行状态只记录在本 change 的 `tasks.md`。
5. 若出现回滚需求，删除新增命令入口和 service 即可；不会迁移或修改用户 vault 正文。

## Open Questions

- MVP 是否需要展示总体 health score，还是只展示 issue count 和 severity 分布？建议先只提供 severity 分布，避免过早引入主观评分。
- Dashboard 前端是否使用 Go embed 静态资源，还是先返回服务端渲染 HTML？建议 MVP 使用 Go embed + 简单 HTML/CSS/JS，避免引入 Node 构建链。
- `stale_note` 默认阈值采用 90 天还是可配置？建议默认 90 天，并提供 `--stale-after` 覆盖。
