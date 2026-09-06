# Design

## Context

pinax human summary 的列表渲染散布六处 `limit > 10` 字面量，提示只有两处。渲染层集中收口在 `RenderWithOptions`（所有命令经 domain.Projection 自动获得全部模式），因此行界调整只动 `internal/output/render.go` 一处文件。

## Decisions

### D1 常量 `summaryListLimit = 20`

六处字面量统一为常量（含注释指回根仓 advisory）。选 20：普通本地列表（项目注册表、plan tasks、repair plans、subprojects）典型量级 ≤20，默认全量；真正的长尾（activity 等）保持有界。

### D2 提示统一 `  showing N/M`

在 limit 计算后、表格渲染前输出（与既有 renderSummaryDataList 位置一致）。M = 完整长度（来自 projection data，机器模式同源）。--json 是 pinax 既有的完整数据逃生路径，advisory 的 remedy 条款以此满足。

### D3 范围排除

- sync preview：`syncPreviewLimit` 由 `--limit` 显式控制且输出 `changes_shown`，属预览语义。
- database board 每列 cap 5：布局约束，且已有 "... N more, use --json" 提示。
- plan operations 的 "N more entries; use --json to view the full plan"：已有提示。

## Risks / Trade-offs

- 20 行在窄终端偏长——人类摘要的可扫读性由 summary cell 列宽截断兜底（本就存在），行数不是唯一因子。
- 六处行为同时变化可能影响依赖截断后行数的 golden 测试——检索无此类断言（grep showing/limit>10 于 tests 为零命中）。

## Migration Plan

纯加法 + 人类文本；无机器契约影响。
