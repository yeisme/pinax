## Why

promptrepo v0.5.1 已发布：Search 命中扩展到跨 locale 打分（选中 locale 之外的其他 locale 文本也参与评分），Source kind 检测支持 bare GitHub remote、scp 形态与 http/https/ssh scheme。Pinax 作为 v0.5 消费者应跟进补丁版，且 spec 合同以精确版本钉住消费版本。

## What Changes

- 升级 promptrepo v0.5.0 → v0.5.1 并刷新 vendor。
- `pinax-prompt-repository-import` 能力合同同步 v0.5.1。

## Capabilities

### Modified Capabilities
- pinax-prompt-repository-import: 消费版本从 v0.5.0 提升到 v0.5.1。

## Impact

SDK 公共 API、官方来源、共享 profile 根、rights-gated install 与本地 `pinax://prompt/` 资产分离均不变；仅 vendor 内部行为增强（跨 locale 打分、更多 git remote 形态识别）。
