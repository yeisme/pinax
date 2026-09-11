# research-brief-pilot-verification Specification

## Purpose
TBD - created by archiving change pinax-evidence-research-pilot-v1. Update Purpose after archive.
## Requirements
### Requirement: 研究试验应复用现有边界

系统 SHALL 以现有 Pinax 命令和当前 Agent 组合试验，只使用选定 vault 与用户指定来源；不新增产品 API、CLI 命令或自动模型调用。

#### Scenario: 缺少索引时只读降级

- **WHEN** 索引不可用且研究以 `--lazy-index off` 执行
- **THEN** 原生搜索 SHALL 返回可用候选或明确缺口，并且不写入笔记、索引、Git 或回执

#### Scenario: 未找到相关证据

- **WHEN** 已执行的限界查询没有结果
- **THEN** 简报 SHALL 说明范围和不足，不断言不存在资料，不编造来源

### Requirement: 原生片段应保持原文字符

原生搜索片段 SHALL 是原文的有效 UTF-8 子串，并在存在匹配时保留原始匹配文本。

#### Scenario: 中文、表情与大小写映射

- **WHEN** 截取点附近存在多字节字符，或小写映射改变字节长度
- **THEN** 片段 SHALL 按原文字符边界生成，不产生损坏字符或错误匹配偏移

### Requirement: 研究结论应有可核对来源

研究简报 SHALL 区分事实、来源观点、推断和未知项；来源变化、相互矛盾及日期未知必须可见。

#### Scenario: 来源改名或删除

- **WHEN** 已引用笔记改名或删除
- **THEN** 核验 SHALL 使用既有笔记标识解析，或明确报告来源不可读，不猜测替代来源

### Requirement: 成果保存依赖明确采纳

系统 SHALL 仅在用户明确采纳并要求保存后，通过 Pinax application service 创建笔记并核验；dry-run 不得造成写入。

#### Scenario: 保存失败或结果不确定

- **WHEN** 目标不可写或写入结果无法确认
- **THEN** 系统 SHALL 保留失败状态或核对既有记录，不盲目重放，不宣称保存成功

### Requirement: 技术验证不得替代真实效果

五题真实效果 SHALL 单独使用用户给定的已知笔记与明确反馈评估；测试 fixture、预写简报与模拟采纳不得计入成功分母。

#### Scenario: 只有技术测试通过

- **WHEN** testscript 成功但真实对照或反馈缺失
- **THEN** 真实效果 SHALL 保持未测量，不允许以技术测试结果晋升模板成熟度或固化新 Skill

#### Scenario: 人工补充了漏检资料

- **WHEN** 用户补充遗漏笔记后得到更好的简报
- **THEN** 初轮漏检 SHALL 继续保留，修订后的成功不得覆盖初轮结果

