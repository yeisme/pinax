# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] 1.1 冻结增量合同与原型验证范围 | evidence: specs/pinax-mcp-collaboration/spec.md 与 design.md；增量合同与客户端边界已记录
- [x] 1.2 实现 application service 正文读取、预览与可恢复写入 | evidence: temp/integration-test-runs/20260910T065322Z-3687933/summary.json；race 与持久化恢复引用通过
- [x] 1.3 实现 MCP 能力发现、Markdown 和受限动作 | evidence: temp/integration-test-runs/20260910T080427Z-477909/summary.json；HTML 资源与元数据已撤回，严格 SDK 普通工具通过
- [x] 1.5 验证完整流程、失败恢复和旧客户端兼容 | evidence: temp/integration-test-runs/20260910T065816Z-3812270/summary.json；日常闭环、输入及失败路径通过
- [x] 1.6 完成文档、质量检查与客户端证据归档 | evidence: temp/integration-test-runs/20260910T184839Z-2036941/summary.json；input-intake 遗留文件迁移统一 DSN 并清 errcheck/goimports 后 task check 全量通过，反链失败未复现；文档与 strict SDK 客户端证据已归档于 docs/implementation/mcp-collaboration-verification.md
