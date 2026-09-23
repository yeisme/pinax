# 渲染预览与 Agent 审阅交互体验规格

- 状态：规格稿。本文定义各消费面的**交互合同**（Pinax 拥有交互语义，实现归各面 owner：CLI/MCP 在 `cli/pinax`，Pane 在 DSH 域，全功能阅读面在 `client/yeisme-workbench`）。
- 日期：2026-09-23
- 关联：[Markdown 预览设计](./markdown-rendered-preview-design.md)、[pinax-rendered-view-v1](../openspec/changes/pinax-rendered-view-v1/)、[pinax-agent-note-proposal-v1](../openspec/changes/pinax-agent-note-proposal-v1/)

## 1. 四条交互原则

1. **Bounded-first**：任何交互先给卡片事实（title/kind/status/tags + 引用），用户主动下钻才进正文深度；agent 同理（card→context→body）。
2. **显式失败**：broken link、fence 失败、引用漂移、投影离线都是显式状态，绝不静默省略或伪装成空成功。
3. **一次剪贴板动作**：复制引用/双格式粘贴是单动作；多命中先消歧再复制，宁可多一步选择也不错贴。
4. **键盘可达**：所有交互有键盘路径；motion 遵守 reduced-motion。

## 2. 终端渲染预览（`note preview --view rendered`，P1）

```text
┌ Title（h1，锚 #title）
│ 段落文本……（context 深度截断处显式 `… truncated (display=context)`）
│ ┄ pinax-database-view active-table ┄  → 表格投影（≤20 行，truncated 标记）
│ ┄ pinax-graph ┄                       → 节点/边计数 + top 节点行
│ ✗ pinax-board research/missing        → [fence_failed board_not_found]
│                                         下一步: pinax project board show …
│ [[研究记录]] → 已解析卡片（title·status·version）
│ [[不存在的页]] → ✗ broken
│ badge: object_id · v3 · rev:9f2c… · 2026-09-23T14:02Z
└ backlinks(12, truncated=false)：来源卡片 ≤50
```

- TTY：颜色 + 对齐；管道/`--json`：无 ANSI，机器可解析（cli-output-contract）。
- `--format html` 是导出形态（邮件/存档），不是终端交互面。

## 3. 粘贴即引用流（`note ref --copy`，P1）

| 步骤 | 交互 | 失败态 |
| --- | --- | --- |
| 1. `pinax note ref "研究记录" --copy` | 剪贴板同时得到 `[研究记录](pinax://note/…?v=…)` 与 `[[研究记录]]` | 无剪贴板环境 → 双格式打印到 stdout 并提示手动复制 |
| 2. 粘贴目标 | vault 内编辑器用 wikilink；vault 外任意 markdown 用 `pinax://` 链接 | 两格式都在，用户删哪个用哪个 |
| 3. 多命中 | 输出编号消歧列表（title·kind·updated），`--pick N` 或交互选择后复制 | 零命中 → 建议改用 `pinax search`，不虚构引用 |

- floating 选项 `--floating`：去掉 `v=`，粘贴处显示"当前版"语义；消费端解析漂移时显式 `stale` 徽标（见 §4）。

## 4. `pinax://` 解析体验（P1）

- **固定版本**：解析结果常显 `v=3 · rev 9f2c…`；目标版本被删除/不可达 → `unresolved` 显式结果，给"列出该 note 其他版本"的下一步。
- **floating**：显示 `当前 v4（解析于 v3，已漂移 → stale）`——resolved 值不换、漂移只标状态；下一步给"复制新版本引用"单动作。
- 所有 stale/unresolved 徽标可在 `--json` 中机器读取（`stale`、`resolved_revision`、`current_revision`）。

## 5. DSH Pane rendered surface（P2，合同级交互）

- 复用 pane 快照 envelope：渲染树装进 `payload.rendered`，红线不变（无路径/正文默认 bounded）。
- 交互：点击锚点/卡片 → **gated 深链**（open in workbench / 复制引用），不直接在 Pane 内导航；Pane 保持快照语义，无内嵌滚动大战。
- 状态：快照 `fresh` 显示 rendered_at；无事件流时沿既有 offline 规则——不轮询伪装实时。
- cursor/sequence 落地真实事件游标后，刷新只发增量。

