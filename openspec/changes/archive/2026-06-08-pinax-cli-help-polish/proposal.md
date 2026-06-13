## Why

Pinax 的 CLI tree 已经有 `vault`、`journal`、`note`、`backend` 等主路径，但 root help 仍混入 `stats`、`doctor`、`tag`、`folder`、`storage set-s3` 等兼容入口，导致用户无法快速判断推荐路径。当前 `cli-tree-ux` 与旧 `pinax`/dashboard 规格也存在口径冲突，容易让测试和实现反复把 root 噪音带回第一层。

## What Changes

- 美化 `pinax --help`，按用户工作流展示分组，而不是把所有 root command 平铺成一列。
- 将兼容入口从主要 help 中隐藏，包括 vault root alias、dimension root alias、storage direct set alias 和 organize suggest alias；命令仍可执行，机器输出保持兼容。
- 将 help 示例和错误 next action 优先指向主路径，例如 `pinax vault validate`、`pinax note tags`、`pinax storage set s3`、`pinax organize plan --save`。
- 统一 OpenSpec 规格口径：root help 展示主分组，旧 root 命令是兼容 alias，不再是推荐入口。
- 不改变业务 service、vault 文件格式、`.pinax/` 结构化资产格式或 `--json`/`--agent` 字段。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `cli-tree-ux`: 明确 root help 分组展示、兼容 alias 隐藏、主路径示例和 help 输出可读性要求。
- `pinax`: 将 stats/doctor/dashboard/validate 从 root 推荐入口调整为 vault 主路径能力，并保留 root alias 兼容。
- `vault-dashboard-health`: 将 dashboard/stats/doctor 示例优先改为 `pinax vault ...`，root 路径只作为兼容入口。
- `notebook-workflows`: 将 tag/folder/kind/group 浏览示例优先改为 `pinax note tags|folders|kinds|groups`。

## Impact

- 代码：`internal/cli/root.go` help 模板和 command annotation，`internal/cli/vault_cmd.go`、`inbox_cmd.go`、`storage_cmd.go`、`organize_cmd.go` 的 alias 可见性与示例。
- 测试：`cmd/pinax/main_test.go` 和/或 `internal/cli/root_test.go` 增加 help 分组、alias 隐藏、主路径可见、兼容路径仍可执行的 contract tests。
- 文档/规格：更新本 change 下 delta specs；必要时同步 README/docs 中的 help 示例。
- 兼容性：无 breaking change；隐藏 alias 只影响 help 可见性，不删除命令或变更机器输出。
