# pinax-content-managed-sync-ignore Design

## 方案

Pinax 内容同步使用独立的 `.pinaxignore` 规则，Git 只用于 `.pinax` 项目元数据。Cloud Sync manifest 构建时递归扫描 vault 普通文件，应用 `.pinaxignore` 和内置 hard deny 后再生成 encrypted blob/manifest。

## 边界

- `.pinaxignore` 使用 Git-like 语义，首版只读取 vault 根目录文件。
- `.gitignore` 由 `pinax init` 为新 vault 写入；已有 vault 通过 `pinax vault ignore plan/apply` 在 Pinax 标记块内增量维护。
- hard deny 永远排除 `.git/`、`.pinax/`、运行态缓存、symlink 和特殊文件。
- 首版单文件大小上限为 100 MiB，避免一次性读入过大二进制。

## 验证

- `internal/vaultignore` 单元测试覆盖 ignore/反选/目录规则/hard deny。
- `internal/remote` manifest 测试覆盖 Markdown、脚本、二进制、忽略文件和权限位。
- CLI 测试覆盖 `pinax init` 生成 `.pinaxignore` 和 metadata-only `.gitignore`。
- focused 测试通过后运行 `task check` 与 `openspec validate --all --strict`。
