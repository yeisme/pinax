## 1. Contract and Template Tests

- [x] 1.1 固定 `source.github` 模板创建合同。
  - Owner: Pinax
  - Lane: A
  - Depends on: none
  - Scope: 在 `cmd/pinax` 增加命令级测试，覆盖 `pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --vault <tmp> --json`。
  - Acceptance: 初始运行 `go test ./cmd/pinax -run 'TestSourceTemplate' -count=1` 失败，失败原因是缺少模板或输出路径/metadata 不符合规格；实现后通过。
  - Expected result: JSON envelope 保持现有顶层字段，facts/data 中包含模板名、有效路径、kind、status、tags，不泄露 raw provider payload。
  - Failure re-check: 不允许通过放宽 JSON envelope 或删除现有 note add 断言来通过测试。

- [x] 1.2 固定显式 CLI 参数优先级。
  - Owner: Pinax
  - Lane: A
  - Depends on: 1.1
  - Scope: 测试 `--dir custom --kind reference --status draft --tags custom/tag` 覆盖模板默认值。
  - Acceptance: `go test ./cmd/pinax -run 'TestSourceTemplateExplicitFieldsOverrideDefaults' -count=1` 通过。
  - Expected result: 输出路径位于 `custom/`，frontmatter 使用显式 kind/status/tags，同时仍记录 template fact。
  - Failure re-check: 若旧模板优先级行为被破坏，先修复模板 merge 顺序，不改全局 note add 默认路径。

## 2. Built-In Template Implementation

- [x] 2.1 新增 `source.github` 内置模板。
  - Owner: Pinax
  - Lane: B
  - Depends on: 1.1
  - Scope: 修改 `internal/app/builtin_templates.go`，新增模板 metadata、默认 frontmatter、输出 path pattern 和正文结构。
  - Acceptance: `go test ./internal/app -run 'TestBuiltinTemplatesIncludeSourceGithub' -count=1` 通过。
  - Expected result: 模板可被 `template list`、`template inspect`、`template recommend` 或既有模板发现路径看到；不联网、不调用 GitHub API。
  - Failure re-check: 若模板检测失败，先检查 builtin registry，不新增外部 provider 依赖。

- [x] 2.2 更新模板文档和命令手册。
  - Owner: Pinax
  - Lane: B
  - Depends on: 2.1
  - Scope: 更新 `docs/commands/template.md`、`docs/commands/note.md` 或已有模板文档，说明 `source.github` 用法、字段覆盖和长期资料源边界。
  - Acceptance: 文档包含真实命令示例，且不建议手写 `.pinax/` 资产。
  - Expected result: 用户能从文档知道如何创建 GitHub 资料源卡片。
  - Failure re-check: 文档示例必须是人能直接运行的命令，不使用本地执行 wrapper 或 agent-only 前缀。

## 3. Metadata and Organize Suggestions

- [x] 3.1 增加外部资料源候选识别。
  - Owner: Pinax
  - Lane: C
  - Depends on: 1.1
  - Scope: 在 metadata/organize service 的现有规划路径中识别 GitHub URL、`owner/repo` 标题和粗粒度 tags，生成 source-note 候选事实。
  - Acceptance: `go test ./internal/app -run 'TestDurableSourceCandidateDetection' -count=1` 通过。
  - Expected result: 对含 `https://github.com/iptv-org/iptv` 的普通 reference note，返回候选 source URL、建议 path、建议 tags，不写 Markdown。
  - Failure re-check: 若候选识别误报，优先收紧 GitHub URL parser，不引入网络请求验证。

- [x] 3.2 增加 metadata 建议但保持人工审阅。
  - Owner: Pinax
  - Lane: C
  - Depends on: 3.1
  - Scope: metadata plan 建议 `kind: source`、`source_url`、`last_checked_at`、`source_license`、`review_after` 和分层 tags。
  - Acceptance: `go test ./internal/app -run 'TestMetadataPlanSuggestsDurableSourceFields' -count=1` 通过。
  - Expected result: 默认只生成 plan；未带 `apply --yes` 时不写 frontmatter、index、events 或 Git 状态。
  - Failure re-check: 不允许 metadata plan 自动改正文或创建 related notes。

- [x] 3.3 增加 organize 路径和结构建议。
  - Owner: Pinax
  - Lane: C
  - Depends on: 3.1
  - Scope: organize plan 对 GitHub source note 建议移动到 `sources/github/<slug>.md`，并对缺少 `Use decision`、`Risk and boundary`、`Verification`、`Related notes` 的正文生成 manual review items。
  - Acceptance: `go test ./cmd/pinax ./internal/app -run 'TestOrganizePlanSuggestsDurableSourceLayout' -count=1` 通过。
  - Expected result: 低风险 path/tag/metadata 建议可进入 plan；正文拆分和判断补全只作为 manual review，不自动 apply。
  - Failure re-check: 如果 apply 测试覆盖低风险操作，必须先创建 snapshot 或返回 snapshot-required 行为，不能绕过 proof loop。

## 4. Index, Query, and Graph Support

