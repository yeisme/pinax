## Context

Pinax 当前已经有 Markdown note、record ledger、本地 SQLite/GORM index、附件引用、Git snapshot 保护和 version-aware search 的部分规格。但用户可见命令仍暴露 `git` 口径，实际实现也存在通过 Git evidence 辅助 record event 的路径。随着 Pinax 要管理图片、音频、视频、PDF 和其他二进制资产，继续把版本能力绑定到 Git 命令或本地二进制会让产品边界变窄，也会让测试依赖真实环境。

新的目标是把 Pinax 设计为版本化 vault asset manager：Markdown body 是用户内容真源，record ledger 和 asset manifest 是 CLI-authored 机器事实，index 是可重建 projection，version backend 是可替换证据来源。Pinax 应优先 pure Go；系统 `git`、`ffmpeg`、`exiftool` 等本地二进制不得成为核心路径依赖。

## Goals / Non-Goals

**Goals:**

- 将用户可见命令从 `git` 调整为 `version`，Git 只作为可选 backend 类型。
- 定义 pure Go `VersionBackend`，支持 local/none backend，并为后续 pure Go Git adapter 留出能力边界。
- 增加 `asset` 命令族，管理多媒体和二进制资产的 add/list/show/link/move/remove/verify。
- 扩展 index，使 note、asset、vault file 和 version evidence 能通过统一 lookup/resolver 查询。
- 让 `record adopt <query>`、`metadata plan <query>`、`note show <query>`、`asset show <query>` 共享 resolver 和 ambiguous candidate 输出。
- 保持所有机器输出字段英文稳定，人类输出中文；不泄漏二进制 payload、raw diff、provider secret 或本地绝对路径以外的敏感内容。

**Non-Goals:**

- 不实现长期 daemon、实时 watcher 或后台同步。
- 不把 SQLite index、asset manifest 或 version snapshot 变成 Markdown 正文真源。
- 不在第一阶段实现视频转码、音频波形、PDF 全文抽取或缩略图生成。
- 不默认依赖系统 `git`、`ffmpeg`、`exiftool`、ImageMagick 或网络服务。
- 不在本变更中实现远端云同步协议；sync 只消费这些本地事实。

## Decisions

1. 用户命令使用 `version`，不再直接维护可见 `git` 子命令。
   - 原因：Pinax 管理的是 vault 版本证据，不是 Git porcelain。Git 应只是 backend。
   - 备选：继续扩展 `pinax git status/diff/log`。该方案短期直观，但会把产品绑定到 Git，且不适合 local ledger 或未来非 Git backend。

2. 保留隐藏兼容 alias `pinax git snapshot`。
   - 原因：现有 repair/organize hint 和测试可能仍使用 snapshot 保护。兼容 alias 能降低迁移风险。
   - 备选：立即删除 `git` 命令。该方案更干净，但破坏已有脚本和用户习惯。

3. 默认实现 local/none VersionBackend，pure Go Git adapter 分阶段接入。
   - 原因：local/none 能马上满足 snapshot evidence、content hash、changed path baseline；Git 历史读取更复杂，应单独验证。
   - 备选：直接引入 go-git 并实现完整 Git backend。风险是范围过大，且历史 blob、submodule、LFS 和大文件行为需要更多测试。

4. 多媒体资产通过 asset manifest + content evidence 管理，不把二进制写进 record event 或 stdout。
   - 原因：事件日志和输出合同必须轻量、可审计、可脱敏；二进制 payload 只保存在 vault 文件或可选 CAS object refs 中。
   - 备选：把资产内容纳入 `.pinax/records/events.jsonl`。该方案不可维护，也会污染 Git/fixtures/stdout。

5. index 扩展为 vault object lookup projection，但普通 search 仍默认只返回 registered notes。
   - 原因：Obsidian-like 全局查找需要 unmanaged Markdown 和 asset 候选；普通 note search 仍应保持 Pinax note 语义稳定。
   - 备选：让 `search` 默认搜索所有文件。该方案会破坏已有 `unmanaged Markdown 不进普通结果` 的合同。

6. 写入类 resolver 必须唯一强匹配。
   - 原因：`record adopt yeisme`、`asset remove diagram`、`version restore note` 这类命令一旦猜错会改变 vault。只读命令可以返回 ranked candidates，写入命令必须失败并列候选。
   - 备选：按最高分自动选择。该方案体验快，但数据风险不可接受。

