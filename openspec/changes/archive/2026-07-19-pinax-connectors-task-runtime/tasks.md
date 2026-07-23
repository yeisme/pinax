## 1. 运行时路由

- [x] 1.1 owner=`internal/app/taskbridge_planning.go`：实现 Connectors 优先、独立 TaskBridge 仅在可执行文件缺失时回退；验证 `go test ./internal/app -run 'TestTaskBridge' -count=1` 返回成功，失败后修正路由或 fixture 并重跑同一命令。
- [x] 1.2 owner=`internal/app/service_plan.go`：把新 action draft 的下一步改为 `connectors task agent execute --dry-run` 且禁止 `--confirm`；验证 `go test ./internal/app -run 'TestPlanActions|TestActionDraft' -count=1` 返回成功，失败后检查 projection 并重跑。

## 2. 合同与帮助

- [x] 2.1 owner=`internal/cli/ops_integration_cmd.go`：保留 `--taskbridge` 参数并在帮助中说明 Connectors 提供兼容合同；验证 `go test ./internal/cli/... -count=1` 返回成功，失败后检查 help golden/断言并重跑。
- [x] 2.2 owner=`openspec/changes/pinax-connectors-task-runtime/`：记录 schema、错误码、回退边界、迁移和回滚；验证 `openspec validate pinax-connectors-task-runtime --strict` 返回成功，失败后按诊断修正规范并重跑。

## 3. 稳定验证

- [x] 3.1 owner=`cli/pinax`：运行 `go test ./...` 与 `go build ./cmd/pinax`，预期全部成功；失败时只修复本变更引入的问题并重跑失败命令。
- [x] 3.2 owner=`cli/pinax`：运行 `git diff --check` 并确认没有新增对独立 TaskBridge store、Provider token 或 Provider API 的访问；失败时修复空白或边界违规后重跑。
