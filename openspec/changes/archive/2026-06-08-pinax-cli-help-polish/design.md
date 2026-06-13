## Context

Pinax 当前 root help 使用统一模板平铺所有可用 Cobra command。随着 `vault`、`journal`、`backend`、`note` 等主路径陆续加入，旧 root alias 仍显示在第一层，导致用户看到的入口数量过多，也让 `cli-tree-ux` 与旧规格中的 root 命令要求发生冲突。

当前实现中 help 模板位于 `internal/cli/root.go`，命令注册分散在 `internal/cli/*_cmd.go`。已有 command factory 可以继续复用，不需要迁移业务 service 或 renderer。

## Goals / Non-Goals

**Goals:**

- `pinax --help` 按工作流分组展示主入口，并保持中文可读。
- 兼容 alias 保留执行能力，但默认不出现在主 help 中。
- 主路径示例和错误 next action 统一指向 `vault`、`journal`、`note`、`storage set`、`organize plan` 等推荐路径。
- 增加帮助输出 contract tests，避免 root help 再次退化为平铺命令列表。

**Non-Goals:**

- 不删除任何兼容命令。
- 不修改 `--json`、`--agent`、`--events`、`--explain` envelope 字段。
- 不改变 vault、index、backend、organize plan 等结构化资产格式。
- 不重构整个 `NewRootCommandWithDeps`。

## Decisions

1. 使用 Cobra command annotation 驱动 root help 分组。
   - 原因：分组是 command-layer UI concern，不应进入 app service 或 renderer。
   - 备选：硬编码 command 名称列表到模板函数。该方案更脆弱，新增命令时容易漏同步。

2. 隐藏兼容 alias，而不是删除 alias。
   - 原因：现有脚本可能仍调用 `pinax stats`、`pinax tag list`、`pinax storage set-s3` 或 `pinax organize suggest`。
   - 备选：直接移除旧入口。该方案会造成不必要 breaking change。

3. 将 help 美化限制为人类 help 输出，不触碰 projection renderer。
   - 原因：help 是 Cobra 元信息；业务命令输出仍必须通过统一 projection 渲染。
   - 备选：把 help 也纳入 `internal/output`。当前收益不够，且会增加命令构造复杂度。

4. 优先覆盖 root help，再逐步覆盖子命令 help。
   - 原因：root help 是用户首次扫描入口，收益最大；子命令 help 仍可用现有模板。
   - 备选：一次性重写所有 help。该方案风险大，容易影响 completion/help 默认行为。

## Risks / Trade-offs

- 兼容 alias 隐藏后用户可能不知道旧命令仍可用。→ README 和迁移说明只在兼容章节列出旧路径。
- Cobra 自定义模板可能影响 `help` 子命令显示。→ 增加 `pinax --help`、`pinax vault --help`、`pinax completion --help` 聚焦测试。
- 分组 annotation 可能漏标新命令。→ 增加 root help contract test，要求主要分组出现，兼容 alias 不出现。
- 旧规格仍引用 root 示例。→ 本 change 同步 delta specs，后续归档时统一主规格口径。

## Migration Plan

1. 新增 OpenSpec delta specs 和任务清单。
2. 先写 help contract tests，验证 root help 分组和 alias 隐藏。
3. 实现 root help 分组、compat alias 隐藏和主路径示例调整。
4. 跑聚焦测试、OpenSpec validate 和可用的项目门禁。
5. 归档时同步 main specs，并保留兼容路径说明。

## Open Questions

- 是否在后续变更中隐藏 `note add/create/read/open` 这类 note 子命令 alias？本 change 暂不处理，避免影响常用 note workflow。
