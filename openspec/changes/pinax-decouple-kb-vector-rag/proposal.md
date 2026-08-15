# Pinax 与向量 RAG 解耦

## 目标

移除 Pinax 对向量数据库、embedding provider、Inferrum/LanceDB sidecar 和
Pinax 内置 RAG 检索的运行时依赖。Pinax 继续拥有 Markdown vault、SQLite/GORM
本地索引、确定性的全文检索、memory ledger 和加密同步；未来 RAG 系统由外部
项目独立设计、索引和部署。

## 变更范围

- 删除 `internal/semantic`、KB generation/evaluation/review 服务、Inferrum vendor
  和 Go module 依赖。
- 移除 `kb.sidecar.*` 配置与 `PINAX_KB_*` 环境变量。
- 保留已发布 `pinax kb` 命令名一 个迁移窗口，但所有旧子命令只返回
  `kb_decoupled`，不启动进程、不读写向量文件、不访问 provider。
- 保留 `/v1/kb/review/*` 路径一 个迁移窗口，统一返回 HTTP 410 和
  `error.code=kb_decoupled`，避免消费者误以为 review 数据仍然可信。
- 清理 sidecar canary、向量测试、KB 文档、Taskfile 任务和发布检查。

## 新边界

外部 RAG 系统只能通过稳定的 Markdown 导出或用户明确选择的 API 获取资料，
自行负责 ingest、chunk、embedding、vector store、rerank、context 和质量评测。
Pinax 不保存、不同步、不审计外部向量或 provider payload。

## 兼容、回滚和数据安全

- 兼容窗口为当前发布版本之后的一个 release；窗口内旧 CLI/API 只给出迁移提示，
  不执行旧逻辑。下一 major release 可移除兼容命令和 410 路由。
- 本次清理已移除仓库和测试工作区中明确枚举的历史 `.pinax/kb/**` 产物；不删除
  用户 vault 笔记、`.pinax` 同步配置、加密 revision 或对象存储数据。
- 回滚只能恢复上一版 Pinax binary/commit；被清理的历史向量产物不作为回滚数据，
  外部 RAG 必须从 Markdown export 重建。

## 非目标

本变更不实现新的 RAG 后端、不选择向量数据库、不改变 Cloud Sync 加密格式，
也不把向量数据上传到 S3/COS。
