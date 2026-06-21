## 1. 领域模型和边界

- [x] 1.1 新增 `internal/domain` 发布模型：`PublishProfile`、`PublishTarget`、`PublishRenderer`、`PublishThemeSource`、`PublishThemeContract`、`PublishPlan`、`PublishItem`、`PublishViolation`、`PublishManifest`、`PublishReceipt`，字段使用稳定英文 JSON key。
  - Evidence: 2026-06-18 新增 `internal/domain/publish.go`；`go test ./internal/domain ./internal/app/publishops ./internal/app ./internal/architecture -run 'Publish|CapabilityPackagesDeclareOwnership|CapabilityPackagesDoNotImportCLIOrOutput|CLIImportsAppFacadeOnly' -count=1` 通过。
- [x] 1.2 新增 `internal/app/publishops/doc.go`，声明 command family、责任边界、禁止依赖和 focused tests，并更新 architecture guard 期望。
  - Evidence: 2026-06-18 新增 `internal/app/publishops/doc.go` 并将 `publishops` 加入 architecture guard；同上聚焦命令通过。
- [x] 1.3 新增 `internal/app/publishops` 规则层骨架，放置 profile validation、note eligibility、asset eligibility、target policy、violation classification 和 manifest shaping 纯逻辑。
  - Evidence: 2026-06-18 新增 `internal/app/publishops/profile.go`，包含 profile validation 和 note eligibility；`go test ./internal/app/publishops -count=1` 通过。
- [x] 1.4 在 `internal/app.Service` facade 暴露 `PublishProfileInit`、`PublishProfileValidate`、`PublishDoctor`、`PublishPlan`、`PublishBuild`、`PublishDeploy` 请求/响应类型和方法占位。
  - Evidence: 2026-06-18 新增 `internal/app/publish.go` 和 facade placeholder 测试；聚焦命令通过。
- [x] 1.5 增加基础单元测试覆盖 target/renderer/body_policy 枚举、默认安全策略、路径 traversal 拒绝和稳定错误码。
  - Evidence: 2026-06-18 新增 `internal/domain/publish_test.go`、`internal/app/publishops/publishops_test.go`、`internal/app/publish_test.go`；先运行 RED 失败确认缺失 API，再实现后 `go test ./internal/app/publishops -count=1` 和聚焦命令通过。

## 2. Profile CLI 和 structured asset

- [x] 2.1 在 `internal/cli`/`cmd/pinax` 注册 `publish` 命令族和 `profile init|validate|show|list` 子命令，命令层只做参数校验和输出模式选择。
  - Evidence: 2026-06-18 新增 `internal/cli/publish_cmd.go` 并接入 root command；`go test ./cmd/pinax -run 'PublishProfile' -count=1` 通过。
- [x] 2.2 实现 `.pinax/publish/profiles/<name>.yaml` 的 CLI-authored 读写，新增目录创建、原子写入、schema_version 校验和 YAML round-trip 测试。
  - Evidence: 2026-06-18 `PublishProfileInit` 通过 app service 原子写入 `.pinax/publish/profiles/public.yaml`；`go test ./cmd/pinax -run 'PublishProfile' -count=1` 和 `go test ./tests/e2e -run PublishProfile -count=1` 通过。
- [x] 2.3 `profile init` 支持 `--target github-pages|github-wiki`、`--renderer hugo|none`、`--title`、`--base-url`、`--theme builtin:pinax-encyclopedia|local:<path>`、`--json`、`--agent`，并生成安全默认 profile。
  - Evidence: 2026-06-18 CLI/testscript 覆盖 `--target`、`--renderer`、`--title`、`--base-url`、`--theme`、`--json` 和 `--agent`；profile YAML 包含 safe defaults 和 `theme: builtin:pinax-encyclopedia`。
- [x] 2.4 `profile validate` 检查未知字段、非法 target/renderer、path traversal、绝对 out/repo 路径策略、禁用 safety gate、unsupported secret ref，并返回稳定 issue codes。
  - Evidence: 2026-06-18 `TestPublishProfileValidateRejectsUnsafeHandWrittenProfile` 覆盖 invalid target/renderer/body_policy、path traversal 和 disabled safety gates；`TestPublishProfileValidateRejectsUnknownFieldsWithStableCode` 覆盖 unknown field stable code；`publishops.ValidateProfile` 覆盖 unsupported secret refs。
- [x] 2.5 增加 CLI/testscript 覆盖 profile init/validate/show/list、机器输出 stdout clean、stderr diagnostics 和不修改 Markdown/Git/provider 的只写 profile 边界。
  - Evidence: 2026-06-18 新增 `cmd/pinax/publish_command_test.go` 和 `tests/e2e/testdata/publish/scripts/publish_profile.txt`；`go test ./tests/e2e -run PublishProfile -count=1` 通过，验证 JSON/agent clean stdout、profile validate 只读和 unsafe profile failure。

## 3. 发布计划和选择规则

- [x] 3.1 实现 `publish plan` app service：复用现有 note/index/search/asset/link 投影，收集候选 note、asset、link graph 和来源信息。
  - Evidence: 2026-06-18 `PublishPlan` scans ordinary Pinax note facts, reuses `internal/assets.ExtractLinks` for linked assets and `BuildEnhancedLinkGraph` for note links, and projects selected notes/assets, source metadata, and link graph without note bodies. `TestPublishPlanIncludesSourceInfoAndLinkGraph` failed before source/link projection facts existed and passed after implementation.
