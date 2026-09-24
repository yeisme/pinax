# pinax-rendered-view-v1 Design

## Context

五个渲染面各自为政、缺稳定引用——问题与三层设计（L1 合同 / L2 消费面 / L3 引用）见 [Markdown 预览设计](../../../docs/product/markdown-rendered-preview-design.md)，交互规格见 [交互体验规格](../../../docs/product/rendered-preview-interaction-ux.md)。本 change 只实施 L1 + CLI/MCP 出口 + L3 引用。现状关键事实：`note show --view rendered` 已渲染 `pinax-database-view` fence；pane 合同已固化 paneUnsafe 红线与 bounded 规则（backlinks ≤50、history ≤100）；graph/board/ledger 只读投影已存在；DriveBridge 引用 `drivebridge://file/<id>` 只经 `prepare_reference` 返回稳定引用。

## Goals / Non-Goals

**Goals**

- 一个 versioned、renderer-agnostic、bounded 的渲染树合同，CLI 与 MCP 同源。
- wikilink/backlinks/fence/资产全部解析为引用与占位，永不输出文件系统路径。
- `pinax://` 引用生成与解析闭环，固定版本与 floating 语义显式。
- 全 additive：既有 `note show --view rendered` 行为 golden 固化不变。

**Non-Goals**

- 不实现 Pane rendered surface（P2）、workbench module（P3）、web-renderer 切换与编辑器 URI handler（P4）。
- 不做渲染面写路径、不代理外部资产字节、不新增常驻进程。
- 不做非 note 目标的 `pinax://search|board` 引用（开放问题，P1 只做 note）。

## Architecture

```text
note ref (object_id+version) ──► internal/app/render service
                                   ├─ 树构建（markdown AST + 锚点）
                                   ├─ wikilink 解析（notelinks 投影）
                                   ├─ fence 注册表（database/graph/board/timeline → 现有投影）
                                   ├─ 资产/外部引用（attachments / drivebridge:// 透传）
                                   └─ bounded 装配（display 深度 + paneUnsafe + proof badge）
                                 ▼
                    render.view.v1alpha1（树 + 引用 + 面板 + badge）
                       ├─ CLI note preview --view rendered（tree|json|html）
                       ├─ MCP pinax.render_note（只读）
                       └─ （后续 P2-P4：pane envelope / workbench / 发布构建）
```

### 合同要点

| 件 | 决定 |
| --- | --- |
| envelope | `render.view.v1alpha1`：`ref`、`display`、`tree`、`panels.backlinks`、`badge`、`truncated`、`warnings[]` |
| 节点 | heading / paragraph / list / fence / link / asset / external_ref，各带 `anchor` |
| 锚点 | heading id = slug+序号去重；fence id = 正文序号；link target = 解析后的引用 |
| display 深度 | `card`=树骨架+fence 摘要；`context`=树+头部段落截断；`body`=全树（默认不进 MCP） |
| fence 失败 | 显式 `fence_failed` 占位节点（错误码 + 下一步），不省略 |

## Decisions

- **D1 树不是 HTML**：终端、Pane、workbench、静态发布的渲染介质各异；树+引用是唯一可共享语义，HTML 只是 CLI `--format html` 的一种出口。
- **D2 命令落位**：全部落在 `note` 族（`note preview --view rendered`、`note ref`、`note ref resolve`），不加新顶层命令，尊重 roadmap 对"新的顶层 note/platform command"的暂停令；若门复审后要求独立 `render` 树，迁移仅涉及 Cobra wiring。
- **D3 fence 失败 fail-closed**：沿 dashboard"failed 不能伪装成 ready 空集"原则，失败占位携带稳定错误码。
- **D4 `pinax://` 版本语义**：`v=` 固定版本精确解析；无 `v` 为 floating——解析返回当前版本 + `resolved_revision` + 时间，再次解析若远端已漂移则 `stale=true` 且 `resolved_revision` 不变，**不静默换版**（identity-first：版本钉住、漂移显式）。
- **D5 drivebridge:// 透传**：只识别 scheme 并输出 `external_ref` 状态卡（引用字符串原样 + 正文中声明的元数据），不调用 DriveBridge、不发网络 IO（测试断言零网络/零文件副作用）。
- **D6 附件引用**：`pinax-asset://<object_id>`，mime 分类由投影给出，字节渲染由消费面负责。
- **D7 MCP 默认 bounded**：`pinax.render_note` 默认 `display=card|context`；`body` 深度需显式参数且进审计（沿 NoteDisplay 纪律）。
- **D8 与 pane envelope 的关系**：本 change 不改 pane 合同；P2 的 rendered surface 将把本合同装进 pane envelope，红线（paneUnsafe、fail-closed）已在两侧对齐。

## Risks / Migration / Rollback

- additive only：不迁移既有数据；`note show --view rendered` golden 固化，新出口并存。
- 回滚 = 移除新 flag/子命令/MCP 工具注册，无 schema 变更。
- 主要风险：fence 数据源投影在大 vault 的性能——graph fence 硬上限（默认 1 跳、最多 2 跳 + 节点/边截断）在合同层强制，超限显式 `truncated`。

## Security

- paneUnsafe 全量继承：绝对路径、URL scheme 危险子串（token/authorization/cookie）不进任何节点/面板/badge。
- `--display body` 之外的输出不含完整正文；redaction 沿用 `ApplyProjectionRedaction`。
- MCP 工具只读；不产生 daemon、不监听端口、不访问 vault 外路径。
