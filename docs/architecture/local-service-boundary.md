# Local Service Boundary（本地优先服务边界）

Status: active · Owner: `cli/pinax` · Source of rule: `openspec/changes/pinax-local-async-substrate-v1`

Pinax 是本地优先的统一笔记 Agent CLI。本文冻结 Pinax 的服务化路径边界：什么常驻、什么瞬态、远程如何到达本地。

## 边界规则

1. **无常驻 daemon、无非 loopback 监听端口。** Pinax 不维护后台服务进程；所有命令都是短生命周期进程。`pinax sync daemon run` 是用户显式启动的前台进程，退出即消失，不注册系统服务自启。
2. **本地消费面是 stdio。** `pinax mcp serve` 在 stdio 上提供只读 MCP surface（六层 readiness + lifecycle 事实投影到 `pinax://readiness`），由用户或 gateway 显式启动。
3. **远程消费必经 gateway。** 远程 Agent 一律经 `mcp/gateway`（deny-by-default）前挂本地 stdio MCP；Pinax 自身不开公网端口、不做远程鉴权。
4. **share/publish 是显式瞬态投影。** `pinax share` / `pinax publish` 产生 token 门的临时 HTTP 投影，是显式动作的结果，不是服务面扩张。
5. **远程 mutation 走 gateway 审批 + 本地结构化资产。** MCP/tool/provider 不得绕过 app service 直接写 vault；远程写请求经 gateway 审批面落地为本地 CLI-authored 结构化资产（receipt、operation ledger 等）。

## stdio 生命周期（gateway supervise 语义）

- **stdin EOF = 干净退出。** gateway 关闭上游 stdin 后，`pinax mcp serve` 排空已接收请求的响应、以 exit 0 退出，不留孤儿进程。
- **重启后投影一致。** registry/resource/tool 投影全部派生自 vault 状态与静态 manifest，无进程内可变缓存；崩溃重启后同一 vault 的投影逐字节一致。
- **readiness 投影 lifecycle 事实。** `pinax://readiness` 在六层 readiness 之外携带：
  - `lifecycle_exit=stdin_eof_drain_exit`
  - `lifecycle_restart_projection=vault_state_consistent`
  - `lifecycle_remote_writes=gateway_approval_only`

## SQLite 索引的并发与退出契约

- 所有 SQLite 打开点（operation、promptasset、agentmemory、memory ledger 及后续新增）必须经 `internal/sqlitedsn` 统一 DSN：WAL + busy_timeout(5000) + foreign_keys + 池 4/4。禁止裸 `sqlite.Open(path)` 直连（守卫测试强制）。
- WAL 语义：N 读者 + 1 写者。`pinax mcp` 与另一 CLI 命令并发读同一 vault 索引不串行、不踩 `SQLITE_BUSY`；写方经 busy_timeout 有界等待。
- **进程退出契约：** CLI 进程退出前调用 `sqlitedsn.CloseAll()`，SQLite 在最后一个连接关闭时把 WAL checkpoint 回主库文件并移除 `-wal/-shm` sidecar，vault 树不残留运行时产物。被 SIGKILL 残留的 sidecar 由 SQLite 下次打开自动恢复；vault `.gitignore` 白名单不追踪这些运行时产物。
- **损坏检测独立于 sidecar 状态：** `pinax index doctor` 先校验索引主文件 SQLite 头（前 16 字节魔数），热 WAL 不能掩盖主文件被截断或覆盖。

## sync/delivery 长任务可观测性

- sync run 的事件（逐项 `sync.file` + 终态 `sync.run`）持久化在 `.pinax/events.jsonl`。
- `pinax sync logs status <run-id>` 从 event JSONL 幂等重放 job status（`pinax.sync_job_status.v1`）：accepted / progress / terminal 三相，崩溃前缀流重放为 progress，与 receipt（`sync logs show`）互补。
- cancel 语义、同作用域单执行器 claim 与 `--events` NDJSON 对齐属后续增量，见 change tasks §2.2-2.4。