- [x] 3.2 实现 note eligibility：默认只选择 profile 允许的 `publish` 值、`status=active`、允许 type，跳过 `status=draft`、`privacy=private|secret`、`publish=false` 和非 Pinax note。
  - Evidence: 2026-06-18 `publishops.ClassifyNoteEligibility` and `PublishPlan` select only active public allowed kinds; `go test ./cmd/pinax -run 'PublishPlan' -count=1` covers public/draft/private/unpublished/secret note fixtures.
- [x] 3.3 实现 body policy：只有被 profile 选中的 published note 可输出正文；未选中 note 只能出现在跳过原因或关系摘要中，不能泄漏 body。
  - Evidence: 2026-06-18 `PublishPlan` returns `PublishItem` metadata and redacted violation classes only; CLI and testscript assertions reject draft/private/unpublished/secret body sentinels from JSON and agent output.
- [x] 3.4 实现 asset eligibility：只复制被已发布 note 引用且扩展名、大小、路径均允许的 vault 内资产；拒绝路径逃逸、未链接资产和 manifest 外资产。
  - Evidence: 2026-06-18 `PublishPlan` now extracts assets only from selected published note bodies via `internal/assets.ExtractLinks`, validates safe relative paths, extension allowlist, existence, non-directory files, and max size, and projects allowed assets as `kind=asset`; `TestPublishPlanClassifiesLinkedAssets` first failed with missing `selected_asset_count`/`asset_not_allowed` facts, then passed after implementation.
- [x] 3.5 实现 plan violation 分类：`secret_pattern`、`provider_payload`、`authorization_header`、`cookie_header`、`webhook_url`、`absolute_path`、`pinax_internal_reference`、`private_body_leak`、`asset_not_allowed`。
  - Evidence: 2026-06-18 `TestClassifyNoteViolationsCoversPublishSafetyClasses` covers secret/provider/auth/cookie/webhook/absolute path/.pinax/private body leak classes; linked asset plan tests cover `asset_not_allowed`. The test failed before adding `private_body_leak` detection and passed after implementation.
- [x] 3.6 增加 plan projections：selected/skipped/blocking/manual_review/counts/output_paths/actions/facts，覆盖默认中文、`--json`、`--agent`、`--events`、`--explain`。
  - Evidence: 2026-06-18 `PublishPlan` projects selected/skipped/violations/manual_review under `data.plan`, stable count facts, output paths, and build/review actions. `TestPublishPlanOutputModesExposeStableProjection` failed before the plan summary was localized, then passed after setting the default summary to Chinese and covering JSON, agent, events, and explain modes.
- [x] 3.7 增加 fixture vault 和 testscript：public/draft/private/secret/unpublished notes、linked/unlinked assets、断链、敏感 sentinel，验证 plan 只读且输出稳定。
  - Evidence: 2026-06-18 `tests/e2e/testdata/publish/scripts/publish_profile.txt` now covers public/draft/private/secret/unpublished notes, allowed and disallowed linked assets, unlinked asset sentinel, broken wiki link, JSON/agent stable facts, and no sensitive/private/draft/unpublished body leakage; `go test ./tests/e2e -run PublishProfile -count=1` passed.

## 4. 发布安全扫描和收据

- [x] 4.1 新增 publish tree scanner，递归扫描文件名、路径和文本内容，复用 `internal/redaction` 的敏感模式并扩展 publish violation class。
  - Evidence: 2026-06-18 added `internal/redaction.ScanSensitiveClasses` and `publishops.ScanPublishTree`; scanner recursively scans relative file paths and text content, maps findings to publish violation classes, and returns structured `PublishScanReport` findings.
- [x] 4.2 扫描器必须跳过二进制内容的原文输出，但记录 size/hash/path/violation class；中文注释说明文本/二进制判定和为什么不回显敏感片段。
  - Evidence: 2026-06-18 `TestScanPublishTreeFindsLeaksWithoutEchoingSensitiveContent` covers binary detection, path/size/SHA-256 evidence, and no raw sensitive token/path echo; scanner code includes Chinese comments explaining binary detection and why bytes are not echoed.
- [x] 4.3 实现 redacted evidence writer，覆盖 stdout、stderr、events、manifest、receipt、Hugo staging、Pages output、Wiki output 的递归泄漏扫描。
  - Evidence: 2026-06-18 added `publishops.WriteRedactedEvidence`, which writes `pinax.publish_evidence.v1` JSON evidence containing surface names, finding classes, relative paths, sizes and hashes only; text surfaces cover stdout/stderr/events/manifest/receipt, and tree surfaces cover Hugo staging, Pages output and Wiki output via `ScanPublishTree`.
- [x] 4.4 实现 `.pinax/publish/runs/<run-id>/receipt.json`，记录 profile、target、renderer、vault version/hash、counts、duration、output hash、redaction summary 和 deploy status。
  - Evidence: 2026-06-18 added `publishops.WritePublishReceipt`, `pinax.publish_receipt.v1`, safe run id validation, atomic write, and receipt fields for vault version/hash and duration; `TestWritePublishReceiptCreatesStructuredRunReceipt` covers the structured asset path and required fields.
