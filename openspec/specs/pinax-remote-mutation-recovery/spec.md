# pinax-remote-mutation-recovery Specification

## Purpose
TBD - created by archiving change pinax-multi-access-contract-foundation-v1. Update Purpose after archive.
## Requirements
### Requirement: 支持 recovery 的远程 mutation 必须携带稳定 operation identity

对 manifest 标记 `mutation_recovery=ready` 的远程 mutation，client MUST 在首次提交前生成 opaque `operation_id` 和 `Idempotency-Key`；server MUST 在执行业务写入前校验并绑定二者。

#### Scenario: 缺少 operation identity

- **WHEN** client 对 recovery-enabled mutation 缺少 operation id 或 idempotency key
- **THEN** server MUST 返回 `operation_identity_required`
- **AND** MUST NOT 调用 application service 或写 operation ledger

### Requirement: Idempotency binding 必须防止同 key 异义复用

Server SHALL 将 idempotency key 绑定到 principal/scope、binding id、operation id 和 canonical request digest。同 key 同 digest MUST 返回原 operation/result；同 key 不同 digest MUST 返回 `idempotency_conflict`。

#### Scenario: 相同请求重复到达

- **WHEN** 同 principal 使用同 key、同 operation id 和同 canonical request 再次提交
- **THEN** server SHALL 返回原 operation status 或 bounded result
- **AND** application service MUST NOT 再次执行 mutation

#### Scenario: 相同 key 被不同请求复用

- **WHEN** 同 idempotency key 对应的 title、target path、scope 或 binding 发生变化
- **THEN** server MUST 返回 `idempotency_conflict`
- **AND** 原 operation record MUST 保持不变

### Requirement: Operation ledger 必须先于 domain write 持久化

Server MUST 通过 application-owned GORM repository 在调用 mutation service 前创建 accepted operation record，并在执行过程中记录状态转换、request digest、refs 和 bounded result。Handler/command MUST NOT 直接写 SQL 或手工组装 ledger asset。

#### Scenario: Accepted record 写入失败

- **WHEN** operation ledger 无法持久化 accepted record
- **THEN** server MUST 返回 `operation_store_unavailable`
- **AND** domain mutation MUST NOT 执行

### Requirement: Operation 状态必须可查询与 reconcile

Pinax SHALL 提供 operation show/status 和 reconcile 的 CLI、REST/RPC contract。Operation projection MUST 包含 operation id、capability/binding、status、retry/reconcile facts、revision refs、receipt ref、timestamps 和 redacted error。

#### Scenario: 查询已完成 operation

- **WHEN** authorized client 查询 succeeded operation
- **THEN** server SHALL 返回原 bounded result、receipt ref 和 resulting revision
- **AND** MUST NOT 重放 mutation

#### Scenario: Reconcile 证据不足

- **WHEN** operation 处于 `reconcile_required` 且 receipt/revision inspector 不能证明成功或失败
- **THEN** reconcile MUST 保持 `reconcile_required`
- **AND** MUST 返回安全 next action 而不是执行写入

### Requirement: Ambiguous outcome 后 client 不得盲目重放

Mutation request 在 timeout、connection reset、5xx 或 malformed 2xx 后，client MUST 先查询同一 operation id；不得生成新 operation、换新 idempotency key或自动 replay。

#### Scenario: 响应丢失但写入已成功

- **WHEN** server 已完成 mutation 和 receipt，但 success response 在网络中丢失
- **THEN** client SHALL 通过 operation status/reconcile 获得原 succeeded result
- **AND** owner state MUST 只发生一次变化

#### Scenario: Status 明确可安全重试

- **WHEN** operation status 为 terminal failed，且同时返回 `retryable=true` 和 `replay_safe=true`
- **THEN** client SHALL 只允许使用同一 operation/idempotency binding 重试
- **AND** MUST NOT 创建第二个逻辑 operation

### Requirement: Folder rename 必须使用 revision precondition

Recovery-enabled `folder.rename` MUST 要求 `expected_revision` 或等价 snapshot/restore precondition。Server MUST 在写入前重新验证；不匹配时返回 `revision_conflict` 且不改名。

#### Scenario: Revision 已变化

- **WHEN** client 的 expected revision 早于当前 vault/folder revision
- **THEN** server MUST 返回 `revision_conflict`
- **AND** operation SHALL 终止为 failed/non-replay-safe
- **AND** 原目录结构 MUST 保持不变

