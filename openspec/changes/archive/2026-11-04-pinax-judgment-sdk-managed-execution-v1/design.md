# 设计

## 准入与 owner

fit：收件箱分类和整理建议；vault 写入、tag、move 仍经 owner 保护门。领域状态保留在本项目，公共 SDK 维护 Provider wire 映射；根只做交接。

## 接线与约束

接线入口 `internal/inboxjudgment`。普通构建须能够装配已配置 SDK，不把 tagged/fixture 编译成功升级成业务已接入。shared/env 显式选择；宿主授权检查先于凭据读取。配置 fetch/activate/report 由显式命令或 application service 完成，执行不触发隐式中央同步。

每次操作绑定 owner/project/request/attempt、input/profile/question/policy digest；先 durable claim，再单次调用，回读成功零调用，未知不重提。关闭时零判断、零凭据读取；模型结果只作为建议。历史 JEV pin/legacy 行为保持不变。

MediaOps/Gateway/Console 不装配 Provider executor；分别只交接、转发、管理。credentialctl 只注册 pairing，不提供执行器；真实 grant 由用户显式命令完成。其他 owner 在自己的 state service 保存 neutral result，管理端只收 summary。

## 验收与恢复

复用本 owner 现有测试库、命令与证据包装。验证 default off、授权拒绝、scope/digest 冲突、拒答与 required abstention、持久 claim/restart/unknown、旧 JEV 兼容和真实业务入口。集成证据写入本项目 temp/integration-test-runs。本地 mock 不证明 Provider、内容效果或部署。默认不运行 race。

## 外部上线检查

真实凭据 grant、付费 Provider、部署、人工内容效果单独记录，不占本地 active checklist。
