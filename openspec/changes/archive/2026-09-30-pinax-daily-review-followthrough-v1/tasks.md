# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] 1.1 Owner=Pinax；Scope=contract；Deps=none；明确 scoped plan、分页、兼容和失败合同；验证 openspec validate pinax-daily-review-followthrough-v1 --strict；失败修正规格，不改变旧任务。 | evidence: Scoped schema、只读分页、权限与兼容合同已明确；strict validate 通过。
- [x] 2.1 Owner=Pinax；Scope=app/domain/CLI；Deps=1.1；实现选择集规划、保存、预检、消费状态和失败回执；验证 scoped organize focused tests；失败按最小案例复验。 | evidence: Scoped selection/context/preflight/receipt/replay 聚焦与原 organize 回归通过；20260930T071715Z-2096797；继续执行边界补测。
- [x] 3.1 Owner=Pinax；Scope=app/CLI/MCP/schema；Deps=1.1；实现脱敏提案筛选翻页、单 ID 查询和 vault-bound 动作，MCP 只读 parity；验证 proposal/MCP focused tests；失败保留旧合同。 | evidence: CLI 与 legacy/SDK MCP 筛选翻页、单 ID、vault-bound actions 和默认权限通过；20260930T071715Z-2096797。
- [x] 4.1 Owner=Pinax；Scope=tests/testkit；Deps=2.1,3.1；补 testscript 与 daily-review-followthrough evidence profile；验证成功、失败、只读、权限和兼容；失败保留证据。 | evidence: daily-review-followthrough 已含 testscript、故障注入、CLI/legacy/SDK parity；20260930T071715Z-2096797 通过，首次测试包装/引号失败证据保留。
- [x] 5.1 Owner=Pinax/root-skill；Scope=docs/source-skills；Deps=4.1；同步日常指南和三份源技能，定向同步校验；不改真实 vault。 | evidence: 日常/organize/agent/pipeline 文档及三源技能已同步；validate-custom 与 root/cli/pinax/data/yeisme-notes profile validate 和定向同步通过。
- [x] 6.1 Owner=Pinax；Scope=final；Deps=5.1；运行计划中的 evidence profiles 和 task check（通过 daily-knowledge-quality），strict OpenSpec 与只读审查；未通过则归因后聚焦修复；不运行 race。 | evidence: final task check 通过：20260930T075444Z-701072，fmt/lint 0 issues/Go tests/build/renderer 4/4/tsc/OpenSpec 101/101；四 focused profiles 074927/074949/074955/075001 全通过；证据脱敏全部通过；075009 外层超时不计通过；CGO_ENABLED=0 build 通过；未运行 race。
- [x] 7.1 Owner=Pinax；Scope=closeout；Deps=6.1；归档本 change、同步 canonical specs 和交付记录；原 11 个真实任务保持未勾选。 | evidence: 本次 change 已归档并由 openspec archive 同步 daily-review-followthrough canonical spec；交付记录 docs/implementation/daily-review-followthrough-2026-09-30.md；原五 change 11 项未完成保留。
