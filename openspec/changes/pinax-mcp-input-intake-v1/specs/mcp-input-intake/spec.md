## ADDED Requirements

### Requirement: 自描述的受限输入请求
Pinax SHALL 通过真实 MCP 注册表和 `pinax://input/capabilities` 提供请求 schema、HTTP 合同、身份限制和显式可用状态。新增入口默认关闭，旧凭据不自动扩权。

#### Scenario: 无产品 CLI 且尚未选文件
- **WHEN** 已授权客户端用原任务用途和幂等键创建请求而没有 file
- **THEN** 返回 awaiting_file、稳定 request ID 和独立的临时传输/页面链接
- **AND** 不读客户端路径或请求 MCP base64 字节

### Requirement: 校验、恢复与原生领域回执
Pinax SHALL 复用本项目持久化和领域服务，一个请求仅绑定一个文件；上传、领域消费和 canonical 采纳分别表示。

#### Scenario: 中断后重放完成
- **WHEN** 客户端通过原请求查询并重复 complete
- **THEN** 返回同一稳定引用，不重复创建文件资产或生成任务
- **AND** 缺少领域权限时保留已传输状态并停在原业务门

### Requirement: 一次性页面和传输边界
Pinax SHALL 只持久化 grant 摘要，绑定身份、项目、请求、HTTP 方法与字节限额；页面凭据仅在 fragment 中单次兑换，并校验同源请求。

#### Scenario: 非法输入与错误身份
- **WHEN** 请求带错误身份、项目、过期凭据、路径穿越、错误 MIME 或超限内容
- **THEN** 请求被拒绝，回执与审计不得暴露长期密钥或临时链接

### Requirement: Owner 分域消费
Pinax SHALL 遵循下列领域边界：新 typed tools 为 `pinax.input.prepare/status/renew/abort/preview/apply`。MCP serve 只有同时显式配置两个 input HTTP 参数才启用写入。2 MiB 上限覆盖已验的 Markdown/文本与 PNG/JPEG/WebP/PDF 附件。preview 使用 `input_request_id`、`conflict`（skip/rename/overwrite，默认 skip）和附件所需 `note_ref`。apply 另需 `confirm:true` 与原 `preview_digest`。目标内容变化时摘要冲突；未知 apply 结果标记 unconfirmed，不能盲目重放原写入。旧只读会话不注册新 tools。

#### Scenario: 上传成功但业务尚未批准
- **WHEN** 字节校验成功而下游审核、导入确认或分析许可尚未满足
- **THEN** 回执分别说明输入引用与 domain_state，不执行未授权业务
