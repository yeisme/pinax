# pinax-realtime-sync-daemon Proposal

## 背景

Pinax 已经具备 Cloud Sync 的显式 `diff`、`push`、`pull` 能力，但用户想要更接近 Obsidian Sync 的体验：本地 Markdown vault 变化后无需手动执行同步命令，另一台设备也能通过后台轮询及时拉取远端修订。

现有边界仍然成立：Local Vault 是真源，Cloud Sync 只协调密文，`pinax api serve` 不是 Cloud Sync transport，`remote_write=true` 只能在 durable revision commit 和本地 sync-state 证据写入后出现。本变更只新增一个本地后台同步进程，复用既有 Cloud Sync engine，不把 Pinax Cloud Server 改造成明文云笔记后端。

## 目标

- 新增显式管理的 per-vault 后台同步进程，支持 `pinax sync daemon start|run|status|stop|logs`。
- 本地文件变更通过 watcher + debounce 触发 push；远端变化通过周期性 remote head poll 触发 pull。
- 同步循环复用现有 `SyncDiff`、`SyncPull`、`SyncPush`、conflict preservation、sync receipt、redaction 和 `remote_write=true` 规则。
- 后台进程持有单 vault 锁，避免多个进程同时对同一 vault 执行 Cloud Sync 写入。
- 在 watcher 不可用、事件丢失、网络失败、revision conflict、provider throttling 时进入可诊断的 degraded/backoff 状态，而不是静默丢同步或无限重试。
- 为人和 Agent 提供稳定的状态、事件和日志输出，且不泄漏明文 note body、raw secret、provider payload 或 private tool arguments。

## 非目标

- 不新增 Cloud Sync transport，不改变 server/S3/rclone/embedded transport 协议。
- 不实现远端主动推送、SSE、WebSocket 或移动端 push notification；首版远端变化通过轮询发现。
- 不把 `.pinax/**`、SQLite、LanceDB projection、provider cache 或运行态 PID/state 文件作为 Cloud Sync 内容。
- 不实现系统级安装器、开机自启注册、托盘 UI、TUI 或跨平台 service manager；用户可用 `pinax sync daemon run` 交给 systemd、launchd、Windows Task Scheduler 或外部 supervisor。
- 不绕过 `--yes` 授权、snapshot/conflict gate、path redaction、sync receipt 或输出合同。

## 合同兼容性

- CLI 命令面：新增 `pinax sync daemon ...` 子命令，属于 additive 变更。
- CLI 输出：新增 `command=sync.daemon.*`、新增可忽略的 facts/actions/evidence 字段，属于 additive 变更。
- Events：新增 `sync.daemon.*` event type，旧 consumer 可忽略，属于 additive 变更。
- Config/state：新增 `.pinax/sync-daemon/**` CLI-authored structured assets，属于 additive 变更。
- 既有 `pinax sync diff|push|pull` 语义、`remote_write=true` gate 和 Cloud Sync transport 合同不变。

