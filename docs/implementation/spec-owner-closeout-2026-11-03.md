# Pinax 规格归属与本地验收收口（2026-11-03）

客户端任务已迁出，Pinax 自身本地工程验收通过。未完成的真实发布/使用证据继续保留，不宣称全部产品验收完成。

## 当前 owner

| 内容 | 唯一当前入口 |
| --- | --- |
| Pinax sync/writer/附件/恢复合同 | Pinax canonical `sync-control`、`notebook-source-attachments` 及[本次工程归档](../../openspec/changes/archive/2026-11-02-pinax-sync-attachment-contract-closeout-v1/tasks.md) |
| DriveBridge UI/Electron/DTO/guard/安装/三平台 | `cli/drivebridge/openspec/changes/drivebridge-desktop-knowledge-reader-v1/` |
| 原混合 workbench 全文与历史证据 | `cli/drivebridge/openspec/handoffs/pinax-drivebridge-workbench-v1/`，不作为 active 队列 |
| operator Skill/profile 与双 runtime 交接 | 根仓 `openspec/changes/pinax-continuity-operator-handoff-v1/` |
| Pinax 自身 R02/R03 | [release qualification](../../openspec/changes/pinax-release-qualification-v1/tasks.md)，仍 pending |

迁移保留原包与工作树中的未提交证据、原状态和双向引用。消费者不重复接单，Pinax 不再维护 D 系列任务或等待客户端发布。Skill/profile 未自动重配：Pinax 双 runtime operator 与 source 一致，根仓目前未配置该 profile，后续属于根仓治理，不阻塞 CLI。

## 本地验收证据

以下目录相对 `temp/integration-test-runs/`，均保留六件套且脱敏扫描通过。UTC run-id 为 11 月 2 日，对应上海时间 11 月 3 日。

| 验收 | run-id | 结果 |
| --- | --- | --- |
| notebook-backend | `20261102T164801Z-4105964` | 通过，sync/附件/原 key 恢复 |
| operation-recovery | `20261102T164915Z-4182138` | 通过，跨进程查询与恢复 |
| daily-knowledge | `20261102T164922Z-4187414` | 通过，stdin/稳定 ID/review/restore |
| agent-memory | `20261102T164930Z-6103` | 通过，scope/source/proposal/SDK |
| agent-continuity | `20261102T164938Z-15217` | 通过，handoff/feedback/report 合同 |
| default | `20261102T164952Z-26010` | 通过，本地 Obsidian/Proof Loop/模拟两端同步 |
| notebook-backend-final | `20261102T165114Z-120068` | `task check` exit 0；全量 Go、fmt、lint 0 issues、build、renderer 4/4 与 OpenSpec 112/112 |

DriveBridge 仅复验本次交接涉及的合同：`make contracts-check` 三门通过，其 owner evidence 为 `20261102T164949Z-228c694c`、`20261102T164955Z-aee55610`（113 schemas）、`20261102T164956Z-5e5efddf`（39 desktop DTO）。未重新声称客户端运行、平台或 apply 通过。

迁移工具回归 9/9 通过，覆盖碰撞预检、未提交任务/metadata 保留、跨仓链接重定位、重复执行，以及只处理 owner 纠正、不触发其他项目历史迁移。源包消失、目标完整、迁移链接与 evidence bundle 均已核对。

## Pinax 尚待的真实证据

本轮原 backlog 迁移后，7 个 change 共 15 项真实资格任务仍未完成，均属于 Pinax 自己的领域或产品证据。不将历史 handoff 或工程归档再计入。

| Change | 未完成 | 下一项实际输入 |
| --- | --- | --- |
| daily-knowledge-loop | 7.1 | 真实主力 vault 两周自然使用 |
| daily-interaction-orchestration | 2.1、2.2 | 同一窗口采集 capture/review/找回/复用，单独报告；可与上一行共享原证据 |
| agent-memory-runtime | 8.5、8.6 | 重新准入的真实执行 adapter、四周分母与成熟度决策 |
| trusted-continuity-dogfood | 8.3、8.4 | 固定旧窗口的真实报告与 Go/Iterate/Stop；新采样不能补写旧窗口 |
| passphrase-s3-bootstrap | 7.5–7.9 | 真实 macOS release candidate、COS/Keychain、双向 canary 与恢复矩阵 |
| real-judgment-acceptance | 1.3 | 已授权模型通道、首批真实案例、用户复核；未知结果先恢复原操作 |
| release-qualification | R02、R03 | 真实 endpoint/第二设备/Capsa 合同；三类真实 Obsidian vault 与 restore/使用证据 |

最终 readback 时另发现并行新增的 `pinax-judgment-sdk-managed-execution-v1`（3 项未完成）及对应 Go/vendor 工作树变更，未包含在本轮初始 backlog，未改动或接管。因而当次 active 总数为 8 个 change/18 项未完成；后续继续以当前 CLI/`tasks.md` 为准。归档后的严格 OpenSpec 校验为 114/114。上方完整工程门是该次运行的证据，不证明后续并行 SDK 变更已验收。

这些门未到位时不每天重复 full check、不自动续观察窗口、不补用户 outcome。没有改动任何真实 vault、凭据或远端状态；未运行 race，未付费、推送、发布或部署。已有并行 Go/Skill/DriveBridge 代码改动保持原状。
