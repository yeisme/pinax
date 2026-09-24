# pinax-rendered-view-v1 Tasks

## 0. 前置门禁与基线

- [ ] 0.1 Owner: release；Scope: 本 change 全部实施任务；Dependencies: none；Lane: gate；确认 2026-10-10 连续性主线窗口已结束，或 owner 出具显式优先级重排记录；Verification: `openspec status --change pinax-rendered-view-v1` + owner note in change README；Expected: 门记录先于任何代码任务；Failure re-check: 仅执行合同/spec 文档任务，不动 `internal/`。
- [ ] 0.2 Owner: baseline；Scope: 仓库基线；Dependencies: none；Lane: gate；记录 `note show --view rendered` 与 `pinax-database-view` fence 的 golden 行为并确认 `task check` 基线绿；Verification: `go test ./cmd/pinax -run 'NoteShow|Rendered' -count=1` 与 golden diff 为空；Expected: 既有渲染行为被测试锁定；Failure re-check: 区分既有失败与本 change 影响，不放宽旧 golden。
- [ ] 0.3 Owner: contract；Scope: proposal/design/spec 三件；Dependencies: none；Lane: A；固化 stable surface 分类：新 CLI 子命令与 MCP 工具为 additive experimental，`note` 族内不新增顶层命令；Verification: `rg -n 'additive|experimental' openspec/changes/pinax-rendered-view-v1/`；Expected: 兼容与回滚路径明确；Failure re-check: 补 consumer list 与回滚条目。

## 1. 合同与树构建

- [ ] 1.1 Owner: render-core；Scope: 新建 `internal/render/types.go`、`tree.go` 及测试；Dependencies: 0.3；Lane: A；定义 `render.view.v1alpha1` envelope、节点类型与锚点规则（heading slug 去重、fence 序号、link target）；Verification: `go test ./internal/render -run 'Envelope|Anchor|NodeType' -count=1`；Expected: 同输入树确定、锚点稳定、未知节点类型不 panic；Failure re-check: 固定 markdown fixture 逐节点比对。
- [ ] 1.2 Owner: render-core；Scope: `internal/render/display.go` 及测试；Dependencies: 1.1；Lane: A；实现 card/context/body 三级深度裁剪与 `truncated` 事实；Verification: `go test ./internal/render -run 'Display|Truncate' -count=1`；Expected: 默认深度不含完整正文、body 深度显式且可审计；Failure re-check: 对照 NoteDisplay 三级语义修正裁剪点。
- [ ] 1.3 Owner: render-core；Scope: `internal/render/redline.go` 及测试；Dependencies: 1.1；Lane: A；移植/复用 paneUnsafe 规则到树节点、面板与 badge，输出 `dropped_unsafe`；Verification: `go test ./internal/render -run 'Redline|Unsafe|PathLeak' -count=1`；Expected: 绝对路径与危险子串零泄漏；Failure re-check: 与 `internal/app` pane 红线测试逐条对表。

## 2. 引用与关系解析

- [ ] 2.1 Owner: links；Scope: `internal/render/wikilink.go` 及测试；Dependencies: 1.1；Lane: B；经 notelinks 投影解析 wikilink：resolved → 卡片引用（object_id/version/title/status），broken → 显式标记；Verification: `go test ./internal/render -run 'Wikilink|Broken' -count=1`；Expected: 不做路径推断、broken 不省略；Failure re-check: 用含别名/大小写差异的 fixture 对拍 notelinks 投影。
- [ ] 2.2 Owner: links；Scope: `internal/render/backlinks.go` 及测试；Dependencies: 1.1；Lane: B；装配 backlinks 面板：≤50 实体、title ≤160、ok/broken、`truncated`/`dropped_unsafe`；Verification: `go test ./internal/render -run 'Backlinks|Bounded' -count=1`；Expected: 与 pane backlinks 规则一致；Failure re-check: 超限输入全量扫描 unsafe 再截断（沿 pane 修正过的语义）。
- [ ] 2.3 Owner: refs；Scope: `internal/render/pinaxref.go` 及测试；Dependencies: 1.1；Lane: B；实现 `pinax://note/<id>?v=<rev>` 的生成与解析：固定版本精确解析、floating 带 `resolved_revision`+`stale`、漂移不换版、unresolved 显式；Verification: `go test ./internal/render -run 'PinaxRef|Floating|Stale|Unresolved' -count=1`；Expected: 漂移时 resolved 值不变仅标 stale；Failure re-check: 用双版本 note fixture 重放解析序列。

