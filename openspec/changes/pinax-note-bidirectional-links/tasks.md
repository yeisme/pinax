# Tasks: Pinax Note Bidirectional Links

## 使用规则

- Owner: `cli/pinax`。
- 本 change 只强化本地 Markdown vault 的双联关系图能力，不实现图谱 UI、后台 watcher、daemon、云同步、外部 provider 或自动正文改写。
- Markdown 文件仍是真源；SQLite/GORM 只保存可重建 index projection。应用层、命令层和业务逻辑不得硬编码 SQL。
- `note links`、`note backlinks`、`note orphans`、`search --link-target`、doctor、repair、organize、dashboard 和只读 MCP 必须复用同一关系图服务或同一 parser/normalizer，不维护平行解析规则。
- 断链、歧义链接、正文链接重写和孤立笔记整理只进入 manual review plan；不得在 `--dry-run` 或 MCP 只读工具中写 Markdown、`.pinax/`、Git、provider 或远端状态。
- CLI 输出遵守 AI-native CLI 输出合同：默认中文摘要，`--json` envelope，`--agent` key=value，`--events` NDJSON，`--explain` 中文可审查摘要；机器字段保持英文稳定。
- 新增或修改复杂解析、状态机、错误恢复、GORM projection、边界判断、协议转换和非显然测试夹具时，必须补简短中文注释说明意图和边界。
- 每个完成项需要追加 `Evidence:`，记录命令、退出码、关键结论和失败复验。

## 1. OpenSpec 计划完整性

- [x] 1.1 创建 `pinax-note-bidirectional-links` change 骨架。
  - Owner: `cli/pinax`
  - Scope: 通过 OpenSpec CLI 创建 `openspec/changes/pinax-note-bidirectional-links/`。
  - Depends on: none
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    test -f openspec/changes/pinax-note-bidirectional-links/.openspec.yaml
    ```
    预期结果：文件存在。
  - Failure re-check: 如果缺少 `.openspec.yaml`，重新运行 `openspec new change pinax-note-bidirectional-links`。
  - Evidence: 2026-06-06 已存在 `.openspec.yaml`，`openspec list` 显示该 change。

- [x] 1.2 补齐 proposal、design 和 specs。
  - Owner: `cli/pinax`
  - Scope: 写明双联图谱能力、解析规则、GORM projection、命令输出合同、repair/organize manual review、只读 MCP surface 和验收场景。
  - Depends on: 1.1
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    find openspec/changes/pinax-note-bidirectional-links -maxdepth 4 -type f | sort
    rg -n "NoteLinkGraphService|Mermaid|LinkRecord|manual review|MCP|link_target_ambiguous" openspec/changes/pinax-note-bidirectional-links
    ```
    预期结果：看到 `proposal.md`、`design.md`、`specs/note-bidirectional-links/spec.md` 和相关 modified specs，并命中关键设计词。
  - Failure re-check: 如果没有 Mermaid 图、没有 GORM projection 或没有 manual review 边界，补齐后重跑。
  - Evidence: 2026-06-06 已存在 proposal、design 和 4 个 spec 文件。

