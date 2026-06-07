## Why

`pinax stats`、`pinax doctor` 和 dashboard 已经让用户看见 vault 的健康问题，但还没有形成可审计的维护闭环。下一步需要把 doctor issue 转成安全的本地修复计划，让 Pinax 作为 note CLI 真正降低长期整理成本，而不是停留在报告层。

## What Changes

- 新增 `pinax repair plan`，把 doctor issue 转换为可审查的本地维护操作计划。
- 新增 `pinax repair apply`，在显式 `--yes` 和 Git snapshot 保护下应用低风险修复。
- 新增 repair 操作类型：补齐 Pinax metadata、补空 tags、归档 stale note、重建 index、生成 duplicate/orphan/manual-review 建议。
- 新增 `.pinax/repair-plans/*.json` 或等价 CLI-authored structured asset，用 schema version 记录计划、issue 输入、操作、风险级别和过期状态。
- 修改 dashboard 健康视图：展示 repair plan 摘要和 issue drilldown，但 dashboard 仍只读，不提供写入 API。
- 不引入 LLM 自动改写正文，不自动删除笔记，不接 provider，不做云同步，不绕过 CLI 直接写 vault。

## Capabilities

### New Capabilities

- `vault-maintenance-actions`: 将 vault health issue 转换为安全 repair plan，并通过 CLI apply 执行受保护的本地维护动作。

### Modified Capabilities

- `vault-dashboard-health`: dashboard 和 doctor 输出增加 repair plan/drilldown 入口，但保持只读和 stdout/stderr 输出边界。
- `pinax`: 新增 `repair plan/apply` 作为本地 Markdown vault 管理命令，输出遵守 Pinax AI-native CLI 输出合同。

## Impact

- CLI：新增 `repair plan`、`repair apply` 命令，复用 `--vault`、`--json`、`--agent`、`--yes` 和 Git snapshot 保护语义。
- 应用层：新增 repair planner、repair applier、repair plan repository 和 Git snapshot guard。
- 结构化资产：由 CLI/service 写入 `.pinax/repair-plans/*.json`，不得由 agent 手写。
- Dashboard：新增只读 repair plan endpoint 和 issue drilldown view，不新增写入路由。
- 测试：增加 TDD 覆盖、contract tests、fixture vault、只读 dashboard tests、Git snapshot protection tests 和 path boundary tests。
