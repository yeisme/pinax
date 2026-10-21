# human-reading-api Specification

## Purpose

受监督人类阅读 API：独立 reader scope、可发现绑定、有界正文分页、关系与资产解析，以及无持久正文缓存。

## Requirements

### Requirement: 人类读取scope不改变Agent默认值
Pinax SHALL 新增由owner授予的reader scope，保留旧NoteDisplay/render.view/CLI/RPC/MCP正文门与脱敏预算；caller声明human不能升级权限。

#### Scenario: 旧Agent身份请求完整阅读
- **WHEN** 旧token没有reader.body却请求document或search snippet
- **THEN** 拒绝insufficient_scope，不泄漏正文，不自动扩大旧工具权限

### Requirement: 固定来源与可发现绑定
reader SHALL 只在实现且授权的binding可用时发现；列表/关键词搜索分页默认50/max200，缺索引独立于empty，读取不触发持久化修复。

#### Scenario: 本地搜索索引缺失
- **WHEN** 用户查询且owner索引不可用
- **THEN** 返回index_unavailable与修复提示，不自动refresh、不返回伪空结果或fallback私有扫描

### Requirement: 有界完整正文分页
reader SHALL 遵循人读合同的256KiB/512节点页、1MiB响应、8MiB源及65536节点总预算，以稳定revision/anchor/cursor连续返回；大块continuation不能截断UTF-8或静默丢内容。

#### Scenario: 长中文文档跨页
- **WHEN** 文档超过旧64KiB/512节点但仍在新预算内
- **THEN** 所有页同revision，拼接无缺失重复，目录和代码/表格连续，只有真正结束才complete

#### Scenario: 原版失效或超过总预算
- **WHEN** 后续页原revision不可达或解析超过上限
- **THEN** 明确revision_unavailable或预算错误，不混新版、不自动snapshot；超预算提供显式受权source出口

### Requirement: 关系与资产由owner解析
reader SHALL 校验关系目标权限与水位，asset以opaque ref绑定note/revision，经有界Range读取；不得输出任意路径或自动外网内容。

#### Scenario: 附件指向越界符号链接
- **WHEN** asset解析逃逸获授权内容根、ref被换成其他笔记资产或MIME危险
- **THEN** 拒绝或安全下载，不泄漏隐藏资源、路径和其他对象存在性

### Requirement: 新reader无持久正文缓存与日志
新reader SHALL 使用private,no-store并在每次读取复验scope；正文/查询/asset bytes不进入日志、trace或审计，旧cache合同保持。

#### Scenario: 权限在分页间失效
- **WHEN** 当前session/scope失效后继续请求
- **THEN** 拒绝并要求消费者清内容，不因cursor/旧asset-ref曾有效而继续读取
