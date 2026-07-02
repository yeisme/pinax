# Pinax 文档地图

[中文文档地图](./README.zh-CN.md)

`docs/` 是 Pinax 子项目的产品、设计、运行、协议、实现、QA 和 release 文档真源。根仓库只保留跨项目 handoff 和治理索引，不维护 Pinax 文档镜像。

Pinax 是 **面向 Markdown vault 的 agent-safe 知识控制平面**。三个核心概念：Markdown vault 是用户知识资产的真源，Proof Loop 保护每一次 agent 写入，Cloud Sync 只协调密文。SQLite/GORM 是可重建索引投影，version backend 只提供版本证据和受保护工作流的 snapshot basis，外部平台通过 CLI-backed Provider adapter 接入。

核心保障见 [Agent-Safe Boundary](./overview/agent-safe-boundary.md)：读取命令默认返回 bounded projection，不返回完整 note body；MCP 工具只读；云端不保存明文，也不执行本地工具。

## Agent-Safe 证明循环

Pinax 的主要用户价值和 agent 价值，是围绕真实本地 vault 的可复现 proof loop。每个阶段都保持 bounded：projection 只返回事实和下一步，不返回完整 note body、token 或 provider payload；写入只能通过 plan -> snapshot -> apply -> receipt -> restore 控制链发生。

- [Demo Proof Loop](./demo-proof-loop.md)：复制合成 messy vault fixture，端到端运行 diagnose -> plan -> snapshot -> apply -> restore。
- [文档设计](./overview/documentation-design.md)：说明读者路径、章节归属、命令文档形态和 Pinax 文档维护规则。

## 当前状态

- 当前阶段：本地优先 notebook workflow 已可通过 CLI 使用，适合外部开发者评估。
- 当前实现边界：支持 local init、vault validate、daily/inbox/draft、note add/create/list/read/edit/rename/move/archive/delete/tag、共享 `NoteDisplay`、project workspace/board、task adoption plan、长期学习项目初始化、组织维度浏览、database saved views 的 table/board/list/calendar render、saved-view Markdown tabs、SQLite/GORM index、search、`pinax note links`/`pinax note backlinks`/`pinax note orphans`、`search --link-target`、attachments、Markdown import/export、template create/render/validate/delete、metadata plan/apply、repair plan/apply、agent organize plan/list/apply、version snapshot、asset manifest registration/validation/planning、read-only dashboard repair/database-tab views、read-only MCP、localhost REST/RPC projection adapter，以及 server/file/S3/rclone Cloud Sync transport。Obsidian-style vault compatibility 是 preview。
- 用户可见 note path 使用 vault-relative canonical path。默认普通 note 是根级 `foo.md`，子目录 note 是 `work/foo.md`；历史 `notes/foo.md` 只作为 resolver-compatible 输入，不是 CLI、JSON、agent、record、search 或 MCP 的主要输出。
- 计划和实现跟踪放在 `openspec/`；外部贡献者先读 [CONTRIBUTING.md](../CONTRIBUTING.md)。

## 双向关系入口

- `pinax note links <ref>` 显示 outgoing links，支持 `--broken-only`、`--kind`、`--include-ignored` 和 `--limit`。
- `pinax note backlinks <ref>` 显示 backlinks，支持 `--include-broken` 和 `--limit`。
- `pinax note orphans --mode full|no-incoming|no-outgoing` 分别显示完全孤立 note、无 incoming link 的 note、无 outgoing link 的 note。
- `pinax search <query> --link-target <note-id|path|title|raw-target>` 按关系目标过滤搜索结果；目标有歧义时返回 `link_target_ambiguous`，不会自动替用户选择候选项。
- SQLite/GORM index 只是可重建投影。先用 `pinax index --vault <vault>` 查看摘要；当 `index_status=missing|stale` 时，优先运行 `pinax index refresh --vault <vault>`；遇到结构异常时先用 `pinax index doctor --vault <vault>` 查看问题，再按提示显式执行 `rebuild`。不要手写 `.pinax/*.json` 或 index metadata。
- `repair plan`、`organize plan --save` 和 dashboard 只为 broken/ambiguous/orphan 项生成手工 review 建议；MCP relationship tools 是只读工具，不写 vault、`.pinax/`、Git、provider 或 remote state。

## 版本和资产边界

- Version backend 是版本证据来源，不是用户内容真源。当前主路径是 `pinax version status/snapshot/history/diff/show/changed/restore/backends`；Git 只是可选 backend type 和隐藏兼容 alias，不是用户可见工作流名称。
- Asset manifest 是 CLI-authored metadata，用来登记 vault-relative path、media type、hash、linked note 和 validation status；asset 文件本身仍是 vault 内普通可迁移文件。manifest 和 SQLite index 都可由 CLI repair/rebuild，不应手写。
- Asset payload、raw diff、provider payload、webhook token、secret ref、Authorization/Cookie 等敏感内容不得进入 stdout、stderr、event、record log 或 fixture。

