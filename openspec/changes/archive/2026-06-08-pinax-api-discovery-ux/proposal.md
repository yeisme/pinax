## Why

用户在探索本地 API 能力时自然尝试 `pinax schema`，但当前 CLI 直接返回 unknown command；同时 `pinax api routes` 默认人类输出只显示 routes 数量，无法快速看见可用 endpoint。

## What Changes

- 增加隐藏 root 兼容入口 `pinax schema export`，复用 `pinax api schema export` 的命令投影、flag 和输出合同。
- 保持 root help 不展示 `schema` 兼容入口，避免主命令树继续膨胀。
- 让 `pinax api routes` 默认人类输出展示 REST/RPC route 摘要，并给出 schema export 下一步。
- 不改变 `--json` 的完整 route/capability 数据结构和既有 command 名称。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `cli-tree-ux`: 增加隐藏的 root `schema` 兼容入口，提升 API schema 发现性。
- `project-board-workspace`: 增强本地 API routes 默认人类输出，让 endpoint 可扫读。

## Impact

- 影响代码：`internal/cli/api_cmd.go`、`internal/app/remote.go`、`cmd/pinax/main_test.go`。
- 影响 CLI：新增隐藏兼容路径 `pinax schema export`；`pinax api routes` 默认输出多出 route evidence 和下一步命令。
- 不新增依赖，不触碰 REST/RPC server handler，不修改 vault 文件或 provider 行为。
