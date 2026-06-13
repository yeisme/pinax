## ADDED Requirements

### Requirement: CLI Output Mode Contract Validation
CLI 必须对支持渲染输出的命令统一遵循 Summary（默认中文）、Agent（键值事实）、JSON（信封契约）、Events（流式 NDJSON）等渲染模式。测试套件中的断言机制 SHALL 校验其输出格式的完整性与契约一致性。

#### Scenario: Verify JSON Output Envelope Compliance
- **WHEN** 执行 `pinax` 任意带有 `--json` 标志的命令时
- **THEN** 正常输出（stdout）中 SHALL 返回合法的 JSON，且含有 `spec_version`、`mode`、`command` 和 `status` 标准顶层字段

#### Scenario: Verify Agent Output Fact Formatting
- **WHEN** 执行 `pinax` 带有 `--agent` 标志的命令时
- **THEN** 标准输出中的每一行 SHALL 遵循 `key=value` 的格式规则，并且包含必备字段，值中带有空格时使用双引号包裹
