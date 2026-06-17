# Design: Pinax Agent-Safe Positioning Rewrite

## 定位决策

### 核心叙事

从"local-first Markdown notes CLI"改为：

> **Pinax 让 AI 安全操作你的私人知识库。**
>
> 你的 Markdown vault 是真源。Pinax 的 proof loop 把每次 agent 写入变成可审计、可预览、可回滚的安全流程。Agent 看不到不该看的明文，云端也没有明文。

### 三个可复述概念

1. **Local Vault 是真源**：Markdown 文件永远是 source of truth，SQLite/`.pinax/` 都是可重建投影
2. **Proof Loop 保护 agent 写入**：Capture → Retrieve → Diagnose → Plan → Snapshot → Apply → Restore
3. **Cloud Sync 只协调密文**：服务端永远不存明文笔记、不执行本地工具

### 竞品关系定位

| 竞品 | 关系 | Pinax 不做 |
|---|---|---|
| Obsidian | 互补 | 不替代 UI，成为 vault 的 agent-safe 维护层 |
| Logseq | 差异化 | 不复制 outliner/graph UI |
| Notion | 避开 | 不打团队协作/云工作区 |
| Reflect | 差异化 | 不比个人笔记体验 |

### README 结构调整

当前 README 问题：
- 第一屏是功能列表（five core workflows），不是价值主张
- Quick start 在 installation 之后，但 first impression 不够强
- 没有明确说"为什么用 Pinax 而不用 Obsidian 自带 AI"

新 README 结构：
1. **一句话定位** + agent-safe proof loop 场景描述
2. **The aha moment**：一个代码块展示 proof loop run → plan → snapshot → apply → restore
3. **Why Pinax**：三个差异化点（proof loop、plaintext boundary、self-hosted encrypted sync）
4. **Quick start**：最小安装 + init + proof loop run
5. **Status table**（保留）
6. **Detailed workflows**（保留现有内容，下沉为 H2）

### Agent-safe boundary 文档

需要新增一个文档专门解释安全边界：
- CLI 默认不泄露 full note body（`--display card` vs `--display body`）
- MCP bounded context：agent 通过 MCP 读取的是 projection，不是原始文件
- Cloud no-exec invariant：`cloud_exec=false`、`plaintext_note_body=false`
- Encrypted envelope：客户端加密，服务端只看到密文
- Proof loop 写入控制：plan → snapshot → apply → receipt → restore

## 验证策略

- README 能被一个不了解 Pinax 的人在 30 秒内理解核心价值
- 所有命令示例使用真实可运行命令
- 中英文 README 保持同步
- 不改变任何代码行为，`task check` 仍应通过

## 延期项

- Homebrew/Scoop 安装命令（等 preview release 后再加）
- 完整文档站（暂缓）
- 视频/动画 demo（暂缓）