7. 附件管理复用 `asset`、`note attach` 和 index，不新增第二套附件数据库。
   - 原因：Obsidian 的价值在于 Markdown 引用和文件夹约定可迁移，不是隐藏附件库。Pinax 应把附件视为被 note 引用的 vault asset，通过 `.pinax/index.sqlite` 投影 `asset_links`、反链、孤儿和缺失引用。
   - 备选：新增 `.pinax/attachments.json` 作为附件真源。该方案查询方便，但会让正文引用、文件系统和 metadata 三者容易分叉。

## Obsidian-like Attachment Management

### Product Model

附件不是单独的内容类型，而是 `asset` 在笔记工作流中的角色：

- 文件真源：vault 内的实际文件，例如 `attachments/note_abc/diagram.png` 或 `notes/project/assets/diagram.png`。
- 引用真源：Markdown 正文里的 `![label](relative/path.png)`、`[file](relative/path.pdf)`、`![[diagram.png]]`、`[[diagram.png]]`。
- 机器投影：index 中的 `assets`、`asset_links`、`vault_files` 和 resolver candidates，可重建，不是真源。
- CLI-authored metadata：可选 asset manifest、event evidence、verify receipt、move/remove plan，用于审计和修复，不保存二进制 payload。

默认行为贴近 Obsidian 用户预期：文件在 vault 内，普通 Markdown 编辑器可读，链接使用 vault 相对或 note 相对路径，移动/重命名附件时先生成可审查 plan。

### Attachment Placement Policy

当前实现已经把 `note attach` 放到 `attachments/<note-id>/<filename>`。MVP 继续把它作为默认策略，避免破坏现有 vault；同时把策略显式化，后续由 config/service 管理，而不是在 helper 里硬编码。

| Policy | Output path | Use case |
| --- | --- | --- |
| `per-note` default | `attachments/<note-id>/<safe-filename>` | 当前兼容路径；移动 note 不需要移动附件，反链靠 index 维护 |
| `vault-folder` | `attachments/<safe-filename>` | Obsidian 常见全局 attachment folder，适合共享图片/PDF |
| `note-folder` | `<note-dir>/assets/<safe-filename>` | 笔记和附件放一起，适合导出单目录材料 |
| `by-type` later | `attachments/images|docs|audio|video/<safe-filename>` | 后续按媒体类型归档，MVP 不默认启用 |

命令面：

```bash
pinax note attach "认证方案" ./diagram.png --vault ./my-notes --json
pinax note attach "认证方案" ./diagram.png --placement note-folder --embed --vault ./my-notes --json
pinax asset add ./diagram.png --as-attachment-for "认证方案" --placement per-note --vault ./my-notes --json
pinax asset list --scope attachments --vault ./my-notes --json
pinax asset backlinks diagram.png --vault ./my-notes --json
pinax asset orphans --vault ./my-notes --json
pinax asset verify --scope attachments --vault ./my-notes --json
```

规则：

- `note attach` 是主工作流：复制或移动外部文件到 vault，并可选择追加 Markdown 引用。
- `asset add --as-attachment-for <note>` 是同一能力的 root asset 入口，便于 agent 和批量导入复用。
- `--placement` 覆盖当前命令的落盘策略；未传时读取 vault 配置，配置缺失时使用 `per-note`。
- `--embed` 对图片默认生成 `![label](rel/path)`；非图片默认生成 `[label](rel/path)`，除非用户显式要求 embed/wiki 格式。
- `--link-style markdown|wiki|auto` 控制写入引用：Markdown 是默认，wiki 生成 `![[diagram.png]]` 或 `[[diagram.pdf]]`，auto 根据 vault 配置选择。
- `--mode copy|move|register` 控制外部源文件：copy 默认；move 需要 `--yes` 或 snapshot guard；register 只允许源文件已在 vault 内。
- filename 冲突默认追加 `-2`、`-3`；`--rename <name>` 可显式命名；`--overwrite` 第一阶段不支持，避免覆盖用户文件。
- 文件名 sanitization 只改危险字符、路径分隔符和控制字符，不做过度 slug 化；receipt 保留原始文件名。

### Reference Parsing And Index Reuse

附件引用解析应从当前 `noteAttachmentsFromBody` 升级为共享 parser，并接入 index rebuild/refresh：

- 支持 Markdown image/link：`![alt](../assets/a.png)`、`[PDF](attachments/a.pdf)`。
- 支持 Obsidian wiki embed/link：`![[a.png]]`、`![[folder/a.png|200]]`、`[[a.pdf]]`。
- 支持 URL decode 和 angle target：`[x](<assets/My File.pdf>)`。
- 忽略外部 URL、`mailto:`、纯 heading、data URI 和非 vault path。
- `.md` 仍走 note link graph；非 Markdown、本地可解析文件进入 `asset_links`。

