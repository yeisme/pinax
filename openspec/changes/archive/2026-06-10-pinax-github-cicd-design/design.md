# Pinax CI/CD 架构设计

## CI 触发机制
考虑到 Pinax 子项目以 `cli/pinax` 作为工作根目录，我们将 `.github/workflows/pinax-ci.yml` 布置在根级，但将其 `working-directory` 缩小至 `cli/pinax`，同时只监听 `paths: ["cli/pinax/**", ".github/workflows/pinax-ci.yml"]`。

## 质量门禁矩阵
1. **依赖对齐**: 安装 `go-task`, 验证 go.mod & go.sum
2. **代码风格**: `gofmt` (`task fmt-check`)
3. **单元分析**: `golangci-lint` (利用 GitHub Action 提供的高效缓存)
4. **单元/集成测试**: `go test` (通过 `task test`)
5. **项目规范**: 设定 Node 环境，下发 `@fission-ai/openspec` 然后执行 `openspec validate --all` 验证规范遵守。
6. **产物编译**: `task build` 及 `goreleaser check` 验证打包合法性。

## 发版发布流 (CD)
遵循 monorepo / multi-tag 约定，监听 `push: tags: "pinax/v*.*.*"`。
发版通过 `goreleaser` 实现快照与实际发布的打包分离。由于 Pinax 是 CLI Agent 类项目，我们将构建轻量可复用架构。
