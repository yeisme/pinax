## 1. Renderer 合同和兼容模型

- [x] 1.1 定义 `PublishDocRenderer`、远端对象类型和兼容读取规则。
  - Owner: Pinax domain
  - Scope: 在 document publish domain model 中新增 `renderer`、`render_revision`、`remote_object_type`、`render_warnings` additive 字段；支持 `native-docx` 和 `markdown-file`；旧 profile/mapping 缺字段时可读取。
  - Dependencies: 无。
  - Parallel lane: A。
  - Acceptance: 新 `lark-doc` profile 默认 `renderer=native-docx`；显式 `--renderer markdown-file` 保留旧行为；旧 mapping 不因为缺字段失败。
  - Validation: `go test ./internal/domain ./internal/app -run 'PublishDoc.*Renderer|PublishDoc.*Compatibility' -count=1`
  - Expected: PASS。
  - Failure recheck: 若旧 fixture 读取失败，先补默认值迁移逻辑，不改 schema_version 破坏兼容。

- [x] 1.2 为 CLI profile 增加 `--renderer native-docx|markdown-file`。
  - Owner: Pinax CLI/app
  - Scope: `pinax publish doc profile set lark-doc` 接受 renderer flag；provider profile 写入由 app service 完成；`--json` facts 输出 renderer。
  - Dependencies: 1.1。
  - Parallel lane: A。
  - Acceptance: `pinax publish doc profile set lark-doc --folder <folder> --as user --renderer native-docx --vault ./my-notes --json` 返回 `facts.renderer="native-docx"`；未知 renderer 返回稳定 validation error。
  - Validation: `go test ./cmd/pinax -run 'TestPublishDoc.*Profile.*Renderer|TestPublishDoc.*InvalidRenderer' -count=1`
  - Expected: PASS。
  - Failure recheck: 确认命令层只做 flag 绑定，校验和写入在 app service。

## 2. Markdown publish AST

- [x] 2.1 新增 Markdown-to-publish-AST parser。
  - Owner: Pinax render/app
  - Scope: 从 note body 和 vault template 输出解析标题、段落、heading、列表、引用、代码块、表格、链接、图片、Mermaid fenced block、inline/raw SVG 和 unsupported HTML。
  - Dependencies: 1.1。
  - Parallel lane: B。
  - Acceptance: parser 不调用 provider、不读写 `.pinax/**`；每个节点带 source line、node type 和 safe text/attrs；不保留 raw credential-like URL headers。
  - Validation: `go test ./internal/app ./internal/markdownnote -run 'PublishDoc.*AST|Markdown.*Publish' -count=1`
  - Expected: PASS。
  - Failure recheck: 若现有 markdown parser 包不适合承载 publish AST，新建小包但不要重写已有搜索 parser。

- [x] 2.2 实现 vault template 到 AST 的重复标题处理。
  - Owner: Pinax render/app
  - Scope: 现有 `template=vault` 继续提供 Overview/Reading Notes；AST 层确认同名 H1 只保留一次。
  - Dependencies: 2.1。
  - Parallel lane: B。
  - Acceptance: 以 H1 开头和无 H1 的 note 都能生成稳定 AST；生成的原生文档标题来自 note title，不重复出现在正文第一屏。
  - Validation: `go test ./internal/app -run 'PublishDoc.*VaultTemplate|PublishDoc.*DuplicateTitle' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查 title normalization 是否错误处理中文全角标点。

## 3. 资产渲染管线

- [x] 3.1 实现本地图片和附件解析。
  - Owner: Pinax asset/render
  - Scope: 解析 Markdown 相对图片、Pinax attachment references 和 vault 内资源；拒绝路径逃逸；输出 package-local artifacts metadata。
  - Dependencies: 2.1。
  - Parallel lane: C。
  - Acceptance: package 记录 artifact 相对路径、sha256、media type、source note path；不记录本地绝对路径。
  - Validation: `go test ./internal/app ./internal/index -run 'PublishDoc.*Asset|PublishDoc.*Attachment|PathEscape' -count=1`
  - Expected: PASS。
  - Failure recheck: 若 fixture 使用绝对路径，改成 vault-relative fixture。

- [x] 3.2 实现 Mermaid 预渲染策略。
  - Owner: Pinax render/assets
  - Scope: Mermaid fenced block 转 PNG artifact；renderer 不可用时返回 `render_warning` 和 code block fallback，不静默丢内容。
  - Dependencies: 2.1, 3.1。
  - Parallel lane: C。
  - Acceptance: 成功路径生成 image artifact；失败路径保留 source code block 并输出 warning code `mermaid_render_unavailable`。
  - Validation: `go test ./internal/app -run 'PublishDoc.*Mermaid' -count=1`
  - Expected: PASS。
  - Failure recheck: 不允许测试依赖真实浏览器或公网；用 fake renderer 覆盖成功和失败。

- [x] 3.3 实现 SVG fallback 渲染。
  - Owner: Pinax render/assets
  - Scope: inline SVG 和 `.svg` 图片转 PNG artifact；无法转换时返回 `svg_render_unavailable` warning，不把 raw inline SVG 插入原生文档。
  - Dependencies: 2.1, 3.1。
  - Parallel lane: C。
  - Acceptance: SVG 内容经过大小和安全限制；package/receipt 不保存潜在脚本或外链加载 raw payload。
  - Validation: `go test ./internal/app -run 'PublishDoc.*SVG|PublishDoc.*RenderWarning' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查 sanitizer 是否允许 `<script>` 或外部引用进入 artifact。

