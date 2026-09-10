## Context

在现有 owner application service 上增量实现输入控制与字节传输。

## Decisions

先发现权限与传输 readiness，再按文件元数据选择自动上传或 awaiting_file。单次页面兑换及短时 grant 不接受长期 bearer。状态和文件由本项目持有；不引入中央上传服务。持久化、并发、媒体限制、幂等、恢复和导入服从本项目现有规则。

## Verification

复用项目测试 runner；验证自动/手动、重启、续期、取消、重复完成、身份和项目隔离、容量/类型校验。集成结果写入本项目 temp/integration-test-runs。

## Rollback

默认关闭。禁用入口保留已完成文件和旧接口，不自动部署或变更凭据。

## 最终接线

新 typed tools 为 `pinax.input.prepare/status/renew/abort/preview/apply`。MCP serve 只有同时显式配置两个 input HTTP 参数才启用写入。2 MiB 上限覆盖已验的 Markdown/文本与 PNG/JPEG/WebP/PDF 附件。preview 使用 `input_request_id`、`conflict`（skip/rename/overwrite，默认 skip）和附件所需 `note_ref`。apply 另需 `confirm:true` 与原 `preview_digest`。目标内容变化时摘要冲突；未知 apply 结果标记 unconfirmed，不能盲目重放原写入。旧只读会话不注册新 tools。

运行与兼容说明见 [输入指南](../../../docs/mcp-input-intake.md)。输入表增量创建，不修改旧资产表；关闭入口回滚保留已完成引用，未完成请求经原 owner 取消。公共 stateless helper 注入本 owner repository/backend；Radar 使用本项目 Bun/Drizzle 实现同合同。传输凭据不进入普通 JSON 回执。
