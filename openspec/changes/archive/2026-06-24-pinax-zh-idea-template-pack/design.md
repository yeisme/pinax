# pinax-zh-idea-template-pack Design

## 方案

沿用内置 `note_template` 和 `index_template` 机制。模板名、metadata key、`kind`、`status`、`tags` 保持英文稳定字段；正文标题使用中文，降低中文记录成本。

`idea.*` 模板默认写入 `ideas/**`，设置 `kind: idea`、`status: parked`，正文只包含触发点、观察点、线索、问题和相关笔记，不包含 checkbox 或行动项。完整内容笔记模板默认 `status: active`，按阅读、媒体、游戏和写作场景归档。

## 模板包

- Idea seed：`idea.research_seed`、`idea.drama_watch`、`idea.anime_watch`、`idea.game_explore`、`idea.paper_read`、`idea.novel_read`、`idea.novel_write`、`idea.video_note`。
- 内容笔记：`media.drama`、`media.anime`、`game.playlog`、`reading.paper`、`reading.novel`、`writing.novel`。
- 兼容改造：`learning.video`、`learning.book`、`research.topic` 保留模板名，改为中文详细结构并补默认 tags。
- Index：`index.ideas` 查询 `kind='idea' AND status='parked'`。

## 验证

- 单元测试覆盖模板 catalog metadata、默认 metadata 应用、`index.ideas` inspect。
- CLI 测试覆盖中文 intent 推荐。
- OpenSpec 验证覆盖新增 delta spec。