- [x] 4.5 增加 contract tests：递归拒绝 `body`/`raw_body`/private sentinel、Authorization/Bearer/Cookie/webhook、provider payload、绝对路径、`.pinax` 内部内容。
  - Evidence: 2026-06-18 expanded `TestScanPublishTreeFindsLeaksWithoutEchoingSensitiveContent` to cover raw/private body, Authorization/Bearer, Cookie, webhook, provider payload, absolute path, `.pinax` internal path and secret-pattern binary path without echoing sentinel content.

## 5. GitHub Wiki build

- [x] 5.1 实现 `publish build --target github-wiki`，生成 `Home.md`、slug 化 note 页面、标签/类型/来源索引、`_Sidebar.md`、可选 `_Footer.md` 和 `pinax-publish-manifest.json`。
  - Evidence: 2026-06-18 added `publish build` CLI wiring and GitHub Wiki build service path. `TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt` covers `Home.md`, slugged note pages, `Tags.md`, `Types.md`, `Sources.md`, `_Sidebar.md`, `pinax-publish-manifest.json`, allowed asset copy and CLI-authored receipt.
- [x] 5.2 实现 Wiki link rewrite，把发布 note 之间的 wiki/markdown link 转换为 GitHub Wiki 可解析页面链接；断链或未发布目标进入 manual review 或安全占位。
  - Evidence: 2026-06-18 Wiki build rewrites selected note wiki links to `[[display|slug]]` and replaces unresolved/unpublished targets with plain-text `(unpublished)` placeholders. `TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt` failed before rewrite and passed after implementation.
- [x] 5.3 实现 Wiki asset copy/rewrite，只复制 allowlist 资产并生成相对引用，不复制 `.pinax/**` 或未允许资产。
  - Evidence: 2026-06-18 Wiki build copies only `kind=asset` items selected by the publish plan, rewrites note asset references to output-relative `assets/...`, and blocks disallowed linked assets before writing output. `TestPublishBuildGitHubWikiBlocksDisallowedLinkedAsset` covers plan-blocked `.exe` asset and no copy.
- [x] 5.4 Build 完成前扫描 Wiki 输出；扫描失败时删除或标记 partial output，并返回结构化 `publish_leak_detected` 错误。
  - Evidence: 2026-06-18 Wiki build runs `publishops.ScanPublishTree` before returning success. `TestPublishBuildGitHubWikiFailsWhenOutputScanFindsLeak` covers a leaked output file causing structured `publish_leak_detected` failure with redacted `secret_pattern` finding and no raw token echo.
- [x] 5.5 增加 golden tests 和 testscript：最小百科 vault、中文标题 slug、双链、附件、跳过项、敏感项阻断、Hugo 不存在时 Wiki build 仍成功。
  - Evidence: 2026-06-18 publish testscript now builds a standalone GitHub Wiki fixture with Chinese title slug fallback, bidirectional wiki link rewrite, attachment copy/rewrite and no Hugo dependency. Existing testscript plan fixture covers skipped private/draft/unpublished notes and sensitive blocking; command tests cover manifest/receipt and scan failures.

## 6. Hugo/GitHub Pages build 和主题系统

- [x] 6.1 实现 Hugo staging builder：生成完整 staging project，包括 `hugo.yaml`、`content/entries/<slug>/index.md`、`content/indexes/**`、`data/pinax/*.json`、`static/assets/**` 和 resolved theme source。
  - Evidence: 2026-06-18 added `publishops.BuildHugoStagingProject`, which writes `hugo.yaml`, entry content, tag/type indexes, data files, static assets and the built-in `themes/pinax-encyclopedia` source into a staging root.
- [x] 6.2 实现 `pinax.publish_theme.v1` 数据合同：frontmatter 字段、`manifest.json`、`graph.json`、`search-index.json`、`taxonomies.json`、`sources.json`、`build.json`，并增加 schema/golden 测试。
  - Evidence: 2026-06-18 `TestBuildHugoStagingProjectWritesSafeThemeContract` covers publish entry/index frontmatter plus `data/pinax/manifest.json`, `graph.json`, `search-index.json`, `taxonomies.json`, `sources.json` and `build.json` under the `pinax.publish_theme.v1` contract.
- [x] 6.3 Hugo config 使用安全默认值：normalized `baseURL`、site title、theme contract version、`markup.goldmark.renderer.unsafe=false`、最小环境变量和禁用不需要的输出 kind。
  - Evidence: 2026-06-18 staging test covers generated `hugo.yaml` containing baseURL, title, `theme: pinax-encyclopedia`, `pinax.publish_theme.v1`, `markup.goldmark.renderer.unsafe=false` and disabled output kinds.
- [x] 6.4 新增 Hugo executable adapter，支持 `hugo version`/`hugo --source <stage> --destination <out>`，context cancellation、timeout、stderr 脱敏和 stable `call_id`。
  - Evidence: 2026-06-18 added `publishops.HugoAdapter` with version/build calls, context timeout, call metadata and stderr redaction. `TestHugoAdapterUsesFakeExecutableAndRedactsStderr` covers fake executable build output and redacted Authorization/token/path stderr.
- [x] 6.5 `publish doctor` 检测 Hugo 是否可用、版本是否可解析、profile/theme 是否有效、theme contract 是否匹配、Git 是否可用、输出目录是否安全。
  - Evidence: 2026-06-18 added `publish doctor` CLI/service path. `TestPublishDoctorDetectsFakeHugoAndProfile` uses a fake Hugo executable on PATH and verifies profile/target/renderer, Hugo availability, profile issue count and safe output directory facts without leaking the local root.
