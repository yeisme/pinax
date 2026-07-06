package publishdocast

import (
	"strings"
	"testing"
)

func TestPublishDocNativePlanCommonBlocks(t *testing.T) {
	body := "# Heading\n\nparagraph with [link](https://example.com)\n\n- a\n- b\n\n> quote\n\n```go\ncode()\n```\n\n| H1 | H2 |\n| -- | -- |\n| 1 | 2 |\n"
	doc := Parse("Title", body)
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})

	if plan.Revision != RenderRevision {
		t.Fatalf("revision = %q", plan.Revision)
	}
	if plan.Title != "Title" {
		t.Fatalf("title = %q", plan.Title)
	}
	types := []string{}
	for _, b := range plan.Blocks {
		types = append(types, b.Type)
	}
	for _, want := range []string{"heading", "paragraph", "list", "blockquote", "code", "table"} {
		if !sliceContains(types, want) {
			t.Fatalf("plan blocks missing type %q: %v", want, types)
		}
	}
}

func TestPublishDocNativePlanLocalImageMedia(t *testing.T) {
	doc := Parse("T", "![diagram](assets/diagram.png)\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes/index"})
	// 本地图片同时出现在 body image block 和 NativeAssets（adapter media-insert）。
	if len(plan.Media) != 1 {
		t.Fatalf("expected 1 media ref, got %d", len(plan.Media))
	}
	ref := plan.Media[0]
	if ref.Path != "notes/index/assets/diagram.png" {
		t.Fatalf("media path = %q, want vault-relative", ref.Path)
	}
	if ref.MediaType != "image/png" {
		t.Fatalf("media type = %q", ref.MediaType)
	}
	if !hasAsset(plan.Assets, "image", "notes/index/assets/diagram.png") {
		t.Fatalf("expected image NativeAsset, got %+v", plan.Assets)
	}
}

