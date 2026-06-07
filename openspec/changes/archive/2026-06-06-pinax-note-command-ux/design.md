## Context

当前 Pinax 已经具备 `note new/list/show`，但这只是最小功能面，不足以支撑真实日常笔记管理。用户需要快速创建、马上编辑、按标签/项目/状态找笔记、用标题或 note id 打开笔记、修改标题和路径、安全归档和管理标签。否则 Pinax 会退化成“能生成 Markdown 的工具”，而不是顺手的本地 note CLI。

本设计保持 Pinax 的边界：Markdown 是真源，frontmatter 由 CLI/service 维护，机器资产由 CLI/service 写入，危险写入需要显式确认，所有输出模式来自同一 projection。

## Goals / Non-Goals

**Goals:**

- 让 `pinax note` 覆盖日常笔记操作：创建、列出、读取、打开、编辑、重命名、移动、归档、标签维护和安全删除。
- 降低路径记忆成本：支持 note id、相对路径、标题精确匹配和唯一标题匹配；歧义时给候选。
- 让 `note list` 可扫、可过滤、可脚本化，默认适合人看，`--json`/`--agent` 适合自动化。
- 支持 `$EDITOR`/`--editor` 打开笔记，但测试使用 fake executable，不依赖真实编辑器。
- 对危险操作建立明确保护：默认 archive/trash，hard delete 必须 `--yes --hard`。
+
**Non-Goals:**

- 不做 TUI 或长期 daemon。
- 不接云端、provider 或 LLM 自动改写。
- 不把 note 命令变成 Obsidian/Logseq 的完整替代 UI。
- 不让命令层直接写 Markdown；所有写入仍通过 application service。

## Decisions

### 1. 保持旧命令，新增别名和更好 flags

`note new/list/show` 继续可用；新增 `note create` 作为 `new` 别名，`note read` 作为 `show` 别名，`note open`/`note edit` 作为编辑器入口。这样不破坏已有脚本，同时改善新用户发现路径。

### 2. 引入 NoteRef resolver

新增 `NoteRefResolver`，解析顺序为：note id 精确匹配、vault 内相对路径匹配、`notes/` 前缀容错、标题精确匹配、唯一标题匹配。多个候选时返回 `note_ref_ambiguous`，JSON/agent 输出包含 candidates，默认中文输出给出可复制命令。

替代方案是让用户必须输入路径。该方案实现简单，但体验差，不符合 note CLI 定位。

### 3. 创建命令支持内容来源和编辑器

`note new/create` 支持：

- `--body <text>`：直接写正文。
- `--from <file>`：从文件读取正文。
- `--stdin`：从 stdin 读取正文。
- `--open`：创建后打开编辑器。
- `--dir`：指定 `notes/` 下目录。
- `--slug`：指定文件名 slug。
- `--status`：写入 frontmatter status。
- `--dry-run`：只返回计划路径和 frontmatter，不写文件。

内容来源互斥；冲突返回稳定错误 `note_source_conflict`。

### 4. 单笔维护操作优先安全默认

`note archive` 默认写 frontmatter `status: archived`，不移动文件。`note delete` 默认移动到 `.pinax/trash/` 并记录 receipt；`--hard` 只有在同时提供 `--yes` 时才真实删除。`note rename/move` 是单笔操作，仍必须校验 vault boundary 和冲突；必要时给 next action 建议 Git snapshot。

### 5. 输出以 projection 为中心

新增 note projection data：note、notes、candidates、operation、old_path/new_path、changed_frontmatter、editor、trash_path 等。human 摘要保持中文短输出，复杂列表放 `--json`。

## Risks / Trade-offs

- 命令面扩张过快 -> 本 change 限定在 note 核心日常操作，不做 TUI/agent/provider。
- 标题匹配误开错误笔记 -> 仅在唯一标题匹配时通过；歧义必须失败并列候选。
- 编辑器启动在测试中不稳定 -> 抽象 editor runner，测试用 fake executable，支持 `--no-open`/默认不打开。
- delete 误删 -> 默认 trash，hard delete 需要 `--yes --hard`，并记录 redacted event。
- note move/rename 与 organize 重叠 -> 单笔命令处理用户明确目标；批量整理仍交给 organize plan/apply。

## Migration Plan

1. 新增 NoteRef resolver 和 list query model，不改变旧 `note new/list/show` 行为。
2. 增强 `note list` 过滤、排序和输出。
3. 增强 `note new/create` 内容来源、路径控制和 `--open`。
4. 新增 `show/read/open/edit` resolver 路径。
5. 新增 rename/move/archive/tag/delete 单笔维护操作。
6. 更新 README/help，并补齐 contract tests 和 fake editor e2e。

## Open Questions

- 默认 `note list` 显示最近更新还是路径排序？建议默认最近更新，提供 `--sort path`。
- `note delete` 默认 trash 的路径是否放 `.pinax/trash/YYYYMMDD/`？建议按日期分区，避免冲突。
- `note edit <missing title>` 是否创建新笔记？建议 MVP 不创建，避免误操作；用 `note new --open`。