- [x] 6.6 `publish build --target github-pages --renderer hugo` 在 Hugo 调用前扫描 staging，调用后扫描 final output；任何泄漏都返回结构化错误且不写成功 receipt。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run TestPublishBuildGitHubPagesUsesFakeHugoAndScansOutput -count=1` first failed with `publish_not_implemented`, then passed after wiring Pages/Hugo staging scan, fake Hugo build, final scan and receipt writing. `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after the build wiring.
- [x] 6.7 内置 `pinax-encyclopedia` Hugo theme，提供首页、条目页、标签/类型页、来源页、关系列表、graph JSON 消费、静态搜索、404/未发布目标占位和无 JS fallback。
  - Evidence: 2026-06-18 `go test ./internal/app/publishops -run TestBuiltinThemeProvidesEncyclopediaLayouts -count=1` first failed because `layouts/_default/baseof.html` did not exist, then passed after adding base/list/single/404 layouts, nav/head/sources partials, local CSS/JS, graph/search data hooks and no-JS fallback. `go test ./internal/app/publishops -run 'Publish|Sensitive|Redaction|Scan|Evidence|Receipt|Hugo|Staging|Theme' -count=1` passed after the theme expansion.
- [x] 6.8 内置主题视觉实现使用本地 CSS/JS 和 semantic CSS tokens，禁止默认外部 CDN、远程字体、analytics、远程图片和营销式 hero；增加快照或 golden 检查关键 HTML 结构。
  - Evidence: 2026-06-18 `go test ./internal/app/publishops -run TestBuiltinThemeUsesLocalAssetsAndStableHTMLStructure -count=1` first failed because CSS lacked semantic variables, then passed after adding local-only CSS/JS constraints, semantic CSS variables and stable base/index/single structure checks. `go test ./internal/app/publishops -run 'Publish|Sensitive|Redaction|Scan|Evidence|Receipt|Hugo|Staging|Theme' -count=1` passed after the visual contract update.
