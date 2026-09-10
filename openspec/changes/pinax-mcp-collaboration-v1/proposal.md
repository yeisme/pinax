## Why

Pinax 已有只读 MCP，但客户端收到 JSON，无法完成正文审阅与日常笔记保存闭环。用户要求本机单用户的普通 MCP 与 Markdown 对话交互；2026-09-10 已明确取消 HTML/MCP Apps。

## What Changes

- 新增默认关闭的 collaboration、正文读取和笔记写入选项，旧工具及 JSON 资源原义不变。
- application service 提供预览、版本检查、快照保护、幂等写入和操作查询；MCP 不直接访问笔记存储。
- 新增 Markdown 资源，复用已有输入请求服务。
- 增加协议、失败恢复及兼容验证，区分自动测试和真实客户端验收。

## Capabilities

### New Capabilities
- `pinax-mcp-collaboration`: 可选的人机笔记协作接口。

### Modified Capabilities

## Impact

归属 cli/pinax，分类 fit。保留 CLI flags、普通 MCP tools/resources 和 application API。按用户决定撤回未发布的实验 HTML 资源、扩展声明和 UI 元数据；客户端重连刷新 discovery。无数据库表变更，已完成笔记和操作账本保留。远程部署、独立客户端、批量删除和发布不在范围内。
