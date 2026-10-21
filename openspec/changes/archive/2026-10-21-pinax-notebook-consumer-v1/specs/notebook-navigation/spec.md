## ADDED Requirements

### Requirement: 可见范围的标签树

Pinax MUST 提供 metadata scope 下的 `/v1/reader/tags`，返回规范路径、稳定语义 tag ID、父 ID、虚拟祖先、直接与后代去重计数、水位和分页。计数必须先排除不可见对象；不得使用索引自增行号作为标签身份。

#### Scenario: 多层标签与重复笔记

- **WHEN** 一篇笔记同时含父标签、多个子标签和重复标签
- **THEN** 每个节点的直接计数和后代计数分别按 note ID 去重，后代不包含直接节点，无直接引用的祖先标 virtual

#### Scenario: 列表分页与变更

- **WHEN** 客户端继续原游标，但内容、路径或会话授权变化
- **THEN** 拒绝游标，不能把不同来源或水位页拼成一棵树

### Requirement: 有界关系图与等价列表投影

Pinax MUST 提供 metadata 与 relations scope 下的 `/v1/reader/graph`，支持来源图和固定中心 revision 的1/2跳局部图。每页最多200节点、400条边；边两端必须在本页，跨页节点重复可按来源和 ID 去重。只使用 owner 已解析链接。

#### Scenario: 大图分页

- **WHEN** 子图超过一页节点或边预算
- **THEN** 返回 next_cursor、明确 page_complete/corpus_complete 与可见总量，后续页不丢跨页边，孤立节点也可发现

#### Scenario: 未授权或无法解析的目标

- **WHEN** 边指向隐藏、symlink、无效 trust、外部或歧义目标
- **THEN** 不输出目标、边、名称或计数线索；图范围仅代表允许集合

### Requirement: 日记只读定位

Pinax MUST 提供 `/v1/reader/journals/daily`，要求明确日期与 IANA 时区，按 owner 模板 output path 和既有 legacy 路径定位已存在的日记，不执行任何创建、模板查询或索引刷新。

#### Scenario: 当日日记缺失

- **WHEN** 日期合法但日记不存在或不可见
- **THEN** 返回 missing 与 null note，vault 文件树和 owner 状态保持不变

#### Scenario: 夏令时与模板路径

- **WHEN** 日期为合法时区的本地日历日，模板声明自定义路径
- **THEN** 返回同一日期和时区下的现有固定 note revision，不因 UTC 或夏令时切到另一日

### Requirement: 新旧合同隔离

Pinax MUST 为导航增量注册封闭 schema、catalog、OpenAPI 与 typed client，保持旧 reader/writer/CLI/MCP 行为不变。只读能力不代表日记首建、标签 mutation、rich-document 或跨会话恢复已经完成。

#### Scenario: 能力与旧 digest 校验

- **WHEN** 新增导航 path/schema 从导出合同中移除
- **THEN** 精确恢复此前的 OpenAPI digest，旧预算和枚举不扩大
