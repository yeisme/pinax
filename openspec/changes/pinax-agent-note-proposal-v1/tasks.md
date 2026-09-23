# pinax-agent-note-proposal-v1 Tasks

## 0. 前置门禁与基线

- [ ] 0.1 Owner: release；Scope: 本 change 全部实施任务；Dependencies: none；Lane: gate；确认 2026-10-10 连续性主线窗口已结束或 owner 显式重排；Verification: `openspec status --change pinax-agent-note-proposal-v1` + owner note；Expected: 门记录先于任何代码任务；Failure re-check: 仅执行合同/spec 任务。
- [ ] 0.2 Owner: architecture；Scope: 与 `pinax-agent-memory-runtime` 的共享文件协调；Dependencies: none；Lane: gate；确认其 5.x（`pinax agent` CLI facade）已合并 develop，或本 change 全部使用独立新文件避让共享编辑；Verification: `openspec status --change pinax-agent-memory-runtime && git log --oneline -5 -- internal/cli/agent_cmd.go`；Expected: proposal/review 骨架可复用、无未协调并行编辑；Failure re-check: 未合并则仅做 DTO/spec 任务，服务与 CLI 任务挂起。
- [ ] 0.3 Owner: baseline；Scope: proof loop 与 proposal fixture 基线；Dependencies: none；Lane: gate；固化现有 proof loop apply/receipt 与 memory-runtime proposal fixture 的兼容测试清单并确认基线绿；Verification: `go test ./internal/app ./cmd/pinax -run 'ProofLoop|Proposal|Receipt' -count=1`；Expected: 基线通过并记录现有 receipt schema；Failure re-check: 区分既有失败，不放宽旧测试。

## 1. 领域类型与策略

- [ ] 1.1 Owner: agent-protocol；Scope: 新建 `internal/agentprotocol/noteproposal.go` 及测试；Dependencies: 0.3；Lane: A；定义 note 域 proposal DTO：kind（inbox_capture/journal_append/note_draft/note_edit）、target object_id、expected revision、结构化变更、rationale ≤500；Verification: `go test ./internal/agentprotocol -run 'NoteProposal|Validation|Rationale' -count=1`；Expected: 非法 kind/超长 rationale 返回稳定 English code；Failure re-check: 对拍 memory-runtime proposal 基型字段。
- [ ] 1.2 Owner: policy；Scope: `internal/agentmemory`（或同构位置）risk tier 判定与白名单；Dependencies: 1.1；Lane: A；实现低风险白名单硬编码（capture/journal/draft）、note_edit=pending、organize/repair 既有路径直通；Verification: `go test ./internal/agentmemory -run 'NoteRiskTier|AutoApplyWhitelist' -count=1`；Expected: 白名单外无 auto-apply、无配置键旁路；Failure re-check: 逐 kind 重放 tier 表。

## 2. 应用服务与 proof loop 绑定

- [ ] 2.1 Owner: proposal-service；Scope: 新建 `internal/app/agent_note_proposal.go` 及测试；Dependencies: 0.2、1.1、1.2；Lane: B；实现 propose→validate→tier→apply/pending 编排：复用 proof loop service，目标名带 proposal 派生前缀不覆盖既有文件；Verification: `go test ./internal/app -run 'AgentNoteProposal|AutoApply|NoOverwrite' -count=1`；Expected: auto-apply 有 snapshot+receipt、同名不覆盖；Failure re-check: 检查事务边界与 snapshot 时序。
- [ ] 2.2 Owner: proposal-service；Scope: expected revision 钉住与 plan_stale；Dependencies: 2.1；Lane: B；note_edit 校验 expected revision，漂移返回 `plan_stale` + bounded next action（重读卡片→重新 propose）；Verification: `go test ./internal/app -run 'PlanStale|NextAction|Drift' -count=1`；Expected: 不盲重试、不静默换版；Failure re-check: 双版本 fixture 重放 propose 序列。
- [ ] 2.3 Owner: proposal-service；Scope: approve/reject/receipt redaction；Dependencies: 2.1；Lane: B；实现确认应用与 receipt：rationale 截断、diff 只存行数摘要与 hunk 计数、approve/reject 都留 receipt；Verification: `go test ./internal/app -run 'Approve|Reject|ReceiptRedaction' -count=1`；Expected: receipt 无正文全文、无 token；Failure re-check: 用 redline 断言族对拍。
- [ ] 2.4 Owner: proposal-service；Scope: `RejectHandwrittenMetadata` 覆盖 note payload；Dependencies: 2.1；Lane: B；对 proposal payload 的结构化元数据全量 fail-closed；Verification: `go test ./internal/app -run 'HandwrittenMetadata|FailClosed' -count=1`；Expected: 手写 metadata blob 拒绝且 vault 不变；Failure re-check: 逐字段注入非法 blob。

