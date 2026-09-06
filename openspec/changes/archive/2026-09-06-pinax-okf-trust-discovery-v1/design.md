# Design — OKF 信任与发现体验（pinax-okf-trust-discovery-v1）

## 1. OKF v0.2 → Pinax 映射

| OKF v0.2 概念 | OKF 语义 | Pinax 落地 |
| --- | --- | --- |
| `generated: {by, at}` | 谁在何时产出当前内容 | frontmatter 可选字段；由创建/更新命令如实填写 `agent:pinax/<ver>` 或显式 `human:<id>` |
| `verified: [{by, at}]` | 独立于 generated 的文档级确认事件列表 | frontmatter 可选列表；`pinax note verify` 追加事件；bare mapping 兼容为单元素列表（同 OKF） |
| actor 约定 | `human:<id>` / `<producer>/<version>` / `process:<id>` | 同款三段式；`human:` 前缀用于分级判定 |
| 信任分级 | 无 verified ⇒ unverified；仅非 human ⇒ machine-confirmed；含 human ⇒ human-reviewed；verdict 推断、绝不存储 | 消费时派生（domain helper），索引记录缓存派生结果，可重建 |
| `stale_after` | 绝对时刻；`now >= stale_after` 即 stale（刻意不用相对 TTL） | frontmatter 可选 ISO8601；搜索与 brain 消费时判定 |
| `status: draft/stable/deprecated` | 生命周期 | Pinax 已有 `status`，对齐取值建议，不强制改值 |
| `sources[]`/credibility | 逐源 `resource/usage_count/last_modified` | **v1 不做**（Pinax link-target 已覆盖引用关系；逐源可信度另立 change） |
| `index.md` 渐进披露 | 目录级合成导航 | `pinax browse` 只读合成视图；**不写 vault 文件**（vault 正文不属 CLI-authored） |
| 消费时合成 tag 视图 | 无独立 tag 文件，消费端扫描合成 | `--facets` 从索引投影合成计数 |
| `description` 喂 snippet | 一句话描述供索引/snippet | 搜索结果优先用 note summary/description 字段做 snippet 上下文 |
| Attested Computation | 执行/见证合同 | 对应 Pinax 既有 proof loop，**不在本 change 范围** |

规范真源：OKF 已迁至 `GoogleCloudPlatform/open-knowledge-format`；本设计以 v0.2 冻结文本为准（knowledge-catalog 仓库内为快照）。

## 2. 数据模型与派生规则

### 2.1 frontmatter（可选、additive）

```yaml
generated:
  by: agent:pinax/0.9.0
  at: 2026-09-06T08:00:00+00:00
verified:
  - by: human:ye
    at: 2026-09-06T10:30:00+00:00
stale_after: 2026-12-01T00:00:00+00:00
```

- 时间戳一律带 UTC offset 的 ISO8601（同 OKF）；非法时间戳按解析错误处理（fail-closed，不静默忽略）。
- 未携带字段的 note：trust=unverified、fresh=fresh（不惩罚存量）。
- 兼容 OKF 的 bare mapping `verified: {by, at}` ⇒ 单元素列表。
- 未知 actor 前缀：保留原文，分级判为 machine（不因未知格式崩溃，同 OKF "consumers MUST tolerate unknown" 精神）。

### 2.2 派生（domain helper，绝不存储进 frontmatter）

```go
TrustTier(note)  // unverified | machine | human
Freshness(note)  // fresh | stale   (now >= stale_after ⇒ stale)
```

- 分级只看 `verified` 列表：空 ⇒ unverified；全部非 `human:` ⇒ machine；任一 `human:` ⇒ human。
- 索引投影（`internal/index` note records）增列 `trust_tier`、`stale_after`、`verified_at_latest`（缓存，可重建，rebuild/refresh 路径复用）。

## 3. 维护命令（CLI-authored 红线不变）

- `pinax note verify <ref> [--actor human:ye] [--note <text>]`：追加 verified 事件（幂等：同 actor 同日重复调用不重复追加，返回既有事件）；默认 actor 取配置 `identity`（已有 identity 命令域），无配置时要求显式 `--actor`。写 frontmatter 必须走既有 note frontmatter patch helper（atomicWriteFile）。
- `pinax metadata plan/apply`：新增 backfill 操作类型 `trust_fields`（填 generated/stale_after），与既有 plan/apply 安全模型一致（preview → apply、receipt、atomic write）。
- 红线：不做任何"自动为全部笔记注入信任字段"的批量写；trust 字段只能由显式命令维护。

## 4. 搜索体验

### 4.1 facets（消费时合成）

