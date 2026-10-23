# daily-review-followthrough Specification

## Purpose
TBD - created by archiving change pinax-daily-review-followthrough-v1. Update Purpose after archive.
## Requirements
### Requirement: 选择集规划 SHALL 限制可写对象而保留全库上下文

Pinax SHALL 接受最多 20 个 canonical 笔记的 ID/path 选择集，去重 selector、拒绝非法或歧义身份，并仅对选中对象产生操作。未提供选择集 SHALL 保留既有全库行为。

#### Scenario: 批外链接
- **WHEN** 两个选中对象链接到未选中笔记
- **THEN** 关联判断 SHALL 使用全库上下文且不得产生未选中对象的写操作

#### Scenario: 保存与兼容
- **WHEN** 用户显式保存限定计划
- **THEN** service SHALL 创建 pinax.organize_plan.scoped.v1，包含全部 selection 与 revision，旧 v1 计划保持可用

### Requirement: Scoped apply SHALL 预检并记录可恢复结果

Scoped apply SHALL 要求已保存计划和明确确认，拒绝 stale override，并在快照前复验完整选择集与目标冲突。执行 SHALL 不覆盖已有目标，不自动重试中断或消费过的计划。

#### Scenario: 漂移
- **WHEN** 选中内容漂移或目标路径被占用
- **THEN** apply SHALL 拒绝且不修改笔记，无关正文修改不得触发全库失效

#### Scenario: 部分失败
- **WHEN** 执行开始后操作失败
- **THEN** SHALL 停止后续操作并保存快照、已完成/失败/未处理结果，证据写入失败 SHALL 不宣称成功

### Requirement: 提案队列 SHALL 提供有界无写入续阅

list SHALL 支持 status/kind 精确筛选及 1–50 条分页，默认最新 50 条和数组结构不变。游标 SHALL 绑定 vault、参数与匹配集，读取不得打开 SQLite 或写 sidecar。

#### Scenario: 历史超过 50 条
- **WHEN** 用户筛选待审项并翻页
- **THEN** SHALL 找到较旧待审项且排序稳定、无重复或遗漏

#### Scenario: 游标变化
- **WHEN** 游标非法、跨 vault 或匹配集已变
- **THEN** SHALL 显式拒绝并提示按原筛选刷新，不能静默跳项

#### Scenario: 查询原提案
- **WHEN** 无关投影损坏但已知 ID 的投影有效
- **THEN** show SHALL 仍能读取该原 ID，列表不得静默丢弃坏投影

### Requirement: CLI/MCP SHALL 保持权限与动作边界

CLI 动作 SHALL 绑定正确 vault 并安全 quote。MCP SHALL 通过原工具提供可发现的只读分页/选择参数和结构化后续调用，不接受 caller vault/权限，不增加审批工具、正文或 payload。

#### Scenario: 无本地 CLI 的 MCP 客户端
- **WHEN** 客户端使用 tools/list 和返回的分页参数
- **THEN** SHALL 完成只读筛选翻页，owner 本地审批的位置 SHALL 明确，服务端路径不得伪装为客户端路径

#### Scenario: 真实验收保持
- **WHEN** 本地测试全部通过
- **THEN** 能力 SHALL 保持 experimental，既有 11 项真实验收不得因此完成