- [x] 4.1 确认可选 source metadata 不破坏索引。
  - Owner: Pinax
  - Lane: D
  - Depends on: 2.1
  - Scope: 为含 `source_url`、`last_checked_at`、`source_license`、`review_after` 的 note 增加索引/搜索回归测试。
  - Acceptance: `go test ./internal/index ./cmd/pinax -run 'TestSourceMetadata|TestSearchSourceNotes' -count=1` 通过。
  - Expected result: note list/search 继续能按 kind/status/tags 查到 source note；新增字段可保留，不导致解析失败。
  - Failure re-check: 若需要投影新增字段，只能以 additive 方式加入，不能改变既有 notes table 字段含义。

- [x] 4.2 固定关系检查工作流。
  - Owner: Pinax
  - Lane: D
  - Depends on: 3.3
  - Scope: 增加 e2e/testscript 或命令级测试，创建 source note 和 related concept note 后运行 `note links`、`note backlinks`、`note orphans`。
  - Acceptance: `go test ./cmd/pinax -run 'TestDurableSourceGraphChecks' -count=1` 通过。
  - Expected result: 有内部链接的 source note 不被误报为 fully orphan；缺少 related links 时 organize plan 只提示 manual review。
  - Failure re-check: 不通过硬编码路径跳过 link parser；修复 Markdown link 解析或 fixture。

## 5. Documentation, Skill Handoff, and Gates

- [x] 5.1 新增 Pinax 长期资料源笔记文档。
  - Owner: Pinax
  - Lane: E
  - Depends on: 2.2, 3.3
  - Scope: 新增或更新 Pinax docs，说明长期 source note 的存放、tags、字段、拆分原则、Pinax 命令流程和 skill 边界。
  - Acceptance: 文档包含 `iptv-org/iptv` 示例、推荐命令、风险边界和不自动联网声明。
  - Expected result: 用户能按文档把临时 GitHub repo 笔记整理成长期资料源卡片。
  - Failure re-check: 不把 Pinax 产品文档复制到根 `docs/**`；只更新 `cli/pinax/docs/**`。

- [x] 5.2 为后续 skill 写最小 handoff，不实现 skill。
  - Owner: Pinax + root skill layer later
  - Lane: E
  - Depends on: 5.1
  - Scope: 在设计文档或命令文档中记录未来 `long-term-note-review` skill 的边界：读取、审稿、调用 Pinax；不得直接写 `.pinax/` 或定义独立存储规则。
  - Acceptance: OpenSpec 和 docs 明确 skill 是薄工作流层，Pinax 是长期存储和执行层。
  - Expected result: 后续创建 skill 时可以引用本规格，不重复定义产品合同。
  - Failure re-check: 若后续实现 skill，必须走 `.skills/yeisme/` source + profile sync，不把 runtime copy 当 source。

- [x] 5.3 跑完整质量门禁。
  - Owner: Pinax
  - Lane: sequential
  - Depends on: all prior tasks
  - Scope: 运行 OpenSpec、Go 测试和子项目质量门禁。
  - Acceptance: `openspec validate pinax-durable-source-notes --strict`、`openspec validate --all --strict`、`task check` 通过。
  - Expected result: 无新增破坏性合同；若环境缺少 `task`，运行 `go test ./...` 和 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
  - Failure re-check: 若门禁失败，先定位最小失败包，不跳过合同测试、不删除 fixture sentinel。

## Compatibility Record

- CLI commands: 复用现有命令，新增模板名和建议类型，additive。
- CLI output: 只新增可选 facts/data 字段，additive；不改变 envelope 顶层字段。
- Note frontmatter: 新增可选字段，additive；旧 notes 不需要迁移。
- Tags: 新增推荐词表，additive；不改变 tag 校验。
- Database/index: 只允许可重建 projection 的 additive 扩展。
- Rollback: 隐藏模板和 organize 建议即可停止新行为；已创建 Markdown notes 保持可读。

## Implementation Evidence

- `go test ./internal/app -run 'TestBuiltInNoteTemplatesCatalogMetadata|TestBuiltInNoteTemplateMetadataAppliesToCreateNote|TestDurableSourceCandidateDetection|TestMetadataPlanSuggestsDurableSourceFields|TestOrganizePlanSuggestsDurableSourceLayout' -count=1`: passed.
- `go test ./cmd/pinax -run 'TestSourceTemplate|TestDurableSource' -count=1`: passed.
- `go test ./internal/index ./cmd/pinax -run 'TestSourceMetadata|TestSearchSourceNotes' -count=1`: passed.
- `go test ./cmd/pinax ./internal/app -run 'TestSourceTemplate|TestDurableSource|TestBuiltInNoteTemplatesCatalogMetadata|TestBuiltInNoteTemplateMetadataAppliesToCreateNote|TestDurableSourceCandidateDetection|TestMetadataPlanSuggestsDurableSourceFields|TestOrganizePlanSuggestsDurableSourceLayout' -count=1`: passed.
- `openspec validate pinax-durable-source-notes --strict` and `openspec validate --all --strict`: passed.
- `task check`: passed.
