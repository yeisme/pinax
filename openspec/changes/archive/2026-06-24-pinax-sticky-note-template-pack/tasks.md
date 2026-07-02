# pinax-sticky-note-template-pack Tasks

- [x] 1. 为 sticky 模板写失败测试。
  - 证据：RED 运行 `go test ./internal/app -run 'TestBuiltInNoteTemplatesCatalogMetadata|TestBuiltInNoteTemplateMetadataAppliesToCreateNote|TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallback' -count=1`，失败于缺少 `sticky.capture` 和推荐回落到 `note.quick`。
  - 证据：RED 运行 `go test ./cmd/pinax -run 'TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallbackCLI' -count=1`，失败于 `便签` 推荐回落到 `note.quick`。
- [x] 2. 新增内置 `sticky.*` 模板包和推荐 metadata。
  - 证据：`go test ./internal/app -run 'TestBuiltInNoteTemplatesCatalogMetadata|TestBuiltInNoteTemplateMetadataAppliesToCreateNote|TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallback' -count=1`
  - 证据：`go test ./cmd/pinax -run 'TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallbackCLI' -count=1`
- [x] 3. 更新用户文档和 OpenSpec 说明。
  - 证据：README、中文 README、template 命令文档、本地开发文档和 `notebook-workflows` spec 说明 sticky 模板和 project board 边界。
- [x] 4. 运行最终质量门禁。
  - 证据：`openspec validate --all` 通过，48 passed, 0 failed。
  - 证据：`task check` 通过，覆盖 `openspec validate --all`、`golangci-lint run`、`go test ./...`、`golangci-lint fmt --diff`、`kb:sidecar:protocol` 和 `go build`。
