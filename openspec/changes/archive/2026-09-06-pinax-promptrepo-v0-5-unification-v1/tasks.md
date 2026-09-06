# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] dependency 升级 promptrepo v0.5.0 并保持旧合同 | evidence: go.mod/go.sum/vendor pin promptrepo v0.5.0; focused tests and CGO_ENABLED=0 build passed
- [x] official 统一官方来源与共享状态根说明 | evidence: official source and shared roots documented
- [x] verify 运行聚焦测试和 OpenSpec 严格验证 | evidence: promptbridge/app/cmd tests and shared-store preview passed; OpenSpec strict valid
