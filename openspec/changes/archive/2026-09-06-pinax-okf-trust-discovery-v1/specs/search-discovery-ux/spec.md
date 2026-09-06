## ADDED Requirements

### Requirement: search SHALL 支持消费时合成的 facet 计数
`pinax search <query> --facets` SHALL 输出 facet 计数块（tag、kind、status、folder、trust、fresh），计数基于过滤前的全匹配集合成（消费时合成，不落盘任何 facet 文件），facet 数组按计数降序、同数按字典序稳定排序。`--json`/`--agent` 输出 MUST 携带等价 `facets` 结构。既有过滤器、排序与 engine 语义 MUST 零变更。

#### Scenario: facet 引导收窄
- **WHEN** 执行 `pinax search "auth" --facets --json`
- **THEN** envelope MUST 含 `facets`（含 `trust` 与 `fresh` 分组）且计数覆盖全匹配集
- **AND** 追加 `--tag auth --trust human` 后结果 MUST 为对应子集。

#### Scenario: 无匹配
- **WHEN** 查询无任何命中
- **THEN** facet 计数 MUST 为空集表示，MUST NOT 报错。

### Requirement: search SHALL 支持信任与新鲜度过滤及徽标
`pinax search` SHALL 提供 `--trust unverified|machine|human` 与 `--stale include|only|exclude`（默认 include）过滤器。结果行 MUST 展示信任与新鲜度徽标（human 输出用 ASCII 徽标，notty 退化为纯文本），`--agent` 输出 MUST 含 `trust=`/`fresh=` 字段。默认行为（不带新 flag）MUST 与现状完全一致。

#### Scenario: stale-only 检索
- **WHEN** 执行 `pinax search "runbook" --stale only`
- **THEN** 结果 MUST 只含 `stale_after` 已过期的 note，且每条带 stale 徽标。

### Requirement: search show SHALL 提供一站式有界详情卡
`pinax search show <ref>` SHALL 聚合 bounded snippet、元数据、信任面板（分级、关键事件、`stale_after`）、出链/入链计数与同标签邻居为只读详情卡；MUST NOT 输出 note 正文全文；ref 解析歧义时 MUST fail-closed 报错并列出候选。

#### Scenario: 详情卡聚合
- **WHEN** 执行 `pinax search show auth-design`
- **THEN** 输出 MUST 含信任面板（分级 + 最新 human 验证事件）、snippet、入链/出链计数与 next 提示
- **AND** MUST NOT 出现 note 正文除有界 snippet 外的任何部分。

#### Scenario: ref 歧义
- **WHEN** ref 匹配多个 note
- **THEN** 命令 MUST 报错并列出全部候选，MUST NOT 自动选择。

### Requirement: browse SHALL 合成只读目录导航且不写 vault
`pinax browse [path]` SHALL 从索引投影合成目录视图：子目录（含 note 计数）、该层 notes（按 updated 排序，带信任/新鲜度徽标）、可用的 description/summary 摘要。browse MUST 只读，MUST NOT 在 vault 内创建或修改任何 `index.md` 或其他文件；`--lazy-index off` 时 MUST NOT 写 `.pinax/index.sqlite`。

#### Scenario: 目录浏览
- **WHEN** 执行 `pinax browse notes/architecture --lazy-index off`
- **THEN** 输出 MUST 含子目录列表与该层 note 列表（含徽标）
- **AND** vault 文件树与 `.pinax/index.sqlite` MUST 无任何变更。

#### Scenario: 不存在的路径
- **WHEN** browse 指向 vault 内不存在的路径
- **THEN** MUST 返回稳定错误并提示可用路径，MUST NOT 猜测就近目录。

### Requirement: Agent Brain 候选 SHALL 携带信任标注
`pinax brain answer` 的 staged context 中每个 lexical 候选 MUST 携带派生 `trust`/`fresh` 标注；`unverified` 与 `stale` 候选 MUST 显式标注并默认排序靠后，MUST NOT 被静默剔除或静默同权纳入。`brain maintain` SHALL 能为 `stale` 且 `human` 分级的 note 产出重新验证候选（仅候选，不自动写）。

#### Scenario: 混合信任候选
- **WHEN** staged context 同时含 human-reviewed 与 unverified 候选
- **THEN** 输出 MUST 对两者分别标注 `trust=`，且 unverified 默认排在 human 之后。

#### Scenario: 过期高信任 note 维护候选
- **WHEN** 某 note 分级为 `human` 且已 stale
- **THEN** `pinax brain maintain` 候选 MUST 包含重新验证类建议且不直接修改 note。
