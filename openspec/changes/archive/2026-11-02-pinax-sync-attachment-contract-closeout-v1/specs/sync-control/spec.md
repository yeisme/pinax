## ADDED Requirements

### Requirement: Sync control contract

Pinax SHALL 通过 transport catalog、REST/RPC 和 OpenAPI 暴露 typed `sync.status`、`sync.diff`、`sync.push`、`sync.pull`、`sync.conflicts.list`、`sync.conflicts.diff` 和 `sync.conflicts.resolve`。这些合同 SHALL 委托既有 Capsa service，保留 target、base/remote revision、conflict 和 delete-marker 事实。

#### Scenario: Read sync status and plan

- **WHEN** 已授权消费者请求 status 或 dry-run diff
- **THEN** Pinax 返回 revision、recent run、conflicts、up-to-date 状态和恢复动作，不暴露私有路径、凭据或 provider payload

### Requirement: Durable remote write fact

Pinax SHALL 仅在远端持久提交、返回 revision 且本地 read-back 成功后报告 `remote_write=true`。消费者 upload、hydrate 或 projection SHALL NOT 建立这个事实。

#### Scenario: Remote commit is uncertain

- **WHEN** mutation 回执丢失或 read-back 无法完成
- **THEN** Pinax 保留 unknown/reconcile_required，使用原 operation/run identity，不创建新 mutation key

### Requirement: Human-confirmed sync mutations

Push、pull 和 conflict resolve SHALL 经过既有 write gate 与明确确认。缺失或歧义 target SHALL 被拒绝。

#### Scenario: Read-only consumer attempts push

- **WHEN** 无写权限的 session 请求 mutation
- **THEN** Pinax 拒绝且不改变 vault bytes、remote revision、delete markers 或 operation state