## 3. Fence 注册表

- [ ] 3.1 Owner: fences；Scope: `internal/render/fenceregistry.go` 与 database-view 迁移；Dependencies: 1.1、1.2；Lane: C；建立 fence 注册表接口并把既有 `pinax-database-view` 迁入，行为与 golden 完全一致；Verification: `go test ./internal/render ./cmd/pinax -run 'FenceRegistry|DatabaseView|Golden' -count=1`；Expected: 0.2 的 golden diff 为空；Failure re-check: 逐字段对拍旧渲染输出。
- [ ] 3.2 Owner: fences；Scope: `internal/render/fence_graph.go` 及测试；Dependencies: 3.1；Lane: C；实现 `pinax-graph` 局部图 fence：当前 note ±N 跳（默认 1、上限 2）、节点/边截断、`truncated`；Verification: `go test ./internal/render -run 'FenceGraph|Hops|Truncated' -count=1`；Expected: 截断确定、超限显式；Failure re-check: 用高联通 fixture 验证截断上限与排序确定性。
- [ ] 3.3 Owner: fences；Scope: `internal/render/fence_board.go` 及测试；Dependencies: 3.1；Lane: C；实现 `pinax-board` fence：project/subproject 只读投影、列与卡片 bounded；Verification: `go test ./internal/render -run 'FenceBoard' -count=1`；Expected: 缺失 project → 失败占位不省略；Failure re-check: 对拍 project board 投影字段。
- [ ] 3.4 Owner: fences；Scope: `internal/render/fence_timeline.go` 及测试；Dependencies: 3.1；Lane: C；实现 `pinax-timeline` fence：本 note ledger 事件 ≤100、op/ref/revision/time；Verification: `go test ./internal/render -run 'FenceTimeline|LedgerBounded' -count=1`；Expected: 与 pane history 规则一致；Failure re-check: 大 ledger fixture 验证截断与排序。
- [ ] 3.5 Owner: fences；Scope: 失败占位语义与未知 fence；Dependencies: 3.1-3.4；Lane: C；失败 fence 输出稳定错误码+下一步动作的占位节点；未知 fence 原样透传为 code 节点；Verification: `go test ./internal/render -run 'FenceFailed|FenceUnknown' -count=1`；Expected: 无静默省略、无未知 fence 推断；Failure re-check: 注入投影错误逐 fence 验证占位。

## 4. 资产与外部引用

- [ ] 4.1 Owner: assets；Scope: `internal/render/assets.go` 及测试；Dependencies: 1.1；Lane: D；vault 附件解析为 `pinax-asset://<object_id>` + mime 分类，路径不进投影；Verification: `go test ./internal/render -run 'Asset|Mime' -count=1`；Expected: 附件引用可往返解析回 manifest 记录；Failure re-check: 对拍 asset manifest 字段。
- [ ] 4.2 Owner: assets；Scope: `internal/render/external.go` 及测试；Dependencies: 1.1；Lane: D；`drivebridge://file/` 透传为 external_ref 状态卡；测试断言零网络/零 vault 外文件 IO；未知 scheme 原样保留；Verification: `go test ./internal/render -run 'External|NoIO|Literal' -count=1`；Expected: 不取字节、不调用 DriveBridge；Failure re-check: 用网络禁用测试环境重放。

## 5. CLI 与输出合同