#### Scenario: Revision 匹配

- **WHEN** expected revision 与当前状态匹配且 approval/snapshot gates 均通过
- **THEN** application service SHALL 最多执行一次 rename
- **AND** succeeded operation SHALL 记录 revision before/after 与 restore hint

### Requirement: Inbox capture 必须证明幂等创建

Recovery-enabled `inbox.capture` SHALL 使用 idempotency binding 防止 timeout/retry 产生重复 note。成功 projection MUST 返回 canonical note/resource ref 和 receipt ref。

#### Scenario: Capture 请求被发送两次

- **WHEN** 网络客户端以同一 operation/idempotency binding 重复提交 inbox capture
- **THEN** vault SHALL 只出现一条 captured note
- **AND** 两次响应 SHALL 指向同一 note ref 和 operation id

### Requirement: Dry-run 不得伪装成已提交 operation

Remote mutation 的 `dry_run=true` MUST NOT 进入 applying/succeeded remote-write 状态。若返回 operation projection，其 status MUST 为 `planned`，`remote_write=false`，且不得创建 domain receipt 声称写入成功。

#### Scenario: Dry-run inbox capture

- **WHEN** client 提交 `inbox.capture` with `dry_run=true`
- **THEN** projection SHALL 返回 validated plan/preview
- **AND** vault、operation apply state 和 domain receipt MUST 保持未写入

### Requirement: 首切之外的 mutation 不得声称 recovery ready

在本 change 中，只有 `inbox.capture` 与 `folder.rename` 可以晋级为 `mutation_recovery=ready`。其他 write route 即使共享 RPC dispatcher，也 MUST 报告 `degraded|blocked|not_applicable`，直到后续 capability-specific tests 完成。

#### Scenario: 查询 folder delete readiness

- **WHEN** manifest 包含 existing folder delete route，但该 capability 没有 operation recovery canary
- **THEN** owner binding SHALL 按真实 route backing 报告 available
- **AND** mutation recovery MUST NOT 报告 ready

### Requirement: Operation access 必须按 principal 和 scope 授权

Operation id SHALL 是 opaque ref，但仅持有 ref 不构成读取权限。Status/reconcile MUST 在读取 bounded result、receipt 或 resource existence 前验证 principal/scope。

#### Scenario: 其他 scope 查询 operation

- **WHEN** credential scope 与 operation binding 不匹配
- **THEN** server MUST 返回 `operation_not_found` 或 `insufficient_scope` 的既定安全形式
- **AND** MUST NOT 泄漏 operation、note、folder 或 receipt 是否存在

### Requirement: Operation ledger 与 evidence 必须脱敏

Operation store、logs、events、receipts、tests 和 integration evidence MUST NOT 保存 Authorization header、token、token file contents、raw note body、raw prompt、provider payload、private tool arguments、absolute vault path 或 full chain-of-thought。

#### Scenario: Request 包含 note body 和 bearer token

- **WHEN** inbox capture 通过 authenticated remote request 执行
- **THEN** ledger SHALL 只保存 canonical request digest、bounded title/ref facts 和 credential source type
- **AND** secret/body leakage scan MUST 找不到 token 或完整正文

### Requirement: Operation error taxonomy 必须稳定且可行动

Operation API/client error SHALL 至少支持 `operation_identity_required`、`idempotency_conflict`、`operation_store_unavailable`、`operation_not_found`、`revision_conflict`、`reconcile_required`、`upstream_invalid_response`。Error projection MUST 包含 retryable、required action 和 operation ref（可安全公开时）。

#### Scenario: Client 收到 malformed success

- **WHEN** server 返回 2xx 但缺少 operation identity 或 schema version
- **THEN** client MUST 返回 `upstream_invalid_response`
- **AND** MUST 标记 `reconcile_required=true`

### Requirement: Crash recovery 必须由证据判断而不是重执行

若 process 在 domain mutation 后、operation finalize 前崩溃，reconcile SHALL 检查 application receipt、revision 和 resource identity来判断状态。证据不足时 MUST 保持 reconcile required；MUST NOT 通过重新调用 mutation 猜测结果。

#### Scenario: Rename 已完成但 finalize 未写入

- **WHEN** folder 已按同一 operation 改名且 revision/receipt 可证明结果，但 operation 仍为 applying
- **THEN** reconcile SHALL 将同一 operation 标为 succeeded
- **AND** MUST NOT 再次执行 rename