index 是查询主路径：

- `assets` 表保存 vault-relative path、filename、stem、extension、media type、size、sha256、managed status、mtime、missing/changed facts。
- `asset_links` 表保存 source note path/id、raw reference、resolved asset path/id、link style、embed/link、line number、status。
- `vault_files` 表保存未被 manifest 管理但在 vault 内出现的文件，供 `asset orphans`、`asset adopt` 和 lookup 使用。
- `note attachments`、`asset backlinks`、`asset orphans`、`search --has-attachment` 优先读取 fresh index；index stale/missing 时可降级 scan，但输出必须给 `pinax index refresh --vault <vault>` action。

### Commands And Workflows

保留现有命令，增强语义：

```bash
pinax note attach "认证方案" ./diagram.png --vault ./my-notes --json
pinax note attachments "认证方案" --vault ./my-notes --json
```

新增或强化 root asset 命令：

```bash
pinax asset list --scope attachments --vault ./my-notes --json
pinax asset show diagram.png --vault ./my-notes --json
pinax asset backlinks diagram.png --vault ./my-notes --json
pinax asset orphans --vault ./my-notes --json
pinax asset missing --vault ./my-notes --json
pinax asset move diagram.png attachments/archive/diagram.png --plan --vault ./my-notes --json
pinax asset remove diagram.png --plan --vault ./my-notes --json
pinax asset verify --scope attachments --vault ./my-notes --json
pinax asset repair --plan --vault ./my-notes --json
```

写入边界：

- `asset move/remove/repair` 默认只生成 plan；真正写 Markdown 或删除/移动文件必须 `--yes`，并通过 version snapshot guard。
- `note attach` 是低风险写入，但仍必须通过 service 完成 copy + Markdown append + index event；如果源文件在 vault 外且 `--mode move`，需要 `--yes`。
- 修改引用时只 patch 精确匹配的 raw reference，不全篇格式化 Markdown。
- 多个 note 引用同一附件时，remove plan 默认拒绝删除，除非用户传 `--unlink` 或选择只删除某一条引用。

### Output, Actions, And Completion

所有附件命令继续走 projection：

- default human：中文摘要，包含 attachment path、linked notes、missing/orphan count、推荐下一步。
- `--json`：单一 envelope，`data.assets` / `data.links` / `data.plan` 承载详情。
- `--agent`：低 token facts，例如 `fact.asset.path=...`、`fact.links=3`、`action.primary=pinax asset backlinks ...`。
- 错误和 warning 必须有真实可运行 action：缺失附件推荐 `pinax asset missing --vault <vault> --json` 或 `pinax asset repair --plan --vault <vault> --json`；index stale 推荐 `pinax index refresh --vault <vault> --json`。
- 输出不得包含二进制 payload、base64、raw file bytes、外部绝对路径泄漏；外部 source path 按既有 redaction 策略显示或缩短。

### Path Presentation Modes

附件真实身份使用稳定的 vault-relative canonical path，例如 `attachments/note_abc/diagram.png`。用户查看、复制引用、写 Markdown 和脚本消费时需要不同展示形式，因此 path presentation 必须是输出层能力，不改变存储路径和 index 主键。

命令面：

```bash
pinax note attachments "认证方案" --path-style note-relative --vault ./my-notes
pinax note attachments "认证方案" --path-style absolute --vault ./my-notes --json
pinax asset show diagram.png --path-style markdown --context-note "认证方案" --vault ./my-notes
pinax asset backlinks diagram.png --path-style wiki --vault ./my-notes --json
pinax index lookup diagram --kind asset --path-style vault-relative --vault ./my-notes --json
```

支持的 `--path-style`：

| Style | Example | Notes |
| --- | --- | --- |
| `vault-relative` default | `attachments/note_abc/diagram.png` | canonical display；JSON/agent 默认使用这个字段 |
| `note-relative` | `../../attachments/note_abc/diagram.png` | 需要 note context；适合插入当前 note 的 Markdown link |
| `absolute` | `/home/me/notes/attachments/note_abc/diagram.png` | 只在显式请求时输出；默认不进入 human/json/agent |
| `markdown` | `![diagram.png](../../attachments/note_abc/diagram.png)` | 根据 media type 生成 image 或 normal link；需要 note context 时用 note-relative target |
| `wiki` | `![[attachments/note_abc/diagram.png]]` | Obsidian-style display；如果 basename 唯一，可按后续 config 缩短为 `![[diagram.png]]` |