- [x] 6.9 实现 `publish theme list` 和 `publish theme eject`，列出内置主题、合同版本、required layouts，并把可审查主题文件复制到安全输出目录。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run TestPublishThemeListAndEjectCommands -count=1` first failed because `publish theme` was not registered, then passed after adding built-in theme metadata, theme file ejection, app projections and Cobra commands. `go test ./internal/app ./internal/cli ./internal/app/publishops ./cmd/pinax -run 'Publish|Theme|Command|Help' -count=1` passed after command wiring.
- [x] 6.10 支持 `local:<path>` 主题源：路径规范化、禁止逃逸和指向 `.pinax/**`，materialize 到 staging 后仍运行 staging/final scan。
  - Evidence: 2026-06-18 `go test ./internal/app/publishops -run 'TestValidateProfileRejectsUnsafeLocalThemePath|TestBuildHugoStagingProjectMaterializesSafeLocalTheme' -count=1` first failed because unsafe local theme paths were accepted and local themes were not materialized, then passed after adding safe relative theme validation and vault-local theme copying into Hugo staging. `go test ./internal/app/publishops ./internal/app ./cmd/pinax -run 'Publish|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after local theme support.
- [x] 6.11 增加 fake Hugo 测试：成功构建、Hugo missing、Hugo stderr secret redaction、theme 输出泄漏、context cancellation、非零退出码、theme contract mismatch。
  - Evidence: 2026-06-18 `go test ./internal/app/publishops ./cmd/pinax -run 'TestHugoAdapterFailureMatrixUsesStableRedactedResults|TestValidateProfileRejectsThemeContractMismatch|TestPublishBuildGitHubPagesFailsWhenFakeHugoOutputLeaks' -count=1` first failed because Hugo stderr kept an absolute path and theme contract mismatch was accepted, then passed after adding Hugo stderr path redaction and theme contract validation. Existing Pages fake Hugo success test covers successful build; new tests cover missing executable, non-zero exit, stderr redaction, timeout/cancellation, final output leak and mismatch. `go test ./internal/app/publishops ./internal/app ./cmd/pinax ./tests/e2e -run 'Publish|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after the matrix.
- [x] 6.12 增加真实 Hugo 可选 smoke：当环境存在 `hugo` 时构建 fixture Pages 站点并验证 HTML/search-index/graph JSON 非空、无外部资源链接；环境无 Hugo 时明确 skip。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run TestPublishBuildGitHubPagesRealHugoSmokeWhenAvailable -count=1 -v` passed with explicit skip `hugo executable not available; skipping optional real Hugo smoke` in this environment. The test builds a fixture Pages site and checks HTML/search-index/graph/non-external-resource constraints when Hugo exists. `go test ./cmd/pinax ./internal/app/publishops ./internal/app ./tests/e2e -run 'Publish|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after adding the optional smoke.

## 7. Deploy 到发布仓库

- [x] 7.1 实现 deploy policy parser：`mode=none|git`、`repo`、`branch`、`strategy=clean-worktree|orphan`、Wiki repo target，并拒绝 deploy 到 vault root 或 `.pinax` 内部。
  - Evidence: 2026-06-18 `go test ./internal/app/publishops -run TestParseDeployPolicy -count=1` first failed because `ParseDeployPolicy` and request types did not exist, then passed after adding pure deploy policy parsing, branch/strategy defaults and vault-root/.pinax rejection. `go test ./internal/app/publishops ./internal/app ./cmd/pinax -run 'Publish|Deploy|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after parser work.
- [x] 7.2 实现 `publish deploy --yes` 到本地或远端 Git repo 的受控流程：准备工作树、同步产物、commit、push；未传 `--yes` 返回 `approval_required`。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run TestPublishDeployRequiresApprovalAndCommitsLocalRepo -count=1` first failed because `publish deploy` did not accept deploy flags, then passed after adding CLI wiring, approval gate, local Git repo initialization, worktree sync and commit. Remote push remains covered by later deploy tasks; this slice validates local Git repo deployment. `go test ./internal/app/publishops ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|Deploy|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after deploy implementation and placeholder-test update.
- [x] 7.3 deploy 前必须校验 manifest、receipt、output hash 和最新 scan result；校验失败不得 commit/push。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run TestPublishDeployRequiresReceiptAndCleanScan -count=1` first failed because deploy succeeded without a build receipt, then passed after adding pre-deploy output scanning, latest matching receipt lookup and output hash verification. `go test ./cmd/pinax -run 'TestPublishDeployRequires(ApprovalAndCommitsLocalRepo|ReceiptAndCleanScan)' -count=1` and `go test ./internal/app/publishops ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|Deploy|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after updating deploy success to use a real build receipt.
- [x] 7.4 Git subprocess 输出必须脱敏 remote URL credential、token、Authorization header 和本机绝对路径；错误返回 stable external dependency code。
  - Evidence: 2026-06-18 `go test ./internal/app -run TestPublishGitErrorRedactionCoversCredentialsAndPaths -count=1` first failed because the redaction helper did not exist and then because the Authorization marker remained, then passed after adding centralized Git error redaction for credential URLs, tokens, Authorization headers and local paths. `go test ./internal/app ./cmd/pinax ./internal/app/publishops ./tests/e2e -run 'Publish|Deploy|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after wiring deploy failures through the redactor.
- [x] 7.5 增加临时 bare repo/fake git e2e：Pages branch deploy、Wiki repo deploy、无 `--yes` 不写、目标是 vault root 被拒绝、push 失败脱敏。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run TestPublishDeployMatrixCoversWikiVaultRootAndRemoteRedaction -count=1` first failed because Wiki deploy defaulted to `gh-pages`, then passed after policy parsing defaulted Wiki deploy to `master`. The deploy command matrix now covers Pages branch deploy, Wiki repo deploy, no-`--yes` no-write behavior, vault root rejection and credential URL redaction on remote deploy errors. `go test ./internal/app/publishops ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|Deploy|Theme|Hugo|Staging|Scan|Command|Help' -count=1` passed after the matrix.

## 8. 输出合同和命令文档

- [x] 8.1 为 publish projections 增加默认中文摘要、`--json`、`--agent`、`--events`、`--explain` 渲染测试，确保机器 stdout 单一协议且 diagnostics 走 stderr。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run 'TestPublishThemeAndDeployOutputModesExposeStableProjection|TestPublishPlanOutputModesExposeStableProjection' -count=1` passed after adding publish theme/deploy output mode coverage for JSON, agent, events, explain and approval error redaction.
- [x] 8.2 更新 `docs/commands/README.md` 和新增 `docs/commands/publish.md`，说明 Pages/Wiki 是发布面，不是 vault 真源或 Cloud Sync。
  - Evidence: 2026-06-18 added `docs/commands/publish.md` and linked it from `docs/commands/README.md`; `rg -n "pinax publish|Cloud Sync|source of truth" docs/commands/publish.md docs/commands/README.md` confirmed the publish surface boundary text.
- [x] 8.3 更新 README/中文 README 的最小使用示例：profile init、plan、build Pages、build Wiki、deploy 前安全注意。
  - Evidence: 2026-06-18 updated `README.md` and `README.zh-CN.md` with static publishing examples and deploy safety notes; `rg -n "pinax publish|私有 vault|private vault" README.md README.zh-CN.md` confirmed both languages.
- [x] 8.4 文档明确不要直接发布私有 vault repo，推荐独立 Pages/Wiki 仓库、clean/orphan strategy、CI 中运行 plan/build/scan。
  - Evidence: 2026-06-18 `docs/commands/publish.md` documents separate Pages/Wiki repositories, deploy scan/hash/receipt gates, clean/orphan CI recommendation and private-vault warning.
- [x] 8.5 文档记录 Hugo 安装、内置主题、`theme list/eject`、`local:<path>` 主题、theme contract、无 Hugo 时 Wiki target 可用、GitHub private Pages 权限不由 Pinax 保证。
  - Evidence: 2026-06-18 `docs/commands/publish.md` documents Hugo dependency, Wiki no-Hugo path, `builtin:pinax-encyclopedia`, `theme list/eject`, `local:<path>`, `pinax.publish_theme.v1`, local-only theme assets and GitHub private Pages permission boundary.

## 9. 集成验证和质量门禁

- [x] 9.1 增加 focused tests：`go test ./internal/app ./internal/app/publishops ./internal/publish ./cmd/pinax -run 'Publish|Hugo|Wiki|Deploy|Redaction' -count=1`。
  - Evidence: 2026-06-18 `go test ./internal/app ./internal/app/publishops ./cmd/pinax -run 'Publish|Hugo|Wiki|Deploy|Redaction' -count=1` passed. This repo has no `./internal/publish` package, so the focused command used the actual publish packages present in this codebase.
- [x] 9.2 增加 testscript e2e：`go test ./tests/e2e -run Publish -count=1`，覆盖 profile、plan、Wiki build、Pages fake Hugo build、deploy fake/bare repo。
  - Evidence: 2026-06-18 `go test ./tests/e2e -run Publish -count=1` passed; command-level e2e coverage also exercises Pages fake Hugo and deploy local Git matrix.
- [x] 9.3 增加输出合同测试：`go test ./cmd/pinax -run 'Publish.*Output|Publish.*Contract|Publish.*Redaction' -count=1`。
  - Evidence: 2026-06-18 `go test ./cmd/pinax -run 'Publish.*Output|Publish.*Contract|Publish.*Redaction' -count=1` passed.
- [x] 9.4 增加 architecture guard 验证：`go test ./internal/architecture -count=1`，确保 `internal/cli`/`cmd/pinax` 不导入 `publishops`，`publishops` 不导入输出/CLI。
  - Evidence: 2026-06-18 `go test ./internal/architecture -count=1` passed.
- [x] 9.5 运行 `openspec validate pinax-hugo-static-publish --strict` 和 `openspec validate --all --strict`，修正所有 schema/spec 问题。
  - Evidence: 2026-06-18 `openspec validate pinax-hugo-static-publish --strict` and `openspec validate --all --strict` passed with 39/39 items.
- [x] 9.6 修改 Go 代码后运行 `task check`；如果本地缺少 `task`，运行 fallback：`golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。
  - Evidence: 2026-06-18 `task check` passed after phase 8/9 updates, covering lint, full tests, build and `openspec validate --all` with 39/39 items.

## 10. Closeout

- [x] 10.1 在 `tasks.md` 每个完成项下记录验证命令、退出码、关键证据和任何跳过原因。
  - Evidence: 2026-06-18 every completed task contains evidence lines; Current Slice Verification includes focused tests, strict OpenSpec validates and repeated `task check` runs, including explicit real-Hugo skip reason.
- [x] 10.2 人工 smoke：用 fixture vault 生成 Pages/Wiki 输出，浏览关键 HTML/Markdown、搜索索引、manifest、receipt，确认无 private sentinel。
  - Evidence: 2026-06-18 manual smoke with a temporary vault and fake Hugo generated Pages `index.html`, Wiki `Home.md`, `pinax-publish-manifest.json` and 2 receipts; grep confirmed `PRIVATE_SENTINEL_DO_NOT_PUBLISH` was absent from Pages/Wiki outputs and Pages HTML contained `pinax-search-data`.
- [x] 10.3 复查 docs/spec/design 是否一致：Hugo 是 Pages renderer，Wiki 是 Markdown target，GitHub 是发布面而非 vault 真源。
  - Evidence: 2026-06-18 `rg -n "Hugo|Wiki|GitHub|source of truth|真源|Cloud Sync|publish" openspec/changes/pinax-hugo-static-publish docs/commands/publish.md README.md README.zh-CN.md` and related consistency grep confirmed docs/spec/design align on Pages/Hugo, Wiki Markdown, GitHub as publish surface, and vault as source of truth.
- [x] 10.4 最终运行 `task check` 和 `openspec validate --all --strict`，确认后将 change 标记 complete，准备 archive。
  - Evidence: 2026-06-18 final `task check` passed, covering lint, full tests, build and `openspec validate --all` with 39/39 items. Final `openspec validate --all --strict` also passed with 39/39 items.

## Current Slice Verification

- 2026-06-18: `go test ./internal/domain ./internal/app/publishops ./internal/app -run 'Publish' -count=1` failed before implementation because publish domain types, publishops rules, and app facade methods did not exist.
- 2026-06-18: `go test ./internal/domain ./internal/app/publishops ./internal/app ./internal/architecture -run 'Publish|CapabilityPackagesDeclareOwnership|CapabilityPackagesDoNotImportCLIOrOutput|CLIImportsAppFacadeOnly' -count=1` passed after adding publish domain models, publishops ownership docs/rules, architecture guard coverage, and service facade placeholders.
- 2026-06-18: `go test ./internal/app/publishops -count=1` passed, confirming profile validation and note eligibility tests actually ran.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed.
- 2026-06-18: `task check` passed after lint fixes, covering `golangci-lint fmt --diff`, `golangci-lint run`, `go test ./...`, `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`, and `openspec validate --all` with 39/39 items.
- 2026-06-18: `go test ./cmd/pinax -run 'PublishProfile' -count=1` passed after adding publish profile init/validate/show/list CLI contract tests.
- 2026-06-18: `go test ./internal/domain ./internal/app ./internal/app/publishops ./internal/cli ./cmd/pinax ./internal/architecture -run 'Publish|PublishProfile|CapabilityPackagesDeclareOwnership|CapabilityPackagesDoNotImportCLIOrOutput|CLIImportsAppFacadeOnly|Help|Command' -count=1` passed after wiring profile services and command registration.
- 2026-06-18: `go test ./tests/e2e -run PublishProfile -count=1` passed after adding testscript coverage for profile init/validate/show/list, JSON/agent stdout contracts, and unsafe profile validation failure.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after phase 2 task updates.
- 2026-06-18: `task check` passed after publish profile CLI/service/testscript work, covering lint, full tests, build, and `openspec validate --all` with 39/39 items.
- 2026-06-18: `go test ./cmd/pinax -run 'PublishPlan' -count=1` passed after adding `publish plan --profile --target` CLI tests for selected/skipped/blocking counts and body leak prevention.
- 2026-06-18: `go test ./tests/e2e -run PublishProfile -count=1` passed after extending publish testscript with a read-only `publish plan` JSON/agent scenario.
- 2026-06-18: `go test ./cmd/pinax ./internal/app ./internal/app/publishops ./internal/cli ./internal/architecture -run 'Publish|PublishProfile|CapabilityPackagesDeclareOwnership|CapabilityPackagesDoNotImportCLIOrOutput|CLIImportsAppFacadeOnly|Help|Command' -count=1` passed after adding publish plan service and command wiring.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after phase 3 note-plan task updates.
- 2026-06-18: `task check` passed after publish plan work, covering lint, full tests, build, and `openspec validate --all` with 39/39 items.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishPlanClassifiesLinkedAssets -count=1` failed before asset eligibility implementation because `publish.plan` selected only the note and emitted no `selected_asset_count` or `asset_not_allowed` facts.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishPlanClassifiesLinkedAssets -count=1` passed after adding linked asset extraction, allowlist checks, and asset facts.
- 2026-06-18: `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile' -count=1` passed after adding app/CLI/e2e coverage for linked publish assets.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after linked asset eligibility task updates.
- 2026-06-18: `task check` passed after linked asset eligibility work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/app/publishops -run TestClassifyNoteViolationsCoversPublishSafetyClasses -count=1` failed before implementation because `private_body_leak` was not classified.
- 2026-06-18: `go test ./internal/app/publishops -run TestClassifyNoteViolationsCoversPublishSafetyClasses -count=1` passed after adding explicit `private_body`/`raw_body`/`private body` detection.
- 2026-06-18: `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile' -count=1` passed after violation classifier changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after violation classification task updates.
- 2026-06-18: `task check` passed after violation classification work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishPlanOutputModesExposeStableProjection -count=1` failed before implementation because default publish plan output used the English summary `Publish plan generated.` instead of the required Chinese summary.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishPlanOutputModesExposeStableProjection -count=1` passed after localizing the plan summary and asserting JSON, agent, events, and explain projection surfaces.
- 2026-06-18: `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile' -count=1` passed after publish plan output mode coverage changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after publish plan projection task updates.
- 2026-06-18: `task check` passed after publish plan projection work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishPlanIncludesSourceInfoAndLinkGraph -count=1` failed before implementation because `publish.plan` emitted no `source_count`, `link_count`, `broken_link_count`, `sources`, or `link_graph` projection.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishPlanIncludesSourceInfoAndLinkGraph -count=1` passed after adding safe source metadata and link graph projection.
- 2026-06-18: `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile' -count=1` passed after publish plan source/link graph changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after source/link graph task updates.
- 2026-06-18: `task check` passed after source/link graph work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./tests/e2e -run PublishProfile -count=1` passed after expanding publish plan testscript fixture coverage for draft/private/secret/unpublished notes, linked/unlinked/disallowed assets, broken links, and leak sentinels.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after publish plan e2e task updates.
- 2026-06-18: `task check` passed after publish plan e2e matrix work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/app/publishops -run TestScanPublishTreeFindsLeaksWithoutEchoingSensitiveContent -count=1` failed before implementation because `ScanPublishTree` and `PublishScanFinding` did not exist.
- 2026-06-18: `go test ./internal/app/publishops -run TestScanPublishTreeFindsLeaksWithoutEchoingSensitiveContent -count=1` passed after adding recursive publish tree scanning, sensitive class mapping, and binary evidence handling.
- 2026-06-18: `go test ./internal/redaction ./internal/domain ./internal/app/publishops -run 'Publish|Sensitive|Redaction|Scan' -count=1` passed after scanner and redaction detection changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after publish scanner task updates.
- 2026-06-18: `task check` passed after publish scanner work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/app/publishops -run TestWriteRedactedEvidenceCoversPublishSurfaces -count=1` failed before implementation because `WriteRedactedEvidence` and `PublishEvidenceSurface` did not exist.
- 2026-06-18: `go test ./internal/app/publishops -run TestWriteRedactedEvidenceCoversPublishSurfaces -count=1` passed after adding redacted evidence writer coverage for stdout/stderr/events/manifest/receipt/Hugo staging/Pages output/Wiki output.
- 2026-06-18: `go test ./internal/redaction ./internal/domain ./internal/app/publishops -run 'Publish|Sensitive|Redaction|Scan|Evidence' -count=1` passed after evidence writer changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after redacted evidence writer task updates.
- 2026-06-18: `task check` passed after redacted evidence writer work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/app/publishops -run TestWritePublishReceiptCreatesStructuredRunReceipt -count=1` failed before implementation because receipt writer and vault version/hash/duration fields did not exist.
- 2026-06-18: `go test ./internal/app/publishops -run TestWritePublishReceiptCreatesStructuredRunReceipt -count=1` passed after adding the structured receipt writer and fields.
- 2026-06-18: `go test ./internal/domain ./internal/app/publishops -run 'Publish|Receipt|Evidence|Scan' -count=1` passed after receipt writer changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after receipt writer task updates.
- 2026-06-18: `task check` passed after receipt writer work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/redaction ./internal/domain ./internal/app/publishops -run 'Publish|Sensitive|Redaction|Scan|Evidence|Receipt' -count=1` passed after expanding recursive scanner contract coverage for all required leak classes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after scanner contract task updates.
- 2026-06-18: `task check` passed after scanner contract work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt -count=1` failed before implementation because `publish build --profile` was not wired and later because Wiki index pages were missing.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt -count=1` passed after adding GitHub Wiki build output generation, manifest, asset copy, indexes and receipt writing.
- 2026-06-18: `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after Wiki build changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after GitHub Wiki build task updates.
- 2026-06-18: `task check` passed after GitHub Wiki build output work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishBuildGitHubWikiFailsWhenOutputScanFindsLeak -count=1` passed after adding Wiki output leak scan failure coverage.
- 2026-06-18: `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after Wiki output scan failure test coverage.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after Wiki output scan task updates.
- 2026-06-18: `task check` passed after Wiki output scan work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt -count=1` failed before Wiki link rewrite because `[[Beta]]` and `[[Missing Target]]` were copied through unchanged.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt -count=1` passed after rewriting selected note links and unresolved placeholders.
- 2026-06-18: `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after Wiki link rewrite changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after Wiki link rewrite task updates.
- 2026-06-18: `task check` passed after Wiki link rewrite work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run 'TestPublishBuildGitHubWiki(WritesMarkdownManifestAndReceipt|BlocksDisallowedLinkedAsset)' -count=1` passed after adding Wiki asset rewrite and disallowed asset build coverage.
- 2026-06-18: `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after Wiki asset coverage changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after Wiki asset task updates.
- 2026-06-18: `task check` passed after Wiki asset copy/rewrite work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./tests/e2e -run PublishProfile -count=1` failed after adding Wiki build testscript coverage because Chinese titles fell back to `note.md` instead of the source filename slug.
- 2026-06-18: `go test ./tests/e2e -run PublishProfile -count=1` passed after adding `publishSlug` fallback to the note filename stem for non-ASCII titles.
- 2026-06-18: `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after Wiki e2e fixture and slug fallback changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after Wiki build testscript task updates.
- 2026-06-18: `task check` passed after Wiki build testscript work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishDoctorDetectsFakeHugoAndProfile -count=1` failed before implementation because `publish doctor --profile` was not wired.
- 2026-06-18: `go test ./cmd/pinax -run TestPublishDoctorDetectsFakeHugoAndProfile -count=1` passed after adding publish doctor CLI/service checks with fake Hugo.
- 2026-06-18: `go test ./internal/app ./internal/cli ./cmd/pinax ./tests/e2e -run 'Publish|PublishPlan|PublishProfile|Command|Help' -count=1` passed after doctor implementation and placeholder test updates.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after publish doctor task updates.
- 2026-06-18: `task check` passed after publish doctor work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/app/publishops -run TestHugoAdapterUsesFakeExecutableAndRedactsStderr -count=1` failed before implementation because `HugoAdapter` did not exist.
- 2026-06-18: `go test ./internal/app/publishops -run TestHugoAdapterUsesFakeExecutableAndRedactsStderr -count=1` passed after adding the fake-executable Hugo adapter.
- 2026-06-18: `go test ./internal/redaction ./internal/app/publishops -run 'Hugo|Publish|Sensitive|Redaction|Scan|Evidence|Receipt' -count=1` passed after Hugo adapter changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after Hugo adapter task updates.
- 2026-06-18: `task check` initially failed in existing alias tests because `scan_duration_ms` varied between alias invocations; after normalizing that volatile test fact, `go test ./cmd/pinax -run TestCLITreePrimaryPathAliases -count=1` and `task check` passed, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `go test ./internal/app/publishops -run TestBuildHugoStagingProjectWritesSafeThemeContract -count=1` failed before implementation because `BuildHugoStagingProject` and `HugoStagingRequest` did not exist.
- 2026-06-18: `go test ./internal/app/publishops -run TestBuildHugoStagingProjectWritesSafeThemeContract -count=1` passed after adding Hugo staging project generation.
- 2026-06-18: `go test ./internal/app/publishops -run 'Publish|Sensitive|Redaction|Scan|Evidence|Receipt|Hugo|Staging' -count=1` passed after staging builder changes.
- 2026-06-18: `openspec validate pinax-hugo-static-publish --strict` passed after Hugo staging/theme contract task updates.
- 2026-06-18: `task check` passed after Hugo staging/theme contract work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.

- 2026-06-18: `task check` passed after Pages/Hugo build wiring, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after built-in encyclopedia theme expansion, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after theme visual/local-resource contract work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after theme list/eject implementation, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after local theme source support, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after fake Hugo failure matrix, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after optional real Hugo smoke coverage and completion of Hugo/GitHub Pages phase 6, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after deploy policy parser work, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after local Git deploy implementation, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after pre-deploy receipt/hash/scan validation, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after Git deploy error redaction, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- 2026-06-18: `task check` passed after deploy e2e matrix and completion of deploy phase 7, covering `openspec validate --all` with 39/39 items, `go test ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
