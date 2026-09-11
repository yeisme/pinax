## Why

手写 MCP 协议层发生严格客户端工具列表超时，需评估官方 Go SDK 迁移。当前只有调研和任务草案，未接入 SDK。用户随后取消 HTML/MCP Apps，本变更不再包含任何 UI、AppBridge 或浏览器工作。

## What Changes

- 后续设计仅评估 Go MCP SDK 的协议、工具、资源、取消与兼容边界。
- 保留 Pinax 的领域服务、Markdown/结构化结果和真实 CLI 客户端验证。
- 迁移未实施；当前本地运行构建继续使用已修复的 MCP 实现。

## Capabilities

### New Capabilities
- `pinax-mcp-sdk-runtime`: 官方协议运行时迁移候选。

### Modified Capabilities

## Impact

后续范围限 cli/pinax。当前不改变 runtime、go.mod、已配置客户端或 vault。已取消的 HTML 能力不作为 SDK 迁移的前置条件或验收门。