## 项目看板和远程适配器

- `pinax project board show|plan|configure|export` 提供本地 project workspace。看板由 Markdown note、project metadata、index projection 和 saved planning snapshot 生成；vault 仍是真源，TaskBridge 和 provider 都不是真源。
- `pinax project learning init` 会为长期学习项目创建或复用 project、subproject workspace、learning board columns、starter notes 和 starter work items，例如 stock-learning notes。
- `pinax project item add|move|archive` 通过 application service 写入受控 Markdown。archive 必须先有 `--yes` 和 version snapshot；缺失时返回稳定的 `approval_required` 或 `snapshot_required` projection。
- `pinax task adopt <item> --plan` 只预览 inferred checklist task adoption；只有 `--yes` 才写 task adoption ledger。
- `pinax database view save|render` 存储 query/view 配置，并返回 bounded table、board、list、calendar 或 database-tab projection。Markdown `pinax-database-view <name>` fences 由 app service 渲染，不让 client 解析 `.pinax/**`，也不持久化 result rows。
- `pinax note read/show --display card|detail|context|body`、project board、dashboard、MCP、REST 和 RPC 共用同一个 `NoteDisplay` projection；默认 bounded display 不输出完整 body。
- `pinax api routes`、`pinax api status`、`pinax api schema export` 和 `pinax api serve --readonly --port 0` 是 local REST/RPC projection adapter。服务默认绑定 `127.0.0.1`，不提供 public hosted API、CORS、TLS、多用户权限或 token auth。
- Client CLI parity 和 realtime sync 是同一边界下的两条链路：Remote API Mode 让 CLI client 和本地工具通过已注册 capability 操作一台服务端 vault；`pinax sync daemon` 让多个本地 vault 通过加密 Cloud Sync revision 收敛。详见 [客户端 CLI 覆盖和实时同步说明](./interfaces/client-cli-parity-and-sync.md)。
- `pinax prompt` 存储可复用的 `yeisme.prompt_asset.v1` prompt assets，解析 `pinax://prompt/<id>` 引用，记录 Pinax-owned lifecycle decision，并导入 Eikona 等工具的 metadata-only usage feedback。
- Cloud Sync 是独立分布式同步设计：每台设备保留本地 vault，Cloud backend 协调 encrypted revision、blob 和 conflict。详见 [Cloud Sync Architecture](./architecture/cloud-sync-design.md)。

## 文档入口

- [Agent-Safe 边界](./overview/agent-safe-boundary.md)
- [文档设计](./overview/documentation-design.md)
- [长期资料源笔记](./overview/durable-source-notes.md)
- [产品定位](./overview/product-positioning.md)
- [架构边界](./architecture/architecture-boundaries.md)
- [Cloud Sync Architecture](./architecture/cloud-sync-design.md)
- [Go Development Ecosystem Design](./architecture/go-development-ecosystem.md)
- [CLI Output Contract](./interfaces/cli-output-contract.md)
- [Local REST/RPC Contract](./interfaces/remote-api-contract.md)
- [客户端 CLI 覆盖和实时同步说明](./interfaces/client-cli-parity-and-sync.md)
- [Demo Proof Loop](./demo-proof-loop.md)
- [命令手册](./commands/README.md)
- [本地开发运行手册](./operations/local-development.md)
- [Release Packaging](./operations/release-packaging.md)
- [中文文档地图](./README.zh-CN.md)
- [贡献指南](../CONTRIBUTING.zh-CN.md)
- [安全策略](../SECURITY.zh-CN.md)

## 命令手册

- [命令地图](./commands/README.md)：说明每个 root command 所属工作流。
- [prompt](./commands/prompt.md)：说明 prompt asset lifecycle、`pinax://prompt/<id>` 解析、跨项目边界和 feedback import。
- [organize](./commands/organize.md)：说明整理流程、写入边界和 `pinax organize plan/list/apply` 的 snapshot 保护。
- [version](./commands/version.md)、[asset](./commands/asset.md)、[index](./commands/index.md) 和其他 root commands 在 [命令手册](./commands/README.md) 中维护独立页面。

## 验证入口

只改文档时通常不需要运行 Go 测试。修改 Go 代码后运行：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
```

如果安装了 Task，也可以运行：

```bash
task check
task release:check
```

发布或交接 release artifact 前运行：

```bash
task release:package:validate
```

Package validation target 会以 snapshot/no-publish 模式运行 GoReleaser，验证 checksums，smoke 一个解压后的 archive，检查 SBOM artifact，并在 Linux package inspection 工具不可用时给出明确跳过信息。
