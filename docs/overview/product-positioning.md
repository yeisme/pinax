# 产品定位

Pinax 帮用户把分散在 Markdown、Notion、飞书、网页研究和 agent 输出里的知识，收敛成本地可迁移、可检索、可审计、可回滚的笔记工作流。

一句话：**Pinax 是本地优先的统一笔记 Agent CLI，不是云笔记平台。**

首期重点：

- 初始化和校验本地 Markdown vault。
- 创建、捕获、整理和检索笔记。
- 通过 Git 管理笔记版本与回滚建议。
- 通过 CLI-backed Provider adapter 与外部系统同步或投递。
- 通过稳定 `--agent` / `--json` 输出服务 agent workflow。

非目标：

- 不内置长期 daemon 作为 MVP 必需能力。
- 不把飞书、Notion 或其它外部平台作为笔记真源。
- 不默认直接维护外部平台 native API SDK。
- 不让 agent 直接手写机器可读 metadata。

