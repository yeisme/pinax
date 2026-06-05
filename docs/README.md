# Pinax 文档地图

本目录是 Pinax 子项目的产品、设计、运行、协议、实现、QA 和 release 文档真源。根仓库只保留跨项目 handoff 和治理索引，不维护 Pinax 文档镜像。

Pinax 是本地优先统一笔记 Agent CLI：Markdown vault 是用户真源，SQLite/GORM 是可重建索引投影，Git 是版本与回滚层，外部平台通过 CLI-backed Provider adapter 接入。

## 当前状态

- 当前阶段：子项目开发底座已建立。
- 当前实现边界：只保留最小 CLI 骨架和 OpenSpec 工作流；业务能力必须先进入子项目 OpenSpec change。
- 根级设计来源：`openspec/specs/pinax-project-routing/spec.md`、`openspec/changes/pinax-daily-hot-notes-briefing/`。

## 文档分区

- [产品定位](./overview/product-positioning.md)
- [MVP 范围](./product/mvp-scope.md)
- [架构边界](./architecture/architecture-boundaries.md)
- [Go 开发生态设计](./architecture/go-development-ecosystem.md)
- [CLI 输出合同](./interfaces/cli-output-contract.md)
- [运行手册](./operations/local-development.md)

## 验证入口

只改文档时不默认跑 Go 测试。修改 Go 代码后执行：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
```

如果已安装 Taskfile，也可以运行：

```bash
task check
```