## 6. workbench 阅读面（P3 交互规格，实现归 client/yeisme-workbench）

三栏布局（窄屏折叠为单栏 + 抽屉）：

```text
┌─────────────┬──────────────────┬─────────────┐
│ 局部图(可收) │   主渲染视图      │ 反向链接面板 │
│ ±1跳·可缩放 │  锚点导航·fence  │  来源卡片    │
│             │  hover 卡·版本徽标│  broken 分组 │
└─────────────┴──────────────────┴─────────────┘
```

- **hover 卡**：wikilink/引用悬停 180ms 出卡（卡片投影 + 版本）；Esc 关、Tab 进卡操作（打开/复制引用/看版本）；可 pin 成侧栏常驻。
- **版本徽标**：badge 点击显示该版本 receipt 摘要（快照/操作/时间）；版本对比视图**不进** v1，显式"后续"。
- **fence 交互**：表格/看板行内展开收起（bounded 行数内）；失败占位显示错误码 + "在终端运行下一步"复制按钮。
- **键盘地图**：`j/k` 移动锚点、`Enter` 下钻卡片、`b` 开反向链接、`g` 切局部图、`c` 复制引用、`?` 帮助；焦点环显式。
- **reduced-motion**：图布局动画、卡片过渡全部尊重系统设置，静止降级不损失信息。

## 7. Agent proposal 审阅 UX（P1b，配 note-proposal change）

CLI 审阅闭环（用户视角三步）：

```bash
pinax agent note proposal list            # 概览：kind·风险·rationale 摘要·diff 行数
pinax agent note proposal show <id>       # 结构化变更 + 目标卡片 + expected revision
pinax agent note proposal approve <id> --yes   # 或 reject <id>（同样留 receipt）
```

- **文案铁律**：每个失败/等待状态必带下一步动作（`plan_stale` → "重读卡片后重新 propose"；capability 关 → "在配置中显式开启"），无裸错误码终点。
- **审阅投影 bounded**：list/show 默认无正文全文；diff 只给行数摘要 + hunk 计数，与 receipt 同界。
- **auto-apply 可辨识**：低风险自动应用的 receipt 在 `recent` 里标 `auto_applied`，用户回看不产生"我没批过"的惊吓。
- 图形审阅面（workbench/Pane）为后续，不在本规格 v1。

## 8. 状态矩阵（所有面统一）

| 状态 | 含义 | 展示 |
| --- | --- | --- |
| `ready` | 投影成功 | 正常渲染 + badge |
| `truncated` | 超界截断 | 显式标记 + 计数（如 backlinks 50/127） |
| `stale` | floating 引用漂移 | resolved 值不变 + 漂移徽标 + 换新引用单动作 |
| `broken` / `unresolved` | 链接/引用不可达 | 显式标记 + 下一步（search/列版本） |
| `fence_failed` | 数据源投影失败 | 错误码占位 + 下一步命令 |
| `offline` / `permission_denied` | 面离线/无权 | 空快照 + 状态（沿 pane 规则，绝不伪装空成功） |

## 9. 交互度量挂钩（进 dogfood evidence，无正文）

| 指标 | 采集点 |
| --- | --- |
| hover 卡解析率 | workbench 卡片投影 resolved/broken 比 |
| 粘贴往返成功 | `note ref` 复制 → 后续 resolve 成功率 |
| proposal 审阅时长 | list→approve/reject 间隔（CLI 时间戳） |
| card_hit_rate / accept_rate / stale_rate | 沿 [Agent 优化设计](./agent-optimization-design.md) §3.5 |

## 10. 非目标

- 不做所见即所得编辑、不在渲染面开写路径、不代理外部资产字节。
- 不做实时协作/多人光标。
- 图形审阅面、版本对比视图显式留后续。
