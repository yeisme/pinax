## 任务

- [x] 1. 补共享 parser/resolver 的红灯测试，覆盖 alias、heading、同名冲突、frontmatter alias、附件 embed 忽略和去重边界。
- [x] 2. 抽取共享 link graph 规则并接入 app 查询路径。
- [x] 3. 接入 index rebuild / incremental projection，确保 `LinkRecord` 字段完整一致。
- [x] 4. 补 CLI/search/query focused tests，确认 JSON 字段兼容、`engine/index_status` 真实、断链/歧义计划可审查。
- [x] 5. 运行验证并记录结果：`go test ./internal/app ./internal/index -run 'Link|Incremental|Search|Consistency' -count=1`、`go test ./cmd/pinax -run 'NoteLinkGraphCLI|LinkOutput|BacklinkOutput|SearchLinkTarget' -count=1`、`openspec validate --all`。
