## ADDED Requirements

### Requirement: 部署端工具与同源访问

获批客户端与运行时 MUST 将一个服务部署作为文件、工具与执行环境边界；浏览器只通过该部署的认证 facade 使用能力，Runtime Plane 本机 broker 不因远程浏览器而被直接公开。

#### Scenario: 同一版本部署到不同机器
- **WHEN** 同一版本分别部署在个人电脑和服务器
- **THEN** 每个实例只发现并使用本部署授权的工具和工作目录，浏览器位置不改变工具选择或授权

#### Scenario: 远程页面指定本机路径
- **WHEN** 浏览器提交用户电脑路径或任意 owner endpoint
- **THEN** 服务拒绝把它作为已授权资源；文件只能经显式 owner import 形成可用引用

### Requirement: Canonical owner 分工

Workbench MUST 只组合安全项目、会话、运行与成果投影；Pinax 保持 continuity/memory 权威，Conversation Runtime 保持 content/runtime summary 权威，领域 owner 保持 artifact/version 权威，执行 owner 保持 Team 调度权威（原 owner 已退役，待重新准入）。

#### Scenario: Runtime 产生摘要
- **WHEN** 会话结束后 Runtime 生成工作摘要
- **THEN** 摘要可以作为临时来源显示，但不能自动创建、确认或覆盖 Pinax 长期决定

#### Scenario: 项目未绑定 Pinax
- **WHEN** 项目 continuity binding 缺失或歧义
- **THEN** 返回明确恢复状态，保留已授权项目浏览；不得猜测 scope 或虚构上次工作进度

### Requirement: 续接保留来源与执行身份

续接消费者 MUST 在显式继续动作时验证来源版本、授权、binding 与原 attempt；打开页面不启动任务，换 Runtime 不重放旧工具。

#### Scenario: 上次操作结果未知
- **WHEN** 恢复项目时原 attempt 为 `unknown_accept`
- **THEN** 只提供原操作的 reconcile，不以新 session 或新 idempotency key 重新执行

#### Scenario: 来源发生冲突
- **WHEN** 两个已授权来源对同一决定存在冲突
- **THEN** continuity projection 显示冲突和处理入口，不按排序或更新时间静默选边

### Requirement: Owner packet 与成熟度独立

每个跨项目接入 MUST 有版本/digest 可核验的 provider packet、consumer conformance 与能力范围；fixture、真实 owner、部署和生产成熟度 SHALL 分别记录。

#### Scenario: 只有 UI fixture 通过
- **WHEN** UI 和 mock contract 检查通过但真实 Agent provider 未接通
- **THEN** 只标记对应测试层通过，真实运行能力保持 blocked/needs_contract

#### Scenario: Provider 缺少消费合同
- **WHEN** Pinax 或成果 owner 缺少必须的安全投影或恢复方法
- **THEN** 在该 owner 自己的 OpenSpec 中交付增量；Workbench 和根级设计不得复制其业务状态代替接入
