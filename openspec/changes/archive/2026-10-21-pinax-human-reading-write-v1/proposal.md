## Why

pinax-human-reading-api-v1 已交付只读人读面；DriveBridge 下一阶段需要在阅读空间内受控地捕获、编辑与整理笔记。写入必须走 owner 门禁与既有提案/评审/恢复语义，不能因为换了 HTTP 消费者就引入自我批准或普通文件覆盖旁路。本 change 只冻结后续受控写的 owner 合同，不实现任何写入端点。

## What Changes

- 新增 `human-reading-write` capability 规格：捕获（capture）、编辑提案（propose/preview/apply）、整理（organize）在人类会话上的授权、评审、快照、revision 与 operation/receipt 语义。
- 能力缺口以本 change 的审计表为唯一输入：只补齐审计出的缺口，不复制已有 REST/RPC/CLI 写入面。
- 首片（本 change）只交付合同文档与任务拆分；实现进入后续任务，按 P01–P09 同款 lane/profile 门推进。

## Capabilities

### New Capabilities

- `human-reading-write`：reader 会话之上的受控写合同——独立 write scopes、host 评审、snapshot/revision 门、operation receipt 与 unknown 恢复。

### Modified Capabilities

无。既有 inbox/draft/folder/memory 写入、agent note proposals、MCP collaboration preview/apply、operation.show/reconcile 合同全部保持不变；本 change 不在其上新增领域行为。

## Impact

未来实现复用 internal/app 既有 service 与 operation 框架，只新增 reader 相邻的 HTTP 投影与 pkg/pinaxclient typed calls；不引入 DriveBridge 领域代码、不新增 React/Electron、不绕过 CLI-authored structured assets 约束。详细审计与决策见 [design.md](design.md)。
