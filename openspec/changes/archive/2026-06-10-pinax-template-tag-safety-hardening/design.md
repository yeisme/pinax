## Context

Pinax 的模板、tag 和 record ledger 已经处在同一条用户路径上：用户通过 `template recommend/list/inspect` 选择模板，用 `note add/new --template` 创建 Markdown note，再通过 `note tag`、search/query、index 和 ledger 继续维护。当前实现存在几个边界不一致：tag 写入使用未转义 YAML inline list，设计稿模板只产生 warning 但仍能被执行，query-backed `template preview` 会通过 lazy index rebuild 写 `.pinax/index.sqlite`，note template metadata 的 `output.path_pattern` 和 defaults 没有参与 note 创建。

本变更不引入外部 provider，也不改变 Pinax 的 local-first Markdown vault 真源模型。所有 structured assets 仍必须通过 CLI/application service 创建和修改。

## Goals / Non-Goals

**Goals:**

- 让 tag 写入成为一个统一、安全、可测试的 metadata mutation 边界。
- 保证 template design draft、template preview 和 note template metadata 的行为符合用户直觉和现有 spec。
- 让 `note tag` 的 ledger/index/output facts 与其它 note mutation 一致。
- 保持机器输出合同稳定，新增 error code/facts/actions 时只做兼容扩展。

**Non-Goals:**

- 不重写整个 frontmatter patcher 或引入完整 Markdown AST 编辑器。
- 不把模板引擎扩展为脚本运行时，也不允许文件、环境变量、网络或 shell 访问。
- 不自动迁移现有用户笔记中的历史 tag 格式；只保护后续 CLI-authored 写入。
- 不改变默认 note 正文作为用户内容真源的定位。

## Decisions

### 1. 统一 tag validator 和 YAML 写入边界

新增应用层 tag 规范化入口，例如 `normalizeTagsForWrite([]string) ([]string, error)`。所有会写 frontmatter tag 的路径必须使用它，包括 `CreateNote`、`TagNote`、import defaults、repair/organize tag patch、schema values 中复用 tag-like list 的入口。

默认 tag 字符集采用 conservative boring 方案：允许 Unicode 字母/数字、`_`、`-`、`/`，允许去掉前导 `#`；拒绝空值、换行、控制字符、逗号、方括号、花括号、冒号、引号和其它 YAML 结构字符。这样可以继续支持 `work/research` 和中文标签，同时阻断 YAML 注入。

frontmatter 输出短期继续复用现有 patcher，但 `tags` 必须只接收 validator 产物。后续如果引入 YAML encoder，也仍保留 validator，避免在 tag 维度出现不可预测值。

备选方案是完全依赖 YAML encoder 转义任意 tag 字符串。该方案兼容性更宽，但会让 tag 查询、inline tag、shell completion 和视觉展示混入很复杂的边界字符，当前不采用。

### 2. 模板可执行状态显式化

`templateengine.ParseDocument` 已能识别 `pinax.template_design.v1` 并生成 issue。应用层应在 preview/render/note-create 入口把这个 issue 升级为阻断性错误 `template_design_not_executable`。`template inspect` 和 `template validate` 继续允许读取设计稿，并给出 convert/publish action。

这比在 parser 中直接报错更合适，因为 inspect/validate 仍需要展示设计稿 metadata；执行路径才需要 fail closed。

### 3. Preview 不写 structured assets

模板执行拆成两个查询策略：

- preview/read-only：不允许 lazy index rebuild；缺失或 stale index 时返回 partial/failed projection，并给 `pinax index rebuild --vault ...` action。
- render/write-capable：允许现有 bounded query 执行策略；只有显式 render run 保存才写 render receipt。

实现上可在 `TemplateRequest` 或内部 render options 增加 read-only 标志，不把它暴露成复杂 UI。`PreviewTemplate` 使用 read-only，`RenderTemplate` 保持现有能力。

### 4. Note template metadata 在 service 层应用

`CreateNote` 在解析 body 前读取模板 metadata，形成 effective create request：

- `output.path_pattern` 生成默认目标路径或 prefix/slug，但显式 `--dir`、`--folder`、`--slug`、`--project` 优先。
- `defaults.kind`、`defaults.status`、必要时 `defaults.folder/project` 作为默认 frontmatter 字段，显式 CLI flags 优先。
- `example` 只用于 preview，不用于真实 note 创建，除非用户显式传入相同值。

路径 pattern 必须复用 template output path validator 和 note path safety validator；不能指向 `.pinax`、`.git`、attachments、dist、node_modules、vendor 或 vault 外路径。

### 5. Example context 先合并再构造 render context

`renderTemplateBody` 当前先构造 `renderCtx` 再 `applyTemplateExample`。应调整为先合并 request/example/defaults，再构造 `templateengine.Context`。显式 request 字段仍高于 example。

### 6. Note tag ledger/index facts 对齐

`note tag` 是 CLI-approved metadata operation，成功写 Markdown 后应追加 record metadata event 或 dedicated tag event。优先新增通用 `RecordEventNoteMetadataUpdated`，event evidence 记录前后 tags、content hash、version evidence，不记录未脱敏正文。

索引处理按现有低成本策略：如果能安全调用 incremental refresh，则返回 `index_updated=true` 和 affected facts；如果暂时不刷新，必须返回 `index_status=stale` 和 next action，不能让机器消费者误以为 projection 已更新。

## Risks / Trade-offs

- [Risk] 严格 tag 字符集可能拒绝少量现有用户偏好的标签字符。→ Mitigation：错误提示说明允许字符，保留中文、数字、字母、`_`、`-`、`/`，后续按真实需求放宽。
- [Risk] 阻断设计稿执行会改变已有依赖该行为的临时脚本。→ Mitigation：这是安全修复；错误 action 指向 convert/publish 或显式创建可执行模板。
- [Risk] preview 不再自动 lazy rebuild，用户首次预览 query template 会多一步。→ Mitigation：preview 必须可信只读；错误 projection 给出可复制的 `index rebuild` action。
- [Risk] record event kind 变更影响 replay。→ Mitigation：新增 event kind 保持向后兼容，registry replay 对未知/新 metadata event 做明确处理和测试。

## Migration Plan

1. 先添加失败回归测试，复现 tag YAML 注入、设计稿执行、preview 写 index、example context 忽略和 starter defaults 未应用。
2. 按 validator、模板执行 guard、preview read-only、metadata 应用、ledger/index facts 的依赖顺序实现。
3. 保持旧模板和旧 note 可读；只改变新写入和执行入口。
4. 如出现 record event replay 兼容问题，保留旧 event 处理路径，并让新 metadata event 可被忽略但不破坏 registry。

## Open Questions

- tag 字符集是否允许 `.`？当前建议先不允许，避免和部分 query/property 语义混淆；实现时可根据现有 fixture 决定。
- note template `output.path_pattern` 是否支持完整 Go template 表达式，还是只支持 `.Title` 等少量字段？建议先复用 v2 engine 但在渲染后再做 path validator。
- `note tag` 成功后是否必须同步刷新 index，还是允许返回 stale action？建议优先 incremental refresh，失败时 partial/stale。
