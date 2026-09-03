# Proposal: pinax-knowledge-source-adapter-v1

## Why

root `inferrum-enterprise-multimodal-knowledge-v1`（task 3.1）要求 Pinax 提供可选的 knowledge source adapter：向 Inferrum 发布 allowlisted projection（note/chunk/source ref、permission、citation、freshness、revocation），Pinax vault/Markdown 仍为真源。该任务不阻塞 Inferrum-native + LanceDB local first-support。

## What Changes

- 新增 `pinax knowledge export-projection`（暂名，实现可定）命令：按 allowlist 导出 vault 投影包（note/chunk refs、digest、permission、citation 元数据、freshness、revocation 标记）。
- 导出为 provider-neutral 包（refs-only）；不直接读 Inferrum/txtai DB，不导出未授权完整 vault。
- 可选调度：手动/定时导出 + 变更检测（增量）。
- additive：既有 sync/note 命令与 vault 结构不变。

## Capabilities

### New Capabilities

- `pinax-knowledge-source-adapter`：allowlisted projection 导出、permission/citation/freshness/revocation 元数据与增量检测合同。

### Modified Capabilities

（无。）

## Impact

- 代码：knowledge projection 导出模块 + allowlist 配置。
- 消费方：Inferrum ingestion（其 owner change 拥有消费端）。
- 非目标：Inferrum DB 直写、完整 vault 复制、检索实现。
