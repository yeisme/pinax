## 1. Contract and Planning Gate

- [x] 1.1 固定 CLI 合同测试。
  - Owner: Pinax
  - Lane: A
  - Depends on: none
  - Scope: 在 `cmd/pinax` 增加 `TestMemoryCaptureListRecallContext`、`TestMemoryDryRunDoesNotWrite`、`TestMemoryAgentOutputIsBounded`。
  - Acceptance: `go test ./cmd/pinax -run 'TestMemory' -count=1` 初始失败，失败原因是缺少 `memory` 命令或输出字段。
  - Evidence: 2026-06-19 初始失败为 `unknown command "memory" for "pinax"`；实现后 `go test ./cmd/pinax -run 'TestMemory' -count=1` 通过。
  - Failure re-check: 若测试因 fixture 初始化失败，先修复 vault fixture，不放宽 CLI 断言。

- [x] 1.2 固定输出合同和错误码。
  - Owner: Pinax
  - Lane: A
  - Depends on: 1.1
  - Scope: 约定 `command=memory.capture|memory.list|memory.recall|memory.context|memory.stats`，错误码包含 `memory_record_invalid`、`memory_source_invalid`、`memory_store_unavailable`。
  - Acceptance: `go test ./cmd/pinax -run 'TestMemory.*Output|TestMemory.*Error' -count=1` 初始失败并显示缺少稳定 facts/error code。
  - Evidence: `TestMemoryRejectsInvalidRecord` 固定 `memory_record_invalid`；`TestMemoryCaptureListRecallAndContext` 固定 `memory.capture/list/recall/context` JSON/agent facts。
  - Failure re-check: 不允许通过删除 `--agent` 或 `--json` 断言来通过测试。

## 2. Storage and Repository

- [x] 2.1 新增 GORM model 和 migration service。
  - Owner: Pinax
  - Lane: B
  - Depends on: 1.1
  - Scope: 新建 `internal/memory/store.go`，新增 `memory_records`、`memory_entities`、`memory_record_entities`、`memory_sources`，使用 GORM，不在业务层硬编码 SQL。
  - Acceptance: `go test ./internal/memory -run 'TestMemoryStoreMigratesAndPersistsRecords' -count=1` 通过。
  - Evidence: 2026-06-19 `go test ./internal/memory -count=1` 通过。
  - Failure re-check: 若 SQLite migration 失败，检查 GORM model tag 和临时 DB 初始化，不改用 ad hoc SQL 绕过。

- [x] 2.2 新增 FTS5 projection adapter。
  - Owner: Pinax
  - Lane: B
  - Depends on: 2.1
  - Scope: 将 FTS5 作为 repository 内部 projection，集中处理允许的 raw SQL exception；禁止 handler/app service 拼 SQL。
  - Acceptance: `go test ./internal/memory -run 'TestMemoryRecallUsesFTSAndFiltersStatus' -count=1` 通过，`draft/superseded/expired/rejected` 默认不返回。
  - Evidence: 2026-06-19 `TestMemoryRecallUsesFTSAndFiltersStatus` 通过；FTS raw SQL 集中在 `internal/memory.Store`。
  - Failure re-check: 若 FTS5 在环境中不可用，测试必须给出 `memory_store_unavailable` 或跳过条件说明，不默默退化为无过滤扫描。

## 3. App Service and Recall Policy

- [x] 3.1 实现 `MemoryCapture`、`MemoryList`、`MemoryRecall`、`MemoryContext`、`MemoryStats` app service。
  - Owner: Pinax
  - Lane: C
  - Depends on: 2.1
  - Scope: 新建 `internal/app/memory.go`，命令层只做参数解析；service 负责状态、来源、entity、limit 和输出数据。
  - Acceptance: `go test ./internal/app -run 'TestMemory' -count=1` 通过。
  - Evidence: 2026-06-19 `go test ./internal/app -run 'TestMemory' -count=1` 通过。
  - Failure re-check: 若 service 需要访问 vault，使用现有 vault resolver 和 temp fixture，不读取用户真实 vault。