字段合同：

- `data.assets[].path` 始终是 vault-relative canonical path。
- `data.assets[].display_path` 是按 `--path-style` 生成的展示值。
- `data.assets[].paths` 可包含 `vault_relative`、`note_relative`、`absolute`、`markdown`、`wiki`，但 `absolute` 只有显式 `--path-style absolute` 或 `--include-paths absolute` 时出现。
- `--agent` 默认输出 `fact.asset.path=<vault-relative>`；显式 path style 时可额外输出 `fact.asset.display_path=...`。
- default human 输出尊重 `--path-style`，但仍应在详情中保留 canonical path 或提供 `pinax asset show <ref> --path-style vault-relative --vault <vault>` next action。

上下文规则：

- `note attachments <note>` 天然有 note context，可以生成 `note-relative` 和 `markdown`。
- `asset show/backlinks/list/orphans/missing` 如果需要 `note-relative` 或 `markdown`，用户必须传 `--context-note <note>`；若缺失 context，返回 `path_context_required`，并给出可执行命令。
- `asset backlinks` 的每条 backlink 可以用 source note 作为 context 生成 per-link `note_relative` 和 `markdown`。
- `absolute` path 必须经过 vault boundary 校验，并只指向 vault 内文件；不得输出外部 source absolute path，除非该 source 是当前命令的显式输入且已脱敏。
- `wiki` 展示要避免 basename 歧义：如果多个 asset basename 相同，默认用 vault-relative wiki target；只有唯一时才可输出短 basename。

路径展示不应触发任何写入、hash、index rebuild 或 provider 调用。它只消费 resolver/index/manifest 已有 facts 和当前 vault root。

### Attachment-aware Preview And Rendered Note View

`note show --view rendered` 不能只把附件显示成一个链接。对用户来说，Obsidian-like 预览的价值是“这一篇笔记和它引用的可读附件可以合成一个阅读视图”。MVP 不在终端里渲染图片、音视频或 PDF 版式；先支持 Markdown/text 类内容内联，其它附件显示为可读占位和后续命令。

命令面：

```bash
pinax note show "认证方案" --view rendered --embed-attachments markdown --vault ./my-notes
pinax note show "认证方案" --view rendered --embed-attachments markdown,text --max-embed-depth 1 --vault ./my-notes --json
pinax note preview "认证方案" --embed-attachments markdown --vault ./my-notes
pinax asset preview spec.md --as markdown --context-note "认证方案" --vault ./my-notes
```

规则：

- `note show --view source` 只显示原文，不内联附件、不执行 SQL、不刷新 index。
- `note show --view rendered` 继续执行现有 `pinax-sql` rendered view；新增 `--embed-attachments` 控制是否把可读附件内联。
- `note preview` 是 `note show --view rendered` 的人类友好别名，默认只读，不写 `.pinax/`、Markdown、Git、provider 或 render run。
- `asset preview <asset>` 只预览单个 asset；对 Markdown/text 输出正文，对图片/音频/视频/PDF 输出 metadata placeholder 和打开/导出 next action。
- `--embed-attachments none|markdown|text|all-readable`：默认 `none`，避免突然改变现有输出；`markdown` 只内联 Markdown note/embed 或 `.md` vault 文件；`text` 内联 `.txt`、`.log`、`.csv` 等 bounded text；`all-readable` 是 markdown+text。
- `--max-embed-depth` 默认 1，防止嵌套 embed 展开成整库；深度超过后输出 placeholder。
- 每个内联块必须带来源 heading 和边界 marker，例如 `## 附件：spec.md`，并在 JSON data 中记录 `embedded_assets`。
- 循环引用必须检测并停止，输出 `attachment_embed_cycle` warning，不失败整个预览。
- 单个附件和总预览都有大小上限，例如 `--max-embed-bytes` 和 `--max-preview-bytes`；超过后截断并给 next action。
- 只有 vault 内文件能被内联；外部 URL、unsafe path、缺失附件和二进制 payload 不内联。

渲染输出形态：

```markdown
# 认证方案

正文内容...

## 附件：spec.md

> 来源：attachments/note_abc/spec.md

附件 Markdown 内容...

## 附件：diagram.png

> 图片附件：attachments/note_abc/diagram.png
> 终端预览暂不渲染图片。下一步：pinax asset show diagram.png --vault ./my-notes --json
```

数据合同：

