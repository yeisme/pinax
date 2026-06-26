# pinax-zh-idea-template-pack Proposal

## 背景

用户经常需要把“日后可能调查”的想法存成笔记，例如某篇小说如何写成、某部动漫有什么设定值得观察、某篇论文以后要读。现有 `note add`、template、index 和 search 已能承载这个需求，不需要新增 `idea` 子命令。

## 目标

- 新增中文 `idea.*` 模板，用 `kind: idea` 和 `status: parked` 停放想法种子。
- 新增常见中文内容笔记模板，覆盖看剧、动漫、游戏、论文阅读、小说阅读、小说创作和视频笔记。
- 改造现有 `learning.video`、`learning.book`、`research.topic` 为中文详细结构，保留模板名兼容现有入口。
- 新增 `index.ideas` 汇总 parked idea notes。

## 非目标

- 不新增 `pinax idea` 子命令。
- 不把 idea 模板写成 todo、project item 或 board item。
- 不引入新的模板引擎、远端 provider 或 LLM 推荐。
