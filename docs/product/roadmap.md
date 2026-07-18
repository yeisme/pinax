# Pinax Roadmap

本文定义 Pinax 从当前 v0.1.x "Preview" 阶段毕业到 v1.0 GA 的路线图和触发条件。面向产品决策者，不涉及实现细节。

相关文档：

- [产品定位](../overview/product-positioning.md)
- [MVP 范围](./mvp-scope.md)
- [Release Packaging](../operations/release-packaging.md)
- [Cloud Sync Architecture](../architecture/cloud-sync-design.md)

---

## 定位与现状

Pinax 是 **面向 Markdown vault 的 agent-safe 知识控制平面**。它不是另一个笔记应用，而是给 AI agent（Claude Code、Codex、Cursor、MCP client）提供的安全维护层。三个支柱始终成立：

1. **Local Vault 是真源** — Markdown 文件是用户知识资产的唯一真源；SQLite/GORM 索引和 `.pinax/` 元数据都是可重建投影，不持有独立状态。
2. **Proof Loop 保护每次 agent 写入** — plan → snapshot → apply → receipt → restore 控制链，让每次写入都可审计、可预览、可回滚。Agent 读取默认返回 bounded projection（card/detail/context），不返回完整 body。
3. **Cloud Sync 只协调密文** — 端侧 AES-256-GCM 加密，服务端只见 ciphertext revision，不保存明文、不执行本地工具。

当前版本：**v0.1.8**。本地核心工作流已可用于日常使用，两个领域仍处于 Preview。

---

## 当前版本状态