## 4. Feishu native document renderer

- [x] 4.1 定义 native document render plan。
  - Owner: Pinax provider/render
  - Scope: 将 AST 转为 provider-neutral native doc plan：title、blocks、media uploads、warnings、unsupported fallback blocks。
  - Dependencies: 2.1, 3.1。
  - Parallel lane: D。
  - Acceptance: renderer 产物与 `lark-cli` 命令无关，便于 fake provider 测试；包含 block order 和 media references。
  - Validation: `go test ./internal/app ./internal/provider -run 'PublishDoc.*NativePlan' -count=1`
  - Expected: PASS。
  - Failure recheck: 不要让 app service 拼 provider-specific shell args。

- [x] 4.2 实现 `lark-cli` 原生文档 provider adapter。
  - Owner: Pinax provider adapter
  - Scope: adapter 支持 capability check、create native doc、update native doc、upload media、inspect object type；解析 normalized JSON；脱敏 stderr/stdout。
  - Dependencies: 4.1。
  - Parallel lane: D。
  - Acceptance: `renderer=native-docx` 成功时 mapping `external_object.type` 为 `docx` 或 `doc`；如果 `lark-cli` 不支持原生文档写入，返回 `provider_capability_missing`，不得自动调用 `markdown +create`。
  - Validation: `go test ./internal/provider ./cmd/pinax -run 'Lark.*NativeDoc|PublishDoc.*ProviderCapability' -count=1`
  - Expected: PASS。
  - Failure recheck: fake `lark-cli` 应记录被调用命令，确认没有走 `markdown +create`。

