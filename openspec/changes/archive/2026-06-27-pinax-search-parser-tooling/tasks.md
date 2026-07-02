# 任务

- [x] 1. 搜索 CLI 合同：为 `pinax search` 增加 `--engine auto|index|native`、`--lazy-index auto|off|sync`，补齐 completion/help/contract tests。
- [x] 2. 内置 native 搜索：实现不依赖外部 `rg` 的 case-insensitive fixed-string 基础搜索，复用现有 filters/projection。
- [x] 3. 懒加载索引策略：支持 `--lazy-index off` 禁止 search 写入索引，`--engine index` 不触发 fallback 或 lazy rebuild。
- [x] 4. Markdown parser：新增 `internal/markdownnote`，覆盖 frontmatter、title/headings、links/assets、tasks、inline properties、fenced blocks 的解析测试。
- [x] 5. SQL 索引搜索：让 `internal/index.Search` 使用 `SearchTokenRecord` 倒排表缩小候选，只对候选 note 加载 text/tag/link/attachment 投影，并添加 token-index 回归测试。
- [x] 6. 全量索引批量写入：将 `Rebuild` 的 note/text/tag/token/link/attachment/dimension projection 写入改为 GORM `CreateInBatches`，保持 SQLite 单 writer 事务边界。
- [x] 7. 增量索引 DB 复用：让 `Sync` 复用同一个 GORM 连接调用 update/delete helper，避免每个 note 重新 open/migrate 数据库。
- [x] 8. 增量索引并发解析：`index refresh` 使用有界并发扫描/解析 Markdown，`RefreshChanged`/`Sync` 复用单个 GORM 连接执行 SQLite 单 writer 更新，补充删除投影、稳定 failed paths、benchmark 和 race 验证。
- [x] 9. 交互选择延期决策：延期 `pinax search pick <query>`；后续如需要，基于 SQL 搜索结果实现内置 TUI，不集成外部 `fzf`。
- [x] 10. 文档和验证：更新 `docs/commands/search.md`、`docs/commands/index.md`，运行 focused tests、race/build、OpenSpec 校验和 `task check`。

## 验证命令

```bash
go test ./internal/markdownnote ./internal/search ./internal/app/searchops ./internal/index -count=1
go test ./internal/index -run TestSearchUsesTokenIndexForBodyMatches -count=1
go test ./internal/app -run 'TestScanIndexRefreshNotesReturnsStableOrdinaryNotesAndFailedPaths|TestIndexRefreshChangedSinceUsesVersionBackendWithoutDeletingUnchangedNotes' -count=1
go test ./internal/index -run TestRefreshChangedDeletesRemovedNoteProjection -count=1
go test ./internal/index -run '^$' -bench 'BenchmarkIndexRefreshSkipsUnchanged|BenchmarkIndexSyncUnchangedScan|BenchmarkIncrementalNoteUpdate' -benchmem -count=1
go test -race ./internal/index ./internal/app -run 'TestIndexRefresh|TestRefreshChanged|TestIndexSync|TestIncremental|TestScanIndexRefreshNotes' -count=1
go test ./cmd/pinax -run 'TestSearch|TestIndexSearch' -count=1
go test -race ./internal/index ./internal/search ./internal/app/searchops ./cmd/pinax
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate pinax-search-parser-tooling --strict
```
