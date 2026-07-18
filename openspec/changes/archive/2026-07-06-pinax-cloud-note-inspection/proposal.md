## Why

用户在排查 Pinax 云端笔记状态时，现有入口主要是 `pinax backend object list <name> [prefix]` 和 `pinax backend object stat <name> <key>`。这些命令暴露的是对象存储 key 视角；当 S3/MinIO/R2 配置有误时，还会把底层 AWS SDK 的 region/endpoint 错误直接呈现为 `internal_error`，用户很难判断是“没有云端笔记”、配置错误，还是 provider 无法访问。

Pinax 需要几个只读命令，让用户和 agent 能快速回答：某个 backend 里能看到多少 note-like Markdown 对象、具体有哪些远端 note path、单篇 note 的远端 revision 是什么；同时底层 provider 失败要映射成稳定、可操作的 Pinax error code。

## What Changes

- 新增 `pinax backend notes summary <name> [prefix]`：统计 backend 下对象总数、Markdown note-like 对象数、总字节数和最新更新时间。
- 新增 `pinax backend notes list <name> [prefix]`：列出 backend 中以 `.md` 结尾的 note-like 对象，输出 path、key、size 和 updated time。
- 新增 `pinax backend notes stat <name> <path>`：查看单个远端 note-like 对象的 revision/status。
- 保留 `backend object list/stat` 作为底层对象诊断入口，并把 object list 的 data rows 改为 snake_case 字段，便于 `--json` 和 `--agent` 消费。
- 修正 S3 backend profile 到实际 S3 client 的映射：`region`、`endpoint`、`profile` 必须传入 `NewS3BackendWithOptions`，不能只保存到 profile 而运行时忽略。
- provider 访问失败时返回稳定 `backend_provider_error` 或 `backend_region_invalid`，并给出 `pinax backend show ...` 的下一步，而不是暴露底层 SDK 长错误作为 `internal_error`。

## Non-Goals

- 不把 S3 direct 或 rclone direct 后端当作 Pinax Cloud Server；它们仍是用户凭据直连的 object-store transport。
- 不解密或解析 Cloud Sync 加密 blob；`backend notes` 只识别可见的 Markdown note-like object key。
- 不新增远端写入、删除、同步或 daemon 行为。
- 不保存或输出 access key、secret key、session token、Authorization header、provider raw payload 或完整 SDK trace。

## Impact

- CLI：新增 `backend notes summary/list/stat` 子命令。
- App service：新增 note-oriented backend inspection projection；复用 backend registry 和 `remote.ExtendedBlobStore`。
- Output：新增 backend object/note list 默认摘要表格；`--json` 和 `--agent` 仍来自同一 projection。
- Docs：更新 `docs/commands/backend.md` 的只读查看工作流。
- Tests：新增命令级 contract test 和 S3 options app test。

## Compatibility

本变更是 additive：不删除现有命令，不移除既有 envelope 顶层字段。新增 `backend.notes.*` 命令和 `objects`/`notes` data rows 为可选新输出面。`backend object list` 的 data rows 从 Go struct JSON 名称收敛为 snake_case 字段，既有公开文档未承诺 `Key`/`Size` struct casing；默认 human、`--agent` 和 `--json` envelope 顶层合同保持稳定。
