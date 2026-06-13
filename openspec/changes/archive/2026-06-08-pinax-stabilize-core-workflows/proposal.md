## Why

Pinax 已经形成本地 notebook core 的功能轮廓：init、journal/inbox、note、search/index、links/backlinks/orphans、import/export、template、metadata/repair/organize、version、asset、dashboard、MCP 和命令手册都已有实现或文档入口。但当前仓库不是绿色状态，继续扩展 provider、briefing、cloud、project board 或更多命令会放大集成债务。

2026-06-08 运行 `go test ./...` 失败，失败面覆盖 `cmd/pinax`、`internal/app`、`internal/mcpserver` 和 `tests/e2e`。主要症状包括：

- note path 口径不一致：部分输出、resolver 和 e2e 期待 `notes/foo.md`，当前实现返回 `foo.md` 或 journal root path。
- index freshness 生命周期不一致：note create、journal/template/index refresh 后，query/MCP/search 仍可能看到 stale projection。
- resolver 与 record ledger 口径不一致：`record history notes/alpha.md` 找不到刚创建的 `alpha.md` record。
- link graph 合同漂移：index path 下 resolved count 与 scan/e2e 期望不同。
- CLI completion、render run、daily index、template note 创建等用户流程存在回归。
- OpenSpec 活跃 change、归档 change、docs/commands 和 specs 已经很多，需要一个总控任务把后续收敛工作整合起来。

本 change 的目的不是新增产品功能，而是把 Pinax 拉回可持续开发状态：统一核心合同、恢复绿色门禁、明确后续能力推进顺序。

## What Changes

- 建立 Pinax core workflow stabilization 总任务，成为后续修复和收敛的主 OpenSpec 入口。
- 先冻结当前失败证据，按失败类别拆分执行任务：路径口径、index freshness、resolver/record、link graph、CLI UX/completion、template/journal/index page、MCP query。
- 明确稳定化完成标准：`go test ./...`、`task check`、`openspec validate --all` 通过，且当前用户可见文档与 help 主路径一致。
- 将已实现但未稳定的能力按状态分层：core 必须绿色，provider/briefing/cloud/project board 等后续能力不得阻塞 core 绿色基线。
- 统一命令手册与 OpenSpec 生命周期：后续执行状态只写在本 change 的 `tasks.md` 或具体 owner change，不再散落到 ad hoc docs checklist。

## Non-Goals

- 不在本 change 内新增 cloud 后端、provider 真同步、真实飞书投递、长期 daemon 或外部公网依赖。
- 不重新设计 Pinax 产品定位；Markdown vault 仍是真源，SQLite/GORM 仍是可重建 projection。
- 不删除兼容 alias；只修复主路径和兼容路径的合同一致性。
- 不要求一次性完成所有活跃 feature change；本 change 先恢复核心基线，再决定是否继续、拆分或归档它们。

## Capabilities

### New Capabilities

- `core-workflow-stabilization`: 定义 Pinax 核心工作流稳定化门禁、路径口径、index freshness、resolver 一致性和后续 change 收敛规则。

### Modified Capabilities

- `note-command-ux`: 统一 note path、note ref、editor/open、delete/trash、rendered view 和 completion 的用户可见合同。
- `notebook-index-search`: 修复 index freshness、search/query、link-target、link graph 和 stale 行为。
- `vault-record-ledger`: 修复 record resolver 与 note path 口径一致性。
- `note-bidirectional-links`: 统一 scan fallback 与 fresh index 的 link count 和 resolved/ambiguous/broken 语义。
- `cli-tree-ux`: 保持命令 help/commands docs 与主路径一致。

## Impact

- 代码：`internal/app`、`internal/cli`、`internal/index`、`internal/records`、`internal/mcpserver`、`internal/output`、`cmd/pinax` 和 `tests/e2e`。
- 测试：以当前 `go test ./...` 失败为回归清单，补充聚焦测试后再恢复全量门禁。
- 文档：同步 `README.md`、`docs/README.md`、`docs/commands/**` 中的路径口径和命令状态。
- OpenSpec：本 change 的 `tasks.md` 是稳定化执行总清单；其它活跃 change 只保留明确未完成的 feature work，已完成或被本 change 吸收的必须归档或标明依赖。
