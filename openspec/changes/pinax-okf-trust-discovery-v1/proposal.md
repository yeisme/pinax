## Why

对照 [OKF v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf)（Open Knowledge Format，规范真源已迁至 `GoogleCloudPlatform/open-knowledge-format`）审视 Pinax 搜索体验，发现三个结构性缺口：

1. **无信任与生命周期信号**。OKF 把 `generated {by,at}` / `verified [{by,at}]` / `stale_after` / `status` 作为一等公民，并规定 actor 约定（`human:<id>` / `agent:<producer>/<version>` / `process:<id>`）与派生信任分级（unverified / machine-confirmed / human-reviewed）。Pinax notes 只有 `status`，agent 写入与人工确认在搜索结果中不可区分。
2. **无渐进式披露**。OKF 依赖 `index.md` 目录导航 + `description` 喂 snippet/预览 + 消费时合成 tag 视图。Pinax `search` 有丰富过滤器，但没有 facet 计数引导收窄，没有一站式结果详情（snippet + 元数据 + backlinks + 邻居 + 新鲜度），也没有目录级浏览视图。
3. **检索结果无信任分层**。`brain answer` 的 staged context 无法按信任分级标注候选，human-reviewed 与未经确认的 agent 生成内容同权进入引文。

本 change 将 OKF 的信任/生命周期字段、facet 合成、渐进披露三个体验支柱移植为 Pinax 本地合同；vault 仍是唯一真源，全部新增元数据为 additive、可选、CLI-authored。

## What Changes

- 新增 vault 信任与生命周期元数据合同（`vault-trust-lifecycle` 能力）：frontmatter 可选字段 `generated` / `verified` / `stale_after`，OKF actor 约定，派生信任分级（只派生、绝不落盘），`pinax note verify <ref>` 追加人工验证事件，`pinax metadata plan/apply` 支持信任字段回填；索引投影记录派生信号（可重建）。
- 新增搜索与发现体验（`search-discovery-ux` 能力）：`pinax search --facets` 消费时合成 facet 计数（tag/kind/status/folder/trust/fresh）；`--trust` 与 `--stale` 过滤器；human/agent 输出带信任与新鲜度徽标；`pinax search show <ref>` 一站式结果详情卡；`pinax browse [path]` 只读合成目录导航视图（不向 vault 写 `index.md`）。
- `brain answer` 的候选标注信任分级：unverified/stale 候选必须显式标注而非静默纳入。
- 非目标：不引入向量检索、不改 `search` 既有过滤器语义（全部 additive）、不写 vault 正文、不自动批量注入信任字段（仅显式命令维护）、不把 OKF bundle 作为新的 vault 真源格式。

## Capabilities

### New Capabilities
- `spec:vault-trust-lifecycle` — 信任/生命周期 frontmatter 字段、actor 约定、派生分级、维护命令与索引投影合同。
- `spec:search-discovery-ux` — facet 合成、信任/新鲜度过滤与徽标、`search show` 详情卡、`browse` 合成导航的搜索体验合同。

## Impact

- 代码：`internal/domain`（信任字段与分级推导）、`internal/app`（verify/metadata/search/show/browse）、`internal/index`（记录字段）、`internal/output`（human/agent/json 渲染）、`cmd/pinax`（命令与测试）。
- 兼容：现有 vault 与既有 `search` 输出不受影响；无信任字段的 note 按 `unverified` + 非 stale 处理；索引 schema 只增列，重建路径不变。
- 下游：`brain answer`、DSH pane 投影、后续 `pinax-share-explore-v1`（本波配套 change）消费同一派生信号。
