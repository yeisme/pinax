# 实施与验收任务

- [x] dependency 升级 promptrepo v0.5.1 并保持旧合同 | evidence: go.mod/go.sum pin promptrepo v0.5.1；vendor 全量刷新；`go test ./... -count=1` 全绿、`go build ./...` 通过
- [x] contract 更新 `pinax-prompt-repository-import` 合同到 v0.5.1 | evidence: MODIFIED delta 将消费版本要求从 v0.5.0 提升到 v0.5.1
- [x] verify 运行聚焦测试与 OpenSpec 严格验证 | evidence: prompt bridge/import 相关测试通过；`openspec validate pinax-promptrepo-v0-5-1-consume --strict` 通过
