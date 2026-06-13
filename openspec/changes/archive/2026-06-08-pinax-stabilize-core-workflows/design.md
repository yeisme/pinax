## Context

Pinax 当前不是单一 bug，而是多个已经落地能力之间的合同漂移：note path、resolver、index projection、record ledger、MCP、CLI output 和 e2e fixture 对同一事实有不同表达。继续单点修补会导致测试在不同包之间反复移动失败。

稳定化策略是先定义系统级不变量，再按依赖顺序修复：路径口径是 resolver、record、index、MCP 和 output 的共同基础；index freshness 是 search/query/template/MCP 的共同基础；最后再修补用户工作流、命令文档和活跃 OpenSpec 状态。

## Technical Approach

### 1. 建立当前失败基线

- 保留 `go test ./...` 的失败列表作为首轮验收输入。
- 将失败归类到 contract owner，而不是按测试文件随机修复。
- 每个修复 slice 必须有聚焦测试命令和最终全量门禁证据。

### 2. 统一 note path 和 note ref 口径

Pinax 需要明确两种路径：

- `note.path`: 用户可见、vault-relative 的 canonical note path。它等于 vault 内真实 Markdown 路径；默认普通笔记使用 root-level `foo.md`，按 `--dir` 或移动命令进入子目录后使用 `work/foo.md` 等相对路径。journal 和 index page 也是 root-level 系统笔记，但普通 note list/search/query 默认过滤这些系统笔记。
- compatibility ref：resolver 接受 note id、唯一标题、stem、`foo.md`、历史 `notes/foo.md` 以及可唯一匹配的路径别名，但这些只属于输入兼容层，不改变输出主口径。
- `storage_path` 或 internal path：service 内部用于文件系统操作的路径，可保持相对 vault 的真实文件路径。

所有 CLI JSON facts、agent facts、record ledger、resolver candidates、search results、index rows、MCP payload 和 docs 示例必须使用同一用户可见 canonical path。若需要兼容旧 `notes/foo.md`，应只作为 resolver alias，不作为主输出。

### 3. 修复 index freshness 生命周期

所有写 Markdown 或 `.pinax` projection 的 service 必须明确更新或标记 index 状态：

- note create/edit/rename/move/archive/delete/tag/attach/refresh
- journal open/append
- import markdown
- metadata apply、repair apply、organize apply
- template render/note create from template
- index page create/refresh

受控写入路径审计：Markdown 写入包括 note add/create/template note、journal open/append、inbox capture/triage、note refresh rendered、index page create/refresh、import markdown、note move/archive/delete/tag/attach 以及 organize/metadata/repair apply 的低风险写入；`.pinax` structured assets 写入包括 saved views、record ledger、repair/organize/import/export receipts、template render runs、asset manifest、version evidence 和 provider/cloud/backend state；index projection 写入集中在 index refresh/rebuild/sync/repair/init 以及写入后触发的 service-level freshness maintenance。

策略是：命令层不直接修 index；应用 service 在受控写入后调用 refresh 或标记 stale，读取 query/search/MCP 时共享 freshness 判断。`--dry-run`、preview 和 readonly MCP 不得写 Markdown、`.pinax` structured assets 或 index projection。

Query、MCP 和 search 读取 index 时必须共享同一 freshness 判断。`query run` 不应在刚完成受控写入后误判 `property_index_stale`。

### 4. 统一 resolver、record ledger 和 link graph

Resolver 应接受 note id、唯一标题、stem、`foo.md`、`notes/foo.md` 等常见输入，但输出主路径必须一致。record ledger history、asset link、version show/restore、MCP note context 和 e2e 都应复用同一 resolver。

Link graph 的 scan fallback 和 index path 必须输出同等语义：resolved、ambiguous、broken、external、ignored 的计数规则不能因 engine 不同而变化。

### 5. 修复 CLI UX 和 workflow 回归

聚焦当前失败的真实用户流程：

- search 默认表格路径和 snippet。
- template create/render/note create from template。
- render run snapshot completion。
- daily/inbox workflow 和 daily managed index 内容。
- editor/open 参数传递。
- note command UX hardening。
- index refresh/doctor contracts。
- bidirectional links、record ledger、version/asset lookup e2e。

### 6. OpenSpec 和文档收敛

活跃 change 必须分为三类：

- core stabilization 依赖：由本 change 吸收并完成。
- feature continuation：保留原 change，但显式依赖本 change 绿色基线。
- completed/obsolete：归档或关闭，避免重复任务入口。

命令手册应继续保留为 `docs/commands/`，但执行状态只能写 OpenSpec tasks。

## Validation Strategy

按依赖顺序验证：

1. 路径/resolver 聚焦测试：`go test ./cmd/pinax ./internal/app ./internal/records -run 'Path|Resolver|Record|NoteCommand|VersionAsset' -count=1`。
2. Index/query/MCP 聚焦测试：`go test ./internal/index ./internal/app ./internal/mcpserver ./cmd/pinax -run 'Index|Query|Search|MCP' -count=1`。
3. Link graph/e2e 聚焦测试：`go test ./tests/e2e -run 'BidirectionalLinks|RecordLedger|VersionAssetLookup|Index' -count=1`。
4. CLI workflow 聚焦测试：`go test ./cmd/pinax -run 'SearchDefault|Template|DailyInbox|DatabaseView|NoteCommand|IndexRefresh|IndexDoctor' -count=1`。
5. 全量门禁：`go test ./...`、`task check`、`openspec validate --all`。

## Risks

- 路径口径切换可能影响现有 fixtures 和用户脚本。处理方式：resolver 兼容旧输入，输出主口径一次性统一，并在 docs 中说明。
- Index freshness 修复可能引入过度 rebuild。处理方式：优先复用 incremental update，必要时先保证正确性，再 benchmark 优化。
- 活跃 change 太多导致任务重复。处理方式：本 change 只做 core 绿色基线；feature 变更必须显式依赖或归档。

## Deferred

- 真实 cloud/provider sync、briefing delivery、project board 高级工作流不在本 change 内完成。
- 性能专项优化只在 correctness 恢复后单独开 change。
