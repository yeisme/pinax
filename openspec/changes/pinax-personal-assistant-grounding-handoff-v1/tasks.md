# Tasks

## 1. Contract

- [x] 1.1 [owner: Pinax] 冻结 pre-1.0 grounding evidence、canonical count invariants、ownership 与 rollback；Validation：`openspec validate pinax-personal-assistant-grounding-handoff-v1 --strict`。

## 2. Implementation

- [x] 2.1 [owner: internal/agentcontext] 聚合实际进入 bounded context pack 的去重 source refs；Acceptance：被 budget 截掉的 entry source 不进入 pack。
- [x] 2.2 [owner: internal/agentmemory] GORM additive 持久化 proposal source refs，并在 approve 时完整传递到 confirmed memory；Acceptance：旧 vault 自动迁移、旧表不变、无 raw SQL。
- [x] 2.3 [owner: internal/app] 新增 source-aware context/memory/handoff projection 和四态 evidence；Acceptance：Pinax evidence 不接管 PA canonical turn state。
- [x] 2.4 [owner: internal/mcpserver] 三个既有 readonly tool additive 输出 `grounding`，旧 status/command/body_exposure/payload keys 保持。

## 3. Verification

- [x] 3.1 覆盖 grounded、partially_grounded、ungrounded、grounding_unavailable 与 count invariants。
- [x] 3.2 覆盖 transcript note delete 后 search、context、memory recall、handoff read 均不返回正文 sentinel；tombstone 仍存在。
- [x] 3.3 运行 focused package tests、MCP contract tests、OpenSpec strict validation、`git diff --check`；稳定后再评估 `task check`，并区分既有/并发失败。

  Evidence：

  - `go test ./...`：PASS。
  - `task check`：PASS（fmt、lint、全量 test、build、79 项 OpenSpec、renderer build/test、publish smoke）。
  - `openspec validate pinax-personal-assistant-grounding-handoff-v1 --strict`：PASS。
  - scoped `git diff --check`：PASS。
  - integration/e2e evidence：`temp/integration-test-runs/20260830T185152Z-868656/summary.json`，status=`passed`，redaction scan=`passed`。

## 4. Handoff

- [x] 4.1 更新 Pinax docs 与 root Personal Assistant task 3.2/evidence，明确 local owner evidence 与剩余跨 owner system test。Evidence：`docs/interfaces/personal-assistant-grounding-handoff.md`、root review note `2026-08-30-pinax-grounding-delete-handoff.md`。
