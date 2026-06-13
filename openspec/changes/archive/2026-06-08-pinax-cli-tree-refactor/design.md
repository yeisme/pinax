## Context

Pinax 当前 Cobra 命令树主要在 `cmd/pinax/main.go` 内直接组装，已经包含 vault 检查、dashboard、daily/weekly/monthly、inbox、维度浏览、view、note、search、import/export、project、storage、template、index、sync、metadata、repair、organize、git、mcp 等入口。这个形态便于早期迭代，但随着功能增加，根 help 会变长，命令别名和重复 flag 也更容易发散。

项目架构文档已经预留 `internal/cli` 作为后续 Cobra command factory 和 dependency wiring 的归属。本变更利用这个边界，把命令树优化为用户可扫描的主路径，同时保留旧路径兼容。

## Goals / Non-Goals

**Goals:**

- 形成稳定、可扫描、围绕用户工作流的主 CLI tree。
- 将 `cmd/pinax/main.go` 瘦身为 bootstrap：创建 root command、注入版本、执行。
- 在 `internal/cli` 中拆分 root、deps、command groups、shared flags、render helpers 和 alias helpers。
- 保留现有命令路径兼容，特别是 root-level `stats/doctor/validate`、`daily/weekly/monthly`、storage `set-local/set-s3` 和 note aliases。
- 让 primary path 和 alias path 复用同一 handler、同一 app service、同一 projection 和同一 renderer。
- 更新 help/examples/completion，使主路径更清晰且 completion 不产生写入副作用。

**Non-Goals:**

- 不修改 app service 业务语义。
- 不改变 `--json`、`--agent`、`--events`、`--explain` 输出合同。
- 不删除旧命令路径；本变更只允许隐藏或标注兼容 alias。
- 不实现 Viper 配置层和输出主题，这些属于 `pinax-configurable-output-rendering`。
- 不引入新的 TUI、provider、sync 或 MCP 行为。

## Decisions

### 1. 目标主命令树

推荐主树：

```text
pinax
  init
  vault
    status
    stats
    validate
    doctor
    dashboard
  note
    create/new
    list
    show/read
    edit/open
    rename
    move
    archive
    delete
    tag add/remove
    links
    backlinks
    orphans
    attach
    attachments
    tags
    folders
    kinds
  journal
    daily open/show/append
    weekly open/show/append
    monthly open/show/append
  inbox
    capture
    list
    triage
  search
  view
    save
    list
    show
    delete
  organize
    plan
    list
    apply
  repair
    plan
    apply
  metadata
    plan
    apply
  template
    init
    create
    list
    show
    render
    validate
    delete
  project
    create
    list
    switch
  config
    path
    get
    set
    unset
    doctor
  storage
    set local
    set s3
    status
    doctor
  index
    init
    status
    rebuild
  sync
    diff
    push
    pull
  git
    snapshot
  mcp
    serve
```

理由：根层保留高频入口和领域分组，不让低频维护动作和兼容别名淹没第一屏 help。

### 2. 分阶段迁移，不一次性删除路径

阶段一：抽 command factory，不改变命令树行为。  
阶段二：增加新主路径，并让旧路径作为 alias 复用同一 builder。  
阶段三：更新 help/examples，必要时隐藏兼容 alias。  
阶段四：补齐 e2e 和 machine output 等价测试。

这样可以把结构性重构和行为变更拆开，降低回归面。

### 3. `internal/cli` 包形状

建议结构：

```text
internal/cli/
  root.go
  deps.go
  output.go
  flags.go
  errors.go
  aliases.go
  vault_cmd.go
  note_cmd.go
  journal_cmd.go
  inbox_cmd.go
  view_cmd.go
  organize_cmd.go
  repair_cmd.go
  metadata_cmd.go
  template_cmd.go
  project_cmd.go
  storage_cmd.go
  index_cmd.go
  sync_cmd.go
  git_cmd.go
  mcp_cmd.go
  config_cmd.go
```

`cmd/pinax/main.go` 最终只保留：

```go
func main() {
    root := cli.NewRootCommand(cli.Deps{Version: version})
    if err := root.Execute(); err != nil { ... }
}
```

