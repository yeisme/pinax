# notebook-source-attachments Specification

## Purpose
TBD - created by archiving change pinax-sync-attachment-contract-closeout-v1. Update Purpose after archive.
## Requirements
### Requirement: Pinax 原始源码与 revision 边界

Pinax SHALL 将 rich/composed projection 作为固定 revision 预览，保留 `source_anchor`、`origin` 与目标授权。写入 SHALL 只作用于原始 Markdown source，经 proposal、明确批准、snapshot、revision guard、apply 与 receipt，不接受拼接预览或任意路径作为写指令。

#### Scenario: 嵌入来源不可用

- **WHEN** 来源不可用、歧义、循环、超深度或不在授权域
- **THEN** Pinax 保留明确的不可用状态，不将父文档投影转换为写入

### Requirement: 原子附件引用

Pinax SHALL 复用 bounded input request，验证固定 note identity、MIME、大小、完整 revision、placement、snapshot、引用变更与回读 receipt。附件替换 SHALL 保持不可变内容和原 operation 恢复语义。

#### Scenario: 非法附件请求

- **WHEN** MIME、大小、placement 或 note revision 无效
- **THEN** Pinax 在 mutation 前拒绝，保留原源码、引用、revision 和 receipt

