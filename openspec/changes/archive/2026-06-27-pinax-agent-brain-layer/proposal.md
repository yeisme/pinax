# Pinax Agent Brain Layer 提案

## Why

GBrain 的定位说明了一个清晰缺口：普通搜索只返回原始页面，而 AI Agent 需要一个能长期记忆、结构化关联、综合回答并给出引用的 brain layer。Pinax 已经具备 local-first Markdown vault、`memory`、`kb`、search、query/dataview、link/backlink graph、MCP、Local REST/RPC、proof loop、Cloud Sync 和 Web/Open Design 合同，但这些能力还没有被一个正式 OpenSpec 串成完整的 Agent Brain 产品与实现路线。

如果不先定义合同，后续实现容易走偏：把 answer synthesis 做成无引用聊天、让 MCP 暴露 raw note body、让 Web/Agent 直接读 `.pinax/**` 或 SQLite/LanceDB、把团队知识库做成 hosted 明文平台、让 night maintenance 静默改写用户笔记，或在没有 provider/cost 提示时触发 embedding/reranker/LLM 调用。

本变更把 Pinax 的长期记忆/知识大脑方向正式化：Pinax 是给 Claude Code、Codex、Cursor、OpenClaw、Hermes 和其他 MCP/Local API client 使用的 agent-safe brain layer。它必须优先复用本地 vault 和现有 projection，所有写入继续走 proof loop，所有答案综合必须可引用、可审计、可回滚。

## What Changes

- 定义 `pinax-agent-brain-layer` 能力合同，覆盖资料导入、结构化 memory、semantic KB、关系图谱、answer synthesis、MCP/HTTP 接入、maintenance/dream cycle、团队/权限边界、provider/cost 可见性和证据要求。
- 形成分阶段实现计划：P0 复用现有命令形成 Agent Brain MLP，P1 增加受控 answer synthesis 和 maintenance plan，P2 扩展 team/permission/HTTP/OAuth/rate-limit 方向。
- 明确当前 Pinax 侧 owner：`cli/pinax` 只负责 CLI/API/projection/MCP/proof loop 合同；未来全平台客户端或 hosted/team backend 必须由独立子项目或后续 OpenSpec 拥有。
- 要求所有新增稳定合同保持 additive：新增命令、JSON 字段、MCP tool、API route、registry key 和 schema 都必须向后兼容，不删除或重定义现有输出。

## Out of Scope

- 本变更不直接实现 Go 代码、不新增 CLI 命令、不新增数据库迁移、不启动 hosted 服务。
- 不把 Pinax 变成 Notion/GBrain 的托管替代品；团队多租户、OAuth provider、rate limit backend、组织权限服务只做合同和阶段计划。
- 不允许 Web、MCP 或 Agent 直接读写 `.pinax/**`、SQLite/GORM/LanceDB 文件、provider config、token 文件、sync state 或 receipt。
- 不承诺默认批量替换、静默夜间改写、全量图谱加载、浏览器直接扫描本地文件或无成本提示的 provider 调用。

## Impact

- 新增 OpenSpec delta spec：`pinax-agent-brain-layer`。
- 后续实现会触及 CLI 输出、MCP tool registry、Local REST/RPC capability registry、GORM-backed projections、KB provider、memory ledger、proof loop 和 integration evidence；这些都是稳定合同面，必须按 additive/evolutionary policy 执行。
- 现有 docs 中 GBrain 启发的产品叙事将被本 OpenSpec 接管为正式交付计划，避免只停留在设计文案。

## Validation

```bash
openspec validate pinax-agent-brain-layer --strict
openspec validate --all --strict
```

后续实现每个阶段还必须运行对应 focused tests；触及 Go 代码后运行：

```bash
task check
```
