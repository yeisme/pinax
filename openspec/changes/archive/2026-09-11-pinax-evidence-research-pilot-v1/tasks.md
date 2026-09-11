# 原子任务与验收

## 本地技术验证

- [x] 1.1 Owner: Pinax；Scope: 默认 vault 只读入口和固定模板参考；Dependencies: none；Lane: inline；Acceptance: CLI 可用、默认 vault 与索引可读取，不修改用户笔记正文；Validation: `pinax vault list --agent`、`pinax index status --json`；Expected: 明确入口和状态；Failure re-check: 缺配置、歧义、不可读。2026-09-10：69 篇已登记笔记、索引 fresh；模板参考保留 experimental，不运行正式编译。
- [x] 1.2 Owner: Pinax；Scope: `FirstSnippet` Unicode 片段；Dependencies: 1.1；Lane: inline；Acceptance: 先复现失败再修复，ASCII 不变；Validation: `go test ./internal/app/searchops -count=1`；Expected: Unicode 与原文子串断言通过；Failure re-check: 大小写映射偏移、中文/表情截断、无匹配前缀。2026-09-10：先复现两类映射错误，修复后 package tests 通过。
- [x] 1.3 Owner: Pinax；Scope: testscript 完整流程和原有 evidence runner 的开发 profile；Dependencies: 1.2；Lane: inline；Acceptance: 检索、原文、关联、无索引、dry-run、模拟采纳、改名/删除、保存失败可重复验证；Validation: `go run ./tools/testkit/integrationevidence --profile research-brief`；Expected: exit 0，完整脱敏证据；Failure re-check: vault 全树摘要变化、退出码、fixture 与真实效果混淆。2026-09-10：最终 evidence `temp/integration-test-runs/20260910T054503Z-2605463` exit 0；包括默认监控行为和既有关联/孤立笔记回归。
- [x] 1.4 Owner: Pinax；Scope: 稳定差异的格式、lint、测试、构建与 OpenSpec；Dependencies: 1.3；Lane: inline；Acceptance: 归因并保留并发改动，不修复无关问题；Validation: `task check`、`git diff --check`；Expected: 所有必要门通过或列出确切外部失败；Failure re-check: introduced / concurrent / pre-existing / environment。2026-09-10：本次文件格式通过，限定包 lint 0 issues、go vet 与 CGO_ENABLED=0 构建通过；task check 未全过，证据 `temp/integration-test-runs/20260910T053906Z-research-quality`。三个并发 MCP 文件的格式问题未修改；自己的临时检查程序格式问题已通过移除临时源文件清除。OpenSpec 全量 90 项通过；全量 Go 测试/构建门未完成，不标记通过。

## 五题真实试验（不得以合成测试关闭）

- [x] 2.1 Owner: 用户 + 当前 Agent；Scope: 用户批准的回顾性来源基准；Dependencies: 1.1；Acceptance: 先固定目标和换词查询，再跑检索，首轮找回率仍为未测量。2026-09-10：用户明确选择该替代口径，五题重测 5/5；非盲测，不用于证明首轮找回率。证据见 acceptance.md。
- [x] 2.2 Owner: 当前 Agent + 用户；Scope: 五份简报与明确效果评价；Acceptance: 不用模型自评或沉默充当反馈。2026-09-10：用户整体认可五题结论，记录为批次认可，不伪造逐题评分；8 个实际来源可读取、相关原文已核对；外部现状及泛化能力未验证。
- [x] 2.3 Owner: 用户 + Pinax；Scope: 既定采纳后保存；Dependencies: 2.2；Acceptance: 明确采纳及继续原方案授权后通过 CLI 保存、回读并核对来源。2026-09-10：5 份保存，11 条来源关系验证；再次执行复用相同 ID、零重复创建；真实未确认写入为 0，远端写入为 0。原记录汇总误写 12，已由重跑生成的 11 修正，保留旧证据，不改写历史记录。技术总 gate 仍由 1.4 单独控制。

## 后续门

- [x] 3.1 Owner: 公共 Skill source / Pinax；Scope: 已验证流程固化的 owner OpenSpec；Dependencies: 2.1–2.3 的 gate；Lane: follow-up；Acceptance: 当前 gate 未过时不改 active Skill / 正式使用文档；Validation: 用户反馈及独立固化 change；Expected: 有证据再推进；Failure re-check: 无真实分母或效果不足时保留 pilot。

最终技术门：隔离工作树 `task check` 全部通过，证据 `temp/integration-test-runs/20260910T071955Z-research-isolated-quality/`。改名反链回归已修复；旧失败记录保留。固化任务已由 CLI 分别建立 Pinax `pinax-research-brief-workflow-v1` 与公共 Skill `pinax-research-brief-skill-v1`。

固化完成：两 owner change 已建立并完成本地正文、校验与 Skill runtime 同步；未提交、发布、归档或替换已安装 CLI。
