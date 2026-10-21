## ADDED Requirements

### Requirement: 固定版本无损编辑源码

Pinax MUST 提供 `GET /v1/writer/notes/{object_id}/editor-source`，要求明确的当前 revision，返回无损 Markdown 正文、整文 revision、body digest、受保护 metadata 和预算。不得从阅读 AST 或导出投影反拼正文，不得隐式更新到不同 revision。

#### Scenario: 已授权编辑会话读取

- **WHEN** 会话同时拥有 `writer.editor` 和 `reader.body`，对象可见且 revision 匹配
- **THEN** 返回封闭 `pinax.writer.editor_source.v1alpha1` 投影，正文保留空白和换行，frontmatter 与私有路径不进入正文
- **AND** 不初始化 vault、不更新索引、不创建快照或 operation

#### Scenario: 权限不完整或已撤销

- **WHEN** 缺少任一 scope、bearer 无效或会话被撤销
- **THEN** 拒绝访问且不返回正文，所有响应为 no-store

#### Scenario: 版本冲突和超限

- **WHEN** 请求 revision 过期或完整源文件超过 64KiB
- **THEN** 分别返回 typed revision conflict 或 body_too_large，不截断正文或盲目返回新版本

### Requirement: 受保护元数据与既有写入门

Pinax MUST 将编辑正文与 metadata 分离，并保持现有提案、差异确认、apply 与原 operation receipt 写入流程。新接口不等于写入许可。

#### Scenario: 既有 splitter 无法保证无损保存

- **WHEN** 特殊 frontmatter 与旧 collaboration splitter 的正文边界不一致
- **THEN** 返回 `editable=false` 和明确 readonly_reason，客户端不得调用普通文件保存替代 Pinax 提案

#### Scenario: 确认提案后保存

- **WHEN** 人确认了 exact before/after，apply 使用相同 operation、expected_revision、preview_digest
- **THEN** 使用既有快照和 owner 写入门，返回 receipt；重放同 operation 不产生第二次写入

### Requirement: 来源隔离和机器合同一致性

Pinax MUST 排除隐藏、符号链接、非普通文件与无效 trust metadata；重复 object ID 必须拒绝。新增 schema、catalog、OpenAPI 与 typed client 保持一致，并证明旧合同未变。

#### Scenario: 隐藏对象或重复身份

- **WHEN** ID 仅出现在隐藏/链接对象，或可见范围内存在重复 ID
- **THEN** 前者不可读，后者返回明确 identity ambiguity；不暴露文件系统路径

#### Scenario: 非法上游投影

- **WHEN** typed client 收到错误对象、revision、digest、预算或不受支持 schema
- **THEN** 在返回正文给消费方之前拒绝投影，不自动重试写入
