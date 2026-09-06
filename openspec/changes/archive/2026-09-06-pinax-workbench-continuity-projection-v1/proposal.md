## Why

根仓 `workbench-project-continuity-program-v1` 任务 1.1（2026-09-06 只读核对）确认：Pinax 现有 continuity 合同（`pinax-trusted-continuity-dogfood-v1` 冻结面）已覆盖零配置 resume、CLI-authored bounded binding registry、partial/conflict/missing 语义、checkpoint proposal/review 与 fail-closed 歧义处理（`continuity_binding_ambiguous` 等），但 Workbench 作为 typed consumer 仍缺一层安全投影合同。本 change 承接该缺项清单；全部改动 additive，不修改已冻结 dogfood 合同。

## What Changes

- 新增 Workbench 消费用 typed projection facade：将 `pinax continue`（Resume Card machine data）、binding status 诊断与 `continue checkpoint` 的 machine envelope（合同 identity/version/digest、稳定错误码、恢复 action）固化为 Workbench BFF 可稳定消费的合同。
- 新增 safe `projectRef` ↔ continuity binding 映射语义：Workbench 只持 opaque projectRef，不读 binding registry、不按目录名/最近使用猜 scope；missing/ambiguous/invalid 一律透传 Pinax 错误码与唯一下一步恢复 action。
- 新增 provider packet 生成入口：按根 provider packet 格式（合同 identity/version/digest、支持动作及 effects、scope/revision、receipt、错误/恢复语义、availability、固定调用入口、脱敏证据引用）输出，供根仓接收。
- 明确投影时效：Workbench 持有的安全投影必须 time-bounded（freshness 来自 evidence observed/revision，非生成时间）；过期只能重新解析，不得本地续命或缓存改写。
- 非目标：不新增跨 vault 搜索、不裁定 confirmed memory、不拥有会话正文/Runtime 内容、不在 Workbench 建 canonical state、不改 dogfood 已冻结六周 scope。

## Capabilities

### New Capabilities
- `spec:pinax-workbench-continuity-projection` — typed facade、projectRef 映射、provider packet 与投影时效的安全消费合同。
