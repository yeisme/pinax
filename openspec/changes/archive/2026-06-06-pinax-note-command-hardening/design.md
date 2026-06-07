## Context

`pinax-note-command-ux` 已完成 note 子命令的核心功能面，当前风险集中在真实用户日常使用时的可靠性边界：editor 环境变量经常包含参数，文件写入需要避免半状态，trash 需要避免同名冲突，frontmatter 写回需要尽量减少用户 Markdown churn，`--recent` 需要可解释的稳定语义。

本设计不扩展 note 命令的产品边界，而是把已有行为打磨成可长期依赖的本地笔记 CLI。所有写入仍必须通过 application service，输出仍由同一 projection 渲染，测试必须使用 fake executable、fixture vault 和临时文件树。

## Goals / Non-Goals

**Goals:**

- 让 `note edit/open/new --open` 支持常见 editor 配置，例如 `code --wait`、`vim -n` 和显式 `--editor` 参数。
- 让 `note rename`、`note archive`、`note tag` 等写入路径具备更清晰的原子性和失败语义。
- 让 `note delete --yes` 默认 trash 在同日同名冲突时生成安全唯一目标，不覆盖既有 trash。
- 降低 frontmatter 写回对用户手写 YAML 的格式扰动，并明确机器字段的 patch 范围。
- 明确 `note list --recent` 是排序语义，后续时间窗口过滤另行设计为 `--since`。
- 补齐 contract tests，保证 human、`--json`、`--agent` 输出和 stdout/stderr 边界稳定。

**Non-Goals:**

- 不新增批量整理、repair apply、provider、sync、TUI 或 daemon。
- 不做正文内容改写、LLM metadata 推断或自动删除。
- 不把 Pinax 变成完整 YAML 编辑器；只在 Pinax note frontmatter 场景内提供保真 patch。
- 不改变 `note new/list/show/read/edit/rename/move/archive/delete/tag` 的既有命令名和基本参数兼容性。

## Decisions

### 1. 引入 EditorCommand 解析和 runner

新增轻量 `EditorCommand` 解析函数，把 editor 字符串拆成 executable 和 args，再追加 note path。解析规则应覆盖普通 shell quoting，但不执行 shell。这样避免把 `code --wait` 当作单个 executable，同时不引入 shell 注入风险。

替代方案是通过 `sh -c "$EDITOR <path>"` 执行。该方案兼容 shell，但会扩大注入边界，不符合本地 CLI 的安全默认。

### 2. 写操作走 note mutation helper

新增 `NoteMutation` helper，统一负责读取 note、解析 frontmatter、构建目标内容、写临时文件、rename/replace、append event 和 projection。`note rename` 的目标路径变化必须先写目标临时文件，再完成 rename/replace；失败时尽量保留原文件原内容。

替代方案是在各命令函数里继续手写 `os.WriteFile` + `os.Rename`。该方案短期快，但已经出现半状态风险，后续维护成本会继续上升。

### 3. Trash path 使用唯一目标生成

`note delete --yes` 的 trash 目标仍放 `.pinax/trash/YYYYMMDD/`，但必须在目标存在时生成稳定后缀，例如 `name-2.md`、`name-3.md`。projection 和 event 记录最终 `trash_path`。

替代方案是按 timestamp 目录分桶。它降低冲突概率，但路径不如日期 + 原路径可扫；本 change 先保留日期分区并解决冲突。

### 4. Frontmatter patch 先做局部保真，不引入重型 YAML AST

MVP 中可以通过 `patchFrontmatterFields` 只更新 Pinax 管理字段，并保留未知字段、注释和字段顺序；当缺少 frontmatter 或目标字段缺失时，再按 Pinax canonical block 补齐。复杂 YAML 多行结构如果无法可靠 patch，应降级到 canonical render，并在测试中记录该边界。

替代方案是立刻引入完整 YAML AST。该方案长期更稳，但会增加依赖和设计面；本 change 优先解决普通用户文件的高频 churn。

### 5. `--recent` 只表达排序

`note list --recent` SHALL 等价于 `--sort updated`，并在 facts 中输出 `sort=updated` 或 `recent=true`。时间窗口过滤不塞进 `--recent`，后续如果需要用 `--since 7d` 单独设计。

## Risks / Trade-offs

- Editor 解析无法覆盖所有 shell 语法 -> 支持常见 quoting 和参数，复杂命令建议用户传 wrapper script。
- 临时文件替换跨文件系统失败 -> 临时文件放在同一目标目录，减少跨设备 rename 风险。
- Frontmatter patch 对复杂 YAML 不完整 -> 明确 fallback，并用 tests 锁住常见注释、未知字段和 tag/status/title 更新。
- Trash 唯一路径生成仍可能并发冲突 -> 单进程 CLI 场景先通过存在性检查和后缀重试处理；长期可加文件锁。
- 输出字段增加可能影响脚本 -> 只增加 facts/action/data 字段，不移除既有字段。

## Migration Plan

1. 先写 RED tests：editor 参数、rename 原子失败、trash 冲突、frontmatter 保真、recent facts。
2. 增加 editor parser/runner，并改造 `EditNote` 和 `note new --open` 路径。
3. 增加 note mutation helper，迁移 rename/archive/tag 的写回逻辑。
4. 增加 unique trash helper，迁移 delete trash 路径。
5. 增加 frontmatter patch helper，优先用于 title/tags/status/updated_at 字段更新。
6. 更新 README/docs 的 note edit/delete/list 边界说明。
7. 运行聚焦测试、全量测试、构建、OpenSpec 校验和 `task check`。

## Open Questions

- 是否要在本 change 中新增 `--since`？建议不做，避免把 hardening change 变成功能扩张。
- 是否要把 mutation helper 放 `internal/app` 还是拆 `internal/notes`？建议若改动范围可控先放 `internal/app`，下一次再拆包。
- 是否需要文件锁？当前本地单命令优先，不做跨进程锁；后续 daemon/TUI 出现前再评估。
