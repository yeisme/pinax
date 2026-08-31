## ADDED Requirements

### Requirement: Continuity MUST 只 inline 提示会改变当前工作的 review item
Memory Inbox MUST 提供 deterministic relevance projection，使 Continuity Orchestrator 能识别会改变当前 objective、decision、blocker、conflict 或 recommended next action 的 pending item。默认 human Resume Card 最多 inline 提示一个 bounded review item；其他 item MUST 留在完整 inbox。

#### Scenario: Pending proposal 改变当前 recommended next action
- **WHEN** 当前 scope 的一个 pending proposal 与 active objective 直接相关，且批准或拒绝会改变下一步
- **THEN** Resume Card MUST 显示一个 bounded review attention notice、reason code 和 review action
- **AND** MUST NOT 展开完整 proposal body 或自动批准。

#### Scenario: Pending item 与当前 continuation 无关
- **WHEN** inbox item 属于同一 vault 但不影响当前 scope/objective/decision/blocker
- **THEN** Resume Card MUST 不展示该 item
- **AND** item MUST 继续出现在 weekly Memory Inbox，不得被删除或降级。

### Requirement: Memory review MUST 支持五分钟内完成的 weekly bounded workflow
Operator 与 Pinax review projection MUST 支持按当前 bounded scope 展示 unresolved proposals、conflicts、stale/expired candidates 和 receipts，并记录一次 weekly review 的总用时。任何 mutation MUST 继续复用 canonical lifecycle/proof service。

#### Scenario: Weekly inbox 在五分钟内完成
- **WHEN** 用户完成一次 weekly review 并提交 review duration
- **THEN** continuity dogfood report MUST 按周显示 item count、action count、unresolved count 和 total seconds
- **AND** weekly review 总用时不超过 300 秒时该周 burden gate 才能通过。

#### Scenario: Weekly inbox 没有待处理项
- **WHEN** bounded scope 没有 pending/conflict/stale review item
- **THEN** review MUST 返回空 inbox success，并允许记录 `review_seconds=0`
- **AND** 不得创建虚假 approval/rejection receipt。

### Requirement: Inline 与 weekly review MUST 保持 source、privacy 和 stale-action 边界
任何 inline notice 或 weekly item MUST 返回 bounded source metadata、scope、risk、reason codes 和 stable item reference。Approve/reject MUST 在 action 前重新验证 proposal/source/conflict state，并拒绝 stale action。

#### Scenario: Inline item 在用户操作前发生变化
- **WHEN** Resume Card 提示的 proposal、source revision 或 conflict state 在 review action 前改变
- **THEN** canonical review service MUST 拒绝旧 action 并要求 refresh
- **AND** continuity receipt MUST 记录 stale warning category，而不是声称操作成功。
