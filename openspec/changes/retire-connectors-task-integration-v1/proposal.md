# 退役 Pinax 的 Connectors 任务集成

根仓库已永久移除 `backend-server/connectors`。Pinax 当前工作树仍保留由该项目提供的 `connectors task` 规划调用、旧 `taskbridge` 兼容合同和相关测试，导致 Pinax 在构建、CLI 帮助和规划资产中继续依赖已删除项目。

本变更将规划流程收敛为 Pinax 本地能力：保留 vault、项目看板、每日/每周/月度规划、每日任务复盘和本地 action 草稿；删除外部任务运行时探测、`--taskbridge` 入口、Connectors/TaskBridge schema、Provider/Connector task 来源字段以及外部执行建议。

这是用户明确授权的立即退役，不提供运行期兼容窗口或备份。历史 OpenSpec 归档和历史 vault JSON 可继续作为历史资料，但不再是当前代码的输入合同。已提交文件可通过对应提交的 `git revert <commit>` 恢复；当前未提交、未跟踪的 Connectors 相关内容不可恢复。
