## 背景

根仓库 OpenSpec 变更 `dsh-pane-plugin-ecosystem-v1` 与 `agent/harness-plugins` 的 `dsh-pinax-pane-v1` 定义了 DSH Pane 插件平台：DSH Host 通过 owner 投影消费领域状态，领域 owner（Pinax）提供 canonical projection / action / receipt 与领域测试证据。Pinax 侧目前只有未追踪的 `internal/app/pane.go` 切片与 `dsh-pane` 证据 profile，缺少本仓库自己的变更记录、规格与文档。

本变更把 Pinax 侧 DSH Pane 合同落地为正式规格：快照投影（`pane.event.v1alpha1`）、artifact 引用（`pane.artifact.v1alpha1`）、门控 action、手写 metadata 拒绝、offline/permission_denied 负例，以及配套的红线（无绝对路径、无凭据、失败投影不得伪装 ready）。

## 目标

- 将 `internal/app/pane.go` 的快照组装固化为 `pinax-pane` 规格：`AssemblePaneSnapshot` 从 `note.list` 投影派生有界实体（ref/title/kind/status/tags），不含绝对路径与凭据。
- 失败投影（`failed`/`error`/`offline`）SHALL 映射为 `offline`，`permission_denied` 保持原义；不得以空实体 + `ready` 伪装成功。
- `PaneArtifactFromNote` 输出不含文件系统路径的 `pane.artifact.v1alpha1` 引用；不安全 ref（绝对路径、URL、凭据关键词）直接拒绝。
- `PaneGatedActions` 声明 capture/sync 必须走 `pinax` CLI/service，携带 expected revision、幂等与回执要求。
- `RejectHandwrittenMetadata` 对未经 Pinax parser 的 metadata blob fail closed。
- `dsh-pane` 证据 profile（component 层）纳入常规验证。

## 非目标

- 不在 Pinax 内实现 DSH Host 桥、Client 视图注册或 Pane 生命周期（归属 `agent/harness-plugins`）。
- 不新增 CLI/RPC/MCP 命令面；本变更只固化领域侧合同与测试证据。
- 不实现事件流（push/change 订阅）与 cursor 续传；快照阶段 cursor/sequence 保持占位（`c-1`/`-1`）。
- 不改变现有 `note.list` 投影或输出合同。
