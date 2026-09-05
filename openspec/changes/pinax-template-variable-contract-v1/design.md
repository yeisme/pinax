## 设计

`builtInNoteTemplate` 增加可选 variable metadata 参数。未提供变量的既有调用保持原样；需要变量的模板显式传入 map，生成器按 key 排序输出，确保内置模板正文与 snapshot 稳定。

首批只登记正文已经引用的三个 URL 变量：

| Template | Variable | Required | 说明 |
| --- | --- | --- | --- |
| `learning.video` | `url` | true | 原视频或课程 URL |
| `learning.source` | `url` | true | 学习资料来源 URL |
| `source.github` | `url` | true | GitHub repository URL |

选择 `variables` 而不是同时手写 `variable_schema`，复用现有 `templateWorkflowMetadata` 的兼容归一化。Pinax 的 render engine 当前已对缺失 `.Vars.url` 返回 `template_variable_missing`，因此 required 标记只让 inspect/completion 提前暴露既有约束。

## 回滚

移除三个变量声明和生成器可选参数即可；没有 vault 数据迁移，已生成的普通 Markdown 笔记不受影响。
