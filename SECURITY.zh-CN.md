# 安全策略

[English](./SECURITY.md)

Pinax 是本地优先软件。它把用户笔记保存在本地 Markdown vault 中，并把机器生成的投影放在 `.pinax/` 下。安全报告应重点关注可能暴露 note content、credential、provider payload、本地文件、sync metadata 或 command output contract 的问题。

## 报告漏洞

在专门的公开安全联系方式发布前，请优先使用 GitHub private security advisory（如果仓库可用）。如果 private advisory 不可用，请通过你能访问到的最小公开渠道联系项目 owner，并只提供最小复现。不要在公开 issue 中粘贴真实 secret、真实 note body、原始 provider payload、cookie 或 Authorization header。

报告中建议包含：

- 受影响的 command、API route、MCP tool 或 sync transport。
- 预期行为和实际行为。
- 使用临时 vault 的最小复现。
- stdout、stderr、JSON/agent output、receipt、event、fixture 或 log 是否暴露了敏感数据。

## 敏感数据规则

Pinax 不应在 stdout、stderr、event、receipt、log、fixture、docs 或 OpenSpec evidence 中暴露以下内容：

- Provider token、webhook URL、cookie、Authorization header、API key、password，或带有真实 secret value 的 secret ref。
- 可能包含 credential 的原始 provider payload 或 provider stderr。
- bounded agent/MCP/dashboard/remote projection 中的明文 note body。
- Hidden prompt、private tool argument 或 model-internal reasoning。

## Local API 和 MCP 范围

`pinax api serve` 默认绑定 localhost，只作为本地 projection adapter，不是 public hosted API。MCP tool 当前只读，除非未来 spec 明确引入带 approval gate 的写入 surface。

## Cloud Sync 范围

Cloud Sync transport 交换加密 blob、manifest 和 revision metadata。只有在 revision durable commit 且本地 sync-state evidence 写入后，才能输出 `remote_write=true`。失败、dry-run、unsupported 或安全性不确定的 transport path 必须报告 `remote_write=false`。
