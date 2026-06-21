# 长期资料源笔记

长期资料源笔记用于保存一个外部来源的可复用判断，而不是保存一次阅读摘要。GitHub 仓库、标准文档、论文、官网页面、发行说明、API 文档都适合做成 source note；一次性的想法、任务、摘录和草稿仍放在普通 note、inbox 或 project note 中。

## 推荐存放

GitHub 仓库默认放在 `sources/github/<owner>-<repo>.md`。例如 `https://github.com/iptv-org/iptv` 对应：

```text
sources/github/iptv-org-iptv.md
```

创建命令：

```bash
pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --vault ./my-notes --json
```

`source.github` 是本地内置模板，不联网、不调用 GitHub API，也不会把仓库内容抓进 vault。它只生成一个可审阅的 Markdown 卡片，并使用普通 Pinax note 创建流程写入 vault。

## 推荐 metadata 和 tags

模板默认写入：

```yaml
kind: source
status: active
tags: [source/github, reference/source]
```

后续人工审阅时可以补充这些可选字段：

```yaml
source_url: https://github.com/iptv-org/iptv
last_checked_at: 2026-06-20
source_license: unknown
review_after: 2026-09-20
```

tags 的粒度建议保持两层：`source/github` 表示来源类型，`reference/source` 表示它是长期引用源。具体主题不要堆进 source note 本身，优先通过 `[[Related Concept]]` 链接到概念、项目或决策 note。

## 应该如何分解

source note 只回答这些长期问题：

- 这个来源是什么，canonical URL 是什么。
- 它能被用来支持哪些判断，不能支持哪些判断。
- 使用它有什么风险，例如维护状态、license、数据时效、范围边界。
- 哪些本地概念、项目、决策或实现 note 依赖它。
- 下次什么时候复查，以及复查什么。

不要把完整 README、issue 列表、安装教程或大段摘录塞进 source note。需要沉淀的主题应拆成独立 concept/project/decision note，并从 source note 的 `Related notes` 链接过去。

## Pinax 能做什么

Pinax 负责长期存储和受控执行：

```bash
pinax metadata plan "iptv-org/iptv" --vault ./my-notes --json
pinax organize plan --vault ./my-notes --json
pinax note links sources/github/iptv-org-iptv.md --vault ./my-notes --json
pinax note backlinks "M3U Playlist Format" --vault ./my-notes --json
```

`metadata plan` 会对已有 GitHub URL 或 `owner/repo` 标题生成人工审阅建议，例如 `kind=source`、`source_url`、`last_checked_at`、`source_license`、`review_after` 和推荐 tags。默认只是 plan，不写 frontmatter、index、events 或 Git 状态。

`organize plan` 会建议把 GitHub 资料源移动到 `sources/github/<slug>.md`，并在正文缺少 `Use decision`、`Risk and boundary`、`Verification`、`Related notes` 时生成 `manual_review` 项。正文拆分、判断补全和风险解释都需要人或上层 workflow 审阅，不会自动 apply。

`note links`、`note backlinks` 和 `note orphans` 用来检查 source note 是否真正接入知识图谱。source note 没有相关链接时，Pinax 只提示 review，不会替你创建概念 note。

## Skill 边界

未来可以做一个薄的 `long-term-note-review` skill：它读取临时笔记或 URL，上下文审稿，调用 Pinax 命令创建或整理 source note，并把需要人工判断的部分留在 plan/review 中。

这个 skill 不应该定义独立存储规则，不应该手写 `.pinax/` 结构化资产，不应该绕过 `pinax note add`、`metadata plan`、`organize plan`、snapshot/apply 等产品合同。Pinax 是长期存储和执行层；skill 只是工作流层。
