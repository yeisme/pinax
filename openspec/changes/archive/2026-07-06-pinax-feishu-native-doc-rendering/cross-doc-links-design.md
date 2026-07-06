# 子文档引用（Cross-Doc Links）设计方案

## 背景

笔记之间通过 `[[wikilink]]` 和 `[label](relative/note.md)` 互相引用。发布到飞书原生 docx 时，这些引用当前变成：
- wikilink → 字面文本 `[[target]]`（飞书不识别）
- 相对路径链接 → 指向 `notes/other.md` 的死链（飞书端无法解析本地路径）

目标：发布 note A 时，若 A 引用的 note B 已经发布到同一飞书 target（lark-doc），则把 A 中的引用改写为 B 的飞书原生文档 URL，使飞书文档间形成可点击的跨文档链接（子文档引用）。

## 解析策略

复用已有 notelinks 包（与 `note links` / backlinks 同源），保证发布端解析与索引端一致：

1. `scanNotes(root)` 读取 vault 全部 notes
2. `notelinks.BuildResolverSnapshot(notes)` 构建确定性解析索引（note_id / path / title / alias）
3. `notelinks.ParseNoteLinks(body)` 提取 wikilink + markdown 相对链接
4. `notelinks.ResolveLinkTarget(sourceNote, rawLink, snap)` 解析每条引用 → `TargetNoteID`（确定性匹配；歧义则不解析）
5. `readPublishDocMapping(root, targetNoteID, lark-doc)` 查目标 note 的飞书 mapping
6. 若 mapping 有 `ExternalObject.URL`，把 raw link 文本改写为 `[label](feishu-docx-url)`

## 改写规则

| 原始 | 改写后（B 已发布） | B 未发布 |
| --- | --- | --- |
| `[[B]]` | `[B](https://…/docx/B_TOKEN)` | 保留原样 |
| `[[B\|alias]]` | `[alias](https://…/docx/B_TOKEN)` | 保留原样 |
| `[text](../b.md)` | `[text](https://…/docx/B_TOKEN)` | 保留原样 |

heading 锚点（`[[B#section]]`）首版丢弃（飞书 docx block-id 锚需要额外 fetch，后续增强）。

## 时机

在 `PublishDocPrepare` 解析后、构建 native plan 前，对 `pkg.BodyMarkdown` 做一次改写。改写后的正文同时进入 `pkg.BodyMarkdown`（送 `docs +create`）和 native plan（审计）。

**一致性权衡**：prepare 时刻捕获目标 note 的当前飞书 URL。若目标 note 之后重新发布得到新 URL，本 note 的链接会过时——需要重新 prepare + push 本 note。这是 local-first 发布的预期语义（不做反向回写）。

## 输出

prepare/push 输出新增 fact `cross_doc_links=N`（改写成功的跨文档链接数）。未解析的引用不产生 warning（它们可能是指向尚未发布的 note，属于正常状态，不是错误）。

## 飞书原生表示

首版用 markdown 链接（`docs +create --doc-format markdown` 原生支持）。未来增强可改用 XML `<cite type="doc" token="TOKEN">label</cite>`（飞书原生文档卡片引用），但需要切到 XML 格式或创建后 block 插入，复杂度更高，留作后续。

## 非目标

- 不反向回写：不在 note A 发布时修改 note B 的本地 Markdown。
- 不解析跨 vault 引用。
- 不自动检测循环发布依赖（用户控制发布顺序）。
