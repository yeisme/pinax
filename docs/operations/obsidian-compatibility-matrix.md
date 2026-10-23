# Obsidian 兼容矩阵

本文是 Pinax 从 Preview 毕业到 Supported 的验收记录。真实 vault 必须先复制到临时目录；验证不得修改用户原始 vault。

## 验证命令

对每个复制后的 vault 执行：

```bash
pinax vault validate --vault <copied-vault> --json
pinax index refresh --vault <copied-vault> --json
pinax search "<known-term>" --vault <copied-vault> --json
pinax note links "<known-note>" --vault <copied-vault> --json
pinax note backlinks "<known-note>" --vault <copied-vault> --json
```

验证完成后，检查原始 vault 与复制前的文件树和 Markdown 内容摘要一致；`.pinax/` 投影可以在复制目录中重建。

## 矩阵

| Vault | 结构覆盖 | 规模 | 结果 | Evidence | 已知差异 |
| --- | --- | ---: | --- | --- | --- |
| 研究库 A | wikilink、alias、backlink、附件 | 待填写 | `pending` | 待真实运行 | 待填写 |
| 项目库 B | properties、Dataview、附件、broken/ambiguous links | 待填写 | `pending` | 待真实运行 | 待填写 |
| 日记库 C | daily managed block、模板、`.obsidian/` | 待填写 | `pending` | 待真实运行 | 待填写 |

## 通过标准

- `[[wikilink]]` 和 `[[Title|Alias]]` 解析正确。
- YAML frontmatter/properties 保持可读，未被重写。
- `pinax:managed` daily block 不被破坏。
- Dataview query block 原样保留。
- `.obsidian/` 不进入笔记索引。
- attachments、broken links、ambiguous links 和 orphan notes 分类稳定。
- 三个真实 vault 均有命令、运行时间、脱敏 evidence 和 Pass/Partial/Known-gap 结论。

在三份真实 vault 证据完成前，README 的 Obsidian 状态保持 `Preview`，不得宣称已毕业。