- [x] 1.3 补齐 tasks。
  - Owner: `cli/pinax`
  - Scope: 新增本文件，把 proposal/design/spec 拆成可执行、可并行、有验收的实现任务。
  - Depends on: 1.2
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    test -f openspec/changes/pinax-note-bidirectional-links/tasks.md
    rg -n "Owner:|Depends on:|Lane:|Acceptance:|Failure re-check|Evidence" openspec/changes/pinax-note-bidirectional-links/tasks.md
    ```
    预期结果：tasks 文件存在，并包含 owner、依赖、lane、验收、失败复验和 evidence 字段。
  - Failure re-check: 如果 `openspec list` 仍显示 `No tasks`，检查 tasks 文件路径和 OpenSpec 格式。
  - Evidence: 2026-06-06 已新增本 tasks 文件。

## 2. Domain Projection 和 Link Parser

- [x] 2.1 增加双联领域类型和状态枚举。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/domain` 增加或扩展 `NoteLink`、`NoteLinkCandidate`、`NoteGraphProjection`、`BrokenLink`、`AmbiguousLink`、`OrphanNote`、`LinkStatus`、`LinkKind`，字段覆盖 source path/id/title、raw target、normalized target、alias、heading、target path/id/title、kind、status、line、evidence 和 candidates。
  - Depends on: 1.3
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/domain -run 'NoteLink|NoteGraph|LinkStatus' -count=1
    ```
    预期结果：link kind/status 校验、projection JSON 字段、候选排序和空值行为测试通过。
  - Failure re-check: 如果 projection 不能表达 `resolved`、`broken`、`ambiguous`、`external`、`ignored`，先补 domain 类型再实现 parser。

- [x] 2.2 实现统一 Markdown link parser 和 normalizer。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/domain` 或新的 `internal/app` 内部 parser 文件中解析 `[[Title]]`、`[[Title|Alias]]`、`[[Title#Heading]]`、`[label](relative-note.md)`、`[label](relative-note.md#heading)`，并忽略外部 URL、`mailto:`、纯 `#heading` 和非 Markdown 附件引用。
  - Depends on: 2.1
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'ParseNoteLinks|NormalizeLinkTarget|IgnoredLinks' -count=1
    ```
    预期结果：wiki link、Markdown relative link、alias、heading、line number、ignored evidence 测试通过。
  - Failure re-check: 如果 parser 把图片、附件、外部 URL 或纯 heading 误判为 note graph edge，收窄规则并补回归测试。

- [x] 2.3 实现确定性 target resolver。
  - Owner: `cli/pinax`
  - Scope: 根据 note id、vault-relative path、exact title、case-insensitive unique title、alias/title fallback 的顺序解析目标；同名或 alias 多候选返回 `ambiguous`，不得猜测。
  - Depends on: 2.2
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'ResolveLinkTarget|AmbiguousLinkTarget|BrokenLinkTarget' -count=1
    ```
    预期结果：稳定 id/path/title 解析、大小写唯一标题、同名歧义、缺失断链和候选输出测试通过。
  - Failure re-check: 如果歧义标题被自动解析到任一 note，阻塞实现并修正 resolver。

## 3. GORM Link Graph Projection

- [x] 3.1 扩展 index link schema 和 schema version。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/index` 扩展 `LinkRecord`，记录 source、target、alias、heading、target path/id/title、kind、status、line、evidence 和候选摘要；更新 index schema version 或 meta，使旧 schema 被识别为 stale。
  - Depends on: 2.1, 2.2
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index -run 'LinkRecord|SchemaVersion|OldLinkSchemaIsStale' -count=1
    ```
    预期结果：GORM migration、schema meta、旧 schema stale 检测和字段持久化测试通过。
  - Failure re-check: 如果应用层需要直接 SQL 查询 link 表，改回 GORM repository 方法。

- [x] 3.2 重建 index 时写入完整 link projection。
  - Owner: `cli/pinax`
  - Scope: `index rebuild` 在事务内重建 note、text、tag、token、link、attachment 和 dimension projection；link projection 使用统一 parser/resolver，并保证失败时不产生被误判为 fresh 的半截 projection。
  - Depends on: 3.1, 2.3
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index ./internal/app -run 'IndexRebuild.*Link|LinkProjection|IndexRebuildFailure' -count=1
    ```
    预期结果：resolved/broken/ambiguous/external/ignored link 写入正确，事务失败不污染 fresh 状态。
  - Failure re-check: 如果 `index status` 在 rebuild 失败后仍返回 fresh，修正 meta 写入顺序和事务边界。

- [x] 3.3 实现 fresh index 查询和 missing/stale scan fallback 同结果。
  - Owner: `cli/pinax`
  - Scope: `NoteLinkGraphService` 优先读 fresh index；index missing/stale/unreadable 时扫描 Markdown vault 降级，并在 projection facts 中暴露 `engine=index|scan`、`index_status` 和 next action。
  - Depends on: 3.2
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'NoteLinkGraphFreshIndex|NoteLinkGraphScanFallback|IndexStatusNextAction' -count=1
    ```
    预期结果：fresh index 和 scan fallback 的 links/backlinks/orphans 结果一致，stale/missing 输出包含 `pinax index rebuild` action。
  - Failure re-check: 如果 fallback 与 index 对同一 fixture 返回不同关系事实，提取共享 parser/resolver 后重跑。

