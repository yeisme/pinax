# 任务

- [x] 生成本变更 OpenSpec，并记录兼容窗口、回滚和数据安全边界。
- [x] 用兼容提示层替换 `pinax kb` 的向量实现。
- [x] 删除 Pinax semantic、Inferrum/LanceDB、provider、sidecar 和 KB generation/evaluation/review 代码。
- [x] 删除 `kb.sidecar.*` 配置、环境变量、Go module/vendor 依赖。
- [x] 将旧 review API 改为 HTTP 410 `kb_decoupled`，保留一个迁移窗口。
- [x] 清理 sidecar canary、Taskfile 质量门和 KB provider 测试。
- [x] 清理仓库和测试工作区中明确枚举的 `.pinax/kb/**` 历史产物；保留 vault、同步配置、加密 revision 和对象存储数据。
- [x] 【2026-08-15 决策：立即删除兼容层】删除 `pinax kb` 兼容命令树与 `kb.context`/`kb.review.*` capability；旧命令返回 unknown command，CHANGELOG 记 breaking change。
- [x] 删除 `/v1/kb/review/*` HTTP 410 路由、middleware/cache kb 特判、`config set kb.*` 特判拒绝（残留 `kb.*` YAML 键改为惰性忽略）。
- [x] 清理残留：release-convergence spec 的 sidecar 质量门要求、agent-memory-ledger 的 KB semantic search 措辞、agent-safe-boundary 的 embedding/rerank 披露要求、memory scorer 死 "kb" topic、general-agent-memory-prd 的 vector index 说法、kb 文档收敛为单一移除说明。

## 验证

```bash
CGO_ENABLED=0 go test ./internal/config ./internal/cli ./internal/api ./cmd/pinax -run 'Config|KB|HTTPRoutes|Routes' -count=1
CGO_ENABLED=0 go test ./internal/app ./internal/index ./internal/memory ./internal/sync -count=1
CGO_ENABLED=0 go build ./cmd/pinax
openspec validate pinax-decouple-kb-vector-rag --strict
```
