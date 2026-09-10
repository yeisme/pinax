## Context

保留当前工作树的 input-intake 与研究检索改动，在同一 checkout 完成本地实现，因为新交互需要消费尚未提交的输入能力。仅根 agent 写入。旧 MCP 的 bounded projection 和 JSON 合同继续有效。

2026-09-10 用户明确取消 HTML/MCP Apps 能力。移除未发布的 ui://pinax/collaboration-v1.html 资源、工具 UI 元数据、UI 扩展声明、浏览器资源及验收任务；保留 tools、Markdown 与 structuredContent。已缓存旧 UI 的客户端通过重连刷新 discovery；旧 URI 返回 resource_not_found，不再提供 HTML 兼容入口。此为用户批准的实验能力撤回，不影响已保存笔记、操作账本及既有工具名。历史截图和测试证据保留为历史，不再作为交付条件。恢复此方向需新的明确需求，不提供复活开关。

## Decisions

启用 `--collaboration` 提供独立 `pinax.interaction.*` 工具；`--allow-note-body` 控制指定正文读取，`--allow-note-write` 另行控制保存并要求正文能力。旧接口不获得这些权限。静态 manifest 不虚报可选实例能力，实例能力资源明确列出当前启用动作。

预览不落盘：返回带操作 ID、基础版本和摘要的提案；摘要绑定 vault、完整修改内容和版本。提案正文属于用户审阅内容，不进入日志或操作账本。取消仅丢弃提案。保存通过现有 GORM operation ledger 记录同一 ID 的结果，正文仍在 Markdown vault，已有内容修改前创建 version snapshot。不同请求复用同一 ID 拒绝；超时和不确定结果保持原操作，不自动重新执行。

明确指令与已审阅提案是调用方声明的授权来源，均不是服务端验证过的人工点击。宿主负责对话意图和自身审批；服务端独立校验功能开关、封闭输入、目标、版本和请求绑定。单用户 stdio owner 不推导远程身份或组织权限。

同一 vault 的协作写入复用 owner operation lock 串行化；保存前重新检查实际文件内容。任意外部编辑器不遵守 Pinax 锁，最后检查与文件替换间存在文件系统固有限制，不能宣称任意编辑器间严格线性一致。此限制须在运行文档中披露。

展示使用共享语义结果：Markdown 文本和 JSON structuredContent。正文中的 HTML、图片链接和围栏保持被引用的用户内容，不作为指令执行。

## Verification

复用 Go testing/testscript、现有 integration evidence runner 和官方严格 MCP SDK。验证所有发现入口不再声明 UI，旧 UI URI 被拒绝，Markdown 查询与保存仍能完成。真实模型会话未实测时保持未验收，不以协议测试替代。

## Rollback

关闭 collaboration/body/write flags；保留 vault、快照、操作账本和旧 MCP 接口。无部署或凭据变更。

## 严格客户端 JSON-RPC envelope 修复

用户在 Claude Code 重连后 tools/list 超时。使用已安装的官方 TypeScript MCP SDK 1.29.0 重现：顶层 tools/resources 兼容投影不符合 strict JSONRPCResultResponseSchema，SDK 丢弃响应后以 -32001 超时。既有宽松 JSON 解析和 Grok doctor 未发现此问题。

修复仅作用于 stdio wire boundary：显式协商协议版本或 current metadata 的客户端只收到 jsonrpc、id、result/error，保持 result.tools/resources 与全部工具语义；内部 Handle API 的 Tools/Resources 投影保留。不携带 protocolVersion 的旧私有 initialize 客户端暂时保留旧 envelope，并通过 stderr 发出弃用提示。迁移到 result.tools/resources 和显式版本 initialize；至少保留一个发布版本，本次不安排删除私有兼容入口。回滚可恢复旧构建，但会重新触发严格客户端超时，不应作为正常恢复方式。回归测试分别验证标准帧及私有兼容路径，并增加真实 SDK 校验。
