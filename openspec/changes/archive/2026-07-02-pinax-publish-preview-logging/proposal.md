# Pinax 发布预览日志增强

## 背景

Pinax 当前已经支持 `publish profile init -> plan -> build -> dev/serve -> preview approve` 的本地静态预览链路，但这些命令的运行时反馈主要依赖最终 projection。对 `publish dev --watch`、renderer 调用、扫描、receipt 写入和本地服务启动等阶段，用户只能等待最终结果，自动化也无法稳定消费阶段进度。

## 目标

- 为完整发布预览链路增加人类可读阶段日志。
- 为同一链路增加 `--events` NDJSON 阶段事件，方便脚本和 agent 判断当前阶段。
- 保持 `--json`、`--agent`、`--explain` 单一 projection 输出，不混入进度文本。
- 所有事件和日志都遵守现有脱敏边界，不暴露 vault 绝对路径、token、Authorization/Cookie、provider payload 或私密正文。

## 非目标

- 不新增持久化 publish event log；已有 receipt 继续作为审计证据。
- 不改变已有 JSON envelope、agent key、status enum 或旧事件语义。
- 不改变 publish 构建、部署、分享的业务行为。

## 兼容性

本变更只 additive 新增 `--events` event type 和可选字段；不删除、不重命名、不重释义已有 CLI 输出字段。旧脚本可以忽略新增事件。
