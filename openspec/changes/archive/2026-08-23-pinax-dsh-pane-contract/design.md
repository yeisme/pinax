# 设计

## 边界

DSH Pane 平台的所有权切分：Harness Plugins 拥有 Host 桥、视图注册、action 准入与 bundle 装配；Pinax 拥有 vault、note、index、graph、history、sync 状态与领域合同。Pinax 侧本变更只交付"领域 owner 合同切片"——纯函数组装器 + 领域测试 + 证据 profile，不引入 DSH 运行时依赖。

## 快照组装

`AssemblePaneSnapshot(projection, context)` 消费进程内 `note.list` 投影（`Data["notes"]` 为 `[]domain.Note`）：

- 实体只暴露 `ref`（note ID）、`version`、有界 `value`（title/kind/status/tags）；`Path`、`Body`、frontmatter 一律不进入 envelope。
- `paneUnsafe` 红线：空 ref、绝对路径（POSIX/Windows）、URL scheme、`token`/`authorization`/`cookie` 子串全部拒绝；不安全实体被跳过（快照仍可下发其余实体）。
- 状态映射：`success`→`ready`；`failed`/`error`/`offline`→`offline`；`permission_denied`→`permission_denied`。注意 `NewErrorProjection` 的 Status 是 `failed` 而不是 `error`，漏掉 `failed` 会把失败投影伪装成 `ready`+空实体（已修复并有回归测试）。
- `OccurredAt`/`ObservedAt` 使用组装时刻的真实 UTC 时间（RFC3339）；`Freshness` 在快照阶段为 `fresh`，负例为 `unknown`。harness 侧 `snapshot.ts` 的固定时间戳是 Host 测试便利，Pinax 作为 canonical owner 不复制该占位。
- `Cursor`/`Sequence` 在快照阶段保持 `c-1`/`-1` 占位，事件流（push/续传）不在本变更范围。

两个组装函数共享 `newPaneSnapshotEnvelope` 构造器，schema/stream/payload 骨架只定义一次。

## Artifact 与门控 action

- `PaneArtifactFromNote` 输出 `pane.artifact.v1alpha1`：owner=`pinax`、mediaType=`text/markdown`、capabilities=`open/link/attach_context`；ref 复用同一 `paneUnsafe` 红线，命中即返回错误（fail closed，不静默跳过）。
- `PaneGatedActions(revision)` 声明 `inbox.capture` 与 `sync.run`：gated=true、携带 expected_revision、幂等与回执要求、给出 owner command。Pane 不得自行组装 canonical 变更。

## 负例

`AssemblePaneNegative(kind, context)`：`offline`、`permission_denied` 返回对应状态的空快照；`handwritten_metadata` 返回 `RejectHandwrittenMetadata` 错误；未知 kind 返回错误。客户端不得以轮询伪装实时——无事件流时 Host 应显示 `offline`。

## 证据

`tools/testkit/integrationevidence` 增加 `dsh-pane` profile（layer=component）：运行 `go test ./internal/app -run Pane -count=1`，extra checks 标记 `pane_snapshot_redaction` 与 `handwritten_metadata_rejected`。

## 风险与权衡

- `paneUnsafe` 已升级为双层红线（2026-08-22）：闭合白名单 `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$` 从结构上拒绝路径/URL/空格/超长，再叠加凭据形黑名单（token/authorization/cookie/secret/password/api_key/bearer）。title 是自由文本但只进入有界 value，不经 ref 红线；后续若放宽 ref 来源（如允许 `/` 分隔的层级 ref），必须同步收紧 value 侧脱敏。
- `paneNotes` 只接受进程内 `[]domain.Note`；其他形状（如 JSON round-trip 后的 `[]any`）直接 fail closed 返回错误。远端消费方接入 RPC/API 边界时需要显式反序列化层，而不是依赖隐式容错。
