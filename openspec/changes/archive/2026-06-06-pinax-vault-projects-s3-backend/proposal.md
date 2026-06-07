# Proposal: Vault Projects and S3 Backend Foundation

## Why

Pinax 已经具备本地 vault 初始化、校验、笔记读取、metadata 和 organize 基础能力。下一步需要让 vault 能承载多个项目，并为后续远端对象存储同步建立明确的后端配置边界。用户不应该手写 `.pinax/*.json` 或把 S3 secret 暴露到 stdout、fixture 或 Git。

## What Changes

- 增加 vault 内项目管理基础命令：创建项目、列出项目、切换当前项目。
- 使用 CLI/service 写入 `.pinax/projects.json`，记录项目 slug、名称、描述、notes 前缀和当前项目。
- 增加 storage backend 配置命令：设置 local 或 s3 backend、查看 backend 状态、doctor 校验配置完整性。
- S3 本轮只落地配置和诊断，不连接真实公网、不读取真实 secret、不上传/下载对象。
- 所有命令遵守现有 projection 输出合同，支持默认摘要、`--agent`、`--json`、`--events` 和 `--explain`。

## Out Of Scope

- 真实 S3 API 调用、MinIO 集成测试、对象上传下载和冲突合并。
- SQLite/GORM 索引迁移。
- 多用户权限、云端账号体系或长期 daemon。

## Validation

- `go test ./...`
- `task check`
- CLI smoke：`pinax project create/list/switch`、`pinax storage set-s3/status/doctor --json`
