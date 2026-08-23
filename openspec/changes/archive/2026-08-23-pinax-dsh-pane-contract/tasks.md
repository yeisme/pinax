## 任务

- [x] 1. `internal/app/pane.go`：快照/artifact/门控 action/负例组装器 + `paneUnsafe` 红线；`AssemblePaneSnapshot`、`AssemblePaneNegative` 共享 `newPaneSnapshotEnvelope`。
- [x] 2. 修复 `AssemblePaneSnapshot` 状态映射：`failed` 投影映射为 `offline`，不得输出 `ready`+空实体；增加回归测试 `TestAssemblePaneSnapshotMapsFailedProjectionToOffline`。
- [x] 3. `OccurredAt`/`ObservedAt` 使用真实 UTC 时间；消除 `AssemblePaneNegative` 的 ineffectual assignment。
- [x] 4. `tools/testkit/integrationevidence` 增加 `dsh-pane` component profile（`pane_snapshot_redaction`、`handwritten_metadata_rejected`）+ profile 单测；gofmt 通过。
- [x] 5. `internal/app/pane_test.go`：路径/凭据泄漏扫描、artifact 绝对路径拒绝、手写 metadata fail closed、offline/permission_denied 负例。
- [x] 6. `docs/interfaces/dsh-pane.md`：面向 DSH Host 消费方的接口文档（envelope、红线、门控 action、负例、事件约束）。
- [x] 7. 归档前评审：确认 harness-plugins `dsh-pinax-pane-v1` 的 Host 侧消费与本规格一致；事件流/cursor 续传需求出现时另立变更。（2026-08-23 关闭：harness `dsh-pinax-pane-v1` 已归档落地，`packages/client/ui-pane-domain/src/pinax.ts` 为本合同 TS 侧移植——同封闭 allowlist 正则与 7 项 denylist、failed/error/offline→offline 与 permission_denied 映射、inbox.capture/sync.run 门控 action 与 owner 命令、手写 metadata fail closed 全部一致；harness 侧 66/66 conformance 绿，证据 temp/integration-test-runs/dsh-pinax-pane-v1-20260823T081658Z-3785522。事件流/cursor 仍为占位，需求出现时另立变更。）
- [x] 8. Review 硬化收尾（2026-08-22）：新建笔记路径（`note.new`、journal ensure、`project.item.add`）统一 `atomicWriteFile`；`paneUnsafe` 升级为闭合白名单 `paneRefPattern`（拒绝对路径/URL/空格/超长）+ 扩展黑名单（token/authorization/cookie/secret/password/api_key/bearer）；`paneNotes` 对非 `[]domain.Note` 形状（如 JSON round-trip）fail closed 返回错误，不再静默产出空快照；新增 `TestPaneRefAllowlistAndDenylist` 与 `TestAssemblePaneSnapshotFailsClosedOnUnsupportedNotesShape`。

验证要求：`go test ./internal/app -run Pane -count=1`、`golangci-lint run`、`go run ./tools/testkit/integrationevidence -profile dsh-pane` 全部通过。
