# Design

支持集由现有 descriptor 扫描锁定，至少覆盖 object、array、required、additionalProperties、properties、items、enum、const、长度/数值边界和 pattern。未知 keyword 在测试和启动验证中 fail closed。

## 扫描锁定的 keyword 集（task 1.1）

断言 keyword（runtime 强制执行）：`type`（object/array/string/integer/number/boolean）、`properties`、`required`、`additionalProperties`（bool 或 schema）、`items`、`enum`、`const`、`pattern`、`minLength`/`maxLength`、`minimum`/`maximum`/`exclusiveMinimum`/`exclusiveMaximum`、`minItems`/`maxItems`。

注记 keyword（接受但不强制，draft 2020-12 语义）：`description`、`title`、`default`、`examples`、`format`。

文档根 keyword（仅限根）：`$schema`、`$id`。

其余 keyword（`$ref`、`oneOf`/`anyOf`/`allOf`/`not`、`patternProperties`、`minProperties`/`maxProperties`、`uniqueItems`、`multipleOf` 等）一律拒绝：`registeredTool` 启动即 panic，guard test 对全部 tool input schema 扫描。leaf keyword 形状（数值边界必须是数字、`enum`/`required` 必须是数组）同样在发布时校验，畸形边界不会在 runtime 被静默当作缺失。

实现位置：`internal/mcpserver/schema_validation.go`（validator + guard）、`registry.go` registeredTool fail-closed 接线、`schema_validation_test.go`（guard 正/负例 + 子集语义表测 + wire 级嵌套拒绝）。
