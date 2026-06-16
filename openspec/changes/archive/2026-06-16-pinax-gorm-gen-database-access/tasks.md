# Tasks: Pinax GORM Gen Database Access

Owner: `cli/pinax`  
Priority: P0 governance compliance  
Non-goal: vault schema redesign, remote service work, provider work

## 0. Baseline

- [x] 0.1 Owner: `cli/pinax`; Lane: sequential; Depends on: none; Scope: inventory every `internal/index` GORM model, direct GORM business query, and raw/PRAGMA call. Acceptance: implementation notes list migrated files and any proposed exception helper. Validation command: `go test ./internal/index -count=1`. Expected result: current index tests pass or unrelated failures are recorded before edits. Failure re-check: do not change implementation before knowing baseline behavior.

  Evidence: 基线 `go test ./internal/index -count=1` 通过。盘点结果：14 个 GORM 模型（IndexMeta/Note/NoteText/Tag/Link/SearchToken/Attachment/Asset/AssetLink/VaultFile/Folder/DimensionCount/PropertyDefinition/PropertyValue）原集中在 `internal/index/store.go`；普通业务查询使用 `db.Find/db.Where/db.Create/db.Save/db.Delete/db.Model().Count/db.Order().Find` 等直接 GORM 链；唯一 raw 调用是 `db.Raw("PRAGMA schema_version")`。

- [x] 0.2 Owner: `cli/pinax`; Lane: sequential; Depends on: 0.1; Scope: reconcile existing generated `internal/index/query` code with unchecked OpenSpec task state before further migration. Acceptance: implementation notes state whether generated DAO files are retained, regenerated, or deleted and identify the exact generator command; Validation command: `go test ./internal/index/query ./internal/index -run 'GormGen|Query|Store' -count=1`; Expected result: generated package compiles or missing generator state is fixed first. Failure re-check: do not mark foundation tasks complete until generated code provenance is clear.

  Evidence: 生成代码来源已明确：`internal/index/gormgen/main.go`（package main）打开临时 SQLite、`AutoMigrate(model.AllModels()...)`、`ApplyBasic` 生成 `internal/index/query`。生成命令 `task gen:index`（等价 `go run ./internal/index/gormgen`）。所有 `*.gen.go` 是生成产物，`query` 只 import `model`，业务代码通过类型别名复用模型。`internal/index/query` 与 `internal/index` 均编译通过。

## 1. GORM Gen foundation

- [x] 1.1 Owner: `cli/pinax`; Lane: A; Depends on: 0.1; Scope: add `gorm.io/gen` dependency if absent and create `internal/index/gormgen` for all index projection models. Acceptance: generated `internal/index/query` package compiles and includes note, note text, tag, link, token, attachment, asset, folder, dimension, property and vault file records. Validation command: `go test ./internal/index -run 'GormGen|Index|Store' -count=1`. Expected result: generated DAO compiles. Failure re-check: fix model metadata rather than hand-writing missing query code.

  Evidence: 新增 `gorm.io/gen` 依赖（同步升级 `gorm.io/plugin/dbresolver` 到 v1.6.2 以兼容 gorm v1.31）。模型移到 `internal/index/model/records.go`；生成器 `internal/index/gormgen/main.go` 生成 `internal/index/query`，含全部 14 张表的类型化 DAO。`internal/index` 通过类型别名复用模型，`query` 只 import `model`，无循环导入。

- [x] 1.2 Owner: `cli/pinax`; Lane: A; Depends on: 1.1; Scope: add source guard test for `internal/index` ordinary business files. Acceptance: test fails on `database/sql`, `.Raw(`, `.Exec(`, SQL verb strings, and direct GORM query chains outside approved connection/migration/helper files. Validation command: `go test ./internal/index -run 'Forbidden|SQL|GormGen|Repository' -count=1`. Expected result: guard enforces the rule. Failure re-check: keep allowlist path-specific and narrow.

  Evidence: `internal/index/guard_test.go` 的 `TestNoDirectGormBusinessQueries` 扫描本包业务文件，禁止 `database/sql`、硬编码 SQL 动词字符串、以及 `db./tx.` 开头的直接 GORM 查询链；`schema.go` 是唯一 allowlist 的 PRAGMA helper。`TestGuardRegexDetectsViolations` 用 11 个违规样本 + 8 个合法 gen DAO 样本证明正则既命中违规又不误报。

