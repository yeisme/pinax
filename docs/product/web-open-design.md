# Pinax → DriveBridge 阅读消费交接

2026-10-01 用户确认一个 DriveBridge 客户端，桌面选 Electron，阅读先行并要求 macOS/Windows/Linux 实机验收。此文保留原路径以兼容链接，**取代此前把 `client/yeisme-workbench` 作为 Pinax UI 必需目的地的设计**；不恢复旧 Workbench，不新建独立 Pinax Reader。本轮只交规格，不代表新端点或桌面已实现。

## Ownership

| 能力 | 唯一 owner |
| --- | --- |
| Markdown vault、笔记身份、索引、双链、提案、proof gates、版本与恢复 | `cli/pinax` |
| 显式授权人读API、既有CLI/MCP/RPC与写入门 | `cli/pinax` |
| 桌面窗口、共享阅读器、首页/搜索/正文导航、系统集成与安装包 | `cli/drivebridge` 的 web/desktop |
| 文件权限、受管范围保护、收藏/稍后读/位置应用服务 | `cli/drivebridge`；只保存引用，不复制笔记真源 |
| 手机对话读取 | 既有 Hermes → 获授权的 Pinax MCP；本轮不改部署或工具权限 |
| 公开静态发布HTML | 既有 `web/pinax-web-renderer`，不是内部客户端 |

## 合同入口

新消费者以真实 transport manifest/binding/scopes 为准。现有发现命令如下，不以schema存在宣称planned端点可执行：

```bash
pinax api manifest --vault ./my-notes --json
pinax api routes --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json
```

新合同唯一真源：[人类阅读 API](../interfaces/human-reading-api.md)。旧render.view.v1alpha1的64KiB/512节点与Agent默认脱敏不变；新正文授权/分页/资产/session由 [pinax-human-reading-api-v1](../../openspec/changes/pinax-human-reading-api-v1/README.md) 实施，不能用CLI/MCP代替尚未有的HTTP backing。

DriveBridge侧入口：[Electron架构](../../../drivebridge/docs/architecture/electron-desktop.md)、[三屏UI](../../../drivebridge/docs/web/ui/knowledge-reading-spec.md)、[验收矩阵](../../../drivebridge/docs/delivery/desktop-knowledge-acceptance.md)。启动/绑定不init、refresh索引、sync或自动挂载；本机阅读无云登录/模型依赖，外部编辑器仍可直接编辑Markdown。

## 强制边界与后续

- 客户端不得读取 `.pinax/**`、SQLite/GORM、provider配置、私有索引、日志、receipt或任意vault路径；只能用公开合同和受权asset流。
- Pinax笔记不能通过DriveBridge普通文件写入绕过owner；显式受管绑定由DriveBridge application service保护，旧overlay不自动迁移。
- 首片只读正文；捕获、编辑、提案审阅、整理后续仍须scope、revision、review、snapshot与receipt，不提供MCP自批或文件覆盖替代。
- 不把token、raw payload、正文、私有路径或内部推理写进日志/测试证据；人类获授权内容显示不等于诊断可记录。
- 不新增React/Electron代码于Pinax，不扩成云笔记后端、不注册常驻daemon或公网监听。
- 静态publish继续既有输出合同，与内部reader分离；Pane与发布renderer迁移仍有各自门，不要求桌面首片先完成Pane。
- 本地工程、三平台、自然使用及原11项真实验收分别报告；文档strict通过不能替代这些证据。