- [x] 4.3 实现 native update 和 object type migration guard。
  - Owner: Pinax app/provider
  - Scope: active mapping 是 `file` 且 profile renderer 是 `native-docx` 时，不覆盖原 Markdown file；返回 action 建议 unlink/re-publish 或新建 native copy。
  - Dependencies: 1.1, 4.2。
  - Parallel lane: D。
  - Acceptance: 旧 mapping 不被静默改义；新 native mapping update 走原生文档更新。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run 'PublishDoc.*Migration|PublishDoc.*NativeUpdate' -count=1`
  - Expected: PASS。
  - Failure recheck: 检查 detached mapping 分支是否仍能创建新 native object。

## 5. Index page 和输出合同

- [x] 5.1 将 Feishu index page 切换为 native renderer。
  - Owner: Pinax app/provider
  - Scope: `index_page=true` 且 `renderer=native-docx` 时创建或更新原生 index doc；表格包含 folder、title、status、renderer、link。
  - Dependencies: 4.2。
  - Parallel lane: E。
  - Acceptance: profile `index_object` 记录 object type 和 renderer；旧 `_Pinax Vault Index.md` file 不自动删除。
  - Validation: `go test ./cmd/pinax ./tests/e2e -run 'PublishDoc.*Index.*Native' -count=1`
  - Expected: PASS。
  - Failure recheck: 确认 index 更新不会把 provider raw payload 写进 profile。

- [x] 5.2 扩展 `status/list/push` 输出 facts。
  - Owner: Pinax output/CLI
  - Scope: `--json`、`--agent`、`--events` 输出新增 renderer、external object type、render warning count；默认 human summary 说明 fallback 或 warning。
  - Dependencies: 1.1, 4.2。
  - Parallel lane: E。
  - Acceptance: 旧字段不删除；stdout/stderr 分离；`--agent` 只输出稳定英文 key。
  - Validation: `go test ./cmd/pinax -run 'PublishDoc.*Output|PublishDoc.*Agent|PublishDoc.*Events' -count=1`
  - Expected: PASS。
  - Failure recheck: 用 JSON parser 校验 stdout 只有一个 envelope。

## 6. 文档和 agent 操作说明

- [x] 6.1 更新 publish 命令文档。
  - Owner: Pinax docs
  - Scope: `docs/commands/publish.md` 说明 `lark-doc` 默认 native-docx、`markdown-file` fallback、Mermaid/SVG 预渲染、迁移旧 mapping 的推荐命令。
  - Dependencies: 1.2, 5.2。
  - Parallel lane: F。
  - Acceptance: 文档命令都是真实用户可运行命令；不出现 agent-only wrapper；不包含 token 示例。
  - Validation: `rg -n "native-docx|markdown-file|pinax publish doc profile set lark-doc" docs/commands/publish.md`
  - Expected: 输出相关说明。
  - Failure recheck: 若命令名和实现不同，以 CLI help 为准更新文档。

- [x] 6.2 更新 Pinax publish operator handoff。
  - Owner: Pinax skill/docs handoff
  - Scope: 说明 agent 发布飞书阅读版时应选择 `renderer=native-docx`，评论/权限仍使用原生 `lark-cli`，不写 Pinax mapping。
  - Dependencies: 6.1。
  - Parallel lane: F。
  - Acceptance: skill 源目录更新后同步 runtime copies；不手改生成目录作为唯一来源。
  - Validation: `scripts/skills.sh validate-custom` 从仓库根运行。
  - Expected: PASS。
  - Failure recheck: 如果 Pinax operator skill 不在根源目录，先按 root routing 确认 ownership。

## 7. 测试、真实 smoke 和质量门禁

- [x] 7.1 扩展 fake `lark-cli` e2e。
  - Owner: Pinax tests
  - Scope: fake 支持 native doc create/update/upload/inspect；验证 `renderer=native-docx` 不调用 `markdown +create/+overwrite`；验证 `renderer=markdown-file` 仍调用旧路径。
  - Dependencies: 4.2, 5.2。
  - Parallel lane: G。
  - Acceptance: e2e 覆盖 profile、prepare、dry-run、push、status、list、index native、capability missing、redaction。
  - Validation: `go test ./tests/e2e -run TestPublishDoc -count=1`
  - Expected: PASS，并写入 integration evidence。
  - Failure recheck: 检查 PATH fake executable 是否覆盖真实 `lark-cli`。

- [x] 7.2 执行真实飞书 native smoke。
  - Status: 2026-07-02 已用真实 lark-cli v1.0.63（user identity 叶树根）执行。结论记录在 `smoke-checklist.md` Smoke Run Log：`facts.renderer="native-docx"`、`external_object_type="docx"`、`drive +inspect` 独立确认 type=docx、`docs +fetch` 确认原生块渲染（title/headings/tables/code/list/blockquote）。Mermaid 因无 renderer 配置走 fallback code block + warning（spec 合规）。
  - Owner: Pinax maintainer/agent operator
  - Scope: 在用户授权的测试文件夹发布一篇包含标题、表格、代码块、Mermaid、SVG、本地图片的临时 note；检查远端 object type 是 `docx` 或 `doc`；打开飞书确认 Mermaid/SVG 已显示为图片或原生可读块。
  - Dependencies: 7.1。
  - Parallel lane: G。
  - Acceptance: smoke 记录命令、远端 URL、object type、人工/CLI 验证结论；测试文档清理后 index 恢复正式条目。
  - Validation: `pinax publish doc status --note <note-id> --target lark-doc --vault ./my-notes --json`
  - Expected: `facts.renderer="native-docx"` 且 external object type 为 `docx` 或 `doc`。
  - Failure recheck: 若 Feishu API 无法证明视觉渲染，验收记录必须标记需要人工打开检查，不能只用 Markdown fetch 代替。

- [x] 7.3 运行最终质量门禁。
  - Owner: Pinax maintainers
  - Scope: format、lint、unit、e2e、build、OpenSpec validate。
  - Dependencies: 1-7。
  - Parallel lane: Final。
  - Acceptance: 合同、实现、文档和测试都通过；integration evidence 保留且脱敏。
  - Validation: `task check`
  - Expected: PASS。
  - Failure recheck: 若本地缺少 `task`，运行 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。