- [x] 3.2 实现非向量召回排序和 `recall_reason`。
  - Owner: Pinax
  - Lane: C
  - Depends on: 2.2, 3.1
  - Scope: 召回排序按 scope、type/entity/status、FTS score、recency、confidence、source authority；每条结果返回可解释 `recall_reason`。
  - Acceptance: `go test ./internal/app ./internal/memory -run 'TestMemoryRecallReason|TestMemoryRecallRanking' -count=1` 通过。
  - Evidence: 2026-06-19 `go test ./internal/memory ./internal/app -run 'TestMemory' -count=1` 通过，覆盖 `recall_reason` 和默认状态过滤。
  - Failure re-check: 若排序不稳定，测试使用固定时间和固定 confidence，不依赖 map iteration 顺序。

## 4. CLI Wiring and Output

- [x] 4.1 新增 `pinax memory` 命令族。
  - Owner: Pinax
  - Lane: D
  - Depends on: 3.1
  - Scope: 新建 `internal/cli/memory_cmd.go` 并在 root command 注册；命令包括 `capture/list/recall/context/stats` 的可用切片，`link/prune` 保留入口并返回明确 unavailable 错误。
  - Acceptance: `go test ./cmd/pinax -run 'TestMemory' -count=1` 通过。
  - Evidence: 2026-06-19 `go test ./cmd/pinax -run 'TestMemory' -count=1` 通过。
  - Failure re-check: 若帮助分组不稳定，按现有 root help annotation 模式补分组，不改全局 help 结构。

- [x] 4.2 固定 `--json` 和 `--agent` 输出。
  - Owner: Pinax
  - Lane: D
  - Depends on: 4.1
  - Scope: projection 复用 `internal/output` envelope；新增 `fact.memory.records`、`fact.memory.matches`、`fact.memory.types`、`fact.memory.scope`。
  - Acceptance: `go test ./cmd/pinax ./internal/output -run 'TestMemory.*Agent|TestMemory.*JSON' -count=1` 通过，stdout 不包含 raw body、secret、prompt payload。
  - Evidence: 2026-06-19 `TestMemoryCaptureListRecallAndContext` 覆盖 JSON/agent 输出；agent context 不输出完整 body。
  - Failure re-check: 若脱敏测试失败，修复 projection 数据源和 redaction，不删除敏感 sentinel。

## 5. Documentation and Quality Gate

- [x] 5.1 更新 Pinax 命令文档。
  - Owner: Pinax
  - Lane: E
  - Depends on: 4.2
  - Scope: 新增 `docs/commands/memory.md`，更新 `docs/commands/README.md`，说明非向量 memory 与 `kb` 的边界。
  - Acceptance: 文档包含真实命令示例、输出模式、安全边界和与 LanceDB KB 的差异。
  - Evidence: 新增 `docs/commands/memory.md`，更新 `docs/commands/README.md`。
  - Failure re-check: 文档不得推荐 agent 手写 `.pinax/memory/*.json` 或直接编辑 SQLite。

- [x] 5.2 跑完整门禁并记录证据。
  - Owner: Pinax
  - Lane: sequential
  - Depends on: 1.1, 1.2, 2.1, 2.2, 3.1, 3.2, 4.1, 4.2, 5.1
  - Scope: 运行全量测试和 OpenSpec 验证。
  - Acceptance: `task check` 通过；`openspec validate --all --strict` 通过。
  - Evidence: 2026-06-19 `openspec validate pinax-agent-memory-ledger --strict` 通过；`openspec validate --all --strict` 通过；修复 staticcheck `QF1001` 后 `task check` 通过。
  - Failure re-check: 如果 `task check` 失败，先定位最小失败包并修复源头，再重跑完整命令。

## Compatibility Record

- CLI commands: 新增 `pinax memory`，additive。
- CLI output: 新增 memory command facts，additive；不移除现有 envelope 字段。
- Config: 若实现需要 `memory.*` 配置键，只能新增默认值，additive。
- Database: 新增 memory tables 和 FTS projection，additive；不修改既有表。
- Rollback: 可隐藏命令入口并停止写 `.pinax/memory/`；本地 projection 可删除重建。
