# pinax-rendered-view-v1

## Why

Pinax 的可视化能力散落在五个互不复用的面（CLI rendered、静态发布渲染器、DSH Pane 四个 bounded envelope、dashboard、Obsidian 兼容层），没有统一渲染语义；同时 vault 对象缺少对外的稳定引用方案，普通 Markdown 无法引用 note。完整问题陈述与三层设计（L1 合同 / L2 消费面 / L3 引用）见 [Markdown 预览设计](../../../docs/product/markdown-rendered-preview-design.md)（P0 已落盘，design-only）。

本 change 是该设计的 **P1 实施切片**：只做 Pinax 拥有的核心（合同 + CLI 出口 + `pinax://` 引用 + fence 注册表），不碰 DSH Pane surface（P2）、workbench module（P3）、web-renderer 切换与编辑器适配（P4）。

## What Changes

- 新增 versioned rendered projection 合同 `render.view.v1alpha1`：**渲染树 + 已解析引用**，不是 HTML；节点带稳定锚。
- wikilink 解析进树：resolved → 卡片引用（object_id/version/status），broken → 显式 broken 标记；backlinks 作为 bounded 附带面板（≤50，沿用 dsh-pane 规则）。
- fence 注册表从 1 种扩到 4 种：`pinax-database-view`（既有语义不变）、`pinax-graph`（±N 跳局部图）、`pinax-board`、`pinax-timeline`（≤100）；fence 失败输出显式失败占位，不静默省略。
- 资产引用：vault 附件解析为 `pinax-asset://<object_id>`；`drivebridge://file/<id>` 透传为外部引用状态卡（不取字节、不代理、不缓存）；未知 scheme 原样保留。
- 新增稳定引用 `pinax://note/<object_id>?v=<revision>`：`pinax note ref "<title>" [--copy]`（Markdown 链接 + wikilink 双格式）与 `pinax note ref resolve <pinax://…>`；floating 引用（不带 `v`）漂移时显式 `stale` 标注，不静默换版。
- CLI：`pinax note preview <ref> --view rendered [--format tree|json|html] [--display card|context|body]`；proof badge（object_id·version·revision·rendered_at）进渲染 footer。
- MCP：新增只读工具 `pinax.render_note`（同一应用服务，bounded，默认不暴露 body 深度）。
- 渲染是纯读投影：不写 `.pinax/**`、无 daemon、无新常驻进程。

### Capabilities

#### New Capabilities

- `rendered-view`：bounded rendered projection 合同、fence 注册表、wikilink/资产/外部引用解析、`pinax://` 稳定引用的生成与解析、proof badge、CLI/MCP 只读面。

#### Modified Capabilities

- 无。`note show --view rendered` 与 `pinax-database-view` fence 的既有行为保留（golden 固化），新能力只做 additive 扩展。

## Impact

- 新包建议：`internal/render`（合同类型、树构建、fence 注册表、引用解析）+ `internal/app/render_*.go` 应用服务。
- 复用：notelinks/graph/board/ledger 投影、paneUnsafe 红线、`internal/output` 四输出模、NoteDisplay 边界。
- 不动：内部 UI（workbench，按 web-open-design admission 另立）、DSH Pane envelope 扩展（P2 change）、web-renderer 数据源切换（P4）。
- 稳定面：新增 CLI 子命令（`note` 族内）与 MCP 工具均为 additive experimental；不改名、不删除任何既有命令/工具/字段。
- 门禁：实施排在 2026-10-10 可信连续性主线窗口之后，或 owner 显式重排优先级（任务 0.1 硬门）。
