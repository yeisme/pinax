## 1. 行为测试

- [x] 1.1 为 `pinax folder create/list/show` 增加 CLI 流程测试，覆盖 JSON/agent 输出、unsafe path 拒绝、空目录 registry 和 index facts。
- [x] 1.2 为 `pinax folder rename/move/delete/adopt` 增加 CLI 流程测试，覆盖 dry-run、approval、snapshot gate、冲突预检和无半写状态。
- [x] 1.3 为 REST folder routes 增加 handler 测试，覆盖 route registry、OpenAPI export、readonly write_disabled、approval_required、snapshot_required 和 no direct filesystem mutation。
- [x] 1.4 为 RPC folder methods 增加 dispatcher 测试，覆盖 REST/RPC capability metadata 对齐和 projection envelope 合同。
- [x] 1.5 为 folder index projection 增加 service/index 测试，覆盖 empty folder、rename 后 note path/folder 更新、delete 后 projection 清理。

## 2. 实现

- [x] 2.1 新增 folder domain models、folder registry 读写和 redacted event evidence，registry 只能由 service 写入。
- [x] 2.2 新增 application service folder read/plan/apply 方法，统一处理 vault boundary、unsafe path、conflict、approval、snapshot gate、幂等 create 和 hook events。
- [x] 2.3 新增 `internal/cli` folder command factory，接入 `create/list/show/rename/move/delete/adopt/repair --plan`；`note folders` 继续保留为 note 维度入口。
- [x] 2.4 新增 REST/RPC folder capabilities、route registry、OpenAPI schema metadata、handler 和 dispatcher 接线。
- [x] 2.5 扩展 API serve write gate，默认 readonly，显式 `--allow-write` 才允许 mutation route 进入 service apply。
- [x] 2.6 扩展 index folder projection 或 folder refresh path，写入后更新 index facts，失败时返回 partial/stale action。
- [x] 2.7 更新 CLI help、remote API docs 和输出合同文档，示例统一使用 `pinax folder ...`。

## 3. 验证

- [x] 3.1 运行 folder CLI 聚焦测试：`go test ./cmd/pinax -run 'TestFolder.*CLI' -count=1`。
- [x] 3.2 运行 folder service/index/API 聚焦测试：`go test ./internal/app ./internal/index ./internal/api -run 'Test.*Folder' -count=1`。
- [x] 3.3 运行输出合同相关测试：`go test ./cmd/pinax ./internal/output ./internal/api -count=1`。
- [x] 3.4 运行 `task check`，覆盖 fmt、lint、全量测试、build 和 `openspec validate --all`。

  - 2026-06-09 已运行；fmt/lint、`go test ./...`、build 通过，`openspec validate --all` 因无关变更 `pinax-github-cicd-design` 缺少 delta 失败。本变更 `pinax-folder-cli-api-operations` 单独 validate 通过。
  - 2026-06-09 已补齐 `pinax-github-cicd-design` delta 后重新运行 `task check`，fmt/lint、`go test ./...`、build 和 `openspec validate --all` 全部通过。
