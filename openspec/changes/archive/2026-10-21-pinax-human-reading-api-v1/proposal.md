## Why

DriveBridge 将成为用户桌面阅读入口，但 Pinax 当前 Agent 渲染树有64KiB/512节点及脱敏上限，不能直接当作人类全文阅读接口。新增显式授权的人读合同，才能支持长文、关系、附件和安全桌面生命周期而不扩大旧工具权限。

## What Changes

- 新增只读 reader HTTP与公开typed client绑定，包含来源、列表/搜索、分页正文、关系、附件和显式原文导出。
- 人类正文scope独立，旧NoteDisplay、render.view.v1alpha1、CLI/MCP及URI语义不变。
- 新增opt-in受监督本机会话：私有stdin引导、内存bearer、脱敏ready、EOF退出；不新增daemon/公网监听或UI。
- API/schema/fixture使用现有transport catalog生成，能力按真实binding可用性发现。
- 本轮仅规格/文档/待办；不实现接口、不写用户vault、不启动模型/同步/真实验收。

## Capabilities

### New Capabilities

- `human-reading-api`：权限分离、分页固定版本、关系/资产/预算/失败与兼容。
- `supervised-local-session`：固定前台会话、秘密引导、loopback认证、版本发现与清理。

### Modified Capabilities

无旧接口删除/改义。本地服务文档澄清已存在的loopback API例外；新session仍显式启动且不常驻。

## Impact

未来实现位于internal/app、internal/api、internal/render、internal/cli、transportcatalog和pkg/pinaxclient及既有tests/testkit；不导入DriveBridge领域、不增加React/Electron。

唯一详细新合同为[human-reading-api.md](../../../docs/interfaces/human-reading-api.md)。消费方在[DriveBridge change](../../../../drivebridge/openspec/changes/drivebridge-desktop-knowledge-reader-v1/README.md)。旧真实验收5个change/11任务保持，fixture不能替代自然使用、Mac/S3或model evidence。
