## 为什么

内置 `learning.video`、`learning.source` 和 `source.github` 正文都读取 `.Vars.url`，但 metadata 没有声明该变量。实际渲染会在缺少 URL 时失败，而 inspect、catalog、completion 与 agent projection 却无法提前说明必填项，形成模板正文与消费合同断层。

## 变更内容

- 为三个内置模板声明 required `url` 变量及用途说明。
- 扩展内置 note template 生成器，使其可确定性输出 `variables` metadata。
- 添加解析、required projection 与 completion 回归测试。
- 文档明确 `--var url=<value>` 要求。

## 能力归属

- 分类：`fit`
- canonical owner：Pinax 内置 note template catalog
- 与 film Prompt role/capability mapping 无关。

## 影响

该变更显式化既有 Go template 缺参失败语义，不改变模板名、路径、正文输出或 CLI 字段。
