package publishdocast

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// RenderRevision 标记 native render plan 的 schema revision。
const RenderRevision = domain.PublishDocRenderRevision

// NativePlan 是 provider-neutral 原生文档渲染计划。
// 与 lark-cli 命令无关，便于 fake provider 测试。
// app 层在 push 时把它交给 provider adapter，由 adapter 翻译成具体命令。
type NativePlan struct {
	Revision string         `json:"revision"`
	Title    string         `json:"title"`
	Blocks   []PlanBlock    `json:"blocks"`
	Media    []PlanMediaRef `json:"media,omitempty"`
	// Assets 列出可在 push 阶段服务端渲染/上传的可读资产（Mermaid/SVG/本地图片）。
	// plan 不预判 provider 能力；adapter 在 push 时尝试渲染，失败才产生 warning。
	Assets   []NativeAsset `json:"assets,omitempty"`
	Warnings []PlanWarning `json:"warnings,omitempty"`
}

// NativeAsset 描述一个待 provider adapter 渲染并插入原生文档的资产。
// Kind 决定渲染策略：mermaid/svg 走 whiteboard 服务端渲染；image 走 media-upload。
type NativeAsset struct {
	Kind   string `json:"kind"`          // mermaid | svg | image
	Source string `json:"source"`        // mermaid/svg 源码；image 为 vault-relative 路径
	Alt    string `json:"alt,omitempty"` // 说明文字/alt
	// Origin 标记来源（inline-svg / mermaid:fence / image ref），便于调试。
	Origin string `json:"origin,omitempty"`
}

// PlanBlock 是原生文档的一个有序块。Type 用稳定字符串值，provider 各自映射。
type PlanBlock struct {
	Type     string     `json:"type"`
	Text     string     `json:"text,omitempty"`
	Level    int        `json:"level,omitempty"`
	Language string     `json:"language,omitempty"`
	Code     string     `json:"code,omitempty"`
	Ordered  bool       `json:"ordered,omitempty"`
	Items    []string   `json:"items,omitempty"`
	Headers  []string   `json:"headers,omitempty"`
	Rows     [][]string `json:"rows,omitempty"`
	// MediaKey 引用 NativePlan.Media 中的资产 key（图片/渲染图）。
	MediaKey string `json:"media_key,omitempty"`
	Alt      string `json:"alt,omitempty"`
	// Fallback 标记该块是降级内容（如 Mermaid 源码 code block），供输出说明。
	Fallback bool `json:"fallback,omitempty"`
}

// PlanMediaRef 引用待上传的发布资产。Token 由 provider adapter 在上传后回填。
type PlanMediaRef struct {
	Key       string `json:"key"`
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	Source    string `json:"source,omitempty"`
	Token     string `json:"token,omitempty"` // provider 回填
}

// PlanWarning 对应 domain.PublishDocRenderWarning。
type PlanWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// planBuilder 收集 plan blocks / media / assets / warnings，内部保证 block 与 media key 关联一致。
type planBuilder struct {
	opts      AssetOptions
	media     map[string]PlanMediaRef
	warnings  []PlanWarning
	collected []PlanBlock
	assets    []NativeAsset
}

// BuildNativePlan 把发布 AST + 资产选项合成为 provider-neutral 原生文档计划。
// 产物不依赖 lark-cli 具体命令；包含 block order 和 media references。
// 资产解析与 plan 组装在同一遍完成，保证 block.media_key 与 plan.media 对齐。
func BuildNativePlan(doc Document, opts AssetOptions) NativePlan {
	b := &planBuilder{opts: opts, media: map[string]PlanMediaRef{}}
	for _, block := range doc.Blocks {
		b.addBlock(block)
	}
	// 附件检测：扫描正文 Markdown 链接 [text](local-path)，非图片扩展名作为 attachment 资产。
	b.collectAttachments(doc)
	plan := NativePlan{Revision: RenderRevision, Title: doc.Title, Blocks: b.collected, Assets: b.assets, Warnings: b.warnings}
	for _, ref := range b.media {
		plan.Media = append(plan.Media, ref)
	}
	return plan
}

// attachmentLinkRe 匹配非图片的本地文件链接 [text](path.ext)，排除 ![] 图片和 http(s) URL。
var attachmentLinkRe = regexp.MustCompile(`(?m)(?:^|[^!])\[([^\]]*)\]\(([^)]+)\)`)

// collectAttachments 扫描所有块文本中的本地文件链接，把非图片扩展名作为 attachment 资产。
// 同一文件只收集一次（去重）。路径逃逸在 adapter 上传时自然失败为 warning。
func (b *planBuilder) collectAttachments(doc Document) {
	seen := map[string]bool{}
	for _, block := range doc.Blocks {
		if block.Type != BlockParagraph && block.Type != BlockBlockquote {
			continue
		}
		text := block.Text
		if text == "" {
			continue
		}
		for _, m := range attachmentLinkRe.FindAllStringSubmatch(text, -1) {
			label := strings.TrimSpace(m[1])
			target := strings.TrimSpace(m[2])
			if isURLScheme(target) || isImageExt(target) {
				continue
			}
			resolved, err := resolveRelativePath(b.opts.NoteDir, target)
			if err != nil || seen[resolved] {
				continue
			}
			seen[resolved] = true
			b.assets = append(b.assets, NativeAsset{Kind: "attachment", Source: resolved, Alt: label, Origin: "link:file"})
		}
	}
}

func isURLScheme(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "mailto:") || strings.HasPrefix(path, "#")
}

func isImageExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return true
	}
	return false
}

