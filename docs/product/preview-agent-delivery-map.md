# 预览 × Agent 优化后续任务地图

## 2026-10-01 DriveBridge 消费交接

P3目的地已由用户调整为DriveBridge内置知识阅读空间（Electron、阅读先行、三平台实机），不再等待不存在的Workbench，也不要求P2 Pane先交付。新 [pinax-human-reading-api-v1](../../openspec/changes/pinax-human-reading-api-v1/README.md) 独立补人读授权/分页/附件与会话合同；[消费交接](web-open-design.md) 说明owner。此轮只设计，不改变P1已归档事实或原11项真实验收。P2/P4各自门继续保留，P3后续写入仍需独立合同。

## 2026-09-28 日常路径优先例外

用户已批准 `pinax-daily-knowledge-loop-v1`，复用现有 `note open` / preview / search show 先完成 Agent 对话与现有编辑器路径。该 bounded change 不受下文 10-10 独占主线限制；不提前启动完整 P1–P4，也不新增 URI handler 或客户端。原能力保留，重新排序的依据与真实使用观察见 [日常指南](../guides/daily-knowledge.md)。下文是原 09-23 平台扩展的交付顺序，继续约束未获本次批准的能力。

- 状态：交付排序稿（2026-09-23）。把 [Markdown 预览设计](./markdown-rendered-preview-design.md) 的 P1–P4 与 [Agent 优化设计](./agent-optimization-design.md) 的六线合成一张执行地图。
- 2026-09-30 更新：用户解除 P1a/P1b 的时间门；两项共 40 个任务已完成本地验证并归档，保持 experimental。P2–P4 仍按原准入处理；continuity 真实门保留。验证见[本地收尾记录](../implementation/spec-closeout-2026-09-30.md)。

## 波次与依赖

```text
[P1 已于 09-30 本地完成；后续门保留]
   ├─► P1a pinax-rendered-view-v1        (24 任务, Lane A-G)
   │      └─ 合同稳定证据: rendered-view-parity profile
   ├─► P1b pinax-agent-note-proposal-v1  (16 任务)  ──前置: memory-runtime 5.x 已合并
   │      └─ 合同稳定证据: agent-note-proposal profile
   ▼
P2  Pane rendered surface + 真实事件游标（pane 域 change，消费 P1a 合同；
    顺带评估 gated action `note.create_draft`）
   ▼
P3  DriveBridge知识阅读空间（cli/drivebridge，按新reader API与desktop change；
    阅读独立于P2推进，P1b审阅消费留后续写入片）
   ▼
P4  pinax-web-renderer 切换 L1 数据源 + 编辑器 URI handler（`pinax://` 稳定性
    由 P1-P3 使用数据背书后启动）
```

## 并入活跃 change 的线（不另立）

| 优化线 | 去向 |
| --- | --- |
| card-first 两段式读取 + budget（§3.1） | `pinax-agent-memory-runtime` context compiler / MCP 任务附录 |
| context pack 单入口（§3.3） | 同上（同一 compiler 的 CLI 出口） |
| 技能路由同步（§3.6） | 各 change 收尾任务（7.2 / 5.2） |

## 门与验收

| 门 | 判据 |
| --- | --- |
| G1 提前实施授权 | 2026-09-30 用户明确重排 P1a/P1b；continuity 真实门保留 |
| G2 P1a 合同稳定 | `rendered-view-parity` 四断言族全绿 + strict validate 通过 |
| G3 P1b 合同稳定 | `agent-note-proposal` evidence 全绿 + transport parity 通过 |
| G4 P2 进入条件 | P1a 合同消费无 breaking 反馈 + pane 侧 lane 空闲 |
| G5 P3 进入条件 | DriveBridge/Pinax新合同与本地任务就绪；本轮仅设计，真实binding与三平台由各owner验收，不依赖Workbench/P2 |
| G6 P4 进入条件 | `pinax://` 在 P1-P3 中 ≥30 天零格式变更 |

## 首周执行序（G1 之后）

1. P1a 0.2/0.3（golden 基线 + surface 分类）与 P1b 0.2（memory-runtime 协调确认）可并行
2. P1a Lane A（合同与树）→ Lane B（引用）→ Lane C（fence）；Lane D 与 C 并行
3. P1b 在 memory-runtime facade 确认后启动 Lane A
4. 两 change 的 evidence profile 是归档硬门，文档/router 任务随代码同波收尾
