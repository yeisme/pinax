## Context

Pinax 的 folder 既是笔记 frontmatter 中的组织属性，也是常见 vault 文件布局的一部分。现有 `note folders` 可以列出维度，`note move` 可以移动单篇笔记，但缺少批量 folder rename 的计划/确认机制。

本变更保持 CLI-only 边界：Cobra 只解析参数并调用 service，实际文件移动、metadata patch、record event 和 index refresh 都在 application service 内完成。

## Goals / Non-Goals

**Goals:**

- 支持 `note folders rename <old> <new> --dry-run` 生成无写入计划。
- 支持 `note folders rename <old> <new> --yes` 批量移动 note 文件并更新 `folder` frontmatter。
- 在写入前检查目标路径冲突，避免半批量写入。
- 输出 stable JSON/agent facts：`old_folder`、`new_folder`、`matched`、`changed`、`writes`、`record_events`、`index_updated`。

**Non-Goals:**

- 不实现任意目录树删除、跨 vault 移动、asset apply 或附件引用重写。
- 不自动创建 Git snapshot；本命令通过 facts/actions 暴露 `requires_snapshot` 风险，后续可接 `version snapshot` 工作流。
- 不改变现有 `note move` 的单笔语义。

## Decisions

1. folder rename 同时移动文件和更新 frontmatter。

   只改 frontmatter 会让文件布局和组织维度分叉；只移动文件又会让 `folder` metadata 过期。批量 rename 必须保持二者一致。

2. 写入前先完成目标路径冲突检查。

   如果目标路径已存在或多条 note 会写到同一路径，命令返回 `note_path_conflict`，不进入写入循环。

3. 目标路径尽量保留已有前缀，只替换 folder 片段。

   例如 `inbox/a.md` -> `archive/a.md`，`notes/inbox/a.md` -> `notes/archive/a.md`，`projects/p/notes/inbox/a.md` -> `projects/p/notes/archive/a.md`。

4. 写入后统一刷新一次 index。

   每条 note 单独写 Markdown 和 record event，批量结束后调用一次 `refreshIndex`，降低重复索引成本。

## Risks / Trade-offs

- folder rename 会移动多个文件，风险高于单笔 metadata 修改；因此要求 `--dry-run` 或 `--yes`。
- 如果某些 note 的 frontmatter folder 与实际路径不一致，命令会优先匹配 folder dimension，并把目标路径落到新 folder 下，借此恢复一致性。
- 这不是完整文件管理 apply 框架；asset 仍走已有 plan 命令，后续如需批量 file apply 应单独设计 snapshot 和引用重写。

## Migration Plan

无需迁移。已有 vault 可以直接使用 dry-run 预览；确认写入后会刷新本地 index projection。