- `data.body` 是统一渲染后的 Markdown preview。
- `data.embedded_assets[]` 记录 `path`、`display_path`、`media_type`、`render_mode`、`bytes`、`truncated`、`source_note_path`、`status`。
- `facts.embedded_assets`、`facts.skipped_assets`、`facts.truncated_assets`、`facts.embed_depth` 用于 agent/summary。
- `--json` 不输出 ANSI；default human 可以继续走 Markdown renderer。
- 不保存 render run，除非用户显式传现有 `--save-run` 或后续 `note render` 命令；预览默认无副作用。

实现应复用 index 的 `asset_links` 和 note link graph：

- Markdown note embed，例如 `![[project/spec.md]]`，优先通过 note resolver 读取 registered/adoptable Markdown，然后内联正文。
- Markdown/text asset embed，通过 `asset_links` 或 resolver 读取 vault-relative file。
- 普通非 embed link，例如 `[Spec](spec.md)`，默认保留为链接；只有 `--embed-attachments linked` 后续扩展才内联普通 link，MVP 不做，避免把所有参考链接都展开。
- 图片 embed `![[diagram.png]]` 或 `![x](diagram.png)` 在终端中只输出 placeholder；不生成 ANSI 图片、Sixel、iTerm inline image 或 base64。

附件补全复用 resolver/index：

- `pinax asset show <TAB>`、`asset backlinks <TAB>`、`asset move <TAB>`、`asset remove <TAB>` 补 asset filename/path/stem，描述包含 media type、linked note count、missing/orphan 状态。
- `pinax note attach <note> <TAB>` 的第二参数保留文件补全，因为源文件可能在 vault 外。
- `pinax note attachments <note>` 只补 note ref，不补附件名。
- completion 只读 index/manifest/scan fallback，不 hash 大文件、不刷新 index、不写任何资产。

## Risks / Trade-offs

- `version` 迁移导致旧帮助和 next action 口径混乱。→ 所有新输出推荐 `version`，`git snapshot` 只保留 hidden alias，并增加测试防止 help 暴露旧路径。
- pure Go Git backend 覆盖不完整。→ 第一阶段只要求 local/none；Git backend 能力通过 `version backends` 明确 reported capabilities。
- 多媒体 metadata 不够丰富。→ 第一阶段只采集 MIME、size、sha256、图片尺寸等 pure Go 可得事实；复杂媒体解析作为 optional provider 延后。
- asset manifest 和 index projection 不一致。→ `asset verify`、`index doctor`、`repair plan` 暴露一致性问题，修复必须走计划和审批。
- 大文件 hash 成本高。→ add/verify/refresh 支持 bounded worker、streaming hash 和 progress facts；不一次性读取大文件到内存。
- Obsidian wiki embed 和 Markdown link 并存会增加解析歧义。→ index 保留 raw reference、link style 和 line number；写入类 repair 只改精确匹配引用，无法唯一定位时进入 manual review。
- note-folder 策略会让移动 note 时附件位置是否跟随变复杂。→ MVP 默认 per-note，不自动跟随移动；如果用户选择 note-folder，`note move` 只生成附件 move/link rewrite plan，不隐式搬二进制。

## Migration Plan

1. 新建 `version` 命令树和 local/none backend，隐藏 `git snapshot` alias，更新 snapshot next action。
2. 增加 asset manifest/domain/service，先实现 add/list/show/verify 的 readonly/低风险路径。
3. 扩展 index schema 到 vault_files/assets/asset_links，新增 lookup/explain、附件引用解析和 resolver candidate model。
4. 将 `record adopt <query>` 接入 adoptable resolver，保留无参数全库 adopt plan。
5. 将 `note show/read/links/backlinks` 和 `metadata plan <query>` 接入 registered resolver。
6. 增加 `asset link/move/remove --plan` 和 `version restore --plan`，真正 apply 延后到 snapshot/approval 测试稳定后。
7. 更新 docs/help/OpenSpec specs，运行聚焦测试、testscript、`openspec validate --all` 和 `task check`。

## Open Questions

- Pure Go Git backend 是否使用 `go-git`，还是先自研最小 read-only adapter？建议实现前单独 spike。
- Asset CAS 是否第一阶段复制对象到 `.pinax/assets/objects`，还是只记录 vault 文件 sha256？建议先只记录 refs，避免重复大文件。
- `find` 是否作为 root 命令第一阶段实现，还是先通过 `index lookup` 暴露？建议先做 `index lookup` 和 resolver，再新增用户友好的 `find`。
