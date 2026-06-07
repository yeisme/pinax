# pinax-core-notes-templates-search-sync

## 背景

Pinax 已具备本地 vault 初始化、校验、笔记读取、metadata/organize 计划、项目 registry、storage profile 和只读 MCP surface。后续核心 MVP 必须补齐传统 Markdown 笔记软件的日常工作流：新建笔记、模板、Obsidian 兼容链接和标签、高性能检索、本地索引投影，以及可审查的同步计划。

用户明确要求文本化跨平台管理，并同时支持 S3、Git 和 Pinax 云后端。根据仓库子模块规则，云后端实现需要独立 `backend-server/pinax-cloud` 子模块和远端仓库；本 change 先完成 CLI 侧协议、状态边界和 handoff，不在 Pinax CLI 仓库中伪造后端服务源码。

## 变更范围

- 增加 `pinax note new`，创建带 YAML frontmatter 的 Markdown 笔记，并支持项目路径、标签和模板。
- 增加 `pinax template init/list/show/render`，提供 Markdown 模板、YAML frontmatter 模板、Mermaid 模板和日记/项目基础模板。
- 增强检索为 `rg` + SQLite/GORM 索引投影混合模型：全文优先走 `rg`，结构化标签、链接、frontmatter、项目归属进入本地 SQLite index。
- 增加 `pinax index rebuild` 与 backlink/tag 投影，用于 Obsidian 核心格式兼容。
- 增加 `pinax sync diff/push/pull` 的本地计划和审批门禁，目标覆盖 `git`、`s3`、`cloud`，默认不做远端写入。
- 为 Pinax 云后端补 CLI handoff：cloud 目标暴露后端所需 API、状态文件和阻塞条件。

## 非目标

- 本 change 不实现真实 S3 对象上传下载和冲突合并算法，只落地 CLI 状态、计划和审批边界。
- 本 change 不在当前仓库创建 `backend-server/pinax-cloud` 源码；该项目必须由根仓库以独立 Git submodule 接入。
- 本 change 不实现完整 Obsidian 插件生态、加密、协同编辑或移动端 UI。

## 验证

- `openspec validate --all`
- `gofmt -w <changed-go-files>`
- `go test ./...`
- `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`