## 3. CLI 审阅面

- [ ] 3.1 Owner: CLI；Scope: `internal/cli` `agent note proposal list|show|approve|reject` 与 wiring；Dependencies: 2.1-2.3；Lane: C；实现审阅命令树：无 `--yes` 只预览、`--agent`/`--json`/`--events`/`--explain` 同源投影；Verification: `go test ./cmd/pinax ./internal/cli -run 'AgentNoteProposal|PreviewGate|MachineMode' -count=1`；Expected: 四模一致、preview 不写；Failure re-check: 对照 cli-output-contract 模板。
- [ ] 3.2 Owner: CLI；Scope: help/golden 与命令手册；Dependencies: 3.1；Lane: C；更新 golden help 与 docs/commands，确认 `pinax agent` 既有子命令零改名；Verification: `go test ./cmd/pinax -run 'Help|Golden' -count=1`；Expected: 旧 golden 除新增外 diff 为空；Failure re-check: 不改旧 golden 掩盖 breaking。

## 4. MCP 门控面

- [ ] 4.1 Owner: MCP；Scope: `internal/mcpserver` propose/list 工具与 capability 门；Dependencies: 2.1、3.1；Lane: D；实现 `pinax.agent.propose_note_draft|propose_note_edit`（capability 默认关，关闭不进 tools/list）与 `pinax.agent.note_proposals` 只读列表；Verification: `go test ./internal/mcpserver -run 'NoteProposalTool|CapabilityDisabled|ToolsList' -count=1`；Expected: 默认无写工具、开启后 propose 不静默 apply；Failure re-check: 检查注册条件与 call routing。
- [ ] 4.2 Owner: MCP；Scope: transport parity；Dependencies: 4.1、3.1；Lane: D；同一 fixture 经 CLI 与 MCP 的 propose/list 返回语义一致数据；Verification: `go test ./tests/e2e -run 'NoteProposalTransportParity' -count=1`；Expected: 仅 wrapper 差异；Failure re-check: 保存 normalized projection 定位首个差异字段。

## 5. 证据、文档与收尾

- [ ] 5.1 Owner: evidence；Scope: dogfood 计数器与 evidence profile 扩展；Dependencies: 2.3、4.1；Lane: E；发出 proposed/auto_applied/approved/rejected/stale 计数，断言证据无正文/token；Verification: `go run ./tools/testkit/integrationevidence -profile agent-note-proposal`；Expected: 全断言通过、证据脱敏；Failure re-check: 逐计数器注入事件重放。
- [ ] 5.2 Owner: docs；Scope: 命令手册、[Agent 优化设计](../../../docs/product/agent-optimization-design.md) §3.2 状态、[交互体验规格](../../../docs/product/rendered-preview-interaction-ux.md) §7、pinax-agent-router 路由表；Dependencies: 3.1、4.1；Lane: E；文档与 router frontmatter 同步（evolutionary-change-policy 门）；Verification: `rg -n 'propose_note|note proposal' docs .claude/skills/pinax-agent-router`；Expected: 新面可被路由发现；Failure re-check: frontmatter 变更须引用本 change。
- [ ] 5.3 Owner: closeout；Scope: strict 校验与归档；Dependencies: 5.1、5.2；Lane: E；`openspec validate pinax-agent-note-proposal-v1 --strict --no-interactive` 通过后按流程归档；Verification: `openspec validate pinax-agent-note-proposal-v1 --strict --no-interactive`；Expected: strict 通过、任务全勾或如实注记 external gate。
