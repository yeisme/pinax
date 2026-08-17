## Why

`pinax-agent-continuity-experience` 已在十个独立真实任务中达到首次完成、来源解析、跨 Agent continuation 和七日复用门槛，但证据仍来自单一 operator，且未测量付费意愿。现在需要用一个严格限界的个人 action-capture canary 验证用户是否会持续主动使用“可信上下文 → 明确确认 → 外部任务写入 → 审阅后沉淀”的闭环，而不是把任务级成功误判为多用户需求或直接扩大办公集成范围。

## What Changes

- 建立两周个人 canary：Hermes 作为唯一对话入口，只读调用 Pinax MCP 的 `pinax.agent.context`、`pinax.agent.memory_recall`、`pinax.agent.handoff_read`，生成有来源的任务预览并等待明确确认。
- 将外部任务创建留在 Hermes 已配置网关或 `lark-cli`；Pinax 不连接飞书、不保存任务镜像、不接管负责人、截止时间或状态事实。
- 记录脱敏 canary 指标：预览修改次数、来源可打开率、预览延迟、确认写入、重复/错误任务、活跃日期，以及重要完成任务是否产生并通过 Pinax 审阅提案。
- 保持 Pinax capability 为 `experimental`；验证期只允许 correctness、onboarding、retrieval、source resolution、review efficiency 和 MCP compatibility 修复。
- Pinax 不可用时允许 Hermes 继续输出普通任务草案，但必须明确标记上下文不可用或来源未验证，且不得伪造来源。
- 完成结果只有在用户审阅通过后才能进入 Pinax；原始会话、完整提示词、provider payload 和执行日志不得直接沉淀。
- 不新增稳定 `intent=action_capture` API。只有 canary 达到门槛后，后续独立 change 才能评估 provider-neutral 的实验视图。

## Capabilities

### New Capabilities

- `personal-action-capture-canary`: 定义 Hermes × Pinax 只读上下文预览、明确确认、外部任务委派、脱敏指标和审阅后记忆沉淀的实验性个人闭环。

### Modified Capabilities

无。

## Impact

- `internal/mcpserver/`：只允许标准 MCP 兼容、默认 workspace 和旧 consumer 兼容修复，不加入 provider-specific 字段。
- `internal/testkit/continuitydogfood/`：增加由 CLI 生成的指标分析与 CEO decision receipt，输入仅为脱敏 cohort/follow-up evidence。
- 用户级 Hermes profile：启用 Pinax 只读 MCP、最小工具白名单、action-capture skill 和记忆写入审批；该配置不属于 Pinax 稳定合同。
- Hermes 飞书网关或 `lark-cli`：在用户明确确认后负责外部写入及 task ID 映射；Pinax 不依赖其 SDK、payload 或状态枚举。
- 决策依据：`temp/continuity-dogfood-decisions/20260810T021721Z-3240325/artifacts/decision.json`，结论为 `iterate`，失败类别限定为 `external_validity` 与 `commercial_signal`。
