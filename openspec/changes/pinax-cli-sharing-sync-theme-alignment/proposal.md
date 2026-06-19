## Why

Pinax 的主线定位仍然是本地优先的 Agent CLI，但现有 publish/cloud/output 规格有三处漂移：

- 静态发布已覆盖 GitHub Pages/Wiki，但用户目标还包括 Gist、普通 HTTP endpoint 和本地预览分享。
- Cloud Sync 文档容易被读成 Pinax 仓库要承载云笔记后端；真实边界应是 CLI 驱动本地文件同步，server 只是可选传输。
- 部分规格仍写着默认 English human output，而子项目约定和实际用户需求是人类输出中文、机器协议字段英文。

本变更把这些边界收敛到同一主题：Pinax 是 CLI 工具；分享和同步都是 vault 的 delivery/transport surface，不是新的笔记真源；主题设计服务发布产物和 CLI 文档一致性。

## What Changes

- 扩展 `pinax publish` 支持 `github-gist` 和 `http` target，生成受扫描的 Markdown bundle 和 manifest。
- 新增 `publish serve` loopback preview，用于本地预览已构建产物。
- 扩展 deploy policy：`gist` 通过系统 `gh` CLI，`http` 通过受控 HTTP adapter，仍要求 `--yes` 和 receipt/hash/scan 校验。
- 修正文档和规格语言：默认 human 输出中文；`--json`、`--agent`、`--events`、错误码、facts 和 schema key 保持稳定英文。
- 明确 Cloud Server 是外部/可选同步传输能力；Pinax repo 只拥有 CLI client、协议、fake server/transport tests 和本地文件同步收敛，不把 vault 真源搬到 server。
- 完善主题设计表述：CLI 主题是中文可扫、低噪声；发布站点主题是本地资产、可审查、非营销页。

## Capabilities

### New Capabilities

- `pinax-cli-sharing-sync-theme-alignment`: 对齐 Pinax CLI 主题、分享 surface、Cloud Sync 边界和输出语言合同。

### Modified Capabilities

- `static-site-publishing`: 增加 Gist/HTTP/local preview delivery surfaces。
- `cli-output-contract`: 将默认 human 语言修正为中文，机器协议字段继续英文稳定。
- `pinax`: 明确 Pinax 是 CLI-first local-vault tool，cloud/server/publish 都不是 vault 真源。

## Impact

- CLI：`publish profile init --target github-gist|http`、`publish build`、`publish deploy --endpoint|--gist-id`、`publish serve`。
- App service：新增 Markdown bundle build、Gist deploy adapter、HTTP deploy adapter、loopback preview service。
- Domain/publishops：新增 publish targets、deploy modes、endpoint/secret ref/gist policy validation。
- Docs/OpenSpec：修正默认语言、cloud server 边界、publish sharing examples 和主题设计描述。
- Tests：新增 command tests 覆盖 fake `gh`、fake HTTP server、本地 serve smoke；不依赖真实 GitHub、真实 token 或公网。
