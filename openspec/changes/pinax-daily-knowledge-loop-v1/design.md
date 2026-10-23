# 日常知识闭环设计

## 决策与优先级

当前用户已批准此前 CEO 计划及其优先级调整：先实现记录、整理、阅读、找回；使用 Agent 对话与现有编辑器。此次 local implementation 是原 2026-10-10 continuity freeze 的明确 owner 重排，仅适用于本 change，不等于启动完整 rendered-view、MCP note proposal 或新客户端。连续性仍保留，旧实验不得用日常使用样本替代。

## 能力与归属

Pinax 持有 vault、对象身份、索引、变更和恢复（fit）；operator 持有自然语言编排、现有编辑器持有阅读交互（split-owner）。独立工作台、图谱渲染平台、新 provider 与自动合并/删除不在本次交付内（reject-now）。

## 最小实现

1. inbox capture 增加 stdin，CLI 读取后复用 CreateNoteRequest.StdinBody。与 body 同时显式指定时在写入前拒绝，读取失败也不得写入。旧 body 行为不变。
2. CreateNote 在成功写入后回读并核验保存内容；返回 stable note_id、readback 状态及打开动作。保存后的维护失败保留已知 note_id/path，提示先核对原对象，不提示重新创建。
3. inbox review 是独立的只读入口，不改变 inbox list。扫描当前显式 vault，聚合 status=inbox 与 notes/index 下尚未归入 project、非 archived/discarded/system 的普通笔记，按对象 ID 去重并按 path 稳定排序。默认最多五条、上限二十条，以 --after 稳定对象引用跳过已看条目；游标丢失时显式提示刷新，不猜位置。状态不另建、不自动搬文件。
4. 卡片包含 bounded snippet、来源类别、共享标签的相关笔记与建议原因。相关笔记不是自动分类结论，用户或 Agent 通过原 organize plan/review/apply 路径作决定。只读列表不产生待办确认负担。
5. 详情卡使用 UTF-8 安全截断，动作使用稳定 note ID（旧笔记回退 path），提供原文预览和本机编辑器打开动作。note open 不变成远程桌面控制。旧 source/card/body 权限边界不变。

## 兼容与恢复

CLI 新子命令/flag、facts/actions 均 additive；既有字段及命令保留。无 public API 删除、数据库结构或历史笔记迁移。回退新二进制和 operator 指引即可停用新增体验，既有 Markdown 与恢复链仍有效。body 从 stdin 接收，不写入调用参数或证据日志；测试只用合成内容。整理跳过为纯读，批量变更只使用已保存计划的 revision 检查，部分失败先回读再恢复，不静默重试。

## 验证与真实使用边界

复用 Go testing、现有 testscript 与 integrationevidence，新增 daily-knowledge profile。覆盖 stdin 长文/冲突/dry-run、五条批次与继续/去重/只读、中文片段、同名稳定动作、搜索/编辑器往返、失效链接/附件、plan stale 和恢复保护；证据写入本项目 temp/integration-test-runs。最终执行不含 race 的 task check。

两周自然使用只有真实用户行为才能完成；记录捕获成本、五条整理耗时、首次找回、再次使用及主动使用。目标为信息齐全捕获无追加分类问题、五条整理约五分钟、已知目标找回率向 80% 推进；未测量保持 unknown。测试通过不代表真实效果成立。
