# Pinax 模块审查与 TDD 手册

本文记录 2026-07-03 通过 4 个并行只读 subagent 对 Pinax 主要模块做的审查结果，并把发现整理为可执行的 TDD 队列。它不是 closeout 证明；每一项进入实现前仍必须先写失败测试、确认 RED、再写最小实现并跑对应验证命令。

## 审查分片

| 分片 | 覆盖模块 | 主要问题类型 |
| --- | --- | --- |
| CLI 和 app 编排 | `cmd/pinax`、`internal/cli`、`internal/app` | 命令层承载业务逻辑、secret-ref 校验、stdout/stderr 合同 |
| Vault、index、sync、remote | `internal/notes`、`internal/index`、`internal/remote`、`internal/cloudsync`、`internal/assets` | remote key 安全、缓存 revision、frontmatter 注入、GORM/索引边界 |
| Provider、publish、plugin、API | `internal/provider`、`internal/delivery`、`internal/plugin`、`internal/cloudclient`、`internal/remoteapi`、`internal/mcpserver`、`internal/app/*publish*` | provider adapter 隔离、远端写入审批、warning 脱敏、临时源码残留 |
| 测试、证据、文档、输出 | `internal/output`、`internal/redaction`、`tools/testkit`、`tests/e2e`、`docs/`、当前 OpenSpec | integration evidence 入口、脱敏类别、smoke 记录、完整使用样例 |

## 模块覆盖索引

| 模块 | 审查分片 | 聚焦验证入口 |
| --- | --- | --- |
| `cmd/pinax` | CLI 和 app 编排；Provider/publish | `go test ./cmd/pinax -count=1` |
| `internal/api` | Provider、publish、plugin、API | `go test ./internal/api ./cmd/pinax -run 'API|Remote' -count=1` |
| `internal/app` | CLI 和 app 编排；Vault/index/sync；Provider/publish | `go test ./internal/app -count=1` |
| `internal/architecture` | 测试、证据、文档、输出 | `go test ./internal/architecture -count=1` |
| `internal/assets` | Vault、index、sync、remote | `go test ./internal/assets -count=1` |
| `internal/briefing` | Provider、publish、plugin、API | `go test ./internal/briefing -count=1` |
| `internal/cli` | CLI 和 app 编排 | `go test ./internal/cli ./cmd/pinax -count=1` |
| `internal/cloudclient` | Provider、publish、plugin、API | `go test ./internal/cloudclient -count=1` |
| `internal/cloudsync` | Vault、index、sync、remote | `go test ./internal/cloudsync -count=1` |
| `internal/config` | CLI 和 app 编排 | `go test ./internal/config ./cmd/pinax -run 'Config' -count=1` |
| `internal/contentbundle` | Provider、publish、plugin、API | `go test ./internal/contentbundle -count=1` |
| `internal/dashboard` | Provider、publish、plugin、API | `go test ./internal/dashboard -count=1` |
| `internal/delivery` | Provider、publish、plugin、API | `go test ./internal/delivery -count=1` |
| `internal/domain` | CLI 和 app 编排；Provider/publish | `go test ./internal/domain -count=1` |
| `internal/git` | Vault、index、sync、remote | `go test ./internal/git -count=1` |
| `internal/index` | Vault、index、sync、remote | `go test ./internal/index -count=1` |
| `internal/markdownnote` | Vault、index、sync、remote | `go test ./internal/markdownnote -count=1` |
| `internal/mcpserver` | Provider、publish、plugin、API | `go test ./internal/mcpserver -count=1` |
| `internal/memory` | Provider、publish、plugin、API | `go test ./internal/memory -count=1` |
| `internal/notelinks` | Vault、index、sync、remote | `go test ./internal/notelinks ./internal/index -run 'Link|Backlink|Orphan' -count=1` |
| `internal/notes` | Vault、index、sync、remote | `go test ./internal/notes -count=1` |
| `internal/output` | 测试、证据、文档、输出 | `go test ./internal/output -count=1` |
| `internal/plugin` | Provider、publish、plugin、API | `go test ./internal/plugin -count=1` |
| `internal/profile` | CLI 和 app 编排 | `go test ./internal/profile ./cmd/pinax -run 'Profile' -count=1` |
| `internal/promptasset` | Provider、publish、plugin、API | `go test ./internal/promptasset ./cmd/pinax -run 'Prompt' -count=1` |
| `internal/provider` | Provider、publish、plugin、API | `go test ./internal/provider -count=1` |
| `internal/publishdocast` | Provider、publish、plugin、API | `go test ./internal/publishdocast ./internal/app -run 'PublishDoc' -count=1` |
| `internal/records` | Vault、index、sync、remote | `go test ./internal/records ./cmd/pinax -run 'Record' -count=1` |
| `internal/redaction` | 测试、证据、文档、输出 | `go test ./internal/redaction -count=1` |
| `internal/remote` | Vault、index、sync、remote | `go test ./internal/remote -count=1` |
| `internal/remoteapi` | Provider、publish、plugin、API | `go test ./internal/remoteapi -count=1` |
| `internal/search` | Vault、index、sync、remote | `go test ./internal/search ./internal/index -run 'Search' -count=1` |
| External RAG project | Semantic retrieval, embeddings, vector store, rerank | Validate in the external RAG repository; Pinax only validates Markdown export and local text search. |
| `internal/sync` | Vault、index、sync、remote | `go test ./internal/sync -count=1` |
| `internal/templateengine` | CLI 和 app 编排 | `go test ./internal/templateengine ./internal/app -run 'Template' -count=1` |
| `tools/testkit` | 测试、证据、文档、输出 | `go test ./tools/testkit/... -count=1` |
| `internal/vaultignore` | Vault、index、sync、remote | `go test ./internal/vaultignore ./internal/index -run 'Ignore|Vault' -count=1` |
| `internal/vaultregistry` | CLI 和 app 编排；Vault/index/sync | `go test ./internal/vaultregistry ./cmd/pinax -run 'Vault' -count=1` |
| `internal/version` | Vault、index、sync、remote | `go test ./internal/version ./cmd/pinax -run 'Version|Restore|Snapshot' -count=1` |
| `tests/e2e` | 测试、证据、文档、输出 | `go test ./tests/e2e -count=1`；证据入口用 `task test:integration` |
| `docs/` 和 `openspec/` | 测试、证据、文档、输出 | `openspec validate --all` |