func (b *planBuilder) addBlock(block Block) {
	var pb PlanBlock
	switch block.Type {
	case BlockHeading:
		pb = PlanBlock{Type: "heading", Level: block.Level, Text: block.Text}
	case BlockParagraph:
		pb = PlanBlock{Type: "paragraph", Text: block.Text}
	case BlockList:
		pb = PlanBlock{Type: "list", Ordered: block.Ordered, Items: listTexts(block.Items)}
	case BlockBlockquote:
		pb = PlanBlock{Type: "blockquote", Text: block.Text}
	case BlockCode:
		pb = PlanBlock{Type: "code", Language: block.Language, Code: block.Code}
	case BlockTable:
		pb = PlanBlock{Type: "table", Headers: block.Headers, Rows: block.Rows}
	case BlockThematicBreak:
		pb = PlanBlock{Type: "divider"}
	case BlockImage:
		pb = b.imageBlock(block)
	case BlockMermaid:
		pb = b.mermaidBlock(block)
	case BlockHTML:
		pb = b.htmlBlock(block)
	default:
		pb = PlanBlock{Type: "paragraph", Text: block.Text}
	}
	b.collected = append(b.collected, pb)
}

func (b *planBuilder) imageBlock(block Block) PlanBlock {
	if block.IsRemoteImage {
		// 远程图片（含 SVG URL）：作为 NativeAsset，adapter 在 push 时下载到 vault 临时缓存再渲染/上传。
		// 不在 plan 阶段告警——下载/渲染失败才告警。
		kind := "remote-image"
		if strings.EqualFold(fileExt(block.ImageSrc), ".svg") {
			kind = "remote-svg"
		}
		b.assets = append(b.assets, NativeAsset{Kind: kind, Source: block.ImageSrc, Alt: block.Alt, Origin: "image:remote"})
		return PlanBlock{Type: "paragraph", Text: "![" + block.Alt + "](" + block.ImageSrc + ")", Alt: block.Alt}
	}
	resolved, err := resolveRelativePath(b.opts.NoteDir, block.ImageSrc)
	if err != nil {
		b.warnings = append(b.warnings, PlanWarning{Code: "image_path_escape", Message: "image path escapes vault boundary", Detail: block.ImageSrc})
		return PlanBlock{Type: "paragraph", Text: "[" + block.Alt + "](" + block.ImageSrc + ")", Fallback: true}
	}
	isSVG := strings.EqualFold(fileExt(resolved), ".svg")
	if isSVG {
		// SVG 文件：源码在磁盘上，adapter 在 push 时读取并交给 whiteboard 服务端渲染。
		b.assets = append(b.assets, NativeAsset{Kind: "svg-file", Source: resolved, Alt: block.Alt, Origin: "image:svg"})
		return PlanBlock{Type: "paragraph", Text: "[SVG diagram: " + block.Alt + "]", Alt: block.Alt}
	}
	// 本地图片：作为 NativeAsset，adapter 用 docs +media-insert 上传（cwd=vault 传相对路径）。
	b.assets = append(b.assets, NativeAsset{Kind: "image", Source: resolved, Alt: block.Alt, Origin: "image:local"})
	key := mediaKey("img", resolved)
	b.registerMedia(key, resolved, mediaTypeForPath(resolved))
	return PlanBlock{Type: "image", MediaKey: key, Alt: block.Alt}
}

func (b *planBuilder) mermaidBlock(block Block) PlanBlock {
	// Feishu 的 docs +create --doc-format markdown 会把 ```mermaid 围栏块原生转换为
	// <whiteboard type="mermaid"> 并服务端渲染。因此 plan 不把 Mermaid 作为 NativeAsset——
	// 正文保留 fenced code block，由 Feishu 导入时原生渲染，adapter 不重复处理。
	return PlanBlock{Type: "code", Language: "mermaid", Code: block.Code, Alt: "Mermaid diagram"}
}

func (b *planBuilder) htmlBlock(block Block) PlanBlock {
	lower := strings.ToLower(block.RawHTML)
	if strings.Contains(lower, "<svg") {
		// inline SVG 源码作为 NativeAsset，adapter 用 whiteboard --input_format svg 服务端渲染。
		// 不把 raw SVG 插入正文（安全限制），也不在 plan 阶段告警。
		b.assets = append(b.assets, NativeAsset{Kind: "svg", Source: block.RawHTML, Alt: "SVG diagram", Origin: "inline-svg"})
		return PlanBlock{Type: "paragraph", Text: "[SVG diagram rendered below]", Alt: "SVG diagram"}
	}
	b.warnings = append(b.warnings, PlanWarning{Code: domain.PublishDocWarningUnsupportedHTML, Message: "unsupported HTML block omitted from native document", Detail: firstLine(block.RawHTML)})
	return PlanBlock{Type: "paragraph", Text: "[unsupported HTML omitted]", Fallback: true}
}

func (b *planBuilder) registerMedia(key, path, mediaType string) {
	if _, exists := b.media[key]; exists {
		return
	}
	b.media[key] = PlanMediaRef{Key: key, Path: path, MediaType: mediaType, Source: b.opts.NoteDir}
}

func listTexts(items []ListItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Text)
	}
	return out
}

func fileExt(path string) string {
	idx := strings.LastIndexByte(path, '.')
	if idx < 0 {
		return ""
	}
	return path[idx:]
}

// MarshalNativePlan 序列化 plan 为 JSON，存入 package.native_plan。
func MarshalNativePlan(plan NativePlan) ([]byte, error) {
	return json.MarshalIndent(plan, "", "  ")
}

// UnmarshalNativePlan 反序列化 plan，供 provider adapter 读取 package 时使用。
func UnmarshalNativePlan(body []byte) (NativePlan, error) {
	var plan NativePlan
	err := json.Unmarshal(body, &plan)
	return plan, err
}