- [ ] 5.1 Owner: CLI；Scope: `internal/cli`、`cmd/pinax` wiring 与 help tests；Dependencies: 1-4；Lane: E；实现 `note preview --view rendered [--format tree|json|html] [--display card|context|body]`：human 默认可读渲染、`--json` 全树、`--agent`/`--events`/`--explain` 同源投影；Verification: `go test ./cmd/pinax ./internal/cli -run 'NotePreviewRendered|MachineMode' -count=1`；Expected: 四模一致、无 ANSI 泄漏进 machine 输出；Failure re-check: 对照 cli-output-contract 模板修正 envelope。
- [ ] 5.2 Owner: CLI；Scope: `note ref` / `note ref resolve` 命令及测试；Dependencies: 2.3；Lane: E；实现 `--copy` 双格式（markdown 链接 + wikilink）、消歧列表、`resolve` 的 stale/unresolved 展示；Verification: `go test ./cmd/pinax -run 'NoteRef|Disambiguation|Resolve' -count=1`；Expected: 多命中不出错引用、复制失败显式失败；Failure re-check: 检查剪贴板不可用时的回退输出。
- [ ] 5.3 Owner: output；Scope: proof badge 装配与测试；Dependencies: 5.1；Lane: E；所有 render 输出带 object_id/version/revision/rendered_at badge 且与 vault 记录一致；Verification: `go test ./internal/render ./internal/output -run 'Badge|Proof' -count=1`；Expected: badge 值可核验；Failure re-check: 双版本 fixture 对拍 revision。
- [ ] 5.4 Owner: CLI-compat；Scope: help/golden 更新；Dependencies: 5.1、5.2；Lane: E；更新命令手册与 golden help，确认无既有命令改名/删除；Verification: `go test ./cmd/pinax -run 'Help|Golden' -count=1`；Expected: 旧 golden 除新增子命令外 diff 为空；Failure re-check: 不修改旧 golden 掩盖 breaking。

## 6. MCP 只读面

- [ ] 6.1 Owner: MCP；Scope: `internal/mcpserver` render 工具与 schema/tests；Dependencies: 5.1、5.3；Lane: F；新增只读 `pinax.render_note`：schema 与合同同源、默认非 body 深度、body 参数进审计；Verification: `go test ./internal/mcpserver -run 'RenderNote|ToolsList|BodyExposure' -count=1`；Expected: 默认不暴露 body、tools/list 兼容；Failure re-check: 检查 call routing 与 display 参数校验。

## 7. 证据、文档与收尾

- [ ] 7.1 Owner: evidence；Scope: `tools/testkit/integrationevidence` 新 profile `rendered-view-parity`；Dependencies: 5.1、6.1；Lane: G；覆盖 CLI/MCP 等价、红线、fence 失败、floating 漂移四个断言族，证据入 `temp/integration-test-runs/`；Verification: `go run ./tools/testkit/integrationevidence -profile rendered-view-parity`；Expected: 全部断言通过且证据含 summary/command/logs；Failure re-check: 逐断言保存 normalized projection 定位首个差异。
- [ ] 7.2 Owner: docs；Scope: 命令手册、[交互体验规格](../../../docs/product/rendered-preview-interaction-ux.md)、pinax-agent-router 路由表；Dependencies: 5.2、6.1；Lane: G；命令文档更新、router skill frontmatter 同步（走 evolutionary-change-policy 门）；Verification: `rg -n 'render_note|note ref' docs/commands .claude/skills/pinax-agent-router`；Expected: 新命令/工具可被路由发现；Failure re-check: router frontmatter 变更须有本 change 引用。
- [ ] 7.3 Owner: closeout；Scope: 设计稿勾选与归档验证；Dependencies: 7.1、7.2；Lane: G；更新 [Markdown 预览设计](../../../docs/product/markdown-rendered-preview-design.md) P1 状态并 `openspec validate pinax-rendered-view-v1 --strict --no-interactive`；Verification: `openspec validate pinax-rendered-view-v1 --strict --no-interactive`；Expected: strict 通过、任务全勾后按流程归档；Failure re-check: 未勾任务如实保留 external-gate 注记。
