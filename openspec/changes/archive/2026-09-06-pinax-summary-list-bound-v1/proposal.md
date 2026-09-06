# Pinax summary 列表行界对齐根仓 advisory

## Why

对齐根仓 `docs/workflows/ai-native-cli-output-contract.md` 的 List rendering advisory（随 eikona catalog change 引入的跨仓约定）。pinax human summary 列表存在两类偏差：

1. **cap 10 过紧且不一致**：`internal/output/render.go` 六处 `limit > 10`（plan tasks、named scalar、repair plans、project registry、subprojects、renderSummaryDataList），普通本地列表（如项目注册表）在 11+ 条时被折半隐藏；`renderSummaryNoteList` 却全量渲染。
2. **截断无提示**：六处中仅两处打印 `showing N/M`；named scalar、repair plans、project registry、subprojects 截断后静默。

机器模式（--json/--agent）本就完整（RenderWithOptions 直出 projection），不动。

## What Changes

- `summaryListLimit = 20`：六处统一（普通本地列表默认全量展示，如 12 项目的注册表不再被藏）。
- 截断提示补齐：全部六处截断时输出 `  showing N/M`（--json 逃生路径不变，advisory 的 "--all-style remedy" 在 pinax 即 --json）。
- 纯加法：机器 envelope/fact 键/命令面零变化。

## Impact

- 代码：`internal/output/render.go`（常量 + 六处 + 提示）。
- 测试：`internal/output/render_test.go` 新增 `TestSummaryListBoundShowsTwentyRowsWithHint`（25 项目 → 20 行 + showing 20/25；12 项目全量无提示）。
- spec delta：`configurable-output-rendering`（ADDED 人类列表行界 requirement）。

## Non-Goals

- 不加 --all flag（pinax 逃生路径是 --json，加 flag 波及全部命令面，另行评估）。
- 不动 sync preview（`syncPreviewLimit` 有 --limit 显式覆盖且属预览而非目录）、database board 每列 cap 5（布局约束非目录）、plan ops "N more entries" 提示（已有提示）。
- 不动 note list 全量渲染（全量本就是理想形态）。
