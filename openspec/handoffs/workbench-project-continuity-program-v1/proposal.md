## Why

用户已选择把 Workbench 推进为个人高频使用的通用 Agent 工作台，以项目上下文和成果连续性衡量价值。跨会话续接、有界自主执行和部署端工具使用需要明确 owner 合同，防止客户端重复建立记忆、文件、调度或授权真相。

## What Changes

- 冻结统一 C/S 语义：服务部署在哪里，就使用哪里的授权工具、文件和执行环境；Web 是同源访问入口。云端、本地是部署位置，首版不要求跨部署执行或同步。
- Workbench 组合项目总览、会话、运行、成果与 Pinax continuity projection；Pinax 保留 canonical continuity/memory，Conversation Runtime 保留会话正文和运行时摘要，领域 owner 保留文件及正式版本。
- 规划独立、有界的 action grant 扩展：用户明确批准后，服务端可以用该批准满足范围内的 action proposal；既有 chat/session/access grant 不能自动升级为写权限。
- 明确同一 TaskService/ProposalAuthority/owner receipt 链、逐次准入、撤销、预算预留、未知结果对账和跨项目隔离。
- 将实现任务落到 Workbench 子项目；其他 owner 在实施前提供版本化 provider packet 或在自己的 OpenSpec 中记录合同缺口。根目录只维护边界与 handoff。

### Required Capability Ledger

| ID | 用户要求 | 准入 | Canonical owner | 可见宿主 | 切片与证据 |
|---|---|---|---|---|---|
| R1 | 通用 Agent、个人高频工作 | required / split-owner | Conversation Runtime、工具/领域 owner | Workbench | 真实执行与成果 receipt |
| R2 | 项目与成果优先、桌面交互 | required / fit | Workbench composition | 单一 `/agent` | 桌面任务流与无障碍证据 |
| R3 | 跨会话上下文与成果连续性 | required / split-owner | Pinax、Runtime、成果 owner | 项目续接视图 | 来源版本、冲突、重启续接 |
| R4 | 授权范围内自主推进 | required / split-owner | ProposalAuthority、TaskService、Runtime/工具 owner | Profile、Run、Review | scope/revoke/budget/unknown 验证 |
| R5 | 云端 Web 与本地 Web 服务 | required / split-owner | Workbench 部署、Identity、部署端运行时 | 同一 Web 客户端 | 同版本两种部署拓扑验收 |
| R6 | 研发、调研、文本、多模态场景 | exploratory matrix / split-owner | 既有领域 owner | 共用 document/Pane | 分场景 readiness；不裁掉已有专业能力 |

### Non-goals

本 change 不实现子项目代码，不建立新的执行或记忆 owner，不创建远程仓库，不执行部署，不为浏览器增加任意路径/命令/凭据访问。原生桌面封装、跨部署同步、云端控制用户电脑不属于本次切片；已有多租户、团队与专业能力路线继续保留。

## Capabilities

### New Capabilities

- `project-continuity-owner-contract`：部署本地性、项目关联、Pinax/Runtime/成果 owner 分工、provider packet 与真实交付门。
- `bounded-project-autonomy`：独立 action grant 的授权、逐动作验证、撤销、恢复和跨 owner 回执边界。

### Modified Capabilities

无。新增能力不重新解释现有 session/access grant、Task、Pinax memory 或 owner receipt；未启用扩展时既有 proposal-first 合同保持原义。

## Impact

- 上位运行时边界：现有 `agent-conversation-runtime-workbench-integration-v1`；其未来 `agent/conversation-runtime` owner 建立和真实协议门继续独立处理，不在 Workbench 内替代实现。
- Pinax 消费基础：`cli/pinax/docs/product/trusted-agent-continuity.md`，后续对外投影增量必须在 Pinax 自己的 change 中实施。
- Runtime Plane 继续只提供固定 adapter 的受认证本机 broker；执行 owner（原 owner 已退役）、Aigora、credentialctl、Identity 与领域 owner 的既有权限和状态权威不移动。
- 兼容性：新增设计合同为 pre-1.0；`breaking_surfaces: []`。当前仅交付规格与文档，不宣称 wire digest、provider readiness 或生产可用。
