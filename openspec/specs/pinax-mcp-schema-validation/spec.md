# pinax-mcp-schema-validation Specification

## Purpose
TBD - created by archiving change pinax-mcp-schema-validation-v1. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL validate every published schema keyword it uses

Pinax SHALL 完整执行仓库 descriptor 实际使用的 JSON Schema 子集，并拒绝未受支持 keyword 进入 catalog。

#### Scenario: Descriptor adds an unsupported keyword

- **WHEN** 新工具 schema 使用 validator 未实现的 keyword
- **THEN** guard test 或启动校验 SHALL 失败，工具不得发布

