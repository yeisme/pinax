## 1. 命令和服务实现

- [x] 1.1 新增 backend note inspection service projection。
  - Owner: Pinax app service
  - Scope: 增加 `BackendNotesSummary`、`BackendNotesList`、`BackendNotesStat`；复用 backend registry 和 `remote.ExtendedBlobStore`；只识别 `.md` note-like 对象。
  - Dependencies: 无。
  - Parallel lane: A。
  - Acceptance: local backend fixture 下 `notes/cloud-sync.md` 被识别为 note，`notes/raw.txt` 不进入 notes list。
  - Validation: `go test ./cmd/pinax -run 'TestBackendNotesCommandsInspectMarkdownObjects$' -count=1`
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: 如果 Cloud Sync 加密 blob 被误报为 note，检查过滤条件必须基于可见 `.md` key。

- [x] 1.2 接入 Cobra 子命令。
  - Owner: Pinax CLI
  - Scope: 新增 `pinax backend notes summary/list/stat`，命令层只校验参数并调用 app service；`summary/list` 允许省略 backend name 并使用 default backend。
  - Dependencies: 1.1。
  - Parallel lane: A。
  - Acceptance: `pinax backend notes --help` 列出 summary/list/stat；`pinax backend notes summary --vault ./my-notes --json` 可读取 default backend；子命令支持 `--json`、`--agent` 和默认摘要。
  - Validation: `go run ./cmd/pinax backend notes --help`
  - Expected: help includes `summary`, `list`, `stat`。
  - Result: PASS。
  - Failure recheck: 不在 command 层拼 JSON 或 agent 输出。

- [x] 1.3 补充 backend notes/object positional completion。
  - Owner: Pinax CLI
  - Scope: `backend notes summary/list/stat` 和 `backend object list/stat` 使用分层补全；第一个参数补 backend profile，第二个参数补 prefix、note path 或 raw object key。
  - Dependencies: 1.1, 1.2。
  - Parallel lane: A。
  - Acceptance: `__complete backend notes stat` 返回 backend 名称；`__complete backend notes stat <backend> notes/` 返回 `.md` note path 且不返回 non-note object；`__complete backend notes list <backend>` 返回 note prefix；`__complete backend object stat <backend> pinax/` 返回 raw object key。
  - Validation: `go test ./cmd/pinax -run 'TestBackendNotesCommandsInspectMarkdownObjects$' -count=1`
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: completion 只能通过只读 service projection，不得写 `.pinax/**` 或输出 provider secret。

## 2. S3 runtime 和 provider 错误

- [x] 2.1 修正 S3 profile 到 runtime client options 的映射。
  - Owner: Pinax app/remote integration
  - Scope: `backendBlobStore` 创建 S3 backend 时传入 profile `region`、`endpoint`、`profile`，custom endpoint 自动 path-style。
  - Dependencies: 无。
  - Parallel lane: B。
  - Acceptance: app 层测试证明 `backendS3Options` 保留 profile fields。
  - Validation: `go test ./internal/app -run 'TestBackendS3OptionsUseProfileFields$' -count=1`
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: 如果 live S3 仍报 invalid region，先运行 `pinax backend show <name> --vault <vault> --json` 检查 profile 存储值。

- [x] 2.2 provider 错误映射为稳定 CommandError。
  - Owner: Pinax app/output
  - Scope: object/note list/stat provider failure 返回 `backend_provider_error` 或 `backend_region_invalid`，并给出 backend show 下一步。
  - Dependencies: 2.1。
  - Parallel lane: B。
  - Acceptance: 不再把 AWS SDK 长错误直接作为 `internal_error` message 输出。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'Backend' -count=1`
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: 确认 redaction 没有输出 secret、token 或 Authorization header。

## 3. 输出、文档和验证

- [x] 3.1 补 CLI 输出表格和 agent rows。
  - Owner: Pinax output
  - Scope: 默认 human summary 为 backend object/note list 渲染表格；`--json` 是单 envelope；`--agent` 输出稳定 key=value。
  - Dependencies: 1.1。
  - Parallel lane: C。
  - Acceptance: command test 覆盖 JSON、agent 和默认摘要。
  - Validation: `go test ./cmd/pinax -run 'TestBackendNotesCommandsInspectMarkdownObjects$' -count=1`
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: JSON stdout 不能混入 ANSI、表格或日志。

- [x] 3.2 更新 backend 命令文档。
  - Owner: Pinax docs
  - Scope: 在 `docs/commands/backend.md` 增加 summary/list/stat 只读示例。
  - Dependencies: 1.2。
  - Parallel lane: C。
  - Acceptance: 文档命令均为用户可直接运行的 `pinax` 命令。
  - Validation: 人工检查 `docs/commands/backend.md`。
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: 不写 agent-only wrapper 或本地别名。

- [x] 3.3 运行质量门禁。
  - Owner: Pinax verification
  - Scope: 跑聚焦测试、真实 CLI smoke 和 `task check`。
  - Dependencies: 1.1, 1.2, 2.1, 2.2, 3.1。
  - Parallel lane: sequential。
  - Acceptance: 所有验证通过。
  - Validation: `task check`
  - Expected: PASS。
  - Result: PASS。
  - Failure recheck: 若失败，先区分本变更文件和工作树已有未提交改动。