这个索引用于确认审查覆盖和后续分工，不表示每个目录都已完成修复。真正完成标准仍是对应 RED/GREEN 测试和最终质量门禁通过。

## 高优先级 TDD 队列

### 1. profile、vault、config 命令层瘦身

风险：`profile add`、`vault register/use/remote refresh`、`config set/unset` 仍在命令层直接读写配置、registry 或发起 remote refresh。命令层应只处理参数、调用 app service、选择输出模式。

RED：新增 service 层测试，先证明当前缺少 app service API。

```bash
go test ./internal/app -run 'TestProfile|TestVaultRegistry|TestConfigService' -count=1
```

GREEN：新增 app service 方法，迁移命令层直接读写逻辑，保留现有 CLI contract。

验证：

```bash
go test ./internal/app ./cmd/pinax -run 'Profile|VaultRemote|Config|StdoutStderr' -count=1
```

### 2. secret-ref 禁止明文持久化

风险：`--secret-ref plain:text` 会被 profile 命令接受并持久化。Pinax 本地配置只能保存 secret 引用，不保存真实凭据。

RED：在 `cmd/pinax/token_profile_command_test.go` 增加 `TestProfileSecretRefRejectsPlaintextAndRedactsErrors`，断言命令失败且 stdout/stderr 不含明文。

```bash
go test ./cmd/pinax -run 'TestProfileSecretRefRejectsPlaintextAndRedactsErrors' -count=1
```

GREEN：在 app service 或 profile adapter 中只允许 `env://`、`keychain://`、用户级 secret store 引用等非明文来源，返回稳定 error code。

### 3. remote cache key 和 revision 安全

风险：`CachedBlobStore` 用 object key 直接拼缓存路径，并在缓存命中时返回固定 revision `cache`；`FileBackend.Exists` 绕过 `objectPath` 校验。

RED：补 unsafe key 和 revision 测试。

```bash
go test ./internal/remote -run 'TestCachedBlobStoreRejectsUnsafeKeys|TestCachedBlobStoreCacheHitReturnsStableInnerRevisionOrRevalidates|TestFileBackendRejectsKeysOutsideBaseDir' -count=1
```

GREEN：让 cached store 的 path 解析复用 object key 校验；缓存命中返回真实 revision metadata，或限制缓存只覆盖 immutable blob key；`Exists` 改用 `objectPath`。

### 4. briefing candidate frontmatter 注入防护

风险：外部标题、URL、摘要直接拼 YAML/frontmatter，包含换行、冒号或 `---` 时可能注入字段或破坏 note 真源。

RED：补包含换行、冒号和 frontmatter delimiter 的 candidate 测试。

```bash
go test ./internal/notes -run 'TestCandidateMarkdownEscapesFrontmatter' -count=1
```

GREEN：用 YAML encoder 生成 frontmatter；正文标题和摘要做安全渲染，保持 Markdown 可读但不让 metadata 被注入。