详见 [README Status](../../README.md#status)。

| 领域 | 状态 | 说明 |
| --- | --- | --- |
| 本地 Markdown vault（notes、journals、inbox/drafts、templates、search、links/backlinks、assets、project workspaces/boards、database saved views、repair/organize plans） | **Supported** | 核心本地工作流已可用于日常使用，覆盖 Capture → Retrieve → Diagnose → Plan → Apply 全闭环 |
| CLI output modes（`--agent`、`--json`、`--events`、`--explain`） | **Supported** | 四种输出模式共享同一 projection boundary，面向人和 agent 的信息边界一致 |
| Local dashboard、read-only MCP、localhost REST/RPC adapter、workspace/task/database/graph read projections | **Supported** | 本地只读面已就绪，dashboard 绑定 localhost，MCP 默认只读 |
| Obsidian-style vault compatibility（wikilinks/backlinks、properties、daily managed blocks、dataview blocks、`.obsidian/` ignore） | **Preview** | 功能已实现，但缺少多 vault 真实验证矩阵，不保证所有 Obsidian 插件生态兼容 |
| Cloud Sync over server / file / S3-compatible / rclone transports | **Preview** | 协议和 transport 已实现，但缺少真实端到端 dogfooding 证据，当前测试主要依赖 `mlptest` fake server |

**"Preview" 的含义**：功能已落地、有 testscript 覆盖，但尚未在足够多的真实场景中验证。Preview 阶段不承诺向后兼容，不建议作为生产同步的唯一依赖。Preview → Supported 的毕业标准见下节。

---

## 毕业标准

每个 Preview 领域需要满足明确、可验证的 gate 才能标记为 Supported。这些 gate 不是功能清单——功能已经存在——而是**真实世界验证证据**。

Preview 到 Supported 的毕业不是代码变更，而是**证据收集和文档化**。一个 Preview 功能毕业意味着：我们愿意为它在真实使用中遇到的问题提供支持和修复承诺，而不是只说 "它是 preview，后果自负"。

### Cloud Sync → Supported

Cloud Sync 的核心风险是多设备冲突收敛和服务端合同对齐。以下 gate 确保它在真实场景下可承诺。

| Gate | 验证方式 |
| --- | --- |
| **G1: 真实 CLI ↔ 真实 backend 端到端 smoke** | 使用真实 `cli/pinax` 连接真实 `backend-server/capsa`（**不是** `internal/cloudclient/mlptest` fake server），完成 device bind → push → pull → conflict convergence 全链路。冲突收敛指两端在 same-workspace 下经过 push/pull 后达到一致状态，无静默丢写、无 orphan revision。 |
| **G2: 第二台设备真实同步** | 在两台独立设备上通过真实 transport（S3 或 server）完成一次跨设备同步：A 设备写入 → push → B 设备 pull → 验证 B 设备 vault 内容与 A 一致。不能只依赖 `file://` local fixture 路径——`file://` 是开发调试 transport，不是生产 transport。 |
| **G3: 已知 flaky test 稳定化** | `TestObjectStoreTransportLockFallbackRejectsConcurrentFirstHeadCreation`（`internal/cloudsync/object_store_test.go`）必须稳定通过，或以书面方式文档化其 known-flaky 原因及缓解方案（race condition window、test timeout 调整等）。不能以 `-skip` 静默跳过——静默跳过等于隐藏问题。 |

毕业证据写入 `docs/operations/` 下专门的 sync dogfooding 记录，包含运行环境、transport 类型、vault 规模和遇到的问题。

### Obsidian Compatibility → Supported

Obsidian 兼容性的核心风险是 vault 结构多样性和插件生态。以下 gate 确保覆盖主流使用模式。

| Gate | 验证方式 |
| --- | --- |
| **O1: ≥3 个真实 Obsidian vault 验证** | 覆盖至少三种不同结构和使用习惯的 Obsidian vault（例如：学术研究库含大量 wikilinks/backlinks、项目管理库含 properties 和 dataview、个人日记库含 daily managed blocks），确保：`[[wikilinks]]` 和 `[[Title|Alias]]` 正确解析、YAML properties/frontmatter 兼容、`pinax:managed` daily blocks 不被破坏、dataview query blocks 保持原样、`.obsidian/` 配置目录被正确忽略。 |
| **O2: 书面兼容矩阵** | 在 `docs/` 下输出一份 Obsidian compat matrix 文档，列出每个验证 vault 的结构特征、规模、测试命令、已知差异（如某些社区插件语法不支持）和结论（Pass / Partial / Known-gap）。 |

---

## GA / v1.0 触发条件

v1.0 GA 在以下三个条件**全部满足**后触发。v1.0 不新增功能特性——它是关于把已有能力从 "能用" 推进到 "可承诺"。

| 条件 | 说明 |
| --- | --- |
| **C1: 两个 Preview 全部毕业** | Cloud Sync 达到 G1 + G2 + G3，Obsidian Compatibility 达到 O1 + O2。 |
| **C2: 一次 ≥2 周的真实 dogfooding 记录** | 文档化在 `docs/operations/` 下：使用真实 vault（非 fixture）、通过 proof loop 执行真实 repair/organize（含 snapshot + apply + restore 验证）、跨设备真实 sync（非 `file://`）。记录应包含遇到的问题、修复方式和改进 follow-up item。 |
| **C3: CLI ↔ backend 合同对齐验证** | 确认 `cli/pinax` cloudclient 期望的 API 合同与 `backend-server/capsa` 实际 handler 无 drift。验证方式：contract test 或 manual probe 覆盖 device bind、push、pull、conflict resolve 路径。Drift 是分布式系统最常见的隐性故障源，必须在 GA 前显式验证。 |

**v1.0 不等于 "全部功能完成"**。Pinax 会有 v1.x、v2.x 的持续演进。v1.0 的语义是：核心三支柱（Local Vault / Proof Loop / Cloud Sync）已在真实场景中验证，可以承诺向后兼容和稳定合同。在此之前的 breaking change 不需要 deprecation cycle；v1.0 后 breaking change 需要遵循 semver。

---

## v0.2 近期目标（next quarter）

v0.2 不引入新功能领域，核心是关闭三笔 P0 credit debt——这些是当前流程中的已知缺口，不关闭会持续侵蚀产品可信度。

| 优先级 | Debt | 反面案例 | 完成标准 |
| --- | --- | --- | --- |
| **P0-1** | README version freshness | v0.1.2 → v0.1.5 期间 README "current stable tag" 长期停留在旧版本，用户下载到 stale artifact——这是典型 staleness 反例 | 每次打 tag 后 README "current stable tag" 在同一 PR/commit 内同步更新 |
| **P0-2** | OpenSpec closeout discipline | 部分 OpenSpec change 的 tasks 已全部 `[x]` 但未正式 archive，造成 "看起来完成但实际未 release" 的幻觉 | 每个 completed change 在 release 前正式 archive；archive 列表与 release notes 一一对应 |
| **P0-3** | Cloud Sync GA evidence | 当前 Cloud Sync 证据主要来自 `mlptest` fake server 和 `file://` fixture，不是真实 transport 证据 | 完成 Cloud Sync 毕业标准 G1 + G2，证据写入 `docs/operations/` |

### Closeout Gate

为防止 "all tasks [x]" 静默未发布，每次 release 前必须通过以下 closeout gate。该 gate 是流程纪律，不依赖自动化工具，由 release operator 执行：

1. 检查 `openspec/changes/` 中是否有 tasks 全部完成但未 archive 的 change。
2. 如有，在同一 release 中 archive，或显式标注 "deferred to vX.Y+1" 并记录原因。
3. Release notes 中列出本版本 archive 的 change 列表，让消费者可追溯。
4. README "current stable tag" 在 release commit 中同步更新。

---

## 明确不做

以下能力**不在 Pinax 范围内**，明确排除以避免 scope creep（详见 [AGENTS.md 项目定位](../../AGENTS.md#项目定位)）：

| 不做 | 原因 | 替代方案 |
| --- | --- | --- |
| **云笔记后端** | Pinax 不是 Notion 替代品；vault 是真源，cloud 只协调密文 | 用户保留自有 Markdown vault |
| **新闻爬虫 / 内容抓取** | 外部资料通过 CLI-backed Provider adapter 拉取，Pinax 不自建 crawler | 使用 `ntn`、`lark-cli`、Hermes/internet-access 等 provider |
| **飞书知识库** | 飞书是 provider / delivery surface，不是 vault 替代 | 飞书内容通过 adapter 导入 vault |
| **长期 daemon** | Pinax 是 CLI-only 短生命周期进程 | `sync daemon` 是本地 watch-and-poll 进程，不是常驻服务 |
| **内置编辑器 UI** | 完整编辑器属于 Workbench 模块 | `pinax vault dashboard` 提供只读控制台；编辑器归 `client/yeisme-workbench`（参见 [Dashboard PRD §3](./dashboard-prd.md#3-产品定位)） |

核心原则：外部平台永远是 provider 或 delivery surface，Pinax vault 才是笔记真源。

---

## 发布节奏

发布流程详见 [Release Packaging](../operations/release-packaging.md)。

关键规则：

- Pinax 使用独立 `vX.Y.Z` 语义版本 tag，GoReleaser 从 tag 构建 GitHub Release archives、`checksums.txt`、source archives、SBOM、Linux packages、Homebrew formula 和 Scoop manifest。
- **每次打 tag release 后，README 中 "current stable tag" 必须在同一 commit 或紧随的 commit 中同步更新。** v0.1.2 → v0.1.5 的 staleness 是反面案例，不再重复。
- Release 前运行 `task check` + `task release:check` + `task release:package:validate`（credential-free snapshot 验证）。
- Release 后执行 [post-release checklist](../operations/release-packaging.md#post-release-checklist)：下载 archive、验证 checksum、smoke `pinax version` 和 `pinax --help`、检查 SBOM。
- Homebrew formula 和 Scoop manifest 仅从 tagged release 发布，不从 snapshot 发布。

发布节奏不预设固定周期（如 "每月一发"）；以功能完成度、credit debt 清理和 Preview 毕业为驱动。

---

## 里程碑总览

| 里程碑 | 核心交付 | 前置条件 |
| --- | --- | --- |
| **v0.1.x（当前）** | 本地核心工作流 Supported；Cloud Sync 和 Obsidian compat 处于 Preview | — |
| **v0.2** | 关闭 P0 credit debts（README freshness、OpenSpec closeout、Cloud Sync GA evidence）；建立 closeout gate | 当前 develop branch |
| **v0.3–v0.x** | Cloud Sync 和 Obsidian compat 逐项毕业（G1–G3、O1–O2）；积累 dogfooding 记录 | v0.2 closeout discipline 到位 |
| **v1.0 GA** | 两个 Preview 全部 Supported；≥2 周 dogfooding 文档化；CLI ↔ backend 合同对齐验证 | C1 + C2 + C3 全部满足 |

**关键判断**：v1.0 的风险不在功能缺失——功能已经存在——而在**验证证据不足**。Roadmap 的主线是把 fake-server 测试换成真实 transport dogfooding，把 "能用" 换成 "可承诺"。
