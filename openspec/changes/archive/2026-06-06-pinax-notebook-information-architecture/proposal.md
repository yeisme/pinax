## Why

Pinax 当前已经能创建 Markdown note，但默认体验仍像文件写入工具：新 note 不会自动进入本地索引，也没有把分组、文件夹、用途分类和日常入口串成一个笔记软件应有的信息架构。用户创建 note 后需要额外运行 `index rebuild`、记住路径、手动建立 daily index，日常使用摩擦过高。

## What Changes

- `note new/create` 支持 `--group`、`--folder`、`--kind`，并把这些维度写入 frontmatter、facts 和 JSON data。
- `--group` 作为 `--project` 的用户友好别名；`--folder` 表示项目内或 `notes/` 下的相对文件夹；`--kind` 表示用途分类，例如 `reference`、`fleeting`、`project`、`daily`。
- 创建 note 后自动追加到当天 `notes/daily/YYYY-MM-DD.md` daily index。
- 创建 note 后自动刷新 `.pinax/index.sqlite`，让 stats/doctor 不再马上报告 index stale。
- SQLite note projection 记录 frontmatter project/folder/kind，而不是只从路径猜 project。

## Non-Goals

- 不引入 TUI、长期 daemon 或云端索引。
- 不让 agent 手写 `.pinax` 结构化资产。
- 不改变现有 `--project`、`--dir`、`--status`、`--tags` 的兼容语义。
