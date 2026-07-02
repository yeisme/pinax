# Pinax 性能监控 Trace

## 背景

Pinax 已经有 `activity` 聚合日志，但搜索、索引、查询和 database view 的内部步骤不可见。用户需要看到索引建立、每次搜索、查询执行等步骤，并能查看进程级 CPU、内存、GC 和 goroutine 指标，方便定位慢搜索、懒加载索引、索引刷新和查询失败。

## 目标

- 新增 `pinax monitor` 只读命令面，查看 monitor runs、单次 run、tail、summary 和维护状态。
- 为 `index.init`、`index.refresh`、`index.rebuild`、`index.repair`、`note.search`、`query.run`、`dataview.run`、`database.view.render` 写入步骤级性能 trace。
- 保存进程级 runtime/OS 指标：wall time、Go heap、alloc delta、GC delta、goroutine、process user/system CPU、RSS/peak RSS。
- 将 monitor runs 聚合进 `activity` 新 source：`monitor_runs`。
- 新增 readonly REST/RPC/capability：`/v1/monitor/runs`、`/v1/monitor/runs/{run_id}`、`/v1/monitor/summary` 和 `Pinax.Monitor.*`。

## 非目标

- 不实现 pprof 火焰图、函数级采样或持续 daemon profiler。
- 不保存 note body、raw query、provider payload、token、hidden/system prompt 或完整推理。
- 不改变现有 search/index/query/activity 输出合同；本变更只做 additive 扩展。

## 兼容性

- CLI：新增 `monitor` 命令，不删除或重命名现有命令。
- API/RPC：新增 readonly route/method/capability，不改变既有路径语义。
- 存储：新增 `.pinax/monitor/**` telemetry 结构化资产，不迁移既有资产。
- 回滚：删除新增代码后，旧版本会忽略 `.pinax/monitor/**`；可手动清理该 telemetry 目录，不影响 vault notes、record ledger 或 index。
