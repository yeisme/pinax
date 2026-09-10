# 旧笔记参与决策的研究简报试验

## Why

用户已选择先组合现有 Agent、Pinax 和研究模板，以五个真实问题验证积累的笔记是否参与了新判断。现有 `brain answer` 主要返回匹配来源，不能替代跨来源综合。实施中的真实查询还暴露了原生片段按 UTF-8 字节截断导致乱码的问题。

## What Changes

- 复用现有 CLI 建立隔离的 testscript 流程，验证检索、读取、引用、预览与模拟采纳后的保存。
- 在现有集成证据入口加入开发用 `research-brief` profile，保存成功和失败证据；明确合成测试不计入真实使用分母。
- 修复共享 `FirstSnippet` 的 Unicode 截断和大小写映射偏移，保持 ASCII 行为。
- 修复 `search --lazy-index off` 仍写 monitor 文件的问题；显式只读搜索不启动监控记录，默认搜索继续记录。
- 修复关联查询用 `index.Init` 检查状态而重写索引的问题，统一复用 `index.Inspect`。
- 在当前 Agent 对话中开展五题真实试验；效果判定、用户采纳和正式工作流固化保留为外部验收门。

## Capabilities

### New Capabilities

- `research-brief-pilot-verification`：限定来源、原文核验、失败披露及真实效果与技术测试分离的验收合同。

### Modified Capabilities

无新增产品 API、CLI 命令、持久化表或后台服务；既有原生片段函数修复 UTF-8 文本正确性。

## Required Capability Ledger

| 要求 | Owner | 当前交付与证据 | 状态 |
|---|---|---|---|
| 找回已登记的旧笔记 | Pinax / 当前 Agent | 真实只读检索与合成 CLI 测试；用户已知笔记对照待补 | retained |
| 指定补充来源与引用核验 | 当前 Agent / 来源 owner | 不扩张来源范围，标注缺失和来源日期 | retained |
| 支持判断的简报 | 当前 Agent / 官方模板内容 owner | 参照固定模板规范，五题草稿等待实际评价 | retained |
| 认可后保存 | Pinax | CLI dry-run、真实写入及失败路径在隔离 vault 测试 | retained |
| 通过后固化 Skill 和使用文档 | 公共 Skill source / Pinax | 依赖五题 gate；本 change 不提前修改活跃 Skill | staged |

## Impact

实现和证据归 `cli/pinax`。模板和 Registry 不改动；不新建 root 实施任务、客户端、向量运行时或编译器。

## Boundary Decisions and Scope Changes

- `fit`：现有笔记命令、检索片段修复和本地流程验证归 Pinax。
- `split-owner`：Agent 负责问题拆解与综合；Pinax 不自动调用模型。公共模板正文继续由内容仓管理。
- 真实查询发现 UTF-8 乱码，因此加入同一研究流程直接依赖的局部修复；无协议字段、原笔记或元数据迁移。
- 全树对比进一步复现显式只读查询写入 `.pinax/monitor`，因此修复该路径；不删除已有监控记录，不影响默认搜索监控。
- 跨秒复核发现关联查询重写索引元数据，加入共享状态检查的局部修复；已有过时索引正确降级为 scan。
- Inferrum、DSH 入口和正式 Registry 编译包留待证据支持后的独立决策。
