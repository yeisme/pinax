# Pinax

Pinax 是本地优先的统一笔记 Agent CLI。Markdown vault 是用户知识资产真源，`.pinax/` 保存由 CLI 或 application service 创建的配置、索引、映射、事件和审计投影。

当前仓库状态是开发底座：已建立 Go CLI 入口、子项目文档入口、OpenSpec 工作流和 skills 运行配置。业务能力实现必须先进入 `openspec/changes/<change-id>/`，再按任务验收落地。

## 本地验证

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
```

## 文档入口

- [子项目指令](./AGENTS.md)
- [文档地图](./docs/README.md)
- [OpenSpec](./openspec/config.yaml)

