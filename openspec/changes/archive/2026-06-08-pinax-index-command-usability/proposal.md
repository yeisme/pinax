## Why

`pinax index` 现在能初始化、检查、同步和重建索引，但用户需要先理解底层状态机才能判断下一步该运行哪个命令。索引又是搜索、query、链接图和 organize 的关键本地 projection，因此命令体验需要从“维护子命令集合”提升为“可诊断、可恢复、可由 agent 稳定驱动的维护入口”。

## What Changes

- 为 `pinax index` 增加更清晰的主路径设计：默认入口展示当前状态、影响范围和推荐下一步，而不是只打印 help。
- 规范 `index status`、`index sync`、`index rebuild` 的人类输出、机器 facts、证据和下一步动作，让用户能直接判断“是否需要处理”。
- 引入 `index refresh` 作为默认低成本维护动作，用于跳过未变更笔记并增量补齐缺失 projection。
- 引入 `index doctor` 作为诊断入口，解释 missing/stale/unreadable/schema mismatch/partial 的原因和安全修复路径。
- 引入 `index repair` 的受保护流程，只执行可重建 projection 层面的安全修复；破坏性或不确定修复仍转交 `repair plan` 或 `index rebuild`。
- 更新 index help 和错误 next action，优先推荐 `status -> refresh -> doctor -> rebuild` 的决策路径。
- 不改变 `.pinax/index.sqlite` 作为可重建 projection 的定位，不把 SQLite 变成笔记真源。
- 不修改 `--json`、`--agent`、`--events`、`--explain` 顶层 envelope 字段。
- 不依赖公网、provider token、远端服务或真实用户 vault。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `notebook-index-search`: 规范 index 命令的可用性、诊断、增量 refresh、repair 边界、输出 facts 和安全恢复路径。
- `cli-tree-ux`: 调整 `pinax index` help/default 行为和用户推荐路径，使 index 子命令按维护工作流组织。

## Impact

- `internal/cli/index_cmd.go`: index 命令树、help 文案、默认 RunE、flag 和 next action 接线。
- `internal/app/service.go`: index status/refresh/doctor/repair 的 application service 编排和 projection 生成。
- `internal/index/*`: 增量 refresh、状态诊断、schema/version 证据、repair 安全边界和 GORM projection 维护。
- `internal/output/*`: 仅在需要时补充 index 专用 summary 表格；机器输出继续使用稳定英文 key。
- `cmd/pinax/main_test.go`、`tests/e2e/testdata/index_sync/*`: CLI contract、process e2e、stdout/stderr 分离和 fake vault 场景。
- `openspec/specs/notebook-index-search` 和 `openspec/specs/cli-tree-ux`: 归档时同步新的用户体验和维护合同。
