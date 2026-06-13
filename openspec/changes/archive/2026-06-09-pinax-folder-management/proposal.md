## Why

Pinax 已经能列出 note folders，也能单笔移动 note，但缺少对 folder taxonomy 的批量、安全治理入口。用户整理 vault 时需要把同一 folder 下的多篇笔记迁移到新 folder，不应手动移动文件再手改 frontmatter。

这次变更补齐 `note folders rename`，让文件夹重命名通过 CLI/application service 完成，并保留 dry-run、显式确认、record ledger 和索引刷新事实。

## What Changes

- 新增 `pinax note folders rename <old> <new> --dry-run`，返回匹配笔记和目标路径计划，不写 Markdown、`.pinax/` state、provider 或远端服务。
- 新增 `pinax note folders rename <old> <new> --yes`，批量移动匹配笔记文件并更新 frontmatter `folder`。
- 写入前 SHALL 检查目标路径冲突和批量目标重复，发现冲突时整个操作失败且不写入。
- 写入后 SHALL 追加 record event、事件证据，并刷新本地 index projection。
- 不新增任意批量文件删除/移动 apply；asset/file 侧继续使用已有 `asset move/remove/repair --plan` 和单笔 `note move`/`note attach`。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `note-command-ux`: 补充 folder taxonomy 批量 rename 的命令行为、安全确认和输出事实要求。

## Impact

- 影响代码：`internal/app/service.go`、`internal/cli/note_cmd.go`、`internal/output/render.go`、`cmd/pinax/main_test.go`。
- 影响命令：`pinax note folders rename <old> <new> --dry-run|--yes`。
- 不新增依赖、不新增 daemon、不改变 Markdown vault 作为真源的定位。
