package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/publishdocast"
)

// TestPublishDocNativePlanIntegration 验证 app 层 publishDocBuildPlan 把 note 正文 + vault template
// 解析为 provider-neutral plan，包含标题/段落/列表/表格/代码块。
func TestPublishDocNativePlanIntegration(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_x", Title: "X Note", Path: "notes/x.md", Body: "## Section\n\nparagraph text\n\n- a\n- b\n\n```go\ncode()\n```\n\n| H1 | H2 |\n| -- | -- |\n| 1 | 2 |\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, warnings := publishDocBuildPlan(note, profile)

	if plan.Revision != domain.PublishDocRenderRevision {
		t.Fatalf("revision = %q", plan.Revision)
	}
	types := map[string]bool{}
	for _, b := range plan.Blocks {
		types[b.Type] = true
	}
	for _, want := range []string{"heading", "paragraph", "list", "code", "table"} {
		if !types[want] {
			t.Fatalf("plan blocks missing type %q: %v", want, types)
		}
	}
	// vault template 正文里不应重复 note 标题。
	for _, b := range plan.Blocks {
		if b.Type == "heading" && b.Level == 1 && b.Text == note.Title {
			t.Fatalf("note title should not duplicate as H1 in body; vault template handles title")
		}
	}
	_ = warnings
}

// TestPublishDocVaultTemplateDuplicateTitleHandling 验证 stripPublishDocDuplicateTitle
// 在 note body 以与标题同名的 H1 开头时去重，避免原生文档标题重复。
func TestPublishDocVaultTemplateDuplicateTitleHandling(t *testing.T) {
	t.Parallel()
	body := "# Same Title\n\ncontent"
	stripped := stripPublishDocDuplicateTitle(body, "Same Title")
	if stripPublishDocDuplicateTitle(stripped, "Same Title") == "" && stripped == "" {
		t.Fatalf("stripping should not empty non-trivial body")
	}
	// 首行 H1 与 title 同名时被移除。
	if firstLine(stripped) == "# Same Title" {
		t.Fatalf("duplicate H1 not removed: %q", stripped)
	}
	// 标题不同时保留。
	keep := stripPublishDocDuplicateTitle("# Different\n\ncontent", "Same Title")
	if firstLine(keep) != "# Different" {
		t.Fatalf("non-matching H1 should be kept: %q", keep)
	}
}

// TestPublishDocNativePlanNoH1Note 验证无 H1 note 仍生成稳定 plan。
func TestPublishDocNativePlanNoH1Note(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_plain", Title: "Plain", Path: "notes/plain.md", Body: "Just a paragraph.\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, _ := publishDocBuildPlan(note, profile)
	if len(plan.Blocks) == 0 {
		t.Fatalf("expected at least one block for plain note")
	}
}

// TestPublishDocNativePlanChineseTitle 验证中文标题 normalization 不出错。
func TestPublishDocNativePlanChineseTitle(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_zh", Title: "中文标题。", Path: "notes/zh.md", Body: "正文段落。\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, _ := publishDocBuildPlan(note, profile)
	if plan.Title != "中文标题。" {
		t.Fatalf("title = %q", plan.Title)
	}
}

// TestPublishDocNativePlanMermaidKeptInBody 验证 Mermaid 保留在正文 fenced code block，
// 由 Feishu markdown 导入原生转换；plan 不把 Mermaid 作为 NativeAsset，也不在 plan 阶段告警。
func TestPublishDocNativePlanMermaidKeptInBody(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_mermaid", Title: "Mermaid", Path: "notes/m.md", Body: "```mermaid\ngraph TD\n  A-->B\n```\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, warnings := publishDocBuildPlan(note, profile)
	for _, w := range warnings {
		if w.Code == domain.PublishDocWarningMermaidUnavailable {
			t.Fatalf("plan stage must not warn mermaid_render_unavailable: %+v", w)
		}
	}
	for _, a := range plan.Assets {
		if a.Kind == "mermaid" {
			t.Fatalf("mermaid must not be a NativeAsset (Feishu handles natively): %+v", a)
		}
	}
	found := false
	for _, b := range plan.Blocks {
		if b.Type == "code" && b.Language == "mermaid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mermaid code block in body, got %+v", plan.Blocks)
	}
}

