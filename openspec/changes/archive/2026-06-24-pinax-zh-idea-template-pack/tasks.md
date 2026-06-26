# pinax-zh-idea-template-pack Tasks

- [x] 1. 为中文 idea 和内容模板写失败测试。
  - 证据：RED 运行 `go test ./internal/app -run 'TestBuiltInTemplateLegacyAndRecommendedInspect|TestBuiltInNoteTemplatesCatalogMetadata|TestBuiltInNoteTemplateMetadataAppliesToCreateNote' -count=1`，失败于缺少 `index.ideas` 和 `idea.research_seed`。
  - 证据：RED 运行 `go test ./cmd/pinax -run 'TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallbackCLI' -count=1`，失败于中文 intent fallback 到 `note.quick`。
- [x] 2. 新增内置中文模板包和 `index.ideas`。
  - 证据：`go test ./internal/app -run 'TestBuiltInTemplateLegacyAndRecommendedInspect|TestBuiltInNoteTemplatesCatalogMetadata|TestBuiltInNoteTemplateMetadataAppliesToCreateNote' -count=1`
  - 证据：`go test ./cmd/pinax -run 'TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallbackCLI' -count=1`
- [x] 3. 更新用户文档和 OpenSpec 说明。
  - 证据：README、中文 README、template 命令文档和本地开发文档包含 `idea.research_seed`、中文 recommend 和 `index.ideas` 示例。
- [x] 4. 运行最终质量门禁。
  - 证据：`task check` 通过，覆盖 lint、fmt-check、go test ./...、openspec validate --all、kb sidecar protocol 和 build。
