# Agent 对话与现有编辑器的日常笔记流程

Pinax 负责保存、检索、整理和恢复；当前 Agent 负责理解意图与建议；你已有的 Markdown 编辑器负责阅读全文。无需新增工作台、常驻服务或付费判断模型。

## 先确认环境

```bash
pinax vault list --agent
pinax inbox capture --help
pinax inbox review --help
```

选定已有本地 vault 后，下文用 `./my-notes` 表示它。未登记 vault 的机器不能证明另一台机器没有笔记。不要为了“修好空列表”重新初始化已有 vault；使用现有注册/选择命令。旧二进制缺少新参数时先报告版本差异，不能默默改成空正文或另一种存储语义。

## 记录：明确要保存，就一次完成

可以直接说“把这段保存到 Pinax”。Agent 拟一个短标题，保留原文、代码块和已提供的来源，暂不追问分类、标签或模板。长正文通过标准输入传递：

```bash
pinax inbox capture "Reading fragment" --stdin --vault ./my-notes --json < fragment.md
pinax note add "Compiled note" --dir index --stdin --vault ./my-notes --json < compiled.md
```

第一条存零散素材，第二条存已整理成文的内容。`--stdin` 与 `--body` 不得同时指定；空 stdin 会报错，省略该参数仍可创建仅有标题的笔记。`--dry-run` 只预览，不记录为一次成功保存。远程 inbox capture 会在调用端读 stdin 后发送既有 body 字段，仍受原有远程确认和 operation recovery 合同约束。

保存结果中的 `note_id` 是后续读取的稳定引用；`readback_status=verified` 表示落盘内容已回读核验。索引等后续步骤失败时，结果保留已保存对象的 ID/path 和读取动作。先核对原对象，不要重复执行创建。旧二进制没有 readback 字段时，按返回 ID 显式回读。

## 整理：五条一批，允许跳过

```bash
pinax inbox review --limit 5 --vault ./my-notes --json
```

只读聚合 inbox 与 `notes/index/` 下未归入 project 的普通笔记，排除归档、丢弃与系统笔记；对象按 ID 去重。旧 `inbox list` 行为保持不变。卡片附少量片段、共享标签的相关笔记和 inspect/open 动作，标签相同只是关联线索，不是自动归类结论。

Agent 展示建议去向、原因、将改哪些字段；用户可接受、修改或跳过。默认不重写正文、不合并、不丢弃。跳过不写状态。读取下一批使用返回的 `next` 动作；完成修改后重新运行 review，不复用旧 `--after`。游标缺失或歧义时会要求刷新。

批量变更由 organize/repair 计划与 snapshot/apply 合同执行。小批整理使用 experimental 的选择集路径：

```bash
pinax organize plan --note <note-id-a> --note <note-id-b> --vault ./my-notes --json
pinax organize plan --note <note-id-a> --note <note-id-b> --save --vault ./my-notes --json
pinax pipeline describe <plan-id> --vault ./my-notes --json
pinax organize apply --plan <plan-id> --yes --vault ./my-notes --json
```

review 返回的 `organize_preview` 只预览当前批，不保存、不批准。只选其中几条时显式传对应 ID；最多 20 个唯一对象。无 ID 的旧笔记需要单独审阅 adopt。省略 `--note` 仍是全库路径，不能作为缺参数时的降级。关联判断仍参考全库，但写入只限选择集；这不是自动分类或替你决定文件夹。scoped apply 禁止 `--allow-stale`，预检后创建快照；中断、partial 或终态计划不能重复应用。先审阅计划是否仅包含本次选定的内容；不得为处理五条笔记批准整个 vault 的无关操作。内容漂移时重新生成计划。部分失败分别报告已完成、未处理、失败对象，恢复前检查后来编辑。

## 阅读与找回

```bash
pinax search "reading" --limit 5 --lazy-index off --vault ./my-notes --agent
pinax search describe "<note-id>" --vault ./my-notes --json
pinax note preview "<note-id>" --vault ./my-notes
pinax note open "<note-id>" --vault ./my-notes
```

`note open` 使用本机 `EDITOR`，也可显式传 `--editor`。它打开运行 Pinax 的机器，不会控制远程用户桌面。远程 Agent 应提供用户本机可执行的命令，并使用本机 vault 引用。编辑器不可用时，使用 preview 或显式 source/body 阅读。

Agent 默认拿有界卡片；你明确要读原文时不必停留在摘要。笔记可能包含未解析链接、缺失附件或过期信息，这些状态不能被总结为“已验证”。全文、模型输入和个人来源正文不进入测试证据。

不记得标题时，Agent 最多先尝试三组关键词、同义表达或标签，每组最多五条候选。没有命中代表当前检索缺口；不能声称库中不存在。索引陈旧时可以直接查 Markdown：

```bash
pinax search "reading" --engine native --lazy-index off --vault ./my-notes --agent
```

`--lazy-index off` 不更新索引。日常反复使用可显式 `pinax index refresh`；不能把重建索引作为看一篇笔记的必需动作。

## 两周真实使用观察

从主力环境安装本次已验证版本并确认 vault 后开始计时，不从工程测试日期起算。先观察当前流程的真实基线，再在两周自然使用中记录：

| 场景 | 成功行为 | 观察 | 初始标签 |
| --- | --- | --- | --- |
| capture | 原文保存并回读 | 耗时、澄清次数、失败原因 | first-support（本地 fixture） |
| triage | 五条以内、建议清楚、可跳过/暂停 | 耗时、修改建议、撤销 | first-support（本地 fixture） |
| read | 同名可定位、编辑器能读、附件状态真实 | 打开失败、格式/链接问题 | first-support（本地 fixture） |
| retrieve | 第一次或改写查询后找回目标 | 初次命中、补充线索、漏检 | exploratory（真实效果） |
| reuse | 旧内容用于新的阅读或任务 | 具体使用实例、是否主动使用 | exploratory（真实效果） |

用户可以用普通观察笔记或现有 journal 记录，不新增遥测服务。希望保存时仍通过 Pinax CLI 创建；不记录原始模型输入、对话、秘密或完整正文日志。每周只做一次简短回顾，不把每次读取变成问卷。

目标是信息齐全时捕获无需分类追问、五条整理约五分钟、已知目标首次找回率向 80% 推进。没发生需求、发生了但绕开工具、用了却没有记录，应分别说明。缺失反馈不计成功；零样本为未测量。此观察不替代 continuity 原有 30-loop 分母。

本地流程验证：

```bash
go run ./tools/testkit/integrationevidence --profile daily-knowledge
go run ./tools/testkit/integrationevidence --profile daily-knowledge-quality
```

每次结果位于 `temp/integration-test-runs/<run-id>/`，包括 summary、command、stdout、stderr、env 与 artifacts。合成流程覆盖失败与恢复边界，不证明真实找回率、留存、两周使用、生产部署或付费模型效果。