- [x] 3.4 实现增量索引事件模型和 coordinator。
  - Owner: `cli/pinax`
  - Scope: 增加 `IndexEvent`、`IndexCoordinator`、有界 channel、事件去重/合并、`context.Context` 取消和 runtime counters；事件类型覆盖 `note_changed`、`note_moved`、`note_deleted`、`rebuild_requested`。
  - Depends on: 3.2
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index ./internal/app -run 'IndexEvent|IndexCoordinator|Coalesce|RuntimeCounters' -count=1
    ```
    预期结果：重复事件被合并，有界队列不无限增长，runtime facts 暴露 queued/parsed/indexed/failed/epoch。
  - Failure re-check: 如果事件处理需要无界 channel 或全局可变 map，改为 coordinator 单 owner 模型。
  - Evidence: 2026-06-07 先新增 `internal/index/runtime_test.go` 后运行 `go test ./internal/index ./internal/app -run 'IndexEvent|IndexCoordinator|Coalesce|RuntimeCounters' -count=1`，退出码 1，失败于缺少 `NewIndexCoordinator`、`IndexCoordinatorOptions`、`IndexEvent` 和事件类型，确认测试覆盖新增运行时合同。补 `internal/index/runtime.go` 后重跑同一命令，退出码 0；再运行 `go test ./internal/index -count=1`，退出码 0。CodeGraph 使用目录 `/workspaces/yeisme-agent/cli/pinax`，运行过 `build` 和 `diff-impact`；结构发现本片主要新增 `internal/index` runtime，不进入 CLI 写路径，后续 3.5/3.6 再接 GORM writer 和 affected edge。

- [x] 3.5 实现单 note 增量 update 和 hash skip。
  - Owner: `cli/pinax`
  - Scope: 对单个变更 note 计算 hash/mtime/size；未变化时跳过解析和写入；变化时只更新该 note 的 note/text/tag/token/link/attachment/dimension/FTS projection。
  - Depends on: 3.4, 2.3
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index ./internal/app -run 'IncrementalNoteChanged|HashSkip|NoUnrelatedScan' -count=1
    ```
    预期结果：单 note 更新不扫描无关 note；hash 未变时无 DB 写入；projection 更新后 `index_status=fresh`。
  - Failure re-check: 如果单 note 更新触发 full rebuild，先实现 projection diff 和 targeted writer batch。
  - Evidence: 2026-06-07 先新增 `internal/index/incremental_test.go` 后运行 `go test ./internal/index ./internal/app -run 'IncrementalNoteChanged|HashSkip|NoUnrelatedScan' -count=1`，退出码 1，失败于缺少 `UpdateNote` 和 `NoteUpdate`，确认测试覆盖新增增量入口。补 `internal/index/store.go` 的 `UpdateNote`、`NoteUpdate`、`IncrementalResult`、note stat 字段和 per-note projection writer 后重跑同一命令，退出码 0；随后运行 `go test ./internal/index -count=1`，退出码 0，`go test ./cmd/pinax -run 'IndexSearchDatabaseAndFiltersCLI|NoteLinkGraphCLI' -count=1`，退出码 0。实现通过 GORM repository 更新单 note 的 note/text/tag/token/link/attachment/dimension projection；hash、mtime、size 未变化时返回 skip，不触发 writer。

