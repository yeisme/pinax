## Why

Pinax 的本质是本地 Markdown note CLI，但当前 `note` 子命令只有 `new/list/show`，创建、查找、编辑、过滤、归档和标签维护都不顺手。用户一旦进入日常笔记流，会频繁遇到路径难记、输出不够可扫、没有编辑入口、没有安全归档/删除和批量标签操作等摩擦。

## What Changes

- 改善 `pinax note` 命令族的信息架构：保留 `new/list/show`，增加更符合日常使用的别名和子命令。
- 增强创建体验：支持 `note new/create` 的 `--body`、`--from`、`--stdin`、`--open`、`--dir`、`--slug`、`--status` 和 `--dry-run`。
- 增强查找体验：`note list` 支持 `--tag`、`--project`、`--status`、`--recent`、`--limit`、`--sort`、`--path-prefix`，默认输出更适合快速扫描。
- 增强读取体验：`note show/read/open/edit` 支持 note id、相对路径、标题精确匹配和安全的唯一标题匹配；歧义时返回候选列表。
- 新增单笔维护操作：`note edit`、`note rename`、`note move`、`note archive`、`note tag add/remove/set`。
- 新增危险操作保护：`note delete` 默认移动到 `.pinax/trash/` 或返回 approval required，真实删除必须 `--yes --hard`，并记录 redacted event。
- 所有 note 命令输出遵守 Pinax CLI 输出合同，结构化 metadata 仍由 CLI/service 写入。
- 不引入 TUI、长期 daemon、云服务、provider 接入或 LLM 自动改写。

## Capabilities

### New Capabilities

- `note-command-ux`: 本地 Markdown note CLI 的创建、查找、编辑、过滤、标签和安全维护体验。

### Modified Capabilities

- `pinax`: 扩展 note 子命令作为 Pinax 核心 note CLI surface，并明确向后兼容 `note new/list/show`。

## Impact

- CLI：调整 `cmd/pinax` 的 `note` command group、help、aliases、flags 和错误提示。
- 应用层：新增 note query、note reference resolver、note editor launcher、note mutation service 和 single-note safety guard。
- 输出层：新增/扩展 note projection，覆盖 human、`--json`、`--agent`、`--events` 和 `--explain` 边界。
- Vault：写入仍限制在 vault root；frontmatter 维护由 service 完成；trash 和 event 资产由 CLI/service 创建。
- 测试：增加 TDD、contract tests、fixture vault、editor fake executable、路径边界、歧义解析和危险操作保护测试。
