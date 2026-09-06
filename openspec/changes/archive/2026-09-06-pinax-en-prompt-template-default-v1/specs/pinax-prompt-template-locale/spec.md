## ADDED Requirements

### Requirement: Pinax 必须默认请求英文 Agent 模板
Pinax SHALL 在未指定 prompt template locale 时使用 `en`。

#### Scenario: Agent 搜索模板
- **WHEN** 调用方省略 locale
- **THEN** Pinax SHALL 请求英文 catalog 结果

### Requirement: 业务内容语言必须独立
中文笔记、会议资料和最终笔记语言 MUST NOT 因模板默认英文而被强制翻译。

#### Scenario: 中文业务输入
- **WHEN** 用户把中文内容绑定到英文模板
- **THEN** Pinax SHALL 保留输入并按 contract 处理

### Requirement: 旧 exact address 必须保持可检查
Pinax MUST NOT 用默认 `en` 覆盖 exact template address 中明确的旧 locale。

#### Scenario: 检查旧中文 exact address
- **WHEN** 地址包含 `locale=zh-CN`
- **THEN** Pinax SHALL 按地址处理或返回真实兼容错误
- **AND** SHALL NOT 仅因默认值产生 address mismatch