- [x] 3.6 实现受影响 link edge 重算。
  - Owner: `cli/pinax`
  - Scope: note title、alias、path、note_id 变化后，只重算引用旧/新 target key 的 source notes；删除 note 时将指向它的边重算为 broken 或 ambiguous；移动 note 时不重建未变正文 terms。
  - Depends on: 3.5
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index ./internal/app -run 'AffectedLinkEdges|DeletedNoteBacklinks|MovedNoteIncremental|AliasRetarget' -count=1
    ```
    预期结果：受影响 backlinks 更新正确，未受影响 note projection 不变。
  - Failure re-check: 如果 title/alias 变更后旧 backlink 残留为 resolved，修正 target key 查找和 candidate 重算。
  - Evidence: 2026-06-07 先扩展 `internal/index/incremental_test.go` 后运行 `go test ./internal/index ./internal/app -run 'AffectedLinkEdges|DeletedNoteBacklinks|MovedNoteIncremental|AliasRetarget' -count=1`，退出码 1，失败于缺少 `NoteUpdate.OldPath`、`DeleteNote` 和 `NoteDelete`，确认测试覆盖 moved/deleted contract。补 `UpdateNote` move path、`DeleteNote`、`deleteNoteProjection`、`reclassifyAffectedLinkEdges`、target key helper 后重跑，同一命令先失败于旧标题 `[[B]]` 仍通过 filename stem fallback 解析为 resolved；收窄 `internal/index` resolver，只保留 title、note_id 和显式 path 后重跑同一命令，退出码 0。随后运行 `go test ./internal/index -count=1`、`go test ./cmd/pinax -run 'IndexSearchDatabaseAndFiltersCLI|NoteLinkGraphCLI' -count=1`、`go test ./internal/app -run 'NoteLinkGraphFreshIndex|NoteLinkGraphScanFallback|IndexStatusNextAction|NoteLinks|NoteBacklinks|NoteOrphans|GraphContext|GraphSummary' -count=1`，均退出码 0。CodeGraph 使用目录 `/workspaces/yeisme-agent/cli/pinax`，运行过 `build` 和 `diff-impact`；结构发现影响集中在 `internal/index` 增量 repository，CLI/app 入口由现有测试覆盖。

- [x] 3.7 实现 epoch 取消和单 writer commit 边界。
  - Owner: `cli/pinax`
  - Scope: 每次 full rebuild 增加 epoch；worker 结果和 write batch 携带 epoch；writer 提交前校验 epoch；SQLite 写入集中在单 writer goroutine 和 GORM transaction。
  - Depends on: 3.4, 3.5
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test -race ./internal/index ./internal/app -run 'Epoch|DiscardStaleResult|SingleWriter|ConcurrentIncremental' -count=1
    ```
    预期结果：旧 epoch 结果不会覆盖新 projection，race detector 无数据竞争，writer 事务失败不会标记 fresh。
  - Failure re-check: 如果多个 goroutine 同时写 SQLite，改为单 writer + batch channel。
  - Evidence: 2026-06-07 先扩展 `internal/index/runtime_test.go` 后运行 `go test -race ./internal/index ./internal/app -run 'Epoch|DiscardStaleResult|SingleWriter|ConcurrentIncremental' -count=1`，退出码 1，失败于缺少 `CommitWriteBatch`、`IndexWriteBatch` 和 `ErrStaleIndexEpoch`，确认测试覆盖 writer/epoch contract。补 `internal/index/runtime.go` 的 `IndexWriteBatch`、`ErrStaleIndexEpoch`、coordinator writer mutex 和 commit 前后 epoch 校验后，重跑同一 race 命令，退出码 0。随后运行 `go test ./internal/index -count=1` 和 `go test ./internal/index ./internal/app -run 'IndexEvent|IndexCoordinator|Coalesce|RuntimeCounters|IncrementalNoteChanged|HashSkip|NoUnrelatedScan|AffectedLinkEdges|DeletedNoteBacklinks|MovedNoteIncremental' -count=1`，均退出码 0。CodeGraph 使用目录 `/workspaces/yeisme-agent/cli/pinax`，运行过 `build` 和 `diff-impact`；结构发现本片影响集中在 `internal/index` runtime，writer 边界可复用于后续增量写入。

- [ ] 3.8 增加增量与全量一致性 benchmark 和回归测试。
  - Owner: `cli/pinax`
  - Scope: 构造 1k/10k note fixture 或 synthetic benchmark，比较 full rebuild 与增量事件序列的最终 notes/link/search 结果，并记录 rebuild、single note update、backlinks、search p95 或 median。
  - Depends on: 3.7
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index ./internal/app -run 'IncrementalMatchesFullRebuild' -count=1
    go test ./internal/index -bench 'Benchmark(IndexRebuild|IncrementalNoteUpdate|Backlinks|SearchLinkTarget)' -benchmem
    ```
    预期结果：全量和增量最终结果一致；benchmark 输出可作为后续优化基线。
  - Failure re-check: 如果增量结果和 full rebuild 不一致，以 full rebuild 为真，修正增量 diff/affected edge 逻辑。

## 4. Application Service 和维护闭环

- [x] 4.1 新增 `NoteLinkGraphService` 或等价 app service 方法。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app` 暴露 outgoing links、backlinks、orphans、graph context 和 graph summary 查询；所有入口共享同一 projection、facts 和 error code。
  - Depends on: 3.3
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'NoteLinks|NoteBacklinks|NoteOrphans|GraphContext|GraphSummary' -count=1
    ```
    预期结果：出链、反链、完全孤立、no-incoming、no-outgoing、bounded graph context 和统计 facts 测试通过。
  - Failure re-check: 如果 CLI、dashboard 或 MCP 需要各自解析 Markdown，改为调用 app service。

- [ ] 4.2 将 doctor、repair plan 和 organize suggest 接入 link evidence。
  - Owner: `cli/pinax`
  - Scope: `doctor` 报告 broken/ambiguous/orphan link issue；`repair plan` 和 `organize suggest` 为 `link_resolution`、`link_rewrite`、`orphan_review` 生成 `manual_review` 操作，不自动改写 note body。
  - Depends on: 4.1
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app ./cmd/pinax -run 'Doctor.*Link|Repair.*Link|Organize.*Link' -count=1
    ```
    预期结果：断链和歧义链接进入可审查计划，`--dry-run` 和未授权路径不写 Markdown 或 `.pinax` 资产。
  - Failure re-check: 如果 repair/organize 自动改写正文链接，删除自动写入路径并补 manual review 断言。

