## 1. 领域模型和资产边界

- [ ] 1.1 定义 `NoteRecord`、`RecordEvent`、`LedgerState`、`Tombstone`、`RecordIssue`、`RecordRepairOperation`、`VersionEvidence`、`ContentRevision`、`IndexSnapshot` 领域模型和生命周期状态机。
- [ ] 1.2 定义 `.pinax/records/events.jsonl`、`notes.json`、`schemas.json`、`tombstones.json`、`version.json` 和可选 diff/snapshot receipt 的 schema version、路径边界、redaction 和校验规则。
- [ ] 1.3 明确 Markdown content source 与 record source 的读写优先级，并把 frontmatter mirror 冲突规则写入领域测试。

## 2. Ledger Service 和 Repository

- [ ] 2.1 实现 record ledger service，支持 init、append event、materialize registry、replay、status scan、version evidence attach 和 idempotent event 写入。
- [ ] 2.2 实现 CLI-authored structured asset repository，保证命令层不直接手写 `.pinax/records/**` JSON/JSONL。
- [ ] 2.3 实现单写入者 mutation coordinator，串行分配 event seq 和 registry materialization，并暴露 pending count、last duration、registry version 诊断。
- [ ] 2.4 为 append-only event sequence、registry rebuild、重复事件、非法生命周期转换、并发 mutation 和损坏 JSONL 增加单元/集成测试。

## 3. VersionBackend 和 Git Evidence

- [ ] 3.1 定义 `VersionBackend` adapter contract，覆盖 detect、current evidence、file revision、diff summary、snapshot 和 read-at-revision capability。
- [ ] 3.2 实现 Git version backend，通过 git adapter/service 获取 HEAD、branch、worktree status、file blob id、changed paths、diff summary 和 snapshot receipt。
- [ ] 3.3 实现 no-backend fallback，记录 content hash、mtime、size、ledger seq，并输出配置 Git 或其他 backend 的 next action。
- [ ] 3.4 增加二进制附件 evidence 规则，记录 object id、checksum、size、MIME 和 backend revision，禁止把二进制 payload 写入 ledger、索引、stdout 或 fixture。
- [ ] 3.5 为 Git clean/dirty、detached HEAD、untracked file、rename、binary attachment、unsupported read-at-revision 增加 fake git/testscript 覆盖。

## 4. Note 命令接入

- [ ] 4.1 将 `note new/create` 接入 ledger service，成功创建时同时写 Markdown、frontmatter mirror、record event、version evidence 和 registry projection。
- [ ] 4.2 将 `note rename/move/archive/delete/restore` 接入 ledger lifecycle，保证路径、状态、tombstone、version evidence 和输出 facts 一致。
- [ ] 4.3 更新 `--json`、`--agent` 输出 projection，暴露 note id、record version、ledger status、version backend、revision id、worktree state、index status 和 repair next action。
- [ ] 4.4 为 dry-run 增加 event/registry/version evidence preview，验证 dry-run 不写 Markdown、`.pinax/`、Git、provider 或远端状态。

## 5. Adoption、Metadata 和 Repair

- [ ] 5.1 实现 `pinax record init/status/adopt/history` 命令入口和应用服务编排。
- [ ] 5.2 实现 adoption plan，覆盖 unregistered note、missing mirror、duplicate note id、path conflict 和 active record missing file。
- [ ] 5.3 更新 metadata plan/apply，使缺失或冲突的 Pinax-managed frontmatter mirror 从 ledger 生成可审批修复。
- [ ] 5.4 更新 doctor/repair plan/apply，支持 record_missing、record_file_missing、record_frontmatter_mismatch、note_id_conflict、schema_type_conflict、orphan_tombstone、version_read_unavailable 和 replay failure。

## 6. Index Projection 和版本检索

- [ ] 6.1 更新 index rebuild 输入为 record ledger + Markdown files + version evidence，路径不再作为 note identity。
- [ ] 6.2 在 SQLite/GORM index projection 中加入 lifecycle、record status、record version、ledger seq、index epoch、version backend、revision id、worktree state、file blob id、diff summary hash、content hash、path status 和 consistency issue 字段。
- [ ] 6.3 更新 search/list/stats 输出，默认排除 deleted/trashed lifecycle，并为不一致结果提供 record repair next action。
- [ ] 6.4 实现 `pinax search --at HEAD`、`--include-dirty`、`--changed-since <rev>`、`--revision <rev>` 的参数校验、capability 检测、fallback 和稳定错误输出。
- [ ] 6.5 设计 `IndexSnapshot` metadata，支持当前 projection、HEAD projection 和按需历史 projection 的缓存/失效策略。
- [ ] 6.6 实现 `index refresh` 增量路径，基于 ledger seq、version evidence、mtime/size、content hash 和 projection row 缺失跳过未变更 note。
- [ ] 6.7 实现 SQLite WAL、批量 upsert、bounded batch transaction 和 index epoch 提交语义。
- [ ] 6.8 实现 `--memory-budget low|normal|high` 对 parse worker、queue capacity、batch size、snippet 生成和历史 snapshot cache 的影响。

## 7. 并发、性能和增量验证

- [ ] 7.1 设计单写入者 ledger/index mutation 队列或锁边界，避免并发 note mutation 破坏 event sequence 和 version evidence 对齐。
- [ ] 7.2 使用 Go `context`、goroutine worker、channel backpressure 和 atomic epoch/version 标记实现扫描、索引和状态读取的并发边界。
- [ ] 7.3 增加 1k、10k、50k notes fixture generator，覆盖 tags、wiki links、frontmatter properties、attachments、renames、dirty Git changes 和 binary references。
- [ ] 7.4 增加 benchmark，覆盖首次 adopt/rebuild、后续增量 status/index refresh、fresh search、fallback scan、changed-since 搜索、HEAD 搜索的 wall time、alloc/op、RSS/heap 和写放大。
- [ ] 7.5 增加 race/concurrency 测试，覆盖并发 create/rename/delete、重复 apply、stale repair plan、registry replay、Git dirty evidence 变化和 cancel/drain。
- [ ] 7.6 增加 pprof/profile 入口或文档化 benchmark profile 命令，用于 CPU、heap、block/lock contention 定位。
- [ ] 7.7 在 `index status`、`record status` 或 `doctor` JSON 输出中暴露 last batch duration、changed/skipped counts、worker count、batch size、queue depth、index epoch 和 ledger seq。

## 8. 质量门禁

- [ ] 8.1 增加 testscript 覆盖 record init/status/adopt、note mutation、metadata repair、index rebuild/search、version-aware search 和 repair apply 的完整用户流程。
- [ ] 8.2 运行 `go test -bench . -benchmem ./...` 或聚焦 benchmark，并记录性能基线；实现优化后用同一命令复测。
- [ ] 8.3 运行 `gofmt -w <changed-go-files>`、`go test ./...`、`go test -race ./...` 和 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [ ] 8.4 运行 `openspec validate pinax-vault-record-ledger --strict` 和 `openspec validate --all`，记录验证结果。
