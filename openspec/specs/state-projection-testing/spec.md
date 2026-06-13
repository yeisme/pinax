# state-projection-testing Specification

## Purpose
TBD - created by archiving change pinax-e2e-test-suite. Update Purpose after archive.
## Requirements
### Requirement: Local Markdown to SQLite Projection Consistency
当物理 Markdown 笔记文件（或元数据）在测试沙盒发生增、删、改等变动时，测试套件 SHALL 具备校验底层 SQLite 数据库缓存索引投影一致性的机制。

#### Scenario: Verify Bidirectional Link Projection Update
- **WHEN** 写入含有 `[[page-b]]` 的 Markdown notes 文件并运行 `pinax index sync` 同步时
- **THEN** 通过 `pinax query` 对 `page-b` 进行入链查询，返回的结构体数据中 SHALL 精确包含源文件 `page-a.md` 投影条目

