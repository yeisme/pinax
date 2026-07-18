## Context

Pinax 已把每次同步的汇总收据写入 `.pinax/sync-runs/**`，并把一个脱敏的 `sync.run` 摘要追加到 `.pinax/events.jsonl`。`sync logs tail` 当前只做一次性读取，事件没有文件操作粒度，因此用户只能看到“同步成功”，看不到具体上传、下载、删除或冲突了哪些路径。

该时间线是 CLI/service authored structured asset，必须保持 append-only、可截断读取、机器可解析，并遵守现有 `path_policy` 与全局 projection redaction。

## Goals / Non-Goals

**Goals:**

- 在同步完成收据落盘时，为每个计划操作追加一个 `sync.file` 事件。
- 文件事件只包含安全元数据：run ID、方向、操作类型、状态、脱敏路径/路径哈希和时间。
- `sync logs tail` 默认同时读取 `sync.run` 与 `sync.file`。
- `--follow` 从已有尾部事件开始，轮询追加内容并即时渲染新事件。
- 所有输出模式保持各自合同，取消跟随时正常退出。

**Non-Goals:**

- 不记录正文、blob 内容、provider payload、凭据或未脱敏绝对路径。
- 不改变同步传输协议、manifest schema 或远端对象布局。
- 本次不提供字节级传输进度、速率或 ETA。
- 本次不保证事件在单个文件传输开始前出现；事件表示计划操作已形成最终 run 结果。

## Decisions

1. **复用 `.pinax/events.jsonl`，新增可选 `sync.file` 类型。** 这是对现有 append-only 时间线的增量扩展，不新增第二套日志资产。备选方案是在 receipt 中只展示 `operations`，但无法被 `tail --follow` 持续消费。
2. **事件在 `finishSyncRun` 中由已脱敏的 receipt operations 生成。** 这样成功、失败、dry-run 和 approval-required 都共享同一持久化边界，且路径策略只有一个来源。备选方案是在传输循环中直接写事件，能更细粒度但会扩大事务/失败语义，本次延期。
3. **`--follow` 放在 CLI 流式边界，不把 `io.Writer` 注入 Service。** Service 提供读取安全事件的纯应用能力；CLI 负责轮询、上下文取消和选择 renderer，避免业务层依赖终端。
4. **默认 human/agent 每个事件一条记录；`--events` 每个时间线项输出一个 NDJSON `progress` 事件。** `--json` 在非 follow 模式继续返回单个 envelope；为避免无限 JSON 文档，`--follow` 仅允许 summary、agent 或 events。

## Risks / Trade-offs

- [文件事件在 run 完成时批量追加，不是字节级实时进度] → 文档明确语义，后续若需要传输级 progress 再引入执行回调。
- [轮询可能重复输出] → 使用文件偏移或稳定事件游标，仅输出新追加行。
- [日志增长] → 复用现有 prune/保留策略，并保持事件记录紧凑。
- [路径泄漏] → 只从 `syncops.SanitizeOperations` 的结果构造事件，并补 default/hash/omitted 测试。

## Migration Plan

1. 增量写入 `sync.file`，旧 reader 继续忽略未知类型。
2. 新 reader 同时接受旧 `sync.run` 和新 `sync.file`；已有日志无需迁移。
3. 若需回滚，停止写入和展示 `sync.file`，历史附加事件可安全保留。

## Open Questions

- 后续是否需要在 transport 执行循环加入 per-file started/completed 和 bytes transferred 事件。
