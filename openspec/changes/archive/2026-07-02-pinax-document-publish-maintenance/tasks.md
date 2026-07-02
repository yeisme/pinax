## 1. 领域模型和状态合同

- [x] 1.1 新增 `DocumentPublishTarget`、`DocumentPublishProfile`、`DocumentPublishPackage`、`DocumentPublishMapping`、`DocumentPublishReceipt`、`ExternalDocStatus` 和 `DocumentPublishStatus` 领域模型。
  - Owner: Pinax domain
  - Scope: `internal/domain` 或既有发布模型包；字段使用稳定英文 JSON key。
  - Dependencies: 无。
  - Parallel lane: A。
  - Acceptance: `notion-page`、`lark-doc`、状态枚举和 schema version 有单元测试。
  - Validation: `go test ./internal/domain -run 'DocumentPublish|PublishDoc' -count=1`
  - Expected: PASS。
  - Failure recheck: 若失败，先确认没有复用 static-site target 枚举导致语义混淆。

- [x] 1.2 实现 content digest 和 stale 判定纯逻辑。
  - Owner: Pinax domain/app
  - Scope: note 正文、允许 frontmatter 和允许附件引用生成稳定 digest；mapping digest 不一致时返回 `stale`。
  - Dependencies: 1.1。
  - Parallel lane: A。
  - Acceptance: 同一内容 digest 稳定；正文变化、标题变化、附件引用变化会 stale；私有 metadata 不参与 digest。
  - Validation: `go test ./internal/domain ./internal/app -run 'DocumentPublish|Digest|Stale' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查测试 fixture 是否包含时间戳等非稳定输入。

## 2. App service 和 CLI 命令骨架

- [x] 2.1 新增 document publish app service facade。
  - Owner: Pinax app service
  - Scope: `Prepare`、`Push`、`Status`、`List`、`Link`、`Unlink`、`ProviderDoctor` 请求/响应类型和占位实现。
  - Dependencies: 1.1。
  - Parallel lane: B。
  - Acceptance: 命令层可只调用 app facade，不直接触达 provider executable 或 `.pinax/**` 文件。
  - Validation: `go test ./internal/app ./internal/architecture -run 'DocumentPublish|Architecture|CLIImportsAppFacadeOnly' -count=1`
  - Expected: PASS。
  - Failure recheck: 若 architecture guard 失败，移动逻辑到 app/provider adapter。

- [x] 2.2 注册 `pinax publish doc` 命令族。
  - Owner: Pinax CLI
  - Scope: `provider list|doctor`、`profile set`、`prepare`、`push`、`status`、`list`、`link`、`unlink`。
  - Dependencies: 2.1。
  - Parallel lane: B。
  - Acceptance: `--help` 列出命令；命令层只做参数校验和输出模式选择。
  - Validation: `go test ./cmd/pinax -run 'PublishDoc|Help|Command' -count=1`
  - Expected: PASS。
  - Failure recheck: 确认没有破坏现有 `pinax publish profile|plan|build|deploy` 命令。

## 3. CLI-authored structured assets

- [x] 3.1 实现 `.pinax/publish/doc/profiles/<target>.yaml` 的 profile 写入和校验。
  - Owner: Pinax storage/app
  - Scope: `pinax publish doc profile set ...` 通过 service 原子写入 profile；拒绝未知 target、路径逃逸、空 workspace/folder/parent 配置。
  - Dependencies: 1.1, 2.1。
  - Parallel lane: C。
  - Acceptance: agent 不需要也不允许手写 profile 文件。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishDocProfile' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查 YAML round-trip 是否保留 schema_version 和 target。

- [x] 3.2 实现 package、mapping 和 receipt CLI-authored 写入。
  - Owner: Pinax storage/app
  - Scope: `prepare` 写 package；`push/link/unlink/status` 维护 mapping；真实 push/link/unlink 写 run receipt。
  - Dependencies: 3.1。
  - Parallel lane: C。
  - Acceptance: `.pinax/publish/doc/**` 只由 app service 写入；receipt 不含 raw provider payload、token、Authorization/Cookie。
  - Validation: `go test ./internal/app ./cmd/pinax -run 'PublishDocPackage|PublishDocMapping|PublishDocReceipt|Redaction' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查错误路径是否也写入脱敏 receipt 或明确不写。

## 4. Provider adapter

- [x] 4.1 定义 `DocumentPublishProvider` adapter interface 和 fake provider。
  - Owner: Pinax provider adapter
  - Scope: `Doctor`、`Preflight`、`Create`、`Update`、`Status`，fake provider 支持成功和失败脚本化响应。
  - Dependencies: 1.1。
  - Parallel lane: D。
  - Acceptance: app service 通过 interface 调用 provider；测试不需要真实 Notion/飞书。
  - Validation: `go test ./internal/app ./internal/provider ./cmd/pinax -run 'DocumentPublishProvider|FakeProvider' -count=1`
  - Expected: PASS。
  - Failure recheck: 若包路径不同，使用项目既有 provider adapter 目录并更新 architecture guard。

- [x] 4.2 实现 Notion CLI adapter。
  - Owner: Pinax provider adapter
  - Scope: 通过受控 subprocess 调用 Notion CLI；解析 normalized JSON；脱敏 stderr/stdout；支持 executable missing/auth failed/create/update/status。
  - Dependencies: 4.1。
  - Parallel lane: D。
  - Acceptance: 不直接引入 Notion SDK；不把 raw payload 写入输出或 receipt。
  - Validation: `go test ./internal/provider ./cmd/pinax -run 'Notion|DocumentPublish|Redaction' -count=1`
  - Expected: PASS。
  - Failure recheck: 用 fake executable 复现 provider 输出，先修 parser 再接真实命令名。

- [x] 4.3 实现 `lark-cli` adapter。
  - Owner: Pinax provider adapter
  - Scope: 通过受控 subprocess 调用 `lark-cli`；解析 normalized JSON；脱敏 stderr/stdout；支持 executable missing/auth failed/create/update/status。
  - Dependencies: 4.1。
  - Parallel lane: D。
  - Acceptance: 不直接引入飞书 SDK；不管理评论、批注、权限、审批。
  - Validation: `go test ./internal/provider ./cmd/pinax -run 'Lark|DocumentPublish|Redaction' -count=1`
  - Expected: PASS。
  - Failure recheck: 若 `lark-cli` 当前缺少能力，adapter 返回稳定 `provider_capability_missing`，不要扩大 Pinax scope。

## 5. 发布流程

- [x] 5.1 实现 `prepare`。
  - Owner: Pinax app service
  - Scope: 从 note 读取发布安全内容，生成 package、digest、summary、warnings 和 next action。
  - Dependencies: 1.2, 3.2。
  - Parallel lane: E。
  - Acceptance: `prepare` 不触达外部 provider，不写 mapping，不写远端。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run 'PublishDocPrepare' -count=1`
  - Expected: PASS。
  - Failure recheck: 确认 dry-run/prepare 没有写 provider state。

- [x] 5.2 实现 `push --dry-run`。
  - Owner: Pinax app service
  - Scope: 加载 package 和 profile，执行 provider `Preflight`，返回 plan 和 warnings，不写远端和 mapping。
  - Dependencies: 4.1, 5.1。
  - Parallel lane: E。
  - Acceptance: dry-run 可写脱敏 evidence，但不写 mapping，不调用 Create/Update。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run 'PublishDocDryRun' -count=1`
  - Expected: PASS。
  - Failure recheck: fake provider 记录调用序列，确认只有 Preflight。

- [x] 5.3 实现真实 `push` create/update。
  - Owner: Pinax app service
  - Scope: 无 mapping 时 create；已有 mapping 时 update；成功后写 mapping 和 receipt。
  - Dependencies: 5.2。
  - Parallel lane: E。
  - Acceptance: 成功输出 external URL；失败输出稳定 error code；所有 provider 内容脱敏。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run 'PublishDocPush' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查 create/update 分支是否都更新 content_digest。

- [x] 5.4 实现 `status`、`list`、`link`、`unlink`。
  - Owner: Pinax app service
  - Scope: `status` 显示 linked/published/stale/detached/failed；`list` 按 target 列 mapping；`link` 只建立外部 URL/id 映射；`unlink` 标记 detached 或删除 active mapping。
  - Dependencies: 3.2, 5.3。
  - Parallel lane: E。
  - Acceptance: `link` 不触达远端；`unlink` 不删除远端文档；状态判断可重复。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run 'PublishDocStatus|PublishDocLink|PublishDocUnlink|PublishDocList' -count=1`
  - Expected: PASS。
  - Failure recheck: 确认 unlink 没有调用 provider delete。

## 6. 输出合同和 agent handoff

- [x] 6.1 实现所有 document publish 命令 projection。
  - Owner: Pinax output
  - Scope: 默认中文摘要；`--json`、`--agent`、`--events`、`--explain` 从同一 projection 渲染。
  - Dependencies: 2.2, 5.1。
  - Parallel lane: F。
  - Acceptance: 机器 stdout 单协议；diagnostics/provider stderr 不混入 JSON/agent/events stdout。
  - Validation: `go test ./cmd/pinax -run 'PublishDoc.*Output|PublishDoc.*Contract|PublishDoc.*Redaction' -count=1`
  - Expected: PASS。
  - Failure recheck: 用 JSON parser 和 agent key assertions 定位污染输出。

- [x] 6.2 更新 Pinax publish operator skill 或文档，说明 agent handoff。
  - Owner: Pinax docs/skills handoff
  - Scope: agent 先调用 `pinax publish doc status|prepare|push`；评论、批注、权限、协作者等需求直接调用 Notion/lark 原生 CLI，不写 Pinax mapping。
  - Dependencies: 6.1。
  - Parallel lane: F。
  - Acceptance: 文档显示真实用户可运行命令，不包含 agent-only wrapper。
  - Validation: `rg -n "pinax publish doc|lark-cli|notion" docs .agents .claude 2>/dev/null || true`
  - Expected: 能找到新增说明；没有 token 示例或 shell credential script。
  - Failure recheck: 若 skill runtime 是生成目录，改源目录并运行对应 sync 命令。

## 7. 集成测试证据和质量门禁

- [x] 7.1 增加 testscript e2e fixture。
  - Owner: Pinax tests
  - Scope: fixture vault、fake notion executable、fake `lark-cli` executable、prepare/dry-run/push/status/link/unlink/list、失败和脱敏路径。
  - Dependencies: 4.2, 4.3, 5.4。
  - Parallel lane: G。
  - Acceptance: 不依赖真实公网、真实 token 或用户 vault。
  - Validation: `go test ./tests/e2e -run PublishDoc -count=1`
  - Expected: PASS。
  - Failure recheck: 检查 PATH 中 fake executable 是否覆盖真实工具。

- [x] 7.2 增加 integration evidence writer 或复用现有测试 evidence 入口。
  - Owner: Pinax tests/tooling
  - Scope: 每次 document publish integration/e2e run 写入 `temp/integration-test-runs/<run-id>/summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`。
  - Dependencies: 7.1。
  - Parallel lane: G。
  - Acceptance: 失败测试也保留脱敏 evidence，并保持原始 exit code。
  - Validation: `go test ./tests/e2e -run PublishDoc -count=1`
  - Expected: PASS，且 evidence 目录存在。
  - Failure recheck: 检查证据写入是否吞掉测试失败码。

- [x] 7.3 运行最终质量门禁。
  - Owner: Pinax maintainers
  - Scope: lint、test、build、OpenSpec validate。
  - Dependencies: 1-7。
  - Parallel lane: Final。
  - Acceptance: 所有相关测试通过；OpenSpec 无 schema/spec 问题。
  - Validation: `task check`
  - Expected: PASS。
  - Failure recheck: 若本地缺少 `task`，运行 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。

## 8. Cloud vault reading experience

- [x] 8.1 实现 Feishu mirror layout。
  - Owner: Pinax app/provider adapter
  - Scope: profile 支持 `layout=mirror`；push 按 note path 创建或复用远端 folder；旧 flat mapping 在下一次 push 时移动到目标 folder；folder token 写入 CLI-authored `.pinax/publish/doc/folders/**`。
  - Dependencies: 4.3, 5.3。
  - Parallel lane: H。
  - Acceptance: 发布 `notes/index/foo.md` 后，飞书目标 folder 下存在 `notes/index/foo.md`，mapping 包含 `remote_path` 和 `remote_folder_token`。
  - Validation: `go test ./tests/e2e -run TestPublishDoc -count=1`。
  - Expected: PASS。
  - Failure recheck: 检查 folder search 是否复用已有 folder，避免重复创建。

- [x] 8.2 实现 vault reading template。
  - Owner: Pinax app render
  - Scope: profile 支持 `template=vault`；prepare package 写入包含 Pinax overview 的 `body_markdown`；去除重复 H1；保留 `plain` 兼容路径。
  - Dependencies: 5.1。
  - Parallel lane: H。
  - Acceptance: package 和远端 fetch 内容包含 Pinax overview、source path、Reading Notes，且没有重复同名 H1。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run TestPublishDoc -count=1`。
  - Expected: PASS。
  - Failure recheck: 对以 H1 开头和无 H1 正文分别测试。

- [x] 8.3 实现 Feishu vault index page。
  - Owner: Pinax app/provider adapter
  - Scope: profile 支持 `index_page=true`；push 成功后创建或更新 `_Pinax Vault Index.md`，列出 folder/title/status/link；profile 记录 `index_object`。
  - Dependencies: 5.3, 8.1。
  - Parallel lane: H。
  - Acceptance: 目标 folder 下存在 `_Pinax Vault Index.md`，profile 记录 index file token，index 包含 5 篇已发布 note 链接。
  - Validation: `lark-cli markdown +fetch --file-token <index-token> --as user --output <file> --overwrite --json`。
  - Expected: PASS。
  - Failure recheck: 检查 profile 是否被 profile set 重置，以及 update 分支是否使用 index_object。


## Current Slice Verification

- 2026-07-02: `go test ./cmd/pinax -run TestPublishDocLarkWorkflowCreatesMappingAndStatus -count=1` first failed because `pinax publish doc profile set ... --space` was not registered, proving the command contract was absent before implementation.
- 2026-07-02: Added document publish domain models, CLI-authored profile/package/mapping/receipt service methods, `pinax publish doc` Cobra wiring, and CLI-backed `lark-cli` adapter using `lark-cli markdown +create/+overwrite` with temporary relative Markdown files.
- 2026-07-02: `go test ./cmd/pinax -run 'TestPublishDoc' -count=1` passed, covering lark-doc profile/prepare/dry-run/push/status/list, provider list, Notion profile, and redacted agent failure output.
- 2026-07-02: `go test ./internal/domain ./internal/app ./internal/cli ./cmd/pinax -run 'PublishDoc|Publish|Command|Help' -count=1` passed.
- 2026-07-02: `go test ./internal/architecture -count=1` passed.
- 2026-07-02: `openspec validate pinax-document-publish-maintenance --strict` passed.
- 2026-07-02: Configured the default vault `/workspaces/yeisme-agent/data/yeisme-notes` with `pinax publish doc profile set lark-doc --folder https://px5mwgtxwel.feishu.cn/drive/folder/XEXHfNVBvllsKldiGyGcmH95noc --vault /workspaces/yeisme-agent/data/yeisme-notes --json`; profile normalized folder token `XEXHfNVBvllsKldiGyGcmH95noc`.
- 2026-07-02: Selected the five most recently updated `notes/index` notes from the default vault: `note_0c1c1243063d`, `note_ac6b212be785`, `note_6d7f7d4589c5`, `note_8c8133e9ccde`, `note_fa3c45e5927f`.
- 2026-07-02: `pinax publish doc prepare` plus `pinax publish doc push --dry-run` succeeded for all five selected notes with `publish_status=dry_run_verified`.
- 2026-07-02: Real `pinax publish doc push` to the requested Feishu folder failed with stable `provider_permission_denied`; `lark-cli` bot identity can inspect the folder but cannot write Markdown files to it, and user identity is missing/expired. Generated a no-wait Feishu auth URL and QR code at `temp/lark-auth/pinax-publish-auth.png` for re-authorization.
- 2026-07-02: Added `lark-doc` profile identity selector support with `--as auto|user|bot`; configured the default vault profile with `--as user` so subsequent writes use Feishu user identity once `lark-cli auth login` is completed.
- 2026-07-02: Updated the OpenSpec design/spec to make Pinax responsible only for document publish maintenance and to leave comments, annotations, permissions, collaborators and other platform-native actions to agent-operated `lark-cli` or Notion CLI commands.
- 2026-07-02: `go test ./cmd/pinax -run 'TestPublishDoc' -count=1` passed after adding `--as user` profile coverage.
- 2026-07-02: `go test ./internal/domain ./internal/app ./internal/cli ./cmd/pinax -run 'PublishDoc|Publish|Command|Help' -count=1` passed.
- 2026-07-02: `go test ./internal/architecture -count=1` passed.
- 2026-07-02: `openspec validate pinax-document-publish-maintenance --strict` passed after the identity-selector and native-CLI handoff spec updates.
- 2026-07-02: Tightened `pinax publish doc provider doctor --target lark-doc` so explicit `--as user|bot` profiles verify the requested `lark-cli auth status` identity; the real default vault now fails with stable `provider_auth_failed` until Feishu user authorization is restored.
- 2026-07-02: Feishu user device authorization completed; `lark-cli auth status` reported user identity ready for `叶树根`, and `pinax publish doc provider doctor --target lark-doc --vault /workspaces/yeisme-agent/data/yeisme-notes --json` returned success with `as=user`.
- 2026-07-02: Real `pinax publish doc push` published all five selected `notes/index` notes to the requested Feishu folder and wrote five `lark-doc` mappings with `publish_status=published`.
- 2026-07-02: `pinax publish doc list --target lark-doc --vault /workspaces/yeisme-agent/data/yeisme-notes --json` returned `mappings=5`; per-note `pinax publish doc status` returned `publish_status=published` for `note_0c1c1243063d`, `note_ac6b212be785`, `note_6d7f7d4589c5`, `note_8c8133e9ccde`, and `note_fa3c45e5927f`.
- 2026-07-02: `lark-cli drive +inspect --url <published-url> --json` succeeded for all five published Feishu URLs and returned titles matching the selected notes.
- 2026-07-02: Added document publish command documentation under `docs/commands/publish.md`, including agent handoff to native `lark-cli` and Notion CLI for comments, annotations, permissions and collaborators.
- 2026-07-02: Added `go test ./tests/e2e -run TestPublishDoc -count=1` testscript coverage for `lark-doc` and `notion-page` profile/doctor/prepare/dry-run/push/status/list with fake provider CLIs.
- 2026-07-02: Added `TestPublishDoc` to the integration evidence runner; `task test:integration` passed and wrote `temp/integration-test-runs/20260702T081949Z-3910379/` with `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, and `artifacts/README.txt`.
- 2026-07-02: `task check` passed after the document publish implementation and real Feishu publish path were in place.
- 2026-07-02: Implemented Feishu cloud-vault defaults for `lark-doc`: `layout=mirror`, `template=vault`, and `index_page=true`.
- 2026-07-02: Added folder mapping state under `.pinax/publish/doc/folders/**`, folder search/reuse before create, and migration of existing flat files into mirrored remote folders on push.
- 2026-07-02: Re-published the five selected notes with vault render template; `lark-cli drive +pull --folder-token XEXHfNVBvllsKldiGyGcmH95noc --local-dir temp/pinax-feishu-folder-pull-vault-v2 --as user --json` showed `_Pinax Vault Index.md` and five files under `notes/index/`.
- 2026-07-02: `lark-cli markdown +fetch --file-token Z0L1bnL02oSVIgxbi05cxNvWnqe --as user --output temp/pinax-fetch-check/multica-v2.md --overwrite --json` confirmed the remote note includes the Pinax overview block and no duplicate top-level title.
- 2026-07-02: `lark-cli markdown +fetch --file-token Pay5bV6xNoM9YPxQ7D8c3tJVnxH --as user --output temp/pinax-fetch-check/index.md --overwrite --json` confirmed `_Pinax Vault Index.md` lists all five published notes.
