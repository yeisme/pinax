# pinax-content-managed-sync-ignore Proposal

## 背景

当前 Cloud Sync manifest 只扫描 Markdown 文件，无法把脚本、资源和二进制文件作为 Pinax 管理内容同步。用户希望 Git 只承担 `.pinax` 项目元数据审计，正文和二进制内容由 Pinax 通过 provider/Cloud Sync 管理。

## 目标

- 引入 `.pinaxignore` 作为 Pinax 内容 manifest 的主忽略规则。
- 将 Cloud Sync manifest 从 Markdown-only 扩展为同步全部未忽略普通文件。
- 新 vault 默认生成 `.pinaxignore` 与 metadata-only `.gitignore`。
- 提供 `pinax vault ignore status|plan|apply` 检查和修复 ignore 配置。

## 非目标

- 不新增新的远端 provider 类型，继续复用现有 server/file/S3/rclone transport。
- 不把 `.gitignore` 作为 Pinax 内容规则来源。
- 不同步 symlink、device file、socket、FIFO 或 `.pinax` 运行态资产。
