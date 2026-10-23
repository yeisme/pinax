# 根规格迁入索引

由根仓 `scripts/openspec-distribute.py --migrate --apply` 生成。

原包完整保留，迁移不改变任务勾选、能力承诺或证据等级。正文中的 root、旧执行目录和验收命令是历史上下文，不是当前执行授权。

`handoffs/` 内的跨项目责任仍归原 owner；此处存放设计参考，不把其他仓库交付、付费调用或人工验收变成本项目实施阻塞项。同主题已有本地实施包时继续使用原实施包，不能重复领取任务。已退役 Workbench 等消费者不因迁入而恢复；只有 metadata 的包仍是不完整草稿。

本项目当前实施入口通过 `openspec list` 查询；以下索引用于发现原始设计和交接。

- [pinax-continuity-operator-handoff-v1](changes/pinax-continuity-operator-handoff-v1/) — 迁入设计规格
- [workbench-project-continuity-program-v1](handoffs/workbench-project-continuity-program-v1/) — 跨 owner 历史交接
