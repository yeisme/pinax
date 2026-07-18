# Pinax Memory API 与 Workbench 对齐

## 背景

当前 `pinax memory` CLI 已经具备 `capture`、`list`、`recall`、`context`、`stats` 等能力，但 Local API、RPC 能力注册表和 Workbench 前端尚未对齐。用户在前端使用时无法通过统一能力目录发现 memory 命令，也不能用 HTTP/RPC 复用 CLI 的 dry-run 与确认写入语义。

## 目标

- 为 memory 增加 Local REST 与 RPC 能力，覆盖 CLI 已有的稳定命令。
- `memory.capture` 同时支持正文记录和三元组记录；`dry_run=true` 可预览，真实写入必须显式确认。
- 能力注册表展示 memory 路由、命令、读写边界和可复制 CLI 命令。
- Workbench 增加 memory 页面和 capability explorer，便于浏览、召回、上下文生成、统计和捕获。
- 保持 `pinax vault dashboard` 只读，写入能力只通过 `pinax api serve --allow-write` 暴露。

## 非目标

- 不实现 `memory link` 或 `memory prune`，它们继续保持 CLI 预留/未实现状态。
- 不把 plaintext memory 读写放到 `pinax-cloud`；云端仍只负责同步边界。
- 不变更 memory ledger 存储结构。

## 风险与处理

- HTTP/RPC 是稳定契约面：本变更只新增方法和路由，不删除或重命名已有字段。
- 写入安全：真实 capture 需要 `--allow-write` 服务端开关和 `yes=true`；dry-run 不落盘。
- 前端写入：Workbench 只调用本地 API，不保存凭据或 provider payload。