// TestPublishDocNativePlanSVGPreservedAsAsset 验证 inline SVG 在 plan 阶段作为 NativeAsset 保留，
// 不插入 raw SVG 到正文 block，也不在 plan 阶段发 warning。
func TestPublishDocNativePlanSVGPreservedAsAsset(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_svg", Title: "SVG", Path: "notes/s.md", Body: "<svg><rect/></svg>\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, warnings := publishDocBuildPlan(note, profile)
	// raw SVG 不进入正文 block text。
	for _, b := range plan.Blocks {
		if containsStr(b.Text, "<svg") {
			t.Fatalf("raw SVG must not enter body block text: %q", b.Text)
		}
	}
	// plan 阶段不发 svg warning。
	for _, w := range warnings {
		if w.Code == domain.PublishDocWarningSVGUnavailable {
			t.Fatalf("plan stage must not warn svg_render_unavailable: %+v", w)
		}
	}
	// SVG 源码在 asset 里。
	found := false
	for _, a := range plan.Assets {
		if a.Kind == "svg" && containsStr(a.Source, "<svg") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected svg NativeAsset, got %+v", plan.Assets)
	}
}

// TestPublishDocAssetPathEscapeRejected 验证路径逃逸被拒绝并产生 warning。
func TestPublishDocAssetPathEscapeRejected(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_escape", Title: "Escape", Path: "notes/e.md", Body: "![x](../../../etc/passwd)\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, warnings := publishDocBuildPlan(note, profile)
	hasEscape := false
	for _, w := range warnings {
		if w.Code == "image_path_escape" {
			hasEscape = true
		}
	}
	if !hasEscape {
		t.Fatalf("expected image_path_escape warning, got %+v", warnings)
	}
	// 逃逸图片应降级为 fallback paragraph，不作为 image media block。
	for _, b := range plan.Blocks {
		if b.Type == "image" {
			t.Fatalf("escaped image should not produce image media block: %+v", b)
		}
	}
}

// TestPublishDocNativePlanLocalImageAttachmentResolved 验证 vault 内相对图片被解析为 media ref。
func TestPublishDocNativePlanLocalImageAttachmentResolved(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_img", Title: "Image", Path: "notes/index/img.md", Body: "![diagram](assets/diagram.png)\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, _ := publishDocBuildPlan(note, profile)
	if len(plan.Media) != 1 {
		t.Fatalf("expected 1 media ref, got %d", len(plan.Media))
	}
	ref := plan.Media[0]
	if ref.Path != "notes/index/assets/diagram.png" {
		t.Fatalf("media path = %q, want vault-relative", ref.Path)
	}
	// 不保存绝对路径或 token。
	if ref.Token != "" {
		t.Fatalf("media ref should not carry token before provider upload: %+v", ref)
	}
}

// TestPublishDocResolveRendererCompatibility 验证旧 profile/mapping 兼容读取规则。
func TestPublishDocResolveRendererCompatibility(t *testing.T) {
	t.Parallel()
	// 新 profile：native-docx。
	newProfile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	if newProfile.ResolveDocRenderer() != domain.PublishDocRendererNativeDocx {
		t.Fatalf("new lark-doc profile should default to native-docx")
	}
	// 旧 profile（磁盘读取、无 renderer 字段）：markdown-file 兼容。
	oldProfile := domain.PublishDocProfile{Target: domain.PublishDocTargetLarkDoc}
	if oldProfile.ResolveDocRenderer() != domain.PublishDocRendererMarkdownFile {
		t.Fatalf("old profile without renderer should resolve to markdown-file for compatibility")
	}
	// 旧 mapping（file type）：markdown-file。
	fileMapping := domain.PublishDocMapping{Target: domain.PublishDocTargetLarkDoc, ExternalObject: domain.PublishDocExternalObject{Type: domain.PublishDocObjectTypeFile}}
	if fileMapping.ResolveDocMappingRenderer() != domain.PublishDocRendererMarkdownFile {
		t.Fatalf("file mapping should resolve to markdown-file")
	}
	// 旧 mapping（document type）：native-docx。
	docMapping := domain.PublishDocMapping{Target: domain.PublishDocTargetLarkDoc, ExternalObject: domain.PublishDocExternalObject{Type: "document"}}
	if docMapping.ResolveDocMappingRenderer() != domain.PublishDocRendererNativeDocx {
		t.Fatalf("document mapping should resolve to native-docx")
	}
}

