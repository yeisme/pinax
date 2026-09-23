# 预览 × Agent 优化后续任务地图

- 状态：交付排序稿（2026-09-23）。把 [Markdown 预览设计](./markdown-rendered-preview-design.md) 的 P1–P4 与 [Agent 优化设计](./agent-optimization-design.md) 的六线合成一张执行地图。
- 铁律：**所有实施排在 2026-10-10 连续性主线窗口之后**（各 change 任务 0.1 硬门）；主线期间只允许合同/spec/文档任务。

## 波次与依赖

```text
[10-10 门]
   ├─► P1a pinax-rendered-view-v1        (24 任务, Lane A-G)
   │      └─ 合同稳定证据: rendered-view-parity profile
   ├─► P1b pinax-agent-note-proposal-v1  (18 任务)  ──前置: memory-runtime 5.x 已合并
   │      └─ 合同稳定证据: agent-note-proposal profile
   ▼
P2  DSH Pane rendered surface + 真实事件游标（DSH 域 change，消费 P1a 合同；
    顺带评估 gated action `note.create_draft`）
   ▼
P3  workbench Pinax module（client/yeisme-workbench，按 web-open-design admission
    立模块计划；消费 P1a 合同 + P1b 审阅投影）
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
| G1 10-10 主线收口 | roadmap 连续性 Go/Iterate 判定完成，owner 出 release note |
| G2 P1a 合同稳定 | `rendered-view-parity` 四断言族全绿 + strict validate 通过 |
| G3 P1b 合同稳定 | `agent-note-proposal` evidence 全绿 + transport parity 通过 |
| G4 P2 进入条件 | P1a 合同消费无 breaking 反馈 + DSH 侧 lane 空闲 |
| G5 P3 进入条件 | workbench admission 模块计划过审 + P1a/P1b 均归档 |
| G6 P4 进入条件 | `pinax://` 在 P1-P3 中 ≥30 天零格式变更 |

## 首周执行序（G1 之后）

1. P1a 0.2/0.3（golden 基线 + surface 分类）与 P1b 0.2（memory-runtime 协调确认）可并行
2. P1a Lane A（合同与树）→ Lane B（引用）→ Lane C（fence）；Lane D 与 C 并行
3. P1b 在 memory-runtime facade 确认后启动 Lane A
4. 两 change 的 evidence profile 是归档硬门，文档/router 任务随代码同波收尾
