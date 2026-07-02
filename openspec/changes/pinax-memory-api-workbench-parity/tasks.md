# 任务

- [x] 注册 memory capabilities 与 REST/RPC route 元数据，保留 `memory.link/prune` 未实现状态。
- [x] 为 REST/RPC 写入最小失败测试，覆盖 list、capture dry-run、capture confirmed write、recall/context/stats。
- [x] 实现 `/v1/memory*` REST handler 与 `Pinax.Memory.*` RPC dispatcher。
- [x] 对齐远程 CLI 映射，使 `pinax --api-url ... memory ... --json` 可转发到 Local API。
- [x] 增加 `/workbench` memory 页面和 capability explorer，保持 `vault dashboard` 只读。
- [x] 更新 memory/API/remote contract 文档和 OpenSpec delta。
- [x] 运行聚焦测试与 OpenSpec 校验，记录失败原因或通过证据。

## 验证记录

- `go test ./internal/app ./internal/api ./internal/cli -run 'Memory|RemoteCapabilities|RoutesMatchRegistry|RemoteModeMapsMemory' -count=1`
- `go test ./internal/api -run TestLocalAPIWorkbenchPageExposesMemoryCapabilityAndExpandableInspector -count=1`
- `go test ./internal/app ./internal/api ./internal/cli -run 'Memory|RemoteCapabilities|RoutesMatchRegistry|RemoteModeMapsMemory|WorkbenchPage' -count=1` 通过。
- `go test ./internal/app ./internal/api ./internal/cli -count=1` 通过。
- `openspec validate pinax-memory-api-workbench-parity --strict` 通过。
- `task check` 通过，包含 `go test ./...`、`openspec validate --all`、`golangci-lint fmt --diff`、`golangci-lint run`、sidecar protocol tests、`go build -trimpath`。