// TestPublishDocNativePlanProviderNeutral 验证 plan 与 lark-cli 命令无关（provider-neutral）。
func TestPublishDocNativePlanProviderNeutral(t *testing.T) {
	t.Parallel()
	note := domain.Note{ID: "note_pn", Title: "PN", Path: "notes/pn.md", Body: "text\n"}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	plan, _ := publishDocBuildPlan(note, profile)
	// plan 只含 block order 和 media refs，不包含 provider 命令名。
	planJSON := string(mustMarshalPlan(plan))
	for _, bad := range []string{"lark-cli", "docx +create", "markdown +create", "shell", "exec"} {
		if containsStr(planJSON, bad) {
			t.Fatalf("plan should not contain provider-specific command %q: %s", bad, planJSON)
		}
	}
}

func mustMarshalPlan(plan publishdocast.NativePlan) []byte {
	body, err := publishdocast.MarshalNativePlan(plan)
	if err != nil {
		panic(err)
	}
	return body
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && indexOfStr(s, sub) >= 0
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestPublishDocNativeInsertAssetsUsesManifestCleanup(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "calls.log")
	fake := filepath.Join(fakeBin, "lark-cli")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\n" +
		"if [ \"$1\" = \"docs\" ] && [ \"$2\" = \"+fetch\" ]; then echo '{\"ok\":true,\"data\":{\"document\":{\"document_id\":\"doc1\",\"content\":\"<title>D</title><whiteboard id=\\\"wb_new\\\" token=\\\"wb_tok\\\"></whiteboard><img id=\\\"user_img\\\"></img>\"}}}'; exit 0; fi\n" +
		"if [ \"$1\" = \"docs\" ] && [ \"$2\" = \"+update\" ]; then echo '{\"ok\":true,\"data\":{\"document\":{\"document_id\":\"doc1\"}}}'; exit 0; fi\n" +
		"if [ \"$1\" = \"whiteboard\" ] && [ \"$2\" = \"+update\" ]; then echo '{\"ok\":true}'; exit 0; fi\n" +
		"if [ \"$1\" = \"docs\" ] && [ \"$2\" = \"+media-insert\" ]; then echo '{\"ok\":true,\"data\":{\"block_id\":\"img_new\"}}'; exit 0; fi\n" +
		"echo '{\"ok\":true}'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(root, "image.png"), buildTestPNGHeader(2, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := publishdocast.NativePlan{Revision: domain.PublishDocRenderRevision, Assets: []publishdocast.NativeAsset{{Kind: "svg", Source: "<svg></svg>"}, {Kind: "image", Source: "image.png"}}}
	planBody, err := publishdocast.MarshalNativePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	pkg := domain.PublishDocPackage{NativePlan: planBody}
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	warnings, blocks := publishDocNativeInsertAssets(context.Background(), root, profile, "doc1", pkg, []string{"old_pinax_block", "old_pinax_block", ""})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if strings.Join(blocks, ",") != "wb_new,img_new" {
		t.Fatalf("asset manifest = %+v", blocks)
	}
	logBody, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBody)
	if !strings.Contains(log, "--command block_delete --block-id old_pinax_block") {
		t.Fatalf("manifest cleanup command missing:\n%s", log)
	}
	if strings.Contains(log, "user_img") {
		t.Fatalf("cleanup must not target fetched unmanaged cloud blocks:\n%s", log)
	}
}

func TestPublishDocExecutableUsesOfficialNotionCLI(t *testing.T) {
	t.Parallel()
	if got := publishDocExecutable(domain.PublishDocTargetNotionPage); got != "ntn" {
		t.Fatalf("publishDocExecutable(notion-page) = %q, want ntn", got)
	}
}
