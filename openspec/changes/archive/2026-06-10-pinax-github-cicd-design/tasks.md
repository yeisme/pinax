# Pinax GitHub CI/CD 设计强化任务

1. [x] 设计目标与背景 ( proposal.md )
2. [x] 在根目录建立 `pinax-ci.yml` 工作流
   - 依赖 Node 20 获取 `openspec` 验证
   - 运行项目特有的 `task ci` 质量门禁 (lint / mod check / test / build)
3. [x] 在根目录建立 `pinax-release.yml` 发版工作流
   - 监听 `pinax/v*.*.*` tag，集成 goreleaser
4. [x] review 并测试这些 Action 流程
   - 证据：CI 工作流使用 `actions/checkout@v4`、`actions/setup-go@v5`、`actions/setup-node@v4`、`arduino/setup-task@v2`、`golangci/golangci-lint-action@v6`、`goreleaser/goreleaser-action@v6`；路径过滤 `cli/pinax/**`，working-directory `cli/pinax`。
   - 证据：Release 工作流使用 `pinax/v*.*.*` tag 过滤，goreleaser snapshot 和 publish 分离。
   - 证据：新增 `.goreleaser.yml` 最小配置，支持 linux/darwin/windows 多平台构建。
   - 证据：本地 `task check` 通过，`task ci` 等价完整质量门禁。
