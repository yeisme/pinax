# Tasks

- [x] 1.1 盘点并锁定已使用 keyword 集合。（design.md 记录断言/注记/根三层 keyword 清单，其余 fail closed）
- [x] 1.2 实现嵌套、边界、pattern 和 additionalProperties 校验。（internal/mcpserver/schema_validation.go 重写 matchesToolArgumentSchema：object/array/string/integer/number/boolean + enum/const/pattern/长度/数值/条目边界 + 嵌套 required/additionalProperties bool|schema）
- [x] 1.3 增加 unsupported keyword guard tests。（TestToolInputSchemasUseSupportedKeywordsOnly + TestValidateSchemaKeywordSupportRejectsUnsupportedKeywords：oneOf/$ref/patternProperties/minProperties/uniqueItems/deprecated/畸形边界/嵌套 $id；registeredTool 启动 panic fail-closed）
- [x] 1.4 运行 Pinax MCP 与 dual-era tests。（go test ./internal/mcpserver/ ./internal/transportcatalog/... 全绿；TestInvalidToolArgumentsFailBeforeApplicationService 等既有 dual-era 契约测试不变通过；golangci-lint 0 issues）
