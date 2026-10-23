# Markdown × Pinax 可视化预览结合设计

- 状态：设计稿（design-only）。P1 CLI/MCP 核心已于 2026-09-30 完成本地实现与归档；后续消费面保持独立 owner 与准入。详见 [Roadmap](./roadmap.md)。
- 日期：2026-09-23
- 关联：[Web/UI 归属](./web-open-design.md)、[Pane 合同](../interfaces/pane.md)、[Agent 优化设计](./agent-optimization-design.md)

## 2026-10-01 人读消费修订

用户将P3目的地确定为DriveBridge Electron内置知识阅读空间，不再是Workbench。旧L1 `render.view.v1alpha1` 仍保持64KiB/512节点与Agent脱敏；人类完整长文使用独立 [reader合同](../interfaces/human-reading-api.md)，复用owner解析/引用/fence语义而非把旧树上限放大。该新API尚待实施，不能用CLI/MCP存在推定HTTP可用；P2/发布renderer/URI handler不因本轮设计自动启动。下文P1设计与验收按原受众保留。

## 1. 问题

Pinax 的可视化能力已经存在，但散落在五个互不复用的面上：

| 面 | 现状 | 缺口 |
| --- | --- | --- |
| CLI `note preview` / `note show --view rendered` | 渲染预览正文 + `pinax-database-view` fence | 只支持一种 fence；wikilink/backlink/graph 不参与渲染 |
| `web/pinax-web-renderer` | 静态发布 HTML（graph.json、search-index.json） | 面向公网发布产物，不是内部分析面，不消费运行时投影 |
| Pinax Pane | note list / backlinks / graph summary / history 四个 bounded envelope | 无正文渲染 surface；cursor/sequence 还是占位符 |
| 本地 dashboard | 只读投影 | 独立实现，与上述面不共享渲染语义 |
| Obsidian 兼容层（Preview） | wikilinks/backlinks/properties/attachments 兼容读取 | 无 Pinax 侧富阅读面，依赖用户装 Obsidian |

同时缺两个"结合"件：

1. **统一渲染语义**：同一个 note 在 CLI、Pane、发布站点上应由同一个 bounded rendered projection 驱动，而不是各面自造渲染。
2. **跨工具引用**：普通 Markdown（仓库文档、工作产物、任何编辑器里的文件）无法引用 vault 对象。Pinax 有 canonical `object_id`，但没有对外的引用方案——这是 DriveBridge `drivebridge://file/<id>` 已经验证过、Pinax 尚缺的"粘贴即引用"能力（连接层的价值在嵌入既有写作流，不在功能数量）。

## 2. 设计总览（三层）

```text
L3 引用层    pinax://note/<object_id>?v=<revision> —— 任何 Markdown 可粘贴的稳定引用
                 + vault 内可嵌 drivebridge://file/<id> 外部资产引用
L2 消费层    CLI rendered · Pane rendered surface · DriveBridge reader ·
             pinax-web-renderer 静态发布 · 编辑器薄适配 —— 共用owner语义，受众权限分别登记
L1 合同层    Rendered Projection Contract（versioned、renderer-agnostic、bounded）
             = 渲染树 + 已解析引用，不是 HTML
```

## 3. L1 Rendered Projection Contract

新增 `pinax render preview <ref> [--format tree|json|html] [--display card|context|body]`，输出 versioned 渲染树投影（建议 envelope `render.view.v1alpha1`）。

### 3.1 输入与锚点

- 输入：note 稳定引用（object_id + version），或显式 `--file` 的 vault 内路径；不接受 vault 外任意路径。
- 树节点带稳定锚：heading id、fence id、wikilink target、asset ref。锚点用于 Pane/workbench 深链定位。

### 3.2 Wikilink 与关系解析

- wikilink 解析进树：resolved → `{object_id, title, status, version}` 卡片引用；broken → `broken` 标记。**永不输出文件系统路径**。
- backlinks 不进正文树，作为投影附带的 bounded 面板数据（沿用 pane backlinks 规则：≤50 实体、`truncated`、`dropped_unsafe`）。
- 渲染 footer 带 proof badge：`object_id · version · revision · rendered_at`——可核验来源是 Pinax 的品牌承诺，渲染产物同样要可核验。

### 3.3 Fence 注册表（从 1 种扩到 4 种）

| fence | 内容 | 数据源（全部复用现有投影） |
| --- | --- | --- |
| `pinax-database-view <name>` | bounded tab 投影（已支持） | database saved views |
| `pinax-graph` | 当前 note ±N 跳局部图（默认 1，上限 2） | graph 只读投影 + notelinks |
| `pinax-board <project>[/<sub>]` | 看板只读投影 | project board 投影 |
| `pinax-timeline` | 本 note 的 ledger 修订时间线 | record ledger（同 pane history 规则，≤100） |

fence 渲染失败/超界输出显式失败占位，不静默省略（沿 dashboard "failed 不能伪装成 ready 空集"的原则）。

### 3.4 资产与外部引用

- vault 内 attachments：解析为 bounded asset ref（`pinax-asset://<object_id>`），预览面按 mime 分类（image/video/pdf/text/other），图片视频由消费面就地渲染，路径不进投影。
- `drivebridge://file/<id>`：透传为外部资产引用，渲染为状态卡（引用 + 授权状态 + 体积/摘要），**预览面不代理字节、不取 URL**——与 DriveBridge `prepare_reference` 只返回稳定引用的纪律一致。
- 未知 scheme：原样保留为纯文本，不解析。