### 5. publish doc 单包远端写入审批

风险：`publish doc push --package ...` 非 dry-run 路径没有检查 `--yes`，可能直接 create/update 远端文档并写 mapping/receipt；目前 `--all` 有审批门，单包没有。

RED：补命令测试，断言不带 `--yes` 返回 `approval_required`，fake `lark-cli` 日志没有 create/update。

```bash
go test ./cmd/pinax -run 'TestPublishDocPushRequiresYes' -count=1
```

GREEN：`PublishDocPush` 非 dry-run 分支检查 `req.Yes`；文档中的真实 push 样例统一带 `--yes`，dry-run 样例保持不带。

### 6. native-docx provider adapter 隔离

风险：whiteboard append/update/delete/fetch、folder move 和附件上传仍在 app 层直接拼 `lark-cli`。provider 命令应集中到 `internal/provider` adapter，app service 只编排稳定接口。

RED：补 provider/app 测试，断言 whiteboard、folder、move、attachment 调用全部通过 adapter fake runner。

```bash
go test ./internal/provider ./internal/app -run 'LarkNativeDoc|PublishDoc.*ProviderAdapter' -count=1
```

GREEN：扩展 `LarkNativeDocAdapter` 或新增 provider interface，迁移 app 层 direct CLI 调用。

### 7. publish warning 和 smoke 记录脱敏

风险：Mermaid/SVG 源码第一行、远程图片 URL query、真实 Feishu doc URL/token 可能进入 warning、mapping、smoke 记录。

RED：构造包含 `Authorization: Bearer RAW_TOKEN`、`provider_payload`、带 query token URL 的资产失败测试；扫描 smoke 文档中的非占位 Feishu token。

```bash
go test ./internal/app ./internal/redaction -run 'PublishDoc.*Warning.*Redaction|Smoke.*Redaction' -count=1
```

GREEN：新增 publish warning detail redaction，只保留 asset kind、vault-relative path、host、hash 或稳定 reason；把 smoke log 中真实 URL/token 改为 `<redacted-doc-url>`、`<doc-token>`、`<drive-stream-url>`。

### 8. integration evidence 覆盖和脱敏类别对齐

风险：`task test:integration` 会写 `temp/integration-test-runs`，但普通 `go test ./tests/e2e -run ...` 不会写 evidence；evidence runner 的脱敏类别也窄于 `internal/redaction`。

RED：补 evidence runner 测试，输出 Cookie、webhook URL、raw prompt、provider payload、private body 后断言所有 evidence 文件都不含原文。

```bash
go test ./tools/testkit/... ./tests/e2e -run 'IntegrationEvidence|EvidenceRedaction' -count=1
```

GREEN：runner 复用通用 redaction sensitive class；OpenSpec 任务的验证命令改为 `task test:integration` 或新增参数化 evidence runner，例如：

```bash
go run ./tools/testkit/integrationevidence --run TestPublishDoc
```

## 已覆盖证据

- 输出模式互斥已有 CLI 测试覆盖，`--json` 与 `--agent` 不能同时选择。
- note 创建、stdin、列表、歧义解析、编辑、rename/move/archive/property/tag/delete 已有命令测试覆盖关键用户路径。
- GORM/数据库约束由 `internal/index/guard_test.go` 和 gorm/gen DAO 使用方式约束；集中 SQL 例外在 index schema 级别。
- Cloud Sync direct two-device e2e 覆盖 push/pull/conflict/delete marker；remote stderr 和 unsafe key 已有部分测试，但 cached store 和 `Exists` 仍需补齐。
- `PublishDocPushAll` 已覆盖 dry-run 不写 packages 和 `--all` 审批门；单包 push 仍需补齐审批门。
- Feishu delivery、remote API、cloud client、plugin runner、MCP read-only surface 已有 fake 或 bounded projection 测试，后续重点是失败路径、adapter 隔离和脱敏一致性。
- integration evidence runner 已生成 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`；仍需扩展脱敏类别并让 OpenSpec 验证命令指向 evidence entrypoint。

## 执行顺序建议

1. 先修安全边界：secret-ref 明文、remote path traversal、publish warning 脱敏、single-package publish approval。
2. 再修架构边界：profile/vault/config app service、native-docx provider adapter。
3. 最后补证据链：integration evidence runner 参数化、OpenSpec 验证命令、full usage testscript golden。

每个实现项完成前至少运行聚焦验证；涉及 Go 代码或输出合同变更后运行：

```bash
task check
```

涉及 integration/e2e 证据的变更还要运行：

```bash
task test:integration
```