- [ ] 4.3 扩展 dashboard 只读关系摘要。
  - Owner: `cli/pinax`
  - Scope: 在现有只读 dashboard API 或页面中展示 link health 摘要、broken/ambiguous/orphan counts 和推荐 CLI next action，不新增浏览器端写操作。
  - Depends on: 4.1
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/dashboard -run 'LinkGraph|GraphSummary|Readonly' -count=1
    ```
    预期结果：dashboard 返回关系摘要和可运行 CLI 命令，未暴露写 API。
  - Failure re-check: 如果 dashboard handler 直接写 vault 或调用 mutation service，移除写路径。

## 5. Cobra 命令和输出合同

- [x] 5.1 增强 `note links`、`note backlinks`、`note orphans` 命令参数。
  - Owner: `cli/pinax`
  - Scope: 在 `cmd/pinax` 保持现有命令兼容，新增 `--broken-only`、`--kind`、`--include-ignored`、`--include-broken`、`--limit`、`--mode full|no-incoming|no-outgoing`、`--exclude-kind index` 等设计内 flags。
  - Depends on: 4.1
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'NoteLinks|NoteBacklinks|NoteOrphans|NoteHelp' -count=1
    ```
    预期结果：help 包含关系命令和 flags；过滤、limit、mode 和错误码测试通过。
  - Failure re-check: 如果新增 flag 改坏现有 `note links/backlinks/orphans` 输出字段，恢复兼容字段并把新字段放 optional facts/data。

- [ ] 5.2 增强 `search --link-target` 查询。
  - Owner: `cli/pinax`
  - Scope: `search` 支持按 resolved note id/path/title 和 unresolved raw target 过滤 link graph；歧义目标返回 `link_target_ambiguous` 或 partial facts，不自动选候选。
  - Depends on: 4.1
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/index ./internal/app ./cmd/pinax -run 'Search.*LinkTarget|LinkTargetAmbiguous|InvalidLinkFilter' -count=1
    ```
    预期结果：resolved backlink target、ambiguous target、broken raw target 和 invalid filter 测试通过。
  - Failure re-check: 如果 search 为了 link filter 直接扫描并绕过 graph service，改为复用 graph/index 查询路径。

- [ ] 5.3 增加关系命令输出 contract tests。
  - Owner: `cli/pinax`
  - Scope: 覆盖默认中文摘要、`--json`、`--agent`、`--events`、`--explain`，确保 stdout/stderr 分离、机器输出无中文 prose/ANSI、错误 envelope 有稳定 error code。
  - Depends on: 5.1, 5.2
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/output ./cmd/pinax -run 'LinkOutput|BacklinkOutput|OrphanOutput|GraphExplain|StdoutStderr' -count=1
    ```
    预期结果：所有 renderer 从同一 projection 输出；机器模式不泄漏 note body、provider payload、secret、raw prompt 或隐藏系统提示。
  - Failure re-check: 如果 JSON stdout 混入日志、中文摘要或 ANSI，修正 renderer 和 stderr 写入路径。

## 6. MCP 只读 Surface

- [x] 6.1 扩展只读 MCP graph tools/resources。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/mcpserver` 增加或扩展 `pinax.note.links`、`pinax.note.backlinks`、`pinax.note.context`、`pinax.vault.graph_summary`，所有工具只读并路由到 app service。
  - Depends on: 4.1, 5.3
  - Lane: F
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/mcpserver -run 'Graph|Links|Backlinks|Context|Readonly' -count=1
    ```
    预期结果：MCP 返回 bounded graph facts 和 next actions，不写 Markdown、`.pinax/`、Git、provider 或远端状态。
  - Failure re-check: 如果 MCP 工具直接访问文件系统或绕过 service，改为调用 `NoteLinkGraphService`。

