# Pinax MCP JSON Schema 子集补齐

## Why

手写 validator 只覆盖 type/enum，不能保证已发布 schema 的边界、嵌套与额外字段约束。

## What Changes

- 完整实现仓库实际使用的 JSON Schema 子集。
- 增加 keyword guard，禁止 descriptor 引入 validator 不支持的 keyword。
- 保持 dual-era server 和 hand-written runtime。

## Impact

影响 schema validator 与 contract tests，不引入第三方 runtime。
