# Proposal

## Why

笔记 inbox 分类、关联与重复线索建议，保持原业务真值与权限不变。

## What Changes

新增显式 opt-in 的公共 SDK consumer、领域问题集/策略、可追溯建议与离线测试；默认关闭，提供拒答、不可用、过期与零网络 replay 行为。

本变更当前仅完成设计准备；新增 SDK、接口、命令和适配器均为拟实施内容，不代表已经安装、发布、部署或启用。

## Capabilities

### New Capabilities

- `inbox-judgment`：笔记 inbox 分类、关联与重复线索建议，保持原业务真值与权限不变。

### Modified Capabilities

无。以显式 opt-in 的新增合同接入，既有默认行为、字段语义和审阅权限保持不变；实现如发现必须改变稳定合同，先补充对应 MODIFIED delta。

## Impact

internal/memoryinbox、internal/search、internal/memory 与既有 vault application service。依赖 [公共 SDK 合同](../../../../../apigateway/aigora/openspec/changes/aigora-structured-judgment-sdk-v1/design.md)；领域代码不内嵌 TypeSafe 协议。[跨项目 handoff](../../../../../openspec/changes/structured-judgment-sdk-adoption-v1/design.md)