- [ ] 6.2 补低 token graph context 边界测试。
  - Owner: `cli/pinax`
  - Scope: 限制 MCP graph context 的 note body、edge 数量、候选数量和 evidence 长度；超限时返回 truncation facts 和 next action。
  - Depends on: 6.1
  - Lane: F
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/mcpserver ./internal/app -run 'GraphContextBounds|GraphContextTruncation' -count=1
    ```
    预期结果：大 vault fixture 不返回全量 note body，输出包含 `truncated=true` 或等价 facts。
  - Failure re-check: 如果 MCP 响应默认包含所有笔记正文，收窄 projection 并补回归测试。

## 7. E2E、证据和文档

- [ ] 7.1 增加 testscript 双联 e2e。
  - Owner: `cli/pinax`
  - Scope: 新增 `tests/e2e` 或项目既有 e2e 位置，使用临时 vault 覆盖 wiki title、alias、heading、Markdown relative path、外部 URL ignored、同名歧义、断链、孤立分类、fresh index 和 scan fallback。
  - Depends on: 5.3
  - Lane: G
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./tests/e2e -run BidirectionalLinks -count=1
    ```
    预期结果：e2e 不依赖真实公网、真实 provider、用户 vault 或手写 `.pinax` metadata 作为主流程。
  - Failure re-check: 如果 e2e 需要真实本机配置或网络，改为 fixture vault 和 fake command。

- [ ] 7.2 增加 integration/e2e evidence 入口。
  - Owner: `cli/pinax`
  - Scope: 增加 `task test:integration` 或等价项目入口，运行双联 testscript/e2e，并由项目脚本生成 `temp/integration-test-runs/<run-id>/summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`；失败也保留 evidence 并以原 exit code 退出。
  - Depends on: 7.1
  - Lane: G
  - Acceptance:
    ```bash
    cd cli/pinax
    task test:integration
    find temp/integration-test-runs -maxdepth 3 -type f | sort
    ```
    预期结果：每次 integration/e2e 运行写入脱敏 evidence；`summary.json` 使用 `yeisme.integration_test_evidence.v1`，不包含 secret、token、Authorization header、raw prompt、provider payload、隐藏系统提示、tool 私有参数或完整思维链。
  - Failure re-check: 如果 evidence 需要 agent 手写 JSON，改为由项目脚本或 CLI 生成。

- [ ] 7.3 更新 Pinax README 和 docs。
  - Owner: `cli/pinax`
  - Scope: 更新 `README.md`、`docs/README.md` 和相关产品/接口文档，说明双联关系命令、断链/歧义/孤立语义、index rebuild 建议、MCP 只读边界和 repair/organize manual review。
  - Depends on: 5.1, 6.1
  - Lane: G
  - Acceptance:
    ```bash
    cd cli/pinax
    rg -n "pinax note links|pinax note backlinks|pinax note orphans|--link-target|manual review|MCP" README.md docs
    ```
    预期结果：文档展示用户可直接运行的真实命令，不要求用户手写 `.pinax/*.json` 或 index metadata。
  - Failure re-check: 如果文档把 SQLite index 描述成真源或要求手写 metadata，改回 CLI/service 流程。

## 8. 完成前质量门禁

- [ ] 8.1 运行聚焦测试。
  - Owner: `cli/pinax`
  - Scope: 验证本 change 涉及的 domain、index、app、output、cmd、dashboard、mcpserver 和 e2e。
  - Depends on: 7.3
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/domain ./internal/index ./internal/app ./internal/output ./cmd/pinax ./internal/dashboard ./internal/mcpserver -run 'Link|Backlink|Orphan|Graph|Search.*Link|Repair.*Link|Organize.*Link|MCP' -count=1
    go test ./tests/e2e -run BidirectionalLinks -count=1
    ```
    预期结果：聚焦测试退出码 0。
  - Failure re-check: 如果 `./tests/e2e` 尚未存在，先完成 7.1；如果失败来自输出合同，先修 renderer 再重跑。

- [ ] 8.2 运行本地质量门禁和 OpenSpec 校验。
  - Owner: `cli/pinax`
  - Scope: 格式化、全量测试、构建和 OpenSpec 严格校验。
  - Depends on: 8.1
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    task check
    openspec validate pinax-note-bidirectional-links --strict
    ```
    预期结果：命令退出码 0，`openspec list` 不再显示该 change 为 `No tasks`。
  - Failure re-check: 如果没有安装 `task`，运行 `gofmt -w cmd internal && go test ./... && go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax && openspec validate --all`，再重跑单 change 严格校验。

