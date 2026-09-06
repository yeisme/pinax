# Tasks

- [x] 1. `summaryListLimit = 20` 常量 + 六处 `limit > 10` 统一（plan tasks / named scalar / repair plans / project registry / subprojects / renderSummaryDataList）
- [x] 2. 四处缺失的截断提示补齐（`  showing N/M`）：named scalar、repair plans、project registry、subprojects
- [x] 3. 测试：`TestSummaryListBoundShowsTwentyRowsWithHint`（25→20 行 + 提示；12 行全量无提示）；`go test ./internal/output/` 全绿
- [x] 4. 验证：go build/vet 绿；spec delta + strict 校验；pathspec 提交
