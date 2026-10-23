## ADDED Requirements

### Requirement: 独立批授权与兼容

范围内自主执行 MUST 依赖独立、显式批准的 `workbench.bounded_action_grant.v1alpha1`；既有 chat、session、Aigora access grant SHALL 保持原语义，不能自动授予工具 mutation 权限。

#### Scenario: 只有旧会话授权
- **WHEN** Agent 拥有有效 session grant 但没有对应 action grant
- **THEN** 文件/终端/MCP mutation 继续走原 proposal-first 决定流程，不因项目模式而自动放行

#### Scenario: 未实现扩展的 owner
- **WHEN** 某 owner 无法验证 action grant 或没有声明效果分类
- **THEN** 该操作不纳入批授权，使用既有单次批准或 `needs_contract`，不得由浏览器推测低风险

### Requirement: 逐动作授权交集

批授权 MUST 绑定主体、项目、资源范围、operation/effect、Profile/manifest revision、预算/次数与期限；每个 action proposal dispatch 前 MUST 重验所有 owner 策略与授权交集，并记录原 proposal、grant、Task 和 owner receipt 关联。

#### Scenario: 工具升级或目标版本改变
- **WHEN** operation manifest、目标 revision 或授权范围不再匹配批准
- **THEN** 阻止该 dispatch 并要求重新准备/批准，不自动扩大 grant

#### Scenario: 任意终端脚本
- **WHEN** 请求仅以“项目内命令”为理由提交未验证的 shell/脚本
- **THEN** 不视为批授权低风险操作，继续需要 owner 明确准入与单次决定

### Requirement: 高影响与领域决定不被批授权吞并

外部发布、敏感读取、不可丢弃数据删除、权限/工具/模型范围升级以及领域正式版本接受 MUST 保留相应 owner 的独立决定条件；批授权不得覆盖未声明的效果。

#### Scenario: 普通工作中出现发布步骤
- **WHEN** 已批准的修改/验证过程产生 push、publish 或真实外部发送请求
- **THEN** 在执行该请求前等待对应明确决定，原批授权不能作为替代

### Requirement: 撤销、并发预算与未知结果

授权撤销或到期 MUST 阻止新 dispatch；预算/次数 MUST 原子预留，未知结果 MUST 经原 owner reconcile 后结算。已 accepted 操作的取消状态只能来自原 owner。

#### Scenario: 并发请求竞争最后预算
- **WHEN** 两个动作同时申请剩余预算且总额超过上限
- **THEN** 准入层至多接受预算允许的动作，不能分别读取旧余额后都放行

#### Scenario: 撤销时已有工作执行
- **WHEN** 用户撤销 action grant 而 owner 已接受一项工作
- **THEN** 后续 dispatch 被阻止；已有工作请求取消或对账，收到 owner 确认前不显示已取消

#### Scenario: 重连或回滚
- **WHEN** 网络断开、服务重启或新增 capability 被关闭
- **THEN** 保留原 attempt 与必要对账/取消入口，不重放工具、不重签 grant、不凭未收到结果释放预算
