// Package publishdocast 把 Pinax note Markdown 解析为发布专用的抽象语法树，
// 供原生文档渲染器（飞书 native-docx）转换为 provider-neutral 文档块。
// 它不调用 provider、不读写 .pinax/**，只产出可测试的内存结构。
package publishdocast

// BlockType 枚举发布 AST 支持的块类型。
type BlockType string

const (
	BlockHeading       BlockType = "heading"
	BlockParagraph     BlockType = "paragraph"
	BlockList          BlockType = "list"
	BlockBlockquote    BlockType = "blockquote"
	BlockCode          BlockType = "code"
	BlockTable         BlockType = "table"
	BlockImage         BlockType = "image"
	BlockMermaid       BlockType = "mermaid"
	BlockHTML          BlockType = "html"
	BlockThematicBreak BlockType = "thematic_break"
)

// ListItem 描述列表项。支持嵌套子项（有序/无序列表混排）。
type ListItem struct {
	Text     string
	Children []ListItem
}

// Block 是发布 AST 的最小渲染单元。
// 每个块带 SourceLine（源自 Markdown 的 1-based 行号）便于错误定位。
// 仅相关字段被填充：例如 heading 填 Level；code 填 Language+Code；image 填 ImageRef。
type Block struct {
	Type       BlockType  `json:"type"`
	SourceLine int        `json:"source_line"`
	Level      int        `json:"level,omitempty"`
	Text       string     `json:"text,omitempty"`
	Language   string     `json:"language,omitempty"`
	Code       string     `json:"code,omitempty"`
	Ordered    bool       `json:"ordered,omitempty"`
	Items      []ListItem `json:"items,omitempty"`
	Headers    []string   `json:"headers,omitempty"`
	Rows       [][]string `json:"rows,omitempty"`
	Alt        string     `json:"alt,omitempty"`
	// ImageSrc 保留 Markdown 原始引用（相对路径或 URL），由 asset resolver 解析。
	ImageSrc string `json:"image_src,omitempty"`
	// IsRemoteImage 标记图片来源是远程 URL，而非 vault 内相对路径。
	IsRemoteImage bool `json:"is_remote_image,omitempty"`
	// RawHTML 保留不支持的原始 HTML 片段，供 sanitizer 判定是否省略或回退。
	RawHTML string `json:"raw_html,omitempty"`
}

// Document 是一篇 note 的发布 AST。
// Title 来自 note 标题；Blocks 是正文的有序块列表。
type Document struct {
	Title  string  `json:"title"`
	Blocks []Block `json:"blocks"`
}
