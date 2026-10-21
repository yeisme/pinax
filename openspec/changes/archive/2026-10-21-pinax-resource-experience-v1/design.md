## Context

本轮实现入口为 `pinax project`。详见 [resource-views.md](../../../docs/commands/resource-views.md)。现有命令注册信息是 canonical 路径与 flags 的来源，资源查询是默认输出、Agent、JSON 和补全的来源。

## Goals / Non-Goals

统一发现到详情的操作路径，保留完整 ID、可执行 Next 和稳定失败。没有获批的本地快照时，远程资源补全保持空；不通过补全联网、解析秘密、初始化项目或迁移数据库。导出流和独立业务动词不机械合并。

## Decisions

沿用现有语言、框架、renderer 和测试工具；没有新增跨仓 runtime 框架。兼容 show 保留原机器 identity。新增 describe 复用详情处理器，get 仅包含身份与关键字段。补全安装使用私有目录和受管标记，保护已有用户文件。未知父 shell 加载状态保持 unknown。

## Verification

测试覆盖目录身份、缺参/未知资源、stdout/stderr 分离和隔离安装。迁移资料由根 `scripts/skills.sh validate-cli-commands` 对候选 help 生成；校验器只执行 help，不执行文档中的写入、付费或部署示例。候选 digest、PATH 安装版本和真实 Provider 证据分别报告。尚未发布，兼容到期版本由首次正式功能发布确定。
