# Design — Workbench continuity typed projection facade（pinax-workbench-continuity-projection-v1）

冻结三份合同：machine envelope、投影时效、provider packet。全部 additive；dogfood 冻结面（`pinax-trusted-continuity-dogfood-v1`）不修改。

## 1. Machine envelope `pinax.workbench.continuity_projection.v1`（任务 1.1）

`pinax continue workbench <projectRef>` 输出唯一 envelope（`--json`/`--agent` 同源）：

```json
{
  "schema_version": "pinax.workbench.continuity_projection.v1",
  "contract": {"identity": "pinax.workbench.continuity", "version": "v1", "digest": "<sha256:…>"},
  "project_ref": "bind_…",
  "binding": {"status": "ready|missing|disabled|invalid|not_found", "ready": true,
               "vault_ref": "…", "scope": "project:pinax", "vault_resolved": true, "scope_valid": true},
  "resume_card": {"objective": "…", "current_state": "…", "task": "…",
                   "sections": ["decisions:2", "open_tasks:3", "blockers:1"],
                   "sources": "resolved:1/1", "handoff": "available|h_<id>|none",
                   "conflicts": 0},
  "freshness": {"basis": "evidence_observed_at", "observed_at": "…", "expires_at": "…", "ttl_seconds": 600},
  "recovery": {"code": "binding_not_found", "action": "pinax continue bind --vault <ref> --scope …"}
}
```

- `contract.digest` = sha256 over canonical JSON of `{identity, version, actions, errors}`（无时间戳，跨运行稳定）。
- `resume_card` 是 ContinuityPack 的**显式选字段有界投影**（计数化 sections，不透传未来 pack 字段；不含 body/transcript/路径）。
- 错误态（`binding.status != ready`）envelope 仍为 200 语义投影：`resume_card` 缺省、`recovery` 必填、`freshness` 以 binding 观测为 basis。

### 1.1 projectRef ↔ binding 映射（safe semantics）

- `projectRef` = opaque `binding_id`（`bind_<digest>`，registry 权威）。Workbench 不持路径、不猜 scope。
- 解析只走 registry exact-by-id：`not_found`（id 不存在）/ `disabled` / `invalid`（vault 或 scope 不可解析）/ `ready`。
- **不做**跨 vault 搜索、目录名匹配、最近使用猜测；歧义在 by-id 语义下不存在（id 唯一），多 enabled worktree binding 属既有 worktree 解析路径，不进本 facade。
- 错误码与唯一恢复 action（固定命令字符串）：

| status | code | action |
| --- | --- | --- |
| not_found | `binding_not_found` | `pinax continue bind --vault <vault-alias> --scope <kind>:<id>` |
| disabled | `binding_disabled` | `pinax continue binding enable <projectRef>` |
| invalid | `binding_invalid` | `pinax continue binding disable <projectRef> && pinax continue bind …` |

Envelope 红线：不携带 raw note/handoff 正文、transcript、provider payload、credential、绝对路径（沿 dogfood bounded 规则，逐字段走既有 denylist 思路）。

## 2. 投影时效（任务 1.2）

- `basis` 固定 `evidence_observed_at`：取 ContinuityPack 的 evidence coverage 最新观测时间（无证据时取当次调用 UTC）；**不是**投影生成时间。
- `ttl_seconds` 默认 600、下限 60（`--ttl` 覆盖）；`expires_at = observed_at + ttl`。
- **refresh-only**：过期投影只能重新调用 facade 获取；消费者（Workbench BFF）不得本地续命、缓存改写或用生成时间顶替。该语义为合同要求，写入 spec 场景。

## 3. Provider packet `pinax.provider_packet.v1`

`pinax continue workbench --packet` 输出（根仓消费的紧凑合同描述）：

```json
{
  "schema_version": "pinax.provider_packet.v1",
  "identity": "pinax.workbench.continuity",
  "version": "v1",
  "digest": "<同 envelope contract.digest>",
  "owner": "cli/pinax",
  "availability": {"mode": "local-cli", "entry": "pinax continue workbench <projectRef> --json"},
  "actions": [
    {"name": "resume_card.read", "effect": "read_only", "entry": "pinax continue workbench <projectRef> --json"},
    {"name": "binding.status.read", "effect": "read_only", "entry": "pinax continue workbench <projectRef> --json"},
    {"name": "checkpoint.propose", "effect": "durable_candidate_only", "requires": "--yes confirmation",
     "entry": "pinax continue checkpoint …", "receipt": "handoff id + proposal ids"}
  ],
  "scope_revision": "binding registry schema pinax.continuity_binding.v1",
  "errors": ["binding_not_found", "binding_disabled", "binding_invalid", "validation_failed"],
  "recovery": {"policy": "one stable action per error code, see envelope.recovery"},
  "evidence_refs": ["openspec/changes/pinax-workbench-continuity-projection-v1/", "temp/integration-test-runs/<run>/"]
}
```

- `checkpoint.propose` 复用既有 `continue checkpoint`（durable 只走 proposal service，不自动 confirmed memory）；本 change 不新增写路径。
- packet 是静态合同描述 + digest，可独立核验；`resume_card.read` 的消费合同即 §1 envelope。

## 4. 实现落点

- `internal/domain/workbench_projection.go`：envelope/packet 类型 + canonical digest。
- `internal/app/workbench_projection.go`：`WorkbenchContinuityProjection(req{ProjectRef, ConfigDir, TTL})`——registry exact-by-id → 复用 `ContinuityBindingStatus`（by binding 的 canonical root）→ `AgentContinuity` 组装 card → freshness 派生；错误态走 recovery 组装。
- `internal/cli/continue_workbench_cmd.go`：additive 子命令 `pinax continue workbench [projectRef] [--packet] [--ttl seconds]`。
- 测试：domain digest 稳定性、app 映射矩阵（not_found/disabled/invalid/ready）与 freshness basis/TTL、cmd json/agent 合同 + 递归 leak 扫描、e2e（bind → facade ready → disable → recovery → --packet digest 一致）；`task check` + `task test:integration` 收口。