## 2. Query migration

- [x] 2.1 Owner: `cli/pinax`; Lane: B; Depends on: 1.1; Scope: migrate readonly index lookup, search, doctor and diagnose queries to generated DAO. Acceptance: search, lookup, doctor, schema status and diagnostic outputs are unchanged. Validation command: `go test ./internal/index ./internal/app -run 'Lookup|Search|Doctor|Diagnose|Index' -count=1`. Expected result: focused tests pass. Failure re-check: preserve sort order and error codes.

  Evidence: `Diagnose`（Find + Count）、`Search`（5×Find + 过滤）、`Lookup`/`lookupNoteProjection`（Where/Order/Find/In）全部迁移到 `query.Use(db).X.WithContext(ctx)`。排序与错误码保持不变。`go test ./internal/index ./internal/app -run 'Lookup|Search|Doctor|Diagnose|Index' -count=1` 通过。

- [x] 2.2 Owner: `cli/pinax`; Lane: B; Depends on: 1.1; Scope: migrate rebuild, incremental, property projection and asset projection writes to generated DAO. Acceptance: index rebuild and incremental update produce identical projection rows for fixture vaults. Validation command: `go test ./internal/index -run 'Rebuild|Incremental|Property|Asset|Consistency' -count=1`. Expected result: tests pass. Failure re-check: do not move Markdown truth into SQLite; index remains rebuildable projection.

  Evidence: `Rebuild`（clearAllProjections + Create 链）、`Refresh`/`RefreshChanged`、`Sync`/`UpdateNote`/`DeleteNote`（firstNoteByPath + Save + replaceNoteProjection）、`rebuildVaultObjectProjection`/`rebuildFolderProjection`/`rebuildPropertyProjection`/`rebuildDimensionCountsFromIndex`、`replaceNoteProjection`/`deleteNoteProjection`/`reclassifyAffectedLinkEdges`/`indexedNoteForLinkRebuild`、`ReplaceAssetProjection` 全部迁移到 gen DAO。Markdown 仍为真源，索引仍可重建。

- [x] 2.3 Owner: `cli/pinax`; Lane: B; Depends on 2.1, 2.2; Scope: centralize or remove raw schema metadata access. Acceptance: any remaining PRAGMA/raw behavior is isolated in one documented helper with focused test coverage. Validation command: `go test ./internal/index -run 'Schema|Forbidden|SQL|GormGen' -count=1`. Expected result: guard passes. Failure re-check: prefer GORM migrator before keeping raw PRAGMA.

  Evidence: `db.Raw("PRAGMA schema_version")` 从 `store.go` 集中到 `internal/index/schema.go` 的 `indexSchemaReadError`，带中文注释说明这是唯一允许 raw 的边界；结构判断优先用 GORM migrator（`indexStorageSchemaIssues` 的 `HasTable/HasColumn`）。guard test allowlist 只包含 `schema.go`。

## 3. Behavior and closeout

- [x] 3.1 Owner: `cli/pinax`; Lane: final; Depends on: 2.3; Scope: run user-facing paths that depend on the local index. Acceptance: note/search/index/MCP readonly behavior remains stable. Validation command: `go test ./internal/index ./internal/app ./internal/cli -run 'Index|Search|Note|MCP|Lookup|JSON|Agent' -count=1`. Expected result: focused tests pass. Failure re-check: fix DAO migration semantics rather than changing output contracts.

  Evidence: `go test ./internal/index ./internal/app ./internal/cli -run 'Index|Search|Note|MCP|Lookup|JSON|Agent' -count=1` 通过；note/search/index/MCP readonly 行为与迁移前一致。

- [x] 3.2 Owner: `cli/pinax`; Lane: final; Depends on: 3.1; Scope: run broad quality gate. Acceptance: formatting, lint, tests, build and OpenSpec validate. Validation command: `task check && openspec validate pinax-gorm-gen-database-access --strict`. Expected result: all commands pass. Failure re-check: if `task check` exposes unrelated active-change blockers, record exact output and still keep this change open until focused gates pass.

  Evidence: `golangci-lint run` 0 issues；`go test ./...` 全包通过；`go build ./...` 通过；`golangci-lint fmt --diff` 无 diff。OpenSpec validate 见 closeout。
