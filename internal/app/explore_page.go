package app

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed explore_page.html
var explorePageTemplate string

const exploreBundlePlaceholder = "__PINAX_EXPLORE_BUNDLE_JSON__"

// RenderExplorePage 渲染自包含 explore 单页 HTML。
// bundle 数据默认内嵌（单文件打开即用）；embed=false 时（--no-embed，大 vault 走
// /explore/data.json 模式）页面引导客户端按需拉取。
func RenderExplorePage(bundle ExploreBundle, embedData bool) ([]byte, error) {
	var payload string
	if embedData {
		encoded, err := json.Marshal(bundle)
		if err != nil {
			return nil, err
		}
		// bundle JSON 再编码为 JS 字符串字面量（默认 HTML 转义 <>&），任何
		// 标题/摘要内容都无法打断 </script> 或注入属性。
		quoted, err := json.Marshal(string(encoded))
		if err != nil {
			return nil, err
		}
		payload = string(quoted)
	} else {
		payload = "null"
	}
	if strings.Contains(payload, exploreBundlePlaceholder) {
		return nil, fmt.Errorf("explore bundle contains template placeholder")
	}
	page := strings.Replace(explorePageTemplate, exploreBundlePlaceholder, payload, 1)
	if strings.Contains(page, exploreBundlePlaceholder) {
		return nil, fmt.Errorf("explore page template missing bundle placeholder")
	}
	if !embedData {
		page = strings.Replace(page, `<body data-embed="on">`, `<body data-embed="off">`, 1)
	}
	return []byte(page), nil
}

var (
	// exploreScriptBlockPattern / exploreStyleBlockPattern 剥离内联 <script>/<style>
	// 的正文但保留开标签属性：内嵌数据经双重 JSON 编码（<>& 全转义）无法注入标记，
	// 纯文本形如 href='https://…' 的标题内容只存在于脚本字符串中，不是属性；
	// 而 <script src="//cdn…"> 的开标签属性仍必须被外链扫描捕获。
	exploreScriptBlockPattern = regexp.MustCompile(`(?is)<script\b([^>]*)>.*?</script\s*>`)
	exploreStyleBlockPattern  = regexp.MustCompile(`(?is)<style\b([^>]*)>.*?</style\s*>`)
	// exploreExternalAttrPattern 匹配标记里残留的 src=/href= 属性（自包含页面
	// 合同：零命中外链）。
	exploreExternalAttrPattern = regexp.MustCompile(`(?is)\b(src|href)\s*=\s*["']([^"']*)["']`)
	exploreExternalURLPattern  = regexp.MustCompile(`(?i)^(https?:)?//|://`)
)

// ScanExploreExternalRefs 扫描页面 HTML 标记里的 src=/href= 属性，返回所有指向
// 外部（非相对路径）的引用。自包含合同要求零命中；share --once 冒烟与页面测试共用。
func ScanExploreExternalRefs(pageHTML []byte) []string {
	markup := exploreScriptBlockPattern.ReplaceAllString(string(pageHTML), "<script$1></script>")
	markup = exploreStyleBlockPattern.ReplaceAllString(markup, "<style$1></style>")
	external := make([]string, 0)
	for _, match := range exploreExternalAttrPattern.FindAllStringSubmatch(markup, -1) {
		value := strings.TrimSpace(match[2])
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		if exploreExternalURLPattern.MatchString(value) {
			external = append(external, match[1]+"="+value)
		}
	}
	return external
}
