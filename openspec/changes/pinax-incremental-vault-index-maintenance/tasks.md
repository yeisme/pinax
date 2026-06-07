## 1. Fixture 和测试基线

- [ ] 1.1 新增文件生命周期 fixture vault，覆盖 note_id、无 note_id、内容变更、path move、title rename、delete、trash restore、external edit、ambiguous move candidate。
- [ ] 1.2 新增 full rebuild vs incremental equivalence 测试，比较 note list、search、backlinks、attachments、database properties 和 saved query 结果。
- [ ] 1.3 新增 race/concurrency 测试，覆盖 epoch 丢弃旧结果、事件 coalescing、单 writer transaction。
- [ ] 1.4 新增 benchmark，覆盖 index sync、single note changed、move/rename、delete、large vault unchanged scan。

## 2. Domain 和 Index Records

- [ ] 2.1 在 `internal/domain` 增加 `IndexEvent`、`IndexEventKind`、`IndexEventSource`、`IndexUpdateResult`、`RenameCandidate`、`IndexTombstone`、`IndexConsistencyIssue`。
- [ ] 2.2 扩展 `internal/index` records，保存 file facts：path、note_id、content_hash、size、mtime、title、aliases、schema version、last indexed epoch。
- [ ] 2.3 增加 tombstone records，保存 note_id、old_path、old_hash、title、deleted_at、source、evidence、expires_at。
- [ ] 2.4 更新 `index status` projection，支持 `partial`、pending events、stale path rows、orphan tombstones 和 consistency issue counts。

## 3. Event Coordinator

- [ ] 3.1 实现 index event builder，让 note create/edit/rename/move/delete/archive/tag/metadata/import/organize/repair service 生成结构化事件。
- [ ] 3.2 实现有界 event queue、coalescer 和 runtime counters：queued、coalesced、parsed、planned、committed、skipped、failed、epoch。
- [ ] 3.3 实现 epoch 和 context cancellation；full rebuild/repair 开始时旧 worker 结果不能提交。
- [ ] 3.4 实现单 writer batch commit，所有 SQLite/GORM projection 写入集中在 repository transaction。

## 4. External Sync 和 Reconciliation

- [ ] 4.1 实现 `index sync` file facts scanner，先用 path/size/mtime 快速过滤，候选再计算 hash 和解析 note_id/title。
- [ ] 4.2 实现 diff 分类：created、changed、deleted、same、strong_move、strong_restore、candidate_move、ambiguous。
- [ ] 4.3 实现 note identity matcher，优先 note_id、命令事件、tombstone、Git evidence、content hash，弱候选只报 issue。
- [ ] 4.4 实现 external ambiguous move 输出和 repair plan 接入，不自动猜测。

## 5. Affected Projection Planner

- [ ] 5.1 实现 `PlanIndexUpdate(event, facts)`，为 changed/renamed/moved/deleted/restored 计算受影响 projection。
- [ ] 5.2 实现 content changed 增量：更新 self note/text/token/tag/link/attachment/property/dimension/FTS projection。
- [ ] 5.3 实现 path moved 增量：更新 path/folder/system properties、source path、relative links、old/new path inbound links。
- [ ] 5.4 实现 title/alias changed 增量：更新 title/alias/property/FTS，并重算引用 old/new title/alias 的 link edges。
- [ ] 5.5 实现 delete/tombstone 增量：删除 self projection，重算 incoming links 和 orphan/backlink counts。
- [ ] 5.6 实现 restore 增量：恢复 note projection，清理 tombstone，重算 affected inbound/outgoing links。

## 6. CLI 和输出合同

- [ ] 6.1 Wire `pinax index sync --vault <vault> --json|--agent|--events|--explain`，输出 created/changed/moved/deleted/restored/skipped/candidates/failed facts。
- [ ] 6.2 Wire `pinax index repair` 或 repair plan 接入 index consistency issues：stale path、orphan tombstone、ambiguous move candidate、partial writer failure。
- [ ] 6.3 更新 note mutation projections，暴露 `index_update`、`index_event`、`index_status`、`affected_notes`、`affected_links`。
- [ ] 6.4 增加输出合同测试，确保机器 stdout 无中文 prose/ANSI，错误 envelope 有稳定 code 和 next action。

## 7. Recovery 和 Maintenance

- [ ] 7.1 实现 incremental failure recovery：事务失败保留旧 projection，标记 stale/partial，提供 sync/repair/rebuild action。
- [ ] 7.2 实现 tombstone cleanup 规则，避免 tombstone 无限增长，并保证 cleanup 只写 index/CLI-authored evidence。
- [ ] 7.3 将 doctor/repair plan 接入 index consistency issue，不自动修改 Markdown 正文。
- [ ] 7.4 增加 plan stale 检查，repair apply 前重新比较 vault file facts。

## 8. 验证

- [ ] 8.1 运行聚焦测试：`go test ./internal/domain ./internal/index ./internal/app ./cmd/pinax -run 'IndexEvent|IndexSync|Incremental|Rename|Move|Tombstone|Epoch|Repair' -count=1`。
- [ ] 8.2 运行 race 测试：`go test -race ./internal/index ./internal/app -run 'IndexSync|Incremental|Epoch|SingleWriter' -count=1`。
- [ ] 8.3 运行 benchmark：`go test ./internal/index -bench 'Benchmark(IndexSync|Incremental|Move|Rename|Delete|UnchangedScan)' -benchmem`。
- [ ] 8.4 运行全量门禁：优先 `task check`；没有 task 时运行 `gofmt -w <changed-go-files>`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
- [ ] 8.5 运行 OpenSpec 校验：`openspec validate pinax-incremental-vault-index-maintenance --strict` 和 `openspec validate --all`。
