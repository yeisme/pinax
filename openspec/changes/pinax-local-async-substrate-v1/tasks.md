## 1. SQLite 统一 DSN 与池界

- [x] 1.1 统一 DSN helper（WAL + busy_timeout + foreign_keys + 池 4/4）；迁移 `operation`、`promptasset`、`agentmemory`、`memory` 四处打开点。
- [x] 1.2 守卫测试：禁止裸 `sqlite.Open(path)` 直连（源扫描或 lint 清单）。
- [x] 1.3 并发读写测试：MCP server + CLI 并行读、写方有界等待。

## 2. sync/delivery job 投影

- [x] 2.1 status 投影：从 event JSONL 幂等重放（accepted/progress/terminal 汇总）。
- [ ] 2.2 cancel 语义：cancel 标记结构化资产 + 执行器项边界检查 + receipt 保留。
- [ ] 2.3 同作用域单执行器原子 claim（本地文件、幂等抢占、占用错误可解释）。
- [ ] 2.4 事件流对齐 `--events` NDJSON；MCP 只读投影接入 registry。

## 3. MCP 生命周期与文档

- [x] 3.1 stdin EOF 干净退出 + 重启后投影一致性测试。
- [ ] 3.2 readiness 投影补 lifecycle 事实；`docs/` 本地服务边界说明。

## 验证记录

- 2026-09-07：change 建立，实现未开始。
- 2026-09-07：§1 完成。新增 `internal/sqlitedsn`（DSN/Open/OpenReadOnly，WAL + busy_timeout(5000) + foreign_keys + 池 4/4）；迁移 `operation`（Open/OpenExisting）、`promptasset`（OpenVaultRepository）、`agentmemory`、`memory` 四处裸打开点，并同步迁移 `internal/index/store_migration.go`、`internal/cli/completion.go`（只读投影）、`internal/index/gormgen`，使守卫可全量覆盖。测试：`go test ./internal/sqlitedsn/ ./internal/operation/ -race`（DSN 形态/pragma/池界/只读拒绝写/读者不被未提交写阻塞/写方 busy_timeout 有界等待/双侧并行读写/守卫拦截注入样本）全绿。
- 2026-09-07：§3.1 完成。新增 `internal/mcpserver/lifecycle_test.go`：stdin EOF 后 Serve 返回 nil、已接收请求的响应全部排空且逐帧完整；重启前后两个独立会话对同一 vault 的 initialize/resources/tools 投影逐字节一致。进程级验证：`printf <initialize+readiness> | pinax mcp serve` exit=0、两帧完整排空、stderr 无噪声。
- 2026-09-07：§2.1 完成。新增 `internal/app/sync_job_status.go`（`pinax.sync_job_status.v1`）：`ReplaySyncJobStatus` 为事件流纯函数（accepted/progress/terminal 三相、崩溃前缀流→progress、重复终态以最后为准、幂等）；`Service.SyncLogsStatus` 从 `.pinax/events.jsonl` 按 run_id 过滤重放；CLI 接线 `pinax sync logs status <run-id>`。测试：`go test ./internal/app/ -run 'TestReplaySyncJobStatus|TestSyncLogsStatus'` 全绿；进程级验证 human/--json/--agent 三模式与 stdout/stderr 分离（错误 envelope 走 stdout、exit=1、stderr 0 字节）。2.2-2.4（cancel/claim/MCP 投影）未做。
- 2026-09-07：全量回归发现并修复 WAL sidecar 生命周期问题：(1) `sqlitedsn` 登记连接池 + `CloseAll()`，`main()` 退出前调用（checkpoint 回主文件、清理 -wal/-shm，实测 `index refresh` 后 vault 不残留 sidecar）；(2) `index.Diagnose` 增加主文件 SQLite 头校验，热 WAL 不再掩盖主文件损坏（`TestIndexDiagnoseDetectsCorruptMainFileWithHotWAL`）；(3) api 写门禁树快照排除 SQLite 运行时 sidecar（派生运行时状态非 vault 内容）。`go test ./...` 66 包全绿、`golangci-lint run ./internal/... ./cmd/...` 0 issues。
