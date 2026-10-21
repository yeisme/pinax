# cli-resource-views Specification

## Purpose

统一 pinax 资源的列表、身份读取、详情和只读补全，保留已发布机器字段，并给出可执行的失败恢复。

## Requirements

### Requirement: 同源资源读取

CLI MUST 从 owner 的同一资源目录提供 list、身份读取、详情与补全，并保留已发布机器字段。

#### Scenario: 从列表进入详情
- **WHEN** 用户从 list 取得可见资源 ID
- **THEN** 对应详情入口接受原值，Agent 和 JSON 暴露同一范围下的身份集合

### Requirement: 可恢复错误与只读补全

CLI MUST 区分缺参、未知身份、空目录与目录不可读；补全 MUST 在 250ms 预算内仅使用本地只读元数据，最多返回 50 项，失败不向 stderr 写日志。

#### Scenario: 缺少身份
- **WHEN** 用户调用需要 ID 的 canonical 详情命令但未提供 ID
- **THEN** 返回非零退出、稳定错误码和可执行发现命令

#### Scenario: 未安装或远程目录不可用
- **WHEN** 补全无法读取本地目录
- **THEN** 返回零候选，不初始化状态、不联网或解析秘密

### Requirement: 兼容与安装证据

旧详情 show MUST 保留首次功能发布的隐藏兼容窗口；补全安装 MUST 保护用户已有文件，不修改 rc，并分别报告文件存在与 shell 可发现状态。

#### Scenario: 已有用户补全文件
- **WHEN** 安装目标已存在非受管文件
- **THEN** 默认拒绝覆盖，文件保持不变
