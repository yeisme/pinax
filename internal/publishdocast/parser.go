package publishdocast

import (
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// publishMarkdown 是启用 GFM 扩展（表格、删除线、任务列表）的 goldmark 实例。
// 默认 parser 不含表格扩展，发布文档需要支持 GFM 表格，故显式启用。
var publishMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// Parse 把 Markdown 正文（不含 frontmatter）解析为发布 AST。
// title 由调用方从 note 标题传入，避免重复出现在正文块里（由调用方做去重）。
// parser 不调用 provider、不读写 .pinax/**，纯内存转换。
func Parse(title, body string) Document {
	doc := Document{Title: strings.TrimSpace(title)}
	cleaned := strings.TrimSpace(body)
	if cleaned == "" {
		return doc
	}
	source := []byte(cleaned)
	// 预计算每行起始字节偏移，用于把 goldmark 的 Lines() segment 转成 1-based 行号。
	lineOffsets := computeLineOffsets(source)
	root := publishMarkdown.Parser().Parse(text.NewReader(source))
	walker := &astWalker{source: source, lineOffsets: lineOffsets}
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		walker.handleBlock(child)
	}
	doc.Blocks = walker.blocks
	return doc
}

// computeLineOffsets 返回每行首个字符的字节偏移（第 1 行 = 0）。
func computeLineOffsets(source []byte) []int {
	offsets := []int{0}
	for i, b := range source {
		if b == '\n' && i+1 < len(source) {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// byteOffsetToLine 把字节偏移转为 1-based 行号（二分查找最近的不超过偏移的行起点）。
func byteOffsetToLine(offsets []int, offset int) int {
	lo, hi := 0, len(offsets)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if offsets[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1
}

// astWalker 遍历 goldmark AST 顶层块，转换为发布块。
// 只处理块级节点；行内格式（emphasis/strong/code/link）保留在段落文本中，
// 由原生 renderer 在生成富文本块时解析。
type astWalker struct {
	source      []byte
	lineOffsets []int
	blocks      []Block
}

func (w *astWalker) lineOf(node gast.Node) int {
	lines := node.Lines()
	if lines.Len() > 0 {
		return byteOffsetToLine(w.lineOffsets, lines.At(0).Start)
	}
	return 0
}

func (w *astWalker) handleBlock(node gast.Node) {
	line := w.lineOf(node)
	switch n := node.(type) {
	case *gast.Heading:
		w.blocks = append(w.blocks, Block{Type: BlockHeading, SourceLine: line, Level: n.Level, Text: nodeText(n, w.source)})
	case *gast.Paragraph:
		w.handleParagraph(n, line)
	case *gast.Blockquote:
		w.blocks = append(w.blocks, Block{Type: BlockBlockquote, SourceLine: line, Text: blockChildText(n, w.source)})
	case *gast.FencedCodeBlock:
		// 用 Language() 取语言标识，避免 deprecated Info.Text。
		lang := strings.TrimSpace(string(n.Language(w.source)))
		code := codeBlockBody(n, w.source)
		if lang == "mermaid" {
			w.blocks = append(w.blocks, Block{Type: BlockMermaid, SourceLine: line, Language: lang, Code: code})
		} else {
			w.blocks = append(w.blocks, Block{Type: BlockCode, SourceLine: line, Language: lang, Code: code})
		}
	case *gast.CodeBlock:
		w.blocks = append(w.blocks, Block{Type: BlockCode, SourceLine: line, Code: codeBlockBody(n, w.source)})
	case *gast.List:
		items := listItems(n, w.source)
		w.blocks = append(w.blocks, Block{Type: BlockList, SourceLine: line, Ordered: n.IsOrdered(), Items: items})
	case *gast.TextBlock:
		// 紧凑列表或裸文本行产生 TextBlock（非 Paragraph）。
		w.handleTextBlock(n, line)
	case *extast.Table:
		headers, rows := tableData(n, w.source)
		w.blocks = append(w.blocks, Block{Type: BlockTable, SourceLine: line, Headers: headers, Rows: rows})
	case *gast.HTMLBlock:
		if html := strings.TrimSpace(htmlBlockBody(n, w.source)); html != "" {
			w.blocks = append(w.blocks, Block{Type: BlockHTML, SourceLine: line, RawHTML: html})
		}
	case *gast.ThematicBreak:
		w.blocks = append(w.blocks, Block{Type: BlockThematicBreak, SourceLine: line})
	}
}

// handleParagraph 处理段落。纯图片段落提升为 image 块；
// 行内 HTML（如 <svg>...</svg>）单独标记；其余作为带行内格式的段落文本。
func (w *astWalker) handleParagraph(p *gast.Paragraph, line int) {
	if html := extractInlineHTML(p, w.source); html != "" {
		w.blocks = append(w.blocks, Block{Type: BlockHTML, SourceLine: line, RawHTML: html})
		return
	}
	if img, ok := soleImage(p, w.source); ok {
		src := strings.TrimSpace(string(img.Destination))
		block := Block{Type: BlockImage, SourceLine: line, ImageSrc: src, Alt: nodeText(img, w.source)}
		if isRemoteURL(src) {
			block.IsRemoteImage = true
		}
		w.blocks = append(w.blocks, block)
		return
	}
	w.blocks = append(w.blocks, Block{Type: BlockParagraph, SourceLine: line, Text: inlineText(p, w.source)})
}

// soleImage 判断段落/文本块是否只含一张图片（可前后有空白文本）。
func soleImage(container gast.Node, source []byte) (*gast.Image, bool) {
	var img *gast.Image
	for child := container.FirstChild(); child != nil; child = child.NextSibling() {
		switch typed := child.(type) {
		case *gast.Image:
			if img != nil {
				return nil, false
			}
			img = typed
		case *gast.Text:
			if strings.TrimSpace(string(typed.Segment.Value(source))) != "" {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return img, img != nil
}

// handleTextBlock 处理 goldmark TextBlock（紧凑列表项的文本、裸文本行）。
// 逻辑同 handleParagraph：纯图片提升为 image 块，行内 HTML 单独标记，否则作为段落。
func (w *astWalker) handleTextBlock(tb *gast.TextBlock, line int) {
	if html := extractInlineHTML(tb, w.source); html != "" {
		w.blocks = append(w.blocks, Block{Type: BlockHTML, SourceLine: line, RawHTML: html})
		return
	}
	if img, ok := soleImage(tb, w.source); ok {
		src := strings.TrimSpace(string(img.Destination))
		block := Block{Type: BlockImage, SourceLine: line, ImageSrc: src, Alt: nodeText(img, w.source)}
		if isRemoteURL(src) {
			block.IsRemoteImage = true
		}
		w.blocks = append(w.blocks, block)
		return
	}
	w.blocks = append(w.blocks, Block{Type: BlockParagraph, SourceLine: line, Text: inlineText(tb, w.source)})
}

// nodeText 收集节点的纯文本子节点（含 code span），用于标题/图片 alt。
func nodeText(node gast.Node, source []byte) string {
	var b strings.Builder
	var collect func(gast.Node)
	collect = func(current gast.Node) {
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			switch typed := child.(type) {
			case *gast.Text:
				b.Write(typed.Segment.Value(source))
			case *gast.CodeSpan:
				for segment := typed.FirstChild(); segment != nil; segment = segment.NextSibling() {
					if text, ok := segment.(*gast.Text); ok {
						b.Write(text.Segment.Value(source))
					}
				}
			default:
				collect(child)
			}
		}
	}
	collect(node)
	return strings.TrimSpace(b.String())
}

// inlineText 收集行内格式文本，保留 Markdown link/emphasis/code 语法，
// 让原生 renderer 在富文本块里解析行内 marks。行内图片保留为 ![alt](src) 语法。
func inlineText(node gast.Node, source []byte) string {
	var b strings.Builder
	var collect func(gast.Node)
	collect = func(current gast.Node) {
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			switch typed := child.(type) {
			case *gast.Text:
				b.Write(typed.Segment.Value(source))
			case *gast.CodeSpan:
				b.WriteString("`")
				for segment := typed.FirstChild(); segment != nil; segment = segment.NextSibling() {
					if text, ok := segment.(*gast.Text); ok {
						b.Write(text.Segment.Value(source))
					}
				}
				b.WriteString("`")
			case *gast.Emphasis:
				mark := "*"
				if typed.Level > 1 {
					mark = "**"
				}
				b.WriteString(mark)
				collect(typed)
				b.WriteString(mark)
			case *gast.Link:
				b.WriteString("[")
				collect(typed)
				b.WriteString("](")
				b.Write(typed.Destination)
				b.WriteString(")")
			case *gast.Image:
				b.WriteString("![")
				collect(typed)
				b.WriteString("](")
				b.Write(typed.Destination)
				b.WriteString(")")
			default:
				collect(child)
			}
		}
	}
	collect(node)
	return strings.TrimSpace(b.String())
}

// extractInlineHTML 检测段落是否以 raw inline HTML 为主（如 <svg>...</svg>）。
// goldmark 用 KindRawHTML 表示行内 HTML 片段。
func extractInlineHTML(node gast.Node, source []byte) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if raw, ok := child.(*gast.RawHTML); ok {
			b.Write(raw.Segments.Value(source))
		}
	}
	return strings.TrimSpace(b.String())
}

func blockChildText(node gast.Node, source []byte) string {
	var b strings.Builder
	_ = gast.Walk(node, func(n gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering || n == node {
			return gast.WalkContinue, nil
		}
		if t, ok := n.(*gast.Text); ok {
			b.Write(t.Segment.Value(source))
			b.WriteByte('\n')
		}
		return gast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func codeBlockBody(node gast.Node, source []byte) string {
	var b strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func htmlBlockBody(node *gast.HTMLBlock, source []byte) string {
	var b strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return strings.TrimSpace(b.String())
}

func listItems(list *gast.List, source []byte) []ListItem {
	items := []ListItem{}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item.Kind() != gast.KindListItem {
			continue
		}
		var text strings.Builder
		for child := item.FirstChild(); child != nil; child = child.NextSibling() {
			// 列表项的文本块可能是 Paragraph（松散列表）或 TextBlock（紧凑列表）。
			if isTextContainer(child) {
				if text.Len() > 0 {
					text.WriteString(" ")
				}
				text.WriteString(inlineText(child, source))
			}
		}
		items = append(items, ListItem{Text: strings.TrimSpace(text.String())})
	}
	return items
}

// isTextContainer 判断节点是否为承载行内文本的块（Paragraph 或 TextBlock）。
func isTextContainer(node gast.Node) bool {
	return node.Kind() == gast.KindParagraph || node.Kind() == gast.KindTextBlock
}

// tableData 从 GFM Table 节点提取表头和行。
func tableData(table *extast.Table, source []byte) ([]string, [][]string) {
	var headers []string
	var rows [][]string
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		switch row := child.(type) {
		case *extast.TableHeader:
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				headers = append(headers, cellText(cell, source))
			}
		case *extast.TableRow:
			rowData := []string{}
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				rowData = append(rowData, cellText(cell, source))
			}
			rows = append(rows, rowData)
		}
	}
	return headers, rows
}

func cellText(node gast.Node, source []byte) string {
	return strings.TrimSpace(inlineText(node, source))
}

func isRemoteURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
