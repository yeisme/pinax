## Why

TaskBridge 的运行时能力已经迁入 Connectors，但 Pinax 仍直接依赖独立的 `taskbridge` 可执行文件，阻碍旧子项目删除。现在需要把 Pinax 切换到 Connectors 的任务命令入口，同时保留现有 `--taskbridge` 参数、错误码与 `taskbridge.*.v1` 数据合同，避免用户工作流和已保存资产发生无迁移的破坏。

## What Changes

- Pinax 任务规划优先调用 `connectors task agent today` 获取任务事实。
- 在兼容窗口内，仅当 `connectors` 不可用时回退到 `taskbridge agent today`。
- 保存 action draft 后的建议执行命令改为 `connectors task agent execute --action-file <path> --dry-run`。
- 保留 `--taskbridge` 参数、`TASKBRIDGE_UNAVAILABLE`、`TASKBRIDGE_CONTRACT_UNSUPPORTED` 与 `taskbridge.agent-result.v1` / `taskbridge.today.v1` / `taskbridge.actions.v1` 合同。
- 使用 fake executable 验证 Connectors 优先级、兼容回退、失败只读和 action draft 安全门禁。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `planning-workflows`: 将任务事实和 action draft 的执行入口从独立 TaskBridge 二进制迁移到 Connectors，并定义兼容回退边界。

## Impact

- 代码：`internal/app/taskbridge_planning.go`、`internal/app/service_plan.go`、相关测试与 CLI 帮助。
- 合同：保留既有 Pinax 参数、错误码与 TaskBridge schema；仅迁移底层命令入口和建议命令。
- 外部依赖：Connectors 成为首选运行时；独立 `taskbridge` 仅在迁移窗口内作为可选兼容二进制。
- 根级来源：`backend-server/connectors/openspec/changes/connectors-absorb-taskbridge-runtime/`。
