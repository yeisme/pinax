---
schema_version: pinax.note.v1
note_id: note_c5a9bd150ae6
title: Ory 身份认证方案
tags: [chatgpt-share, ory, auth, pinax]
project: sg-note-pinax
created_at: 2026-06-06T09:32:59Z
updated_at: 2026-06-06T09:32:59Z
---

# Ory 身份认证方案

来源: https://chatgpt.com/share/6a23e853-09c8-83ea-b736-9ca3020cb3da

## 抓取状态

已尝试通过 Pinax 工具链保存这篇 ChatGPT 分享笔记。当前环境能够确认分享页标题为 `ChatGPT - Ory 身份认证方案`，但正文抓取未成功：

- `curl -L` 首次拿到页面 HTML 头部和标题，但连接在传输正文前超时或 TLS 断开。
- `firecrawl scrape` 返回目标站点无法抓取。
- `google-chrome --headless --dump-dom` 返回 `ERR_CONNECTION_CLOSED` 网络错误页。
- ChatGPT share 后端候选接口同样 TLS 断开。

## 后续补全

这条笔记先保留为来源记录。若需要完整正文，可在能访问该分享页的浏览器中复制对话内容，再运行：

```bash
pinax note edit "Ory 身份认证方案" --vault /workspaces/yeisme-agent/cli/pinax --editor "$EDITOR"
```

也可以把正文发给 Pinax，再通过 `note tag` 或 `note rename` 保持 metadata 规范化。
