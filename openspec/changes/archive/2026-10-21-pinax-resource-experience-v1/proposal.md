## Why

pinax 的目录发现、对象详情与补全需要一致的资源身份和恢复动作，避免只显示计数、同名详情语义混用或文档先于二进制。

## What Changes

- 在本 owner 复用现有查询与 renderer，增加 canonical 资源读取和只读补全。
- 为对象详情 show 保留一个功能版本的隐藏兼容窗口；独立动作保持语义。
- 补全安装不编辑 rc，明确文件安装与 shell 可发现的区别。
- 更新产品文档、合同测试及由真实命令树生成的迁移资料。

## Capabilities

### New Capabilities

- `cli-resource-views`: 资源身份、多模式输出、错误恢复与只读补全。

### Modified Capabilities

## Impact

能力判定为 fit；本仓拥有实现和测试，根仓只拥有共享规范及跨 owner 迁移索引。Skills 源由根 manager 同步。保留现有机器字段、脏工作树和权限边界；不调用真实 Provider、不运行 race、不发布或推送。
