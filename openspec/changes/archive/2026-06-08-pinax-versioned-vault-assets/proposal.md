## Why

Pinax 需要从“Markdown 笔记 CLI”扩展为本地优先的版本化知识资产管理工具：同一套 vault 中会同时存在笔记、图片、音频、视频、PDF 和其他附件，用户和 agent 需要按文件名、标题、引用关系、版本证据快速定位它们。现有设计把部分能力命名为 `git`，且一些历史/changed-since 搜索语义隐含依赖外部 Git 二进制；这与 Pinax 的 CLI-only、本地可迁移、pure Go 优先和多媒体资产管理目标不一致。

## What Changes

- 引入 `version` 命令族，替代用户可见的 `git` 命令口径：`version status/snapshot/history/diff/show/restore/changed/backends`。
- 将版本能力抽象为 pure Go `VersionBackend`，默认提供 local ledger/none backend；Git 只作为可选 backend，后续通过 pure Go adapter 接入，不依赖系统 `git` 二进制。
- 保留 `pinax git snapshot` 作为隐藏兼容 alias，help、docs、错误 hint 和 next action 全部推荐 `pinax version snapshot`。
- 引入 `asset` 命令族管理多媒体和二进制资产：`asset add/list/show/link/move/remove/verify`。
- 将附件管理设计为 Obsidian-like 的 asset 使用层：笔记正文里的 Markdown/wiki embed 引用仍可读，附件文件按 vault 策略落盘，index 负责引用、孤儿、反链和缺失附件投影。
- 建立 asset manifest、content evidence 和 content-addressed object refs；事件、stdout、stderr、索引和 fixture 不写入二进制 payload。
- 扩展 `.pinax/index.sqlite` projection：支持 `vault_files`、`assets`、`asset_links`、note/asset lookup 和 version evidence。
- 引入统一 `NoteRefResolver` / `VaultObjectResolver`，让 `find`、`index lookup`、`record adopt <query>`、`metadata plan <query>`、`note show <query>`、`asset show <query>` 使用一致的候选解析和 ambiguous 错误。
- 将 `search --revision`、`search --changed-since`、`index refresh --changed-since` 等版本感知行为路由到 VersionBackend，而不是命令层或 service 层拼 Git porcelain。
- 明确写入保护：restore/remove/move/adopt/apply 类操作先生成 plan，真正写入需要 `--yes`，高风险写入需要 `version snapshot` 保护。

## Capabilities

### New Capabilities

- `version-control`: Pinax vault 版本控制命令、pure Go backend 抽象、snapshot/history/diff/show/changed/restore 合同。
- `asset-management`: 多媒体和二进制资产的添加、索引、引用、移动、删除、校验、版本证据和输出合同。
- `asset-management`: Obsidian-like 附件目录策略、Markdown/embed 引用解析、孤儿附件、缺失引用、附件反链和保守修复计划。

### Modified Capabilities

- `notebook-index-search`: 索引扩展为 note + asset + vault file catalog 的统一 lookup/refresh projection，并支持 version-aware candidate filtering。
- `vault-record-ledger`: record events 和 adoption/history 接入 version evidence、resolver 和 asset reference evidence。
- `note-command-ux`: note/ref 类命令共享 resolver；只读命令可返回 candidates，写入命令要求唯一强匹配。
- `vault-maintenance-actions`: repair/restore/remove 计划接入 version snapshot 保护和 asset/index 修复边界。
- `cli-tree-ux`: 命令树从用户可见 `git` 迁移到 `version`，新增 `asset` 和 `find/index lookup` 主路径说明。

## Impact

- `internal/cli/version_cmd.go`、`internal/cli/asset_cmd.go`、`internal/cli/index_cmd.go`、`internal/cli/record_cmd.go`、`internal/cli/note_cmd.go`：命令树、flags、help、hidden alias 和 completion。
- `internal/app`：Version/Asset/Resolver use case，search/index/record/note/repair 的编排接入。
- `internal/version`：pure Go VersionBackend 接口、local/none backend、可选 pure Go Git backend 适配边界。
- `internal/assets`：asset manifest、content hashing、safe copy/move/remove、media metadata、asset link extraction。
- `internal/assets`：asset manifest、attachment placement policy、content hashing、safe copy/move/remove、media metadata、asset link extraction。
- `internal/index`：GORM schema migration、vault file catalog、asset projection、lookup scoring、version evidence fields。
- `internal/records` 和 `internal/domain`：record event/version evidence、asset record facts、resolver candidate domain model。
- `internal/output`：必要时补充 note/asset candidate summary；JSON/agent/events/explain 顶层 envelope 保持兼容。
- `tests/e2e`、`cmd/pinax/main_test.go`、`internal/*_test.go`：CLI contract、testscript、fixture vault、多媒体 fixture、stdout/stderr 分离和 pure Go fake backend。
- `openspec/specs/*`：归档时同步 version-control、asset-management 以及相关既有能力的行为合同。