### 3.5 安全红线（全部继承，不新造）

- `--display` 三级边界不变；rendered 树默认 `card`/`context` 深度，`body` 深度才含完整正文段。
- paneUnsafe 红线沿用：路径、URL scheme 危险子串（token/authorization/cookie）一律不进 envelope。
- 渲染是纯读投影：不写 `.pinax/**`，不产生 daemon，不新增常驻进程（尊重 roadmap 暂停窗口）。
- redaction 与手工元数据拒绝（`RejectHandwrittenMetadata`）语义不因渲染而放宽。

## 4. L2 消费面映射

| 消费面 | 角色 | 变化 |
| --- | --- | --- |
| CLI `note preview --view rendered` | 管道/终端消费 | 升级为 L1 合同的 CLI 出口，`--json` 输出完整树 |
| Pinax Pane | 工作台内嵌 | 新增 `rendered` surface（envelope 扩展，仍走 `internal/app/pane.go` 装配）；顺带把 cursor/sequence 占位符换成真实事件游标 |
| DriveBridge知识阅读空间（cli/drivebridge） | 人类桌面阅读面，首片只读 | 按 [消费交接](./web-open-design.md) 与新reader合同实施；共用owner语义，不读 `.pinax/**`，不以旧bounded树冒充全文 |
| pinax-web-renderer | 静态发布 | 发布构建改从 L1 合同取树，消除与内部渲染的语义分叉 |
| 编辑器薄适配 | 跨工具 | 见 §5；不内置编辑器，不自研编辑器插件市场 |

## 5. L3 `pinax://` 引用与"粘贴即引用"

- 引用格式：`pinax://note/<object_id>?v=<revision>`（固定版本）；不带 `v` 解析为当前版本并标注 `floating`。
- 复制命令：`pinax note ref "<title>" --copy` 同时输出 Markdown 链接与 `[[wikilink]]` 双格式进剪贴板；`--json`/`--agent` 输出引用对象。
- 任何 Markdown 文件中出现 `pinax://` 引用：L1 渲染时解析为 hover-card 数据（title/kind/status/tags，同 pane note-list 实体边界）；解析失败显示 `unresolved` 占位。
- 双向引用互通：Pinax note 内嵌 `drivebridge://file/<id>`（§3.4）；DriveBridge 侧不需要感知 Pinax。两个引用体系各自稳定、互不代理。
- 编辑器路径分两步：先用 URI handler（`pinax://` → `pinax note describe --view rendered` 终端/workbench 预览），编辑器扩展（VS Code/Obsidian 插件）留到验证引用格式稳定之后。

## 6. 分期（对齐 roadmap）

| 期 | 内容 | 时点约束 |
| --- | --- | --- |
| P0（本设计） | 合同与分期落盘 | 已完成；交互规格见 [rendered-preview-interaction-ux.md](./rendered-preview-interaction-ux.md)，排序见 [preview-agent-delivery-map.md](./preview-agent-delivery-map.md) |
| P1 | L1 rendered projection + CLI rendered 出口 + `pinax://` 引用与 `note ref`（change [`pinax-rendered-view-v1`](../../openspec/changes/archive/2026-09-30-pinax-rendered-view-v1/)，24 任务，strict 已验） | 2026-09-30 本地实现与归档完成；纯 additive，新增能力保持 experimental |
| P2 | Pane `rendered` surface + 真实事件游标（change 归 pane 域） | P1 之后 |
| P3 | DriveBridge内置知识阅读；Pinax人读API | 当前只交设计；真实binding/三平台另验，不依赖Workbench或P2 |
| P4 | pinax-web-renderer 切换 L1 数据源 + 编辑器 URI handler | 最后 |

## 7. 验收要点

- 同一 note 在 CLI `--json`、Pane envelope、发布构建中产生同 revision 的等价树（contract test 固定 fixture）。
- bounded 断言：backlinks ≤50、timeline ≤100、graph ≤2 跳、fence 失败显式占位。
- red line 断言：任何输出不含绝对路径、token 子串、未授权外部 URL；`--display body` 之外不含完整正文。
- `pinax://` 往返：ref 复制 → 粘贴进任意 md → L1 解析回同一 object_id+version；floating 引用在漂移时显式标注而非静默换版。
- 证据：沿用 `tools/testkit/integrationevidence`，新增 `rendered-view-parity` profile。

## 8. 非目标

- 不在 `cli/pinax` 内实现 React/Electron 工作台（web-open-design 红线）。
- 不做所见即所得编辑；渲染面是阅读/预览，编辑回到编辑器与 proof loop。
- 不做渲染面写路径；Pane gated actions 仍只有已批准清单（本次不扩）。
- 不代理、缓存或转存 drivebridge 外部资产字节。
- 不新增常驻服务、不做实时协作。

## 9. 开放问题

1. `pinax-graph` 局部图在大 vault 上的度数上限与截断策略（复用 graph summary 的 top-N 规则还是按跳数硬截断）。
2. `pinax://` 是否需要 `pinax://search/<query>`、`pinax://board/<project>` 等非 note 目标（建议 P1 只做 note，验证后再扩）。
3. 发布构建消费 L1 合同时，静态站点对 `pinax://` 引用的降级渲染策略。
