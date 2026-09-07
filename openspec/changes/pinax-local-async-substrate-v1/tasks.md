## 1. SQLite 统一 DSN 与池界

- [ ] 1.1 统一 DSN helper（WAL + busy_timeout + foreign_keys + 池 4/4）；迁移 `operation`、`promptasset`、`agentmemory`、`memory` 四处打开点。
- [ ] 1.2 守卫测试：禁止裸 `sqlite.Open(path)` 直连（源扫描或 lint 清单）。
- [ ] 1.3 并发读写测试：MCP server + CLI 并行读、写方有界等待。

## 2. sync/delivery job 投影

- [ ] 2.1 status 投影：从 event JSONL 幂等重放（accepted/progress/terminal 汇总）。
- [ ] 2.2 cancel 语义：cancel 标记结构化资产 + 执行器项边界检查 + receipt 保留。
- [ ] 2.3 同作用域单执行器原子 claim（本地文件、幂等抢占、占用错误可解释）。
- [ ] 2.4 事件流对齐 `--events` NDJSON；MCP 只读投影接入 registry。

## 3. MCP 生命周期与文档

- [ ] 3.1 stdin EOF 干净退出 + 重启后投影一致性测试。
- [ ] 3.2 readiness 投影补 lifecycle 事实；`docs/` 本地服务边界说明。

## 验证记录

- 2026-09-07：change 建立，实现未开始。