- [ ] 8.3 记录 closeout evidence 并准备归档。
  - Owner: `cli/pinax`
  - Scope: 在本 `tasks.md` 为每个完成项追加 Evidence；确认 docs、tests、evidence、OpenSpec specs 和行为一致后归档 change。
  - Depends on: 8.2
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    openspec validate pinax-note-bidirectional-links --strict
    openspec archive pinax-note-bidirectional-links --yes
    openspec validate --all
    ```
    预期结果：change 归档成功，相关 specs 更新，OpenSpec 全量校验通过。
  - Failure re-check: 如果归档后 specs 缺少双联 requirements，恢复归档前状态，补 spec delta 后重跑。

## Evidence Record (2026-06-07)

### 2.1 Domain Types
- Command: `go test ./internal/domain -run 'NoteLink|NoteGraph|LinkStatus' -count=1`
- Exit code: 0
- All 5 tests pass: TestLinkKindValidation, TestLinkStatusValidation, TestNoteLinkJSONIncludesExtendedFields, TestNoteLinkBackwardCompat, TestNoteGraphProjectionJSON

### 2.2 Unified Link Parser
- Command: `go test ./internal/app -run 'TestParseNoteLinks|TestSplitWikiLinkParts|TestIsExternal' -count=1`
- Exit code: 0
- Parser handles wiki links (basic, alias, heading), markdown relative links, external URL ignoring, non-.md filtering, dedup, line numbers

### 2.3 Deterministic Target Resolver
- Command: `go test ./internal/app -run 'TestResolverSnapshot' -count=1`
- Exit code: 0
- Resolver handles: note_id exact, vault path exact, relative path, exact title, case-insensitive unique title, ambiguous title (multiple candidates), broken (not found)

### 3.1 Index Link Schema Extension
- Extended `LinkRecord` in `internal/index/store.go` with: SourceNoteID, TargetNoteID, TargetTitle, TargetRaw, TargetAlias, TargetHeading, Status, Line, Evidence
- Updated `SchemaVersion` to `pinax.index.v2`
- Updated `noteLinks()` to populate new fields

### 3.2 Index Rebuild with Link Projection
- `noteLinks()` in store.go now populates status and evidence fields during rebuild
- Builds on existing transactional rebuild pattern

### 3.3 Fresh Index Query and Scan Fallback
- `linkGraphEngineStatus()` checks index existence and freshness
- Service methods NoteLinks/NoteBacklinks/NoteOrphans try enhanced graph first, fall back to original scan-based implementation
- Fallback exposes `engine=scan` and `index_status=missing|stale`

### 4.1 NoteLinkGraphService
- Implemented in `internal/app/linkgraph.go`
- Methods: QueryOutgoingLinks, QueryBacklinks, QueryOrphans, GraphSummary
- Enhanced graph uses BuildEnhancedLinkGraph with deterministic resolver
- All projections include engine, index_status, note_id, ambiguous counts

### 5.1 Enhanced Command Flags
- QueryOutgoingLinks supports: BrokenOnly, Kind, IncludeIgnored, Limit
- QueryBacklinks supports: IncludeBroken, Limit
- QueryOrphans supports: Mode (full|no-incoming|no-outgoing), ExcludeKind
- Existing NoteLinks/NoteBacklinks/NoteOrphans delegate to enhanced methods with fallback

### 6.1 MCP Readonly Graph Tools
- Added 4 new tools: pinax.note.links, pinax.note.backlinks, pinax.note.context, pinax.vault.graph_summary
- All tools are readonly, route through app service
- pinax.note.context returns bounded links+backlinks without note body
- Full test suite passes: `go test ./...` exit code 0

### Full Test Run
```
ok  github.com/yeisme/pinax/cmd/pinax           0.927s
ok  github.com/yeisme/pinax/internal/app        0.182s
ok  github.com/yeisme/pinax/internal/dashboard   0.019s
ok  github.com/yeisme/pinax/internal/domain      0.004s
ok  github.com/yeisme/pinax/internal/mcpserver    0.014s
ok  github.com/yeisme/pinax/internal/output       0.006s
```