```
$ pinax search "auth" --facets
12 notes matched · engine=index fresh

  Title          Kind       Trust    Fresh   Snippet
  auth-design    reference  human ✓  fresh   …token rotation…
  auth-runbook   runbook    machine  stale   …

Facets
  tag    auth=6 security=4 gateway=2
  kind   reference=5 runbook=3 decision=4
  trust  human=4 machine=5 unverified=3
  fresh  fresh=9 stale=3
  folder architecture=6 ops=6
Next: --tag auth --trust human, or `pinax search show auth-design`
```

- `--facets` 与 `--json/--agent` 组合时输出 `facets` 对象（数组按计数降序、稳定 tie-break 按字典序）；facet 计数基于**过滤前**的全匹配集（先看全景再收窄，同 OKF 索引浏览心智）。
- human 表格 facet 块置于结果后；`--output-style compact` 单行摘要。

### 4.2 过滤器与徽标（additive）

- `--trust unverified|machine|human`：按派生分级过滤。
- `--stale include|only|exclude`（默认 include）：include 时结果带 stale 徽标，agent 输出带 `trust=`/`fresh=` 字段。
- 既有过滤器、排序、engine 语义零变更。

### 4.3 `pinax search show <ref>` 一站式详情卡

```
$ pinax search show auth-design
auth-design · reference · notes/architecture/auth-design.md
Trust     human ✓ (human:ye @ 2026-09-06) · fresh (stale_after 2026-12-01)
Meta      tags=auth,security  updated=2026-09-06  kind=reference  status=stable
Snippet   …token rotation must invalidate refresh grants…
Links     3 out (auth-runbook, jwt-notes, +1) · 2 in (gateway-design, security-review)
Neighbors same-tag: auth-runbook, jwt-notes
Next      pinax note show auth-design · pinax note backlinks auth-design
```

- 聚合既有投影（bounded snippet、links/backlinks、graph summary）为只读卡；不输出正文（body 红线不变），snippet 有界。
- ref 解析歧义时 fail-closed 报错并列候选（同 `search` 既有歧义策略）。

### 4.4 `pinax browse [path]` 合成导航

```
$ pinax browse notes/architecture
notes/architecture · 6 notes · 3 subfolders

  Subfolders
  decisions/     4 notes  "ADR 记录"
  reviews/       2 notes  —

  Notes (by updated)
  auth-design    reference  human ✓  fresh  updated 2026-09-06
  …
Next: pinax browse notes/architecture/decisions · pinax search show auth-design
```

- 从索引投影合成；"描述"取 note summary/description 字段（无则空）；不写任何 `index.md`。
- 只读、无副作用、`--lazy-index off` 语义与 search 一致。

## 5. Agent Brain 消费

- `brain answer` staged context 中每个 lexical 候选携带 `trust`/`fresh` 标注；unverified 与 stale 候选默认降权排序并显式标注，不静默剔除（citation-first 不变；是否剔除交给调用方 flag）。
- `brain maintain` 候选新增一条规则：`stale human-reviewed` note 提示重新验证（只出候选，不自动改）。

## 6. 输出合同

- `--agent`：`result.N.trust=`、`result.N.fresh=`、`facets.tag.auth=6` 等稳定 key=value；`--json` envelope 增字段 additive；`--events` 不变（search 本就单阶段）。
- 徽标使用 ASCII（`human ✓` / `stale`），notty/markdown-style 下退化为纯文本标签。

## 7. 测试策略

- domain 单测：分级/新鲜度派生、bare-mapping 兼容、非法时间戳 fail-closed、未知 actor 容忍。
- index 单测：增列 rebuild/refresh 幂等。
- testscript e2e：verify 幂等 → search --facets/--trust/--stale → show 卡 → browse（含 `--lazy-index off` 只读断言：不写 `.pinax/index.sqlite`）。
- CLI 输出合同：agent/json golden；递归 body-leak 扫描复用既有 contract test。
- brain：候选标注断言（unverified/stale 标注存在且排序靠后）。

## 8. 风险与取舍

- **frontmatter map[string]string 现状**：Note.Frontmatter 是扁平 map，嵌套 `generated`/`verified` 需要解析层扩展（frontmatter reader 已支持嵌套读，投影到 typed struct）；实现时在 `internal/domain` 加 `TrustSignals` 解析 helper，不破坏既有 map 语义。
- **facet 计数成本**：全匹配集计数走索引列聚合，limit 只裁剪结果列表不裁剪计数集。
- **OKF 演进风险**：以 v0.2 冻结字段为准；字段全部可选且未知键保留，未来 minor 版增量不破坏本合同。
