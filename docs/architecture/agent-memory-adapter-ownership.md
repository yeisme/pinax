# Agent 记忆运行时：适配器归属

<a id="pinax-owns"></a>
## Pinax 负责什么

`cli/pinax` 拥有 Agent 记忆运行时的 canonical 状态与合同：

| 模块 | 职责 |
| --- | --- |
| `internal/agentprotocol` | 版本化 DTO、校验、稳定错误与 adapter descriptor |
| `internal/agentmemory` | 记忆生命周期、scope、来源、proposal、反馈与 repository |
| `internal/agentcontext` | 权限优先的上下文编译、排序与预算 |
| `internal/agentadapter` | 参考适配器 harness：descriptor 协商、转换与降级状态 |
| `pkg/agentmemory` | 面向嵌入消费者的实验性 Go SDK |
| `pinax agent`、`pinax.agent.*`、`Pinax.Agent.*` | CLI、默认只读 MCP 与 RPC 表面 |

<a id="pinax-does-not-own"></a>
## 运行时集成由谁负责

Pinax 提供 `internal/agentadapter/codex.go` 和 `teamruntime.go` 参考 fixture，证明通用 schema 可迁移；它们不等于真实运行时集成。

Codex 的插件安装、hooks、配置读取、子进程、trace 路径和打包属于 Codex owner。团队执行、角色分配、trace 解析与团队状态属于执行 owner；该 owner 的旧实现已于 2026-09-26 退役，重新准入尚未完成。

<a id="team-runtime-consumer-handoff"></a>
## 团队运行时交接

重新获批的执行 owner 需要跨 Agent handoff 或共享记忆时，再接入适配器：

1. 使用 `pkg/agentmemory` 实验 SDK，或调用 `pinax agent` / `pinax.agent.*`。
2. 复用 `yeisme.agent_memory.v1`、`yeisme.agent_context_pack.v1`、`yeisme.agent_handoff.v1`；团队扩展放 adapter metadata，不另造核心字段。
3. worker 默认只能 `propose`；确认写入需要 owner 批准或明确 policy。
4. 通过 `pinax agent feedback add` 提交反馈；反馈不重写记忆正文。

在实际执行 owner 接入前，teamruntime fixture 只证明合同兼容，不要求 Pinax 内嵌其运行时。

<a id="stable-from-criteria"></a>
## 稳定化条件

仍为 `experimental`，待以下证据齐备：两种参考适配器通过测试（真实团队集成另行恢复）；完成任务 8.5 的四周真实使用；完成任务 6.5 的 CLI/MCP/REST-RPC/SDK 对等验证。

此前不弃用 `pinax memory`、`pinax brain` 或 `pinax.brain.*`。
