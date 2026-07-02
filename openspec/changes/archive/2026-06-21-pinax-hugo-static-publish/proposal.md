## Why

Pinax 已经能维护私有 Markdown vault、索引投影、双链、资产和 proof loop，但还缺少一条安全的“发布面”路径，把经过筛选和脱敏的百科内容发布到 GitHub Pages 或 GitHub Wiki。现在需要把 GitHub 视为静态发布目标，而不是 vault 真源，避免用户把完整私有 vault、`.pinax` 元数据、草稿、provider 证据或历史 secrets 误推到远端。

## What Changes

- 新增 Pinax 静态发布能力，用 `publish profile` 定义哪些 note、asset 和投影可以进入公开或共享站点。
- 新增发布预检与计划：`publish plan` 只读扫描 vault，列出将发布、跳过、阻断和需要人工审查的内容。
- 新增 Hugo 作为首选前端编译渲染路径：Pinax 生成 Hugo content/data/static 输入，Hugo 生成 GitHub Pages 可托管的静态站点。
- 新增 GitHub Pages 输出目标，支持生成到本地目录和可选部署到独立 Pages 仓库或 `gh-pages` 分支。
- 新增 GitHub Wiki 输出目标，生成 GitHub Wiki 兼容 Markdown 页面和附件引用，不包含复杂前端 runtime。
- 新增发布安全合同：默认不发布草稿、私有 note、`.pinax/**`、provider raw payload、token、Authorization/Cookie、绝对路径、未允许的完整 note body 或未纳入 allowlist 的资产。
- 新增发布收据、manifest 和 redaction evidence，保证发布产物可审计、可复现、可在 CI 中验证。
- 不把 GitHub Pages/Wiki 作为 Pinax vault 真源，不把 Cloud Sync、Remote API Mode 或 release GoReleaser pipeline 混入本变更。

## Capabilities

### New Capabilities

- `static-site-publishing`: 定义 Pinax 从私有 vault 生成经过 profile 筛选、脱敏和 Hugo 渲染的静态发布产物，并可部署到 GitHub Pages 或 GitHub Wiki 的行为合同。

### Modified Capabilities

- 无。

## Impact

- CLI：新增 `pinax publish profile|plan|build|deploy|doctor` 命令族，支持 `--json`、`--agent`、`--events` 和 `--explain` 输出合同。
- App service：新增发布用例编排，复用 note、search、link graph、asset、template/render 和 version 能力，不允许命令层直接读写复杂业务状态。
- Domain：新增 publish profile、publish plan、publish manifest、publish receipt、publish target、publish violation 等领域模型。
- Output：新增发布投影，默认中文摘要，机器输出保持稳定英文 key。
- Redaction：复用并扩展投影脱敏 gate，新增静态产物和 Hugo 输入目录的递归泄漏扫描。
- Storage/Version：构建输出只写指定 `--out` 目录或 CLI-authored `.pinax/publish/**` receipt；部署前建议或要求 snapshot，deploy 只作用于发布仓库，不修改私有 vault 正文。
- Dependencies：引入外部 `hugo` CLI 作为可选构建依赖；未安装时 `publish doctor` 和 `publish build --target github-pages` 返回结构化错误和安装提示。
- Tests：新增 contract tests、testscript e2e、fake git remote/fake executable、fixture vault、泄漏扫描、Hugo unavailable/available 路径、Pages/Wiki 输出 golden 验证。
