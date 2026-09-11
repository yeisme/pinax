# 研究简报试点验收（2026-09-10）

本轮完成五题回顾性复用验收。用户整体认可五题结论，并明确批准“用已认可的来源建立回顾性基准，再跑一轮检索；首轮找回率仍标记未测量”。这不是独立盲测，也不证明未来问题有相同找回率。技术质量门见 tasks.md 1.4。

## 检索与引用

固定基准在执行查询前由运行程序生成：`temp/integration-test-runs/20260910T070710Z-research-retrospective/artifacts/baseline.json`。摘要为 `fd8a638de080ca8882b150055e446cf52a87b2e79d1aa9eea632d89e11658fa7`。每题三组关键词，每组候选最多 5 个，使用 `--lazy-index off`；检索阶段没有使用目标 ID 直接读取来冒充召回。查询先于新简报保存，避免新笔记污染旧资料检索。

| 问题 | 固定目标旧笔记 | 回顾性找回 |
| --- | --- | --- |
| DSH design md | 通用 Frontend Design.md 模板 | 是 |
| 项目生态 | OMH 项目偏移复盘：没有需求却制造监控审计需求 | 是 |
| AI 网关 | Local-first Agent Gateway：BYOK 与本地 Agent CLI 的多 Adapter 架构 | 是 |
| 未来方向 | AI 做剧开源生态的产品比较与 CEO 结论（2026-08） | 是 |
| 全自动做剧 | AI剧本的根源性问题 | 是 |

回顾性找回 5/5；首轮找回率未测量。五题结论获批次认可；没有独立逐题评分。8 个去重引用来源均通过 CLI 读取、日期及正文摘要核对，程序检查相关文本位置，Agent 人工阅读支持相应的限界结论。程序的关键词检查不等于自动证明语义支持。证据：`temp/integration-test-runs/20260910T070350Z-research-citations/`。

没有追加网络检索；历史笔记不证明当前 DSH 插件兼容性、Aigora 已实现能力或商业需求。剧本笔记中的模型机理描述未当作科学结论。未来方向保留为待真实用户和交付验证的假设。

## 采纳与保存

按用户采纳并继续原方案的授权，通过 `pinax note add --dir index --stdin` 保存，先 dry-run，再写入、回读并核对 backlinks。未覆盖旧笔记、未同步或发布。

| 简报 slug | 实际 note ID | 来源关系 |
| --- | --- | --- |
| research-brief-dsh-design-20260910 | 01a08a29-c6db-7bbe-a92b-b0e0adfc2b4e | 2 |
| research-brief-ecosystem-20260910 | 01a08a29-d807-7a04-9214-344a87c36219 | 2 |
| research-brief-ai-gateway-20260910 | 01a08a29-e3f0-7e93-8a2b-102e5c3276d6 | 2 |
| research-brief-product-direction-20260910 | 01a08a29-f14d-77e9-9850-bcc87cc7418f | 3 |
| research-brief-ai-drama-automation-20260910 | 01a08a2a-0019-7c51-a6e1-208fb2604490 | 2 |

保存证据为 `temp/integration-test-runs/20260910T071308Z-research-accepted-save/`。其中逐题来源数正确，但汇总误写为 12；已修正运行程序并生成新的验证证据 `temp/integration-test-runs/20260910T071725Z-research-accepted-save/`：五份全部复用原 ID，正文一致，来源关系总数 11，没有重复创建。旧证据保留，不能将其错误汇总当作正确计数。

所有正文记录研究结论、证据分歧、未知项、建议、改变判断的条件及来源 ID、实际更新时间和正文摘要。模板仅参考 `research/evidence-research-brief-beta@2.0.0-beta.1` 英文规范，默认中文输出；未完成 Registry 编译，不承诺重放模型生成。

## 技术检查与限制

已用隔离工作树提取本次改动，未纳入并发 MCP 改动。第一次全量检查遇到未改动的 pinaxclient 超时恢复测试失败，单独重复十次通过。第二次全量检查暴露本次索引状态判断导致的改名反链回归，单独重复十五次稳定失败；修复后重复十次通过。失败证据均保留，不将这些运行报为通过。

合成流程验证和真实使用验收分开计数。合成 evidence profile 的 `real_user_evaluations=0` 仍保持原义。已安装 Pinax 二进制未替换，本次真实操作使用本地构建候选。运行证据仅保留必要状态、引用、计数和摘要，正文存于用户批准的笔记内。

最终隔离 `task check` 全部通过：`temp/integration-test-runs/20260910T071955Z-research-isolated-quality/`。修复后的 research-brief 合成流程再次通过：`temp/integration-test-runs/20260910T072411Z-85295/`。使用文档见 `docs/guides/research-brief.md`；公共 Skill 在对应 source owner 的独立 change 中维护。
