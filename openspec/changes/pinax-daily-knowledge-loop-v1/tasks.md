# 实施与验收任务

由 scripts/openspec-tasks.py 维护状态。

- [x] 1.1 记录已批准的日常笔记优先级、owner 与兼容边界，保留 continuity 历史验收。 | evidence: 用户批准日常笔记主线；proposal/design/spec 与 roadmap/positioning/continuity/交付地图已同步，旧 continuity 样本和门槛保留；openspec validate pinax-daily-knowledge-loop-v1 --strict PASS。
- [x] 2.1 补齐 inbox stdin 捕获、正文校验、保存回读与稳定打开动作。 | evidence: 本地及远端 stdin、冲突/空输入/读取失败、dry-run、正文保留、保存后索引失败保留原 ID 已通过；temp/integration-test-runs/20260928T140541Z-3813950/。
- [x] 3.1 实现只读五条待整理批次，聚合 inbox 与 notes/index，保留跳过及原有变更门。 | evidence: inbox review 默认五条/上限二十、跨入口聚合、去重/排除、继续/失效游标及无写入通过；复用既有变更门，无新审阅状态。证据同 daily-knowledge run。
- [x] 4.1 完善 Unicode 详情卡与稳定打开动作，验证编辑器往返、搜索、链接、附件及恢复。 | evidence: UTF-8 有界摘要、同名对象稳定动作、编辑器打开/失败/正文修改、native fallback 与只读文件树指纹通过；既有恢复和 stale plan 测试通过。证据同 daily-knowledge run。
- [x] 5.1 同步 Agent 操作指引、日常指南、路线与真实使用观察方法。 | evidence: 日常指南、命令文档与 README 已更新；两 operator 源技能更新并经 scripts/skills.sh sync-target data/yeisme-notes 同步，双 runtime 内容比对一致；validate-custom 与 validate-profiles PASS。
- [x] 6.1 完成 focused tests、集成证据、兼容审阅与项目最终检查。 | evidence: 聚焦 daily-knowledge PASS：temp/integration-test-runs/20260928T140541Z-3813950/；最终 daily-knowledge-quality 执行 task check PASS：temp/integration-test-runs/20260928T141432Z-289908/，全量 Go、fmt、lint 0 issues、build、OpenSpec 100/100、renderer 4/4 与 tsc 通过。首次门禁仅本次 io 导入分组错误，修正后完整重验。静态审阅：additive CLI/facts/actions，无 DB/public API breaking，无外部写入；原 operation recovery 保留，snapshot/stale restore 与 readonly 指纹验证通过；未运行 race。
- [ ] 7.1 在真实主力环境完成两周自然使用观察，记录成本、找回与再次使用；不得用 fixture 替代。 | evidence: 20261018T100553Z-1562079 fixture daily-knowledge profile passed with redaction; two-week real primary-vault observation remains pending, so task stays open