命令层只解析参数、构造 request、调用 `internal/app` service、选择 renderer；不得直接写 vault 或 `.pinax` structured assets。

### 4. Alias 共享实现

为每个兼容 alias 建立主 command builder，而不是复制 `RunE`：

```go
func newJournalPeriodCommand(deps Deps, period string) *cobra.Command
func newJournalPeriodAlias(deps Deps, period string) *cobra.Command
```

同一 handler 应产生同一 projection。对于 `projection.Command` 是否显示旧 command 名，需要保持已有机器合同优先。建议新路径和 alias 都保留现有领域 command id，例如 `daily.show`、`note.show`、`organize.suggest/organize.plan` 的兼容策略要在测试中明确。

复杂 alias 映射需要中文注释说明迁移边界，特别是 `organize suggest` 与 `organize plan` 的语义关系。

### 5. Help 策略

主 help 只展示推荐路径。兼容 alias 可以：

- 初期仍显示，但 Short 标注“兼容入口”。
- 后续设置 `Hidden: true`，但保留可执行和测试覆盖。

所有新 help examples 使用主路径：

```text
pinax vault doctor --vault ./my-notes
pinax journal daily show --vault ./my-notes
pinax storage set s3 --bucket notes --region us-east-1 --vault ./my-notes
pinax organize plan --vault ./my-notes --json
```

### 6. Completion 安全

completion 只能做轻量本地读取，不触发写入、远程调用、provider、Git mutation 或 `.pinax` metadata 写入。需要对新增 command factory 中的 completion 函数保持同一约束。

### 7. 测试策略

测试分三类：

- command factory unit：每次 `NewRootCommand` 返回独立 flag set，避免全局污染。
- alias equivalence：同参数下 primary path 和 alias path 的 `--json` envelope 等价。
- help smoke：root help 包含主路径，兼容 alias hidden 或明确标注。

命令级 e2e 优先使用 testscript，覆盖用户实际命令，不依赖真实 provider、真实公网、真实 token 或用户 vault。

## Risks / Trade-offs

- [Risk] 命令树重构容易触发大量测试快照变化。 → Mitigation：先抽 factory 不改 help，再分批切主路径和 help；每批运行命令等价测试。
- [Risk] alias 复制 handler 导致主路径和旧路径行为漂移。 → Mitigation：所有 alias 复用同一 builder 或同一 RunE 函数，并增加 `--json` 等价测试。
- [Risk] 隐藏旧路径可能让用户以为命令被删除。 → Mitigation：旧路径保持可执行，release note/help migration 说明主路径。
- [Risk] `projection.Command` 变更会破坏机器消费者。 → Mitigation：本变更默认不改现有 command id；如必须新增 id，需显式迁移说明和合同测试。
- [Risk] 抽 `internal/cli` 与配置层 change 冲突。 → Mitigation：本变更不实现 Viper，只设计 deps/options 插槽，让后续配置层接入。

## Migration Plan

1. 新增 `internal/cli` 包和 `Deps`，迁移 root template、selected mode、render helpers、flag error helpers。
2. 将现有 root command 拆到 command factory，保持外部行为不变。
3. 增加 `vault` command group，并把 root `stats/validate/doctor/dashboard` 作为兼容 alias。
4. 增加 `journal` command group，并把 root `daily/weekly/monthly` 作为兼容 alias。
5. 调整 storage set command group，新增 `storage set local|s3`，保留 `set-local/set-s3`。
6. 统一 organize 主路径，明确 `plan/suggest` 兼容策略。
7. 增加 note dimension commands 或确定 view dimension 归属，保留旧 root dimension alias。
8. 更新 help/examples/completion。
9. 补齐 e2e、alias equivalence 和 output contract 测试。
10. 运行 `task check` 或 fallback 门禁，并运行 `openspec validate --all`。

## Open Questions

- root-level `search` 是否长期保留为高频入口，还是同时提供 `note search` 作为补充路径。
- `group/tag/folder/kind` 维度浏览最终归属 `note` 还是 `view`，需要根据用户频率和输出形态确认。
- `organize suggest` 是否 hidden alias 到 `organize plan`，还是保留为 agent-oriented 命令名。
