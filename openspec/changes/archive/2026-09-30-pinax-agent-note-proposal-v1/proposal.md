# pinax-agent-note-proposal-v1

## Why

Agent 对 vault 的写入目前只有两条窄路：Pane 的两个 gated actions（`inbox.capture`、`sync.run`）和 proof loop 的 CLI 命令（用户本人终端执行）。MCP 默认只读。`pinax-agent-memory-runtime` 已为 memory 域建立 propose → review → approve 的纪律，但 **note 域**（日常笔记、journal、正文编辑）没有同等面：agent 要么不能写，要么被迫绕到 shell 命令。完整问题与六条优化线见 [Agent 优化设计](../../../docs/product/agent-optimization-design.md) §3.2/§3.5。

本 change 给 note 域补上同一纪律的 proposal 面：**agent 只能 propose，所有 apply 走 proof loop，中风险需用户确认**。

## What Changes

- 新增 note 域 proposal DTO 与应用服务：`propose_inbox_capture`、`propose_journal_append`、`propose_note_draft`、`propose_note_edit`。
- 风险分级：低风险（capture/journal/draft）可按策略 auto-apply（首版硬编码白名单，不配置化）；中风险（note_edit）必须用户确认；高风险（organize/repair）沿用既有 plan 审批路径，不新增。
- note_edit proposal 必须携带 target `object_id` + `expected revision` + 结构化变更 + rationale（≤500 字符）；漂移返回 `plan_stale` 与"重读卡片 → 重新 propose"的下一步动作。
- CLI 审阅面：`pinax agent note proposal list|show|approve|reject`（无 `--yes` 只预览、四输出模同源、receipt 全 redacted）。
- MCP：`pinax.agent.propose_note_draft` / `pinax.agent.propose_note_edit` 默认不注册，capability 显式开启才可见；`pinax.agent.note_proposals`（list/show）只读默认可见。
- proposal 结果计数进 dogfood 证据（proposed/auto_applied/approved/rejected/stale），不含正文。
- 所有 proposal 过 `RejectHandwrittenMetadata`；agent 不获得任何绕过 proof gate 的直写工具。

### Capabilities

#### New Capabilities

- `agent-note-proposal`：note 域 proposal 类型、风险分级与 auto-apply 策略、proof-loop 绑定、审阅 CLI、capability 门控的 MCP 面、dogfood 计数。

#### Modified Capabilities

- 无。`pinax-agent-memory-runtime` 的合同不动（本 change 复用其 proposal 基型与 review 服务骨架，note payload 为新增 DTO）；pane gated actions 清单本 change 不扩（P2 再评估 `note.create_draft`）。

## Impact

- 新增：`internal/agentprotocol` 内 note proposal DTO（或新文件 `noteproposal.go`）、`internal/app/agent_note_proposal.go`、CLI/MCP wiring、evidence profile。
- 复用：proof loop service（plan→snapshot→apply→receipt→restore）、`RejectHandwrittenMetadata`、`internal/output` 四模、memory-runtime 的 proposal/review 骨架与 principal/scope 校验。
- 依赖门：`pinax agent` 命令树 facade 依赖 memory-runtime 的 5.x 任务已合并（共享文件协调见任务 0.2）。
- 稳定面：全部 additive experimental；不改名、不删除既有命令/工具；auto-apply 白名单首版硬编码（开放问题 1 的裁决）。
- 门禁：实施排在 2026-10-10 连续性主线窗口之后（任务 0.1 硬门）；dogfood 指标并入既有 evidence 周期。