func TestPublishDocNativePlanPathEscapeRejected(t *testing.T) {
	doc := Parse("T", "![escape](../../../etc/passwd)\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	// 逃逸图片不产生 NativeAsset，降级为 fallback paragraph + warning。
	for _, a := range plan.Assets {
		if a.Kind == "image" {
			t.Fatalf("escaped image should not produce image asset: %+v", a)
		}
	}
	found := false
	for _, w := range plan.Warnings {
		if w.Code == "image_path_escape" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected image_path_escape warning, got %+v", plan.Warnings)
	}
}

func TestPublishDocNativePlanRemoteImageDownloadedAsAsset(t *testing.T) {
	doc := Parse("T", "![logo](https://example.com/logo.png)\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	// 远程图片现在作为 remote-image asset，adapter 下载后上传；不再产生 remote_image_linked warning。
	if !hasAssetKind(plan.Assets, "remote-image") {
		t.Fatalf("expected remote-image NativeAsset, got %+v", plan.Assets)
	}
	if hasWarning(plan.Warnings, "remote_image_linked") {
		t.Fatalf("plan stage must not warn remote_image_linked (adapter downloads at push)")
	}
}

func TestPublishDocNativePlanRemoteSVGDownloadedAsAsset(t *testing.T) {
	doc := Parse("T", "![diagram](https://example.com/diagram.svg)\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	if !hasAssetKind(plan.Assets, "remote-svg") {
		t.Fatalf("expected remote-svg NativeAsset, got %+v", plan.Assets)
	}
}

func TestPublishDocAttachmentLinksCollected(t *testing.T) {
	body := "See [the report](assets/report.pdf) and [slides](../deck.pptx) for details.\n"
	doc := Parse("T", body)
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes/index"})
	atts := []NativeAsset{}
	for _, a := range plan.Assets {
		if a.Kind == "attachment" {
			atts = append(atts, a)
		}
	}
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachment assets, got %d: %+v", len(atts), atts)
	}
	// 图片链接不被当作附件。
	foundPDF := false
	for _, a := range atts {
		if strings.HasSuffix(a.Source, "assets/report.pdf") {
			foundPDF = true
		}
	}
	if !foundPDF {
		t.Fatalf("expected report.pdf attachment, got %+v", atts)
	}
}

func TestPublishDocAttachmentExcludesImageLinks(t *testing.T) {
	body := "![image](assets/pic.png) and [doc](assets/file.txt)\n"
	doc := Parse("T", body)
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	for _, a := range plan.Assets {
		if a.Kind == "attachment" && strings.HasSuffix(a.Source, ".png") {
			t.Fatalf("png must not be collected as attachment: %+v", a)
		}
	}
}

// TestPublishDocMermaidKeptInBodyNotAsset 验证 Mermaid 保留为正文 fenced code block，
// 由 Feishu docs +create --doc-format markdown 原生转换为 <whiteboard type="mermaid">。
// plan 不把 Mermaid 作为 NativeAsset（adapter 不重复处理），也不在 plan 阶段告警。
func TestPublishDocMermaidKeptInBodyNotAsset(t *testing.T) {
	doc := Parse("T", "```mermaid\ngraph TD\n  A-->B\n```\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	if len(plan.Blocks) != 1 || plan.Blocks[0].Type != "code" || plan.Blocks[0].Language != "mermaid" {
		t.Fatalf("expected mermaid code block in body, got %+v", plan.Blocks)
	}
	for _, a := range plan.Assets {
		if a.Kind == "mermaid" {
			t.Fatalf("mermaid must not be a NativeAsset (Feishu handles natively): %+v", a)
		}
	}
	if hasWarning(plan.Warnings, "mermaid_render_unavailable") {
		t.Fatalf("plan stage must not warn mermaid_render_unavailable")
	}
}

func TestPublishDocSVGImagePreservedAsNativeAsset(t *testing.T) {
	doc := Parse("T", "![diagram](assets/diagram.svg)\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	if !hasAssetKind(plan.Assets, "svg-file") {
		t.Fatalf("expected svg-file NativeAsset, got %+v", plan.Assets)
	}
	// SVG 文件不在 plan 阶段告警（adapter 读文件后服务端渲染）。
	if hasWarning(plan.Warnings, "svg_render_unavailable") {
		t.Fatalf("plan stage must not warn svg_render_unavailable for svg-file")
	}
}

func TestPublishDocInlineSVGPreservedAsNativeAsset(t *testing.T) {
	doc := Parse("T", "<svg><rect/></svg>\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	if !hasAssetKind(plan.Assets, "svg") {
		t.Fatalf("expected inline svg NativeAsset, got %+v", plan.Assets)
	}
	// 不在 plan 阶段告警。
	if hasWarning(plan.Warnings, "svg_render_unavailable") {
		t.Fatalf("plan stage must not warn svg_render_unavailable")
	}
}

// TestPublishDocInlineSVGNoRawInsertion 验证 raw SVG 源码不进入正文 block text。
func TestPublishDocInlineSVGNoRawInsertion(t *testing.T) {
	doc := Parse("T", "<svg onload=\"alert(1)\"><script>alert(2)</script></svg>\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	for _, b := range plan.Blocks {
		if contains(b.Text, "<svg") || contains(b.Text, "<script") || contains(b.Text, "onload") {
			t.Fatalf("raw SVG/script must not enter body block text: %q", b.Text)
		}
	}
	// SVG source 保留在 asset（交给 provider 服务端渲染），不在正文。
	for _, a := range plan.Assets {
		if a.Kind == "svg" && !contains(a.Source, "<svg") {
			t.Fatalf("inline svg asset should carry svg source: %+v", a)
		}
	}
}

func TestPublishDocRenderWarningUnsupportedHTML(t *testing.T) {
	doc := Parse("T", "<div class=\"ad\">spam</div>\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	if !hasWarning(plan.Warnings, "unsupported_html_omitted") {
		t.Fatalf("expected unsupported_html_omitted warning, got %+v", plan.Warnings)
	}
}

func TestPublishDocAssetResolutionAbsoluteRejected(t *testing.T) {
	_, err := resolveRelativePath("notes", "/etc/passwd")
	if err == nil {
		t.Fatalf("absolute path should be rejected")
	}
}

func TestPublishDocAssetResolutionNormalRelative(t *testing.T) {
	resolved, err := resolveRelativePath("notes/index", "./diagram.png")
	if err != nil {
		t.Fatalf("relative path rejected: %v", err)
	}
	if resolved != "notes/index/diagram.png" {
		t.Fatalf("resolved = %q", resolved)
	}
}

func TestPublishDocNativePlanMarshalRoundTrip(t *testing.T) {
	doc := Parse("T", "# H\n\ntext\n\n```mermaid\ngraph TD\n  A-->B\n```\n")
	plan := BuildNativePlan(doc, AssetOptions{NoteDir: "notes"})
	body, err := MarshalNativePlan(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, err := UnmarshalNativePlan(body)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Revision != plan.Revision || back.Title != plan.Title || len(back.Blocks) != len(plan.Blocks) || len(back.Assets) != len(plan.Assets) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, plan)
	}
}

func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func hasWarning(warnings []PlanWarning, code string) bool {
	for _, w := range warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

func hasAsset(assets []NativeAsset, kind, sourceContains string) bool {
	for _, a := range assets {
		if a.Kind == kind && contains(a.Source, sourceContains) {
			return true
		}
	}
	return false
}

func hasAssetKind(assets []NativeAsset, kind string) bool {
	for _, a := range assets {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
