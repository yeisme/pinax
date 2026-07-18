# Pinax 搜索、解析和懒加载索引增强

## 背景

当前 `pinax search` 已能使用 SQLite/GORM 索引，并在索引缺失或过期时走 fallback；但搜索入口仍会先扫描并解析全量 Markdown，缺失或过期索引时还可能在搜索路径触发全量 rebuild。大 vault 下这会把普通搜索延迟、内存占用和索引维护成本绑定在一起。

## 目标

- 让搜索默认不依赖外部 `rg`、`fzf`、`bat` 二进制，提供 Pinax 内置 native 搜索和内置交互选择体验。
- 为搜索提供显式 engine 和 lazy-index 策略，保持现有默认兼容但避免无界 rebuild。
- 建立统一 Markdown note parser，逐步替换 frontmatter、heading、link、asset、task、property 的重复解析。
- 为索引刷新和 native 搜索补充有界并发、取消、性能基准和 race 验证。

## 非目标

- 不在本轮引入 Rust/cgo ripgrep 或外部二进制打包。
- 不把 SQLite index 变成 Markdown vault 的真源。
- 不删除或重命名现有 CLI/JSON/agent 输出字段。
