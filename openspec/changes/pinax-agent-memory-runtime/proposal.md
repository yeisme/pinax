## Why

Pinax 已有 non-vector memory ledger、Agent Brain context bundle、bounded CLI/MCP/API projection 和 proof loop，但这些能力仍以 vault command 和现有 `memory`/`brain` 入口为中心，缺少供应商无关的 principal、scope、proposal、handoff、feedback 和 adapter contract。根据根级 `openspec/changes/general-agent-memory-platform/`，Pinax 需要成为通用 Agent memory state owner，同时保持现有消费者兼容。

## What Changes

- 新增 versioned principal、scope、memory source、context request/pack、proposal、handoff、feedback 和 adapter descriptor domain types。
- 扩展现有 GORM memory ledger，持有 canonical memory lifecycle、scope、source revision、supersession、conflict 和 recall feedback；不保存完整私密 source body。
- 新增 context compiler，复用现有 memory、search、KB、graph、query、project 和 receipt projection，按 permission、scope、lifecycle、confidence、freshness、task fitness 和 budget 编译 context pack。
- 新增 proposal/review/approve/reject/supersede/expire/conflict application services；Agent 默认只能 propose，confirmed mutation 继续受 Pinax service 和 receipt 控制。
- 新增 additive CLI `pinax agent ...`、generic MCP `pinax.agent.*`、localhost REST/RPC 和 Go SDK read/proposal surfaces。
- 建立 generic adapter capability descriptor 和两个 reference adapter harness：Codex + Cohors；runtime-specific 安装和 hook 实现不进入 Pinax core domain。
- 建立 transport parity、redaction、compatibility、integration evidence 和真实跨 Agent handoff dogfooding。
- 保留现有 `pinax memory`、`pinax brain`、`pinax.brain.*`、Remote API Mode 和 stored ledger 行为。本 change 不批准删除、重命名或 repurpose 旧面。

## Capabilities

### New Capabilities

- `agent-memory-runtime`: Pinax 通用 memory domain、context compiler、proposal lifecycle、handoff、feedback、CLI/MCP/API/SDK 和兼容策略。
- `agent-runtime-adapter-contract`: runtime capability descriptor、generic adapter boundary、Codex/Cohors reference harness、故障隔离和 adapter handoff。

### Modified Capabilities

- 无。现有 `agent-memory-ledger` 与 `pinax-agent-brain-layer` 继续作为已发布行为；新 runtime 先通过 additive facade 复用它们。未来收敛或弃用必须另立 change。

## Impact

- 新包建议：`internal/agentprotocol`、`internal/agentmemory`、`internal/agentcontext`、`pkg/agentmemory`。
- 复用/修改：`internal/memory`、`internal/app`、`internal/mcpserver`、`internal/api`、`internal/cli`、`internal/output`、`cmd/pinax`。
- Stored schema：向现有 GORM ledger add tables/nullable fields/indexes；禁止破坏已有 memory rows。
- Stable surfaces：新增 CLI/MCP/API/Go SDK v0.x experimental contract；现有 stable/preview surfaces 保留。
- 前置协调：`pinax-identity-first-note-kernel-v2` 当前仍在执行；涉及共享 `internal/domain`、`internal/app/service.go`、index/sync 的任务必须等该 change closeout 或使用独立新文件避免冲突。
