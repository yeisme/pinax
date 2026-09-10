# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] 1.1 核实官方 SDK 版本、现有协议差异与能力保留清单 | evidence: design.md「1.1 调研核实」；SDK v1.7.0 module cache 源码级核对（协议版本集合 2026-07-28/2025-11-25/2025-06-18/2025-03-26/2024-11-05 与本仓一致、go 1.25 门满足），差异表与能力保留清单（20+6+6 工具、3+6+协作/input 资源）成文
- [x] 1.2 完成 SDK 分层设计、迁移及真实客户端验收合同 | evidence: design.md「1.2 分层设计、迁移路径与验收合同」；dispatch 单点经 Server.Handle、默认 runtime 不变、4.2 外部门不执行、4.4 前不切默认等迁移与回滚约束已冻结
- [x] 2.1 接入官方 Go SDK candidate 与统一注册目录 | evidence: temp/integration-test-runs/20260910T191028Z-2676955/summary.json；internal/mcpserver/sdkruntime 经 mcpserver 导出 inventory 复用统一注册目录，PINAX_MCP_RUNTIME=official-sdk 显式开关，默认 runtime 不变；进程级 e2e TestMCPOfficialSDKRuntimeCandidateProcess 通过
- [x] 2.2 迁移工具资源、schema 校验、错误及脱敏边界 | evidence: strict client temp/integration-test-runs/20260910T191047Z-2682741（TS SDK 1.30.0 双模式 protocol_errors=0）；schema/错误经 Server.Handle 单点同源，invalid-args -32602 与 unknown-tool 差异已冻结 design.md；SDK logger 丢弃防正文入日志
- [x] 2.3 实现取消、并发与持久写入恢复验证 | evidence: 同 20260910T191028Z-2676955；并发 8 路 tools/call、取消请求返回错误、preview→apply→status→version 同 operation_id 恢复闭环测试通过，go test -race 绿
- [x] 3.2 完善 Markdown 阅读、审阅、导入和回执交互 | evidence: strict client collaboration=true（Markdown search/preview/apply/search 回读）+ TestSDKRuntimeInputIntakeToolsWired（pinax.input.* 导入工具与 capabilities 资源）+ pinax://interaction/* 资源模板可发现；全部经 candidate runtime
- [x] 4.1 独立 SDK 与 Inspector 协议自动验收 | evidence: 独立 SDK=temp/integration-test-runs/20260910T191047Z-2682741（官方 TS SDK 1.30.0 strict client）；Inspector v2 CLI 三方法 temp/integration-test-runs/20260910T191223Z-2713262、20260910T191229Z-2715309、20260910T191236Z-2716969（tools/list、resources/list、tools/call 全 passed）；见 docs/implementation/mcp-official-sdk-verification.md
- [ ] 4.2 Codex、Claude Code、Grok、Kimi Code 真实会话验收
- [ ] 4.4 完成兼容窗口、全局门禁及默认 SDK 切换
