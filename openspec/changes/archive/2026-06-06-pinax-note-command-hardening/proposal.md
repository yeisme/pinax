## Why

`pinax note` 已经覆盖日常笔记操作，但 review 暴露出几个会影响长期自用可靠性的边界：带参数的 `$EDITOR` 无法工作、rename 可能留下半更新状态、同日同路径 trash 可能冲突、frontmatter 写回会造成不必要 churn，以及 `--recent` 语义不够清晰。现在需要在继续扩展功能前先把这些高频基础操作做稳。

## What Changes

- 增强 `note edit/open/new --open` 的 editor 执行边界，支持常见带参数 editor 配置，并保持 fake editor 可测试。
- 改造 `note rename` 写入流程，避免“frontmatter 已改但文件未移动”的半状态；失败时返回稳定 projection，并尽量保持原文件不变。
- 改造 `note delete --yes` 的 trash 目标生成，遇到同日同名 trash 文件时自动生成唯一路径，不覆盖已有 trash。
- 增加 frontmatter 写回保真策略，降低 `rename/archive/tag` 对用户手写 YAML、注释和未知字段造成的 churn。
- 明确 `note list --recent` 的产品语义：作为最近更新时间排序别名，后续时间窗口使用独立 `--since`，避免一个 flag 同时承担排序和过滤。
- 补齐 contract/e2e tests，覆盖 editor 参数、rename 失败原子性、trash 冲突、frontmatter 保真、recent 输出事实。
- 不新增云端 provider、LLM 改写、TUI、daemon 或批量整理能力；批量维护继续归属 `repair` 或 `organize` 方向。

## Capabilities

### New Capabilities

- `note-command-hardening`: 覆盖 note 子命令的可靠编辑器执行、原子写入、安全 trash、frontmatter 保真和 recent 语义。

### Modified Capabilities

- `pinax`: 收紧现有 `pinax note` 行为要求，让 note 命令在本地 Markdown vault 中具备更可靠的写入和输出合同。

## Impact

- CLI：`note edit/open/new --open` 的 editor 解析行为变化，但对现有单 executable 配置保持兼容；`note list --recent` 输出 facts 增加明确排序语义。
- 应用层：新增 editor command parser/runner、atomic note mutation helper、unique trash path helper 和 frontmatter patch helper。
- 输出：`--json`/`--agent` 增加稳定 facts，例如 editor executable/args、trash_path、sort/recent 语义、mutation outcome。
- 测试：新增 command-level 和 service-level TDD 覆盖，优先使用 fake executable、fixture vault 和临时文件树，不依赖真实用户 editor 或 vault。
- 文档：更新 README 或 docs 中 note edit/delete/list 的边界说明，不新增独立 checklist。
