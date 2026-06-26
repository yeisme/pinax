# 背景

Pinax 已有 `pinax sync daemon run|start|status|stop|logs` 首版，但当前体验仍偏“定时检查”：`run` 启动后等 ticker 才进入同步循环，本地 watcher 代码也没有完整接入主循环。用户需要更明确的后台同步体验：命令启动后立即做首轮同步，随后持续同步，并且能在终端看到实时进度，同时把脱敏事件持久化到 vault 本地运行态目录。

## 目标

- `pinax sync daemon run --target cloud --vault . --yes` 启动后立即执行一轮 pull-before-push 同步。
- 前台 human 模式实时输出 daemon 生命周期和同步进度；`--events` 输出 NDJSON 事件流。
- `.pinax/sync-daemon/events.jsonl` 持久化脱敏事件，`status` 和 `logs` 可读取这些事件。
- 接入本地 watcher/debounce；远端仍通过 head polling 发现变化。
- 保持 `--json`、`--agent`、既有 envelope、facts 和命令名向后兼容。

## 非目标

- 不实现服务端 push、SSE、WebSocket、移动端 push notification。
- 不自动解决 sync conflict，不合并或重写用户正文。
- 不删除或重命名既有 CLI 输出字段、`--agent` key、JSON envelope 或 daemon state 字段。
- 不改变 Cloud Sync 的端侧加密和 `remote_write=true` durable commit gate。

## 兼容性

- CLI 输出合同为 additive：新增事件类型、事件字段和可选 facts/data，不移除旧字段。
- 本地 `.pinax/sync-daemon/*.jsonl` 是 CLI/service-authored structured assets，只由 Pinax 服务写入。
- 事件和运行态文件继续走现有 redaction，禁止持久化 note body、Authorization、token、raw prompt、provider payload 或私有工具参数。
