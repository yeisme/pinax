package promptrepo

import (
	"bytes"
	"errors"
	stdhtml "html"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	parse "github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	parsehtml "github.com/tdewolff/parse/v2/html"
)

var uiTemplateSlotNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

var uiTemplateHTMLTagNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

var uiTemplateARIAAttributeNamePattern = regexp.MustCompile(`^aria-[a-z][a-z0-9-]*$`)

var forbiddenUITemplateHTMLTags = map[string]struct{}{
	"base": {}, "body": {}, "button": {}, "embed": {}, "form": {}, "head": {}, "html": {},
	"iframe": {}, "input": {}, "link": {}, "math": {}, "meta": {}, "object": {}, "script": {},
	"select": {}, "style": {}, "svg": {}, "textarea": {},
}

var forbiddenUITemplateURLAttributes = map[string]struct{}{
	"action": {}, "archive": {}, "attributionsrc": {}, "background": {}, "cite": {},
	"codebase": {}, "data": {}, "dynsrc": {}, "formaction": {}, "href": {}, "icon": {},
	"imagesrcset": {}, "itemid": {}, "itemtype": {}, "longdesc": {}, "lowsrc": {},
	"manifest": {}, "ping": {}, "poster": {}, "profile": {}, "src": {}, "srcset": {},
	"usemap": {}, "xmlns": {},
}

var allowedUITemplateHTMLTags = map[string]struct{}{
	"abbr": {}, "address": {}, "article": {}, "aside": {}, "b": {}, "bdi": {}, "bdo": {},
	"blockquote": {}, "br": {}, "caption": {}, "code": {}, "col": {}, "colgroup": {},
	"dd": {}, "del": {}, "details": {}, "dfn": {}, "div": {}, "dl": {}, "dt": {},
	"em": {}, "figcaption": {}, "figure": {}, "footer": {}, "h1": {}, "h2": {}, "h3": {},
	"h4": {}, "h5": {}, "h6": {}, "header": {}, "hr": {}, "i": {}, "ins": {}, "kbd": {},
	"li": {}, "main": {}, "mark": {}, "menu": {}, "meter": {}, "nav": {}, "ol": {}, "p": {},
	"pre": {}, "progress": {}, "q": {}, "rp": {}, "rt": {}, "ruby": {}, "s": {}, "samp": {},
	"search": {}, "section": {}, "small": {}, "span": {}, "strong": {}, "sub": {},
	"summary": {}, "sup": {}, "table": {}, "tbody": {}, "td": {}, "tfoot": {}, "th": {},
	"thead": {}, "time": {}, "tr": {}, "u": {}, "ul": {}, "var": {}, "wbr": {},
}

var allowedUITemplateHTMLAttributes = map[string]struct{}{
	"abbr": {}, "alt": {}, "class": {}, "colspan": {}, "datetime": {}, "dir": {},
	"draggable": {}, "headers": {}, "height": {}, "hidden": {}, "high": {}, "id": {},
	"inert": {}, "lang": {}, "low": {}, "max": {}, "min": {}, "open": {}, "optimum": {},
	"reversed": {}, "role": {}, "rowspan": {}, "scope": {}, "span": {}, "spellcheck": {},
	"start": {}, "tabindex": {}, "title": {}, "translate": {}, "type": {}, "value": {},
	"width": {},
}

var uiTemplateHTMLVoidTags = map[string]struct{}{
	"br": {}, "col": {}, "hr": {}, "wbr": {},
}

// ValidateUITemplateBundle validates the exact declarative bundle. It never
// rewrites or executes body bytes.
func ValidateUITemplateBundle(bundle UITemplateBundleV1) error {
	if err := validateUITemplateBundleStructure(bundle); err != nil {
		return err
	}
	if !sha256DigestPattern.MatchString(bundle.ContentDigest) || !sha256DigestPattern.MatchString(bundle.Address.Digest) {
		return NewError(CodeUITemplateDigestMismatch, "UI template content digest binding is invalid", false, nil)
	}
	actual, err := CanonicalUITemplateDigest(bundle)
	if err != nil {
		return err
	}
	if bundle.ContentDigest != actual || bundle.Address.Digest != actual {
		return NewError(CodeUITemplateDigestMismatch, "UI template content digest does not match the canonical bundle", false, nil)
	}
	if bundle.Snapshot == "" || bundle.Address.Snapshot == "" || bundle.Snapshot != bundle.Address.Snapshot {
		return NewError(CodeUITemplateSnapshotMismatch, "UI template snapshot binding does not match the exact address", false, nil)
	}
	return nil
}

func validateUITemplateBundleStructure(bundle UITemplateBundleV1) error {
	if bundle.SchemaVersion != UITemplateSchemaVersionV1 {
		return NewError(CodeUITemplateAddressInvalid, "UI template schema version is unsupported", false, nil)
	}
	if err := ValidateUITemplateAddress(bundle.Address); err != nil {
		return err
	}
	if bundle.Address.Role == "" || bundle.Address.Path == "" {
		return NewError(CodeUITemplateAddressInvalid, "UI template bundle requires role and relative path", false, nil)
	}
	if bundle.Snapshot == "" || !sha256DigestPattern.MatchString(bundle.Snapshot) {
		return NewError(CodeUITemplateSnapshotMismatch, "UI template bundle snapshot is invalid", false, nil)
	}
	if bundle.Address.Snapshot != "" && bundle.Address.Snapshot != bundle.Snapshot {
		return NewError(CodeUITemplateSnapshotMismatch, "UI template bundle snapshot does not match the address", false, nil)
	}
	if bundle.Security != UITemplateSecurityStaticReviewFragmentV1 {
		return NewError(CodeUITemplateHTMLForbidden, "UI template security profile is unsupported", false, nil)
	}
	if err := validateUITemplateLimits(bundle.Limits, len(bundle.HTMLFragment), len(bundle.CSS), len(bundle.Slots)); err != nil {
		return err
	}
	if !utf8.Valid(bundle.HTMLFragment) || !utf8.Valid(bundle.CSS) {
		return NewError(CodeUITemplateLimitExceeded, "UI template bodies must be valid UTF-8", false, nil)
	}
	if err := validateUITemplateSlots(bundle.Slots, bundle.Limits); err != nil {
		return err
	}
	if err := validateUITemplateHTML(bundle.HTMLFragment, bundle.Slots, bundle.Limits); err != nil {
		return err
	}
	if err := validateUITemplateCSS(bundle.CSS); err != nil {
		return err
	}
	return nil
}

func validateUITemplateLimits(limits UITemplateLimitsV1, htmlBytes, cssBytes, slotCount int) error {
	if limits.MaxHTMLBytes <= 0 || limits.MaxHTMLBytes > MaxUITemplateHTMLBytesV1 ||
		limits.MaxCSSBytes <= 0 || limits.MaxCSSBytes > MaxUITemplateCSSBytesV1 ||
		limits.MaxBodyBytes <= 0 || limits.MaxBodyBytes > MaxUITemplateBodyBytesV1 ||
		limits.MaxSlots <= 0 || limits.MaxSlots > MaxUITemplateSlotsV1 ||
		limits.MaxSlotNameBytes <= 0 || limits.MaxSlotNameBytes > MaxUITemplateSlotNameBytesV1 {
		return NewError(CodeUITemplateLimitExceeded, "UI template limits must be positive and within the V1 ceilings", false, nil)
	}
	if htmlBytes > limits.MaxHTMLBytes || cssBytes > limits.MaxCSSBytes || htmlBytes+cssBytes > limits.MaxBodyBytes || slotCount > limits.MaxSlots {
		return NewError(CodeUITemplateLimitExceeded, "UI template body or slot count exceeds the declared limits", false, nil)
	}
	return nil
}

func validateUITemplateSlots(slots []UITemplateSlotV1, limits UITemplateLimitsV1) error {
	seen := make(map[string]struct{}, len(slots))
	for _, slot := range slots {
		if !validUITemplateSlotName(slot.Name, limits.MaxSlotNameBytes) {
			return NewError(CodeUITemplateSlotInvalid, "UI template slot name is invalid", false, nil)
		}
		if _, duplicate := seen[slot.Name]; duplicate {
			return NewError(CodeUITemplateSlotInvalid, "UI template slot declarations must be unique", false, nil)
		}
		seen[slot.Name] = struct{}{}
		switch slot.Kind {
		case UITemplateSlotKindText, UITemplateSlotKindComponent, UITemplateSlotKindCollection:
		default:
			return NewError(CodeUITemplateSlotInvalid, "UI template slot kind is unsupported", false, nil)
		}
		switch slot.Cardinality {
		case UITemplateSlotCardinalityOne, UITemplateSlotCardinalityMany:
		default:
			return NewError(CodeUITemplateSlotInvalid, "UI template slot cardinality is unsupported", false, nil)
		}
	}
	return nil
}

func validUITemplateSlotName(name string, maxBytes int) bool {
	return name != "" && len(name) <= maxBytes && utf8.ValidString(name) && uiTemplateSlotNamePattern.MatchString(name)
}

func validateUITemplateHTML(fragment []byte, slots []UITemplateSlotV1, limits UITemplateLimitsV1) error {
	if bytes.IndexByte(fragment, 0) >= 0 {
		return NewError(CodeUITemplateHTMLForbidden, "UI template HTML contains a forbidden control character", false, nil)
	}
	for _, delimiter := range [][]byte{[]byte("<%"), []byte("%>"), []byte("${")} {
		if bytes.Contains(fragment, delimiter) {
			return NewError(CodeUITemplateHTMLForbidden, "UI template HTML contains executable template syntax", false, nil)
		}
	}
	declared := make(map[string]UITemplateSlotV1, len(slots))
	for _, slot := range slots {
		declared[slot.Name] = slot
	}
	seenSlots := make(map[string]struct{}, len(slots))
	lexer := parsehtml.NewTemplateLexer(parse.NewInput(bytes.NewReader(fragment)), parsehtml.GoTemplate)
	openTags := make([]string, 0, 16)
	pendingTag := ""
	pendingAttributes := map[string]struct{}(nil)
	for {
		tokenType, _ := lexer.Next()
		switch tokenType {
		case parsehtml.ErrorToken:
			if !errors.Is(lexer.Err(), io.EOF) {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML parser rejected the fragment", false, lexer.Err())
			}
			if pendingTag != "" || len(openTags) != 0 {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML elements must be explicitly balanced", false, nil)
			}
			for _, slot := range slots {
				if slot.Required {
					if _, present := seenSlots[slot.Name]; !present {
						return NewError(CodeUITemplateSlotInvalid, "UI template HTML is missing a required slot", false, nil)
					}
				}
			}
			return nil
		case parsehtml.DoctypeToken:
			return NewError(CodeUITemplateHTMLForbidden, "UI template HTML must be a fragment without document nodes", false, nil)
		case parsehtml.SVGToken, parsehtml.MathToken, parsehtml.XMLToken:
			return NewError(CodeUITemplateHTMLForbidden, "UI template HTML namespaces are forbidden", false, nil)
		case parsehtml.TemplateToken:
			return NewError(CodeUITemplateHTMLForbidden, "UI template HTML contains executable template syntax", false, nil)
		case parsehtml.StartTagToken:
			if pendingTag != "" {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML start tag is malformed", false, nil)
			}
			tag := strings.ToLower(string(lexer.Text()))
			if !validUITemplateHTMLTag(tag) {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML contains a forbidden element", false, nil)
			}
			pendingTag = tag
			pendingAttributes = make(map[string]struct{})
		case parsehtml.AttributeToken:
			if pendingTag == "" || lexer.HasTemplate() {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML attribute is malformed", false, nil)
			}
			name := strings.ToLower(string(lexer.AttrKey()))
			if !validUITemplateHTMLAttributeName(name) {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML contains a forbidden attribute", false, nil)
			}
			if _, duplicate := pendingAttributes[name]; duplicate {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML attributes must be unique", false, nil)
			}
			pendingAttributes[name] = struct{}{}
			value, ok := parseUITemplateHTMLAttributeValue(lexer.AttrVal())
			if !ok {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML attribute value is malformed", false, nil)
			}
			if name != "data-promptrepo-slot" {
				continue
			}
			if !validUITemplateSlotName(value, limits.MaxSlotNameBytes) {
				return NewError(CodeUITemplateSlotInvalid, "UI template HTML slot marker is invalid", false, nil)
			}
			if _, exists := declared[value]; !exists {
				return NewError(CodeUITemplateSlotInvalid, "UI template HTML references an undeclared slot", false, nil)
			}
			if _, duplicate := seenSlots[value]; duplicate {
				return NewError(CodeUITemplateSlotInvalid, "UI template HTML slot markers must be unique", false, nil)
			}
			seenSlots[value] = struct{}{}
		case parsehtml.StartTagCloseToken:
			if pendingTag == "" {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML start tag is malformed", false, nil)
			}
			if _, void := uiTemplateHTMLVoidTags[pendingTag]; !void {
				openTags = append(openTags, pendingTag)
			}
			pendingTag = ""
			pendingAttributes = nil
		case parsehtml.StartTagVoidToken:
			if pendingTag == "" {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML start tag is malformed", false, nil)
			}
			if _, void := uiTemplateHTMLVoidTags[pendingTag]; !void {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML non-void elements require explicit closing tags", false, nil)
			}
			pendingTag = ""
			pendingAttributes = nil
		case parsehtml.EndTagToken:
			if pendingTag != "" {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML start tag is malformed", false, nil)
			}
			tag := strings.ToLower(string(lexer.Text()))
			if !validUITemplateHTMLTag(tag) {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML contains a forbidden element", false, nil)
			}
			if _, void := uiTemplateHTMLVoidTags[tag]; void || len(openTags) == 0 || openTags[len(openTags)-1] != tag {
				return NewError(CodeUITemplateHTMLForbidden, "UI template HTML elements must be explicitly balanced", false, nil)
			}
			openTags = openTags[:len(openTags)-1]
		}
	}
}

func validUITemplateHTMLTag(tag string) bool {
	if !uiTemplateHTMLTagNamePattern.MatchString(tag) {
		return false
	}
	if _, forbidden := forbiddenUITemplateHTMLTags[tag]; forbidden {
		return false
	}
	_, allowed := allowedUITemplateHTMLTags[tag]
	return allowed
}

func validUITemplateHTMLAttributeName(name string) bool {
	if name == "" || strings.Contains(name, ":") || strings.HasPrefix(name, "on") ||
		name == "srcdoc" || name == "contenteditable" || name == "style" || name == "slot" || name == "is" {
		return false
	}
	if _, forbidden := forbiddenUITemplateURLAttributes[name]; forbidden {
		return false
	}
	if name == "data-promptrepo-slot" || uiTemplateARIAAttributeNamePattern.MatchString(name) {
		return true
	}
	_, allowed := allowedUITemplateHTMLAttributes[name]
	return allowed
}

func parseUITemplateHTMLAttributeValue(raw []byte) (string, bool) {
	if len(raw) == 0 {
		return "", true
	}
	value := string(raw)
	if value[0] == '\'' || value[0] == '"' {
		if len(value) < 2 || value[len(value)-1] != value[0] {
			return "", false
		}
		value = value[1 : len(value)-1]
	} else if strings.ContainsAny(value, "\"'`=<>") {
		return "", false
	}
	return stdhtml.UnescapeString(value), true
}

func validateUITemplateCSS(stylesheet []byte) error {
	if bytes.IndexByte(stylesheet, 0) >= 0 {
		return NewError(CodeUITemplateCSSForbidden, "UI template CSS contains a forbidden control character", false, nil)
	}
	normalized, err := normalizeCSSForSafetyScan(string(stylesheet))
	if err != nil {
		return NewError(CodeUITemplateCSSForbidden, "UI template CSS contains an invalid comment or escape", false, err)
	}
	for _, sequence := range []string{"</style", "<style", "</script", "<script", "<!--", "-->", "<![cdata[", "]]>"} {
		if strings.Contains(normalized, sequence) {
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS contains an HTML-breaking sequence", false, nil)
		}
	}
	for _, sequence := range []string{"@import", "url(", "expression(", "@font-face"} {
		if strings.Contains(normalized, sequence) {
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS contains external or executable syntax", false, nil)
		}
	}

	lexer := css.NewLexer(parse.NewInput(bytes.NewReader(stylesheet)))
	delimiters := make([]css.TokenType, 0, 8)
	for {
		tokenType, data := lexer.Next()
		if tokenType == css.ErrorToken {
			if errors.Is(lexer.Err(), io.EOF) {
				break
			}
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS tokenizer rejected the stylesheet", false, lexer.Err())
		}
		if err := rejectUITemplateCSSToken(tokenType, data); err != nil {
			return err
		}
		if !updateUITemplateCSSDelimiters(&delimiters, tokenType) {
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS delimiters are unbalanced", false, nil)
		}
	}
	if len(delimiters) != 0 {
		return NewError(CodeUITemplateCSSForbidden, "UI template CSS delimiters are unbalanced", false, nil)
	}

	parser := css.NewParser(parse.NewInput(bytes.NewReader(stylesheet)), false)
	for {
		grammar, tokenType, data := parser.Next()
		if grammar == css.ErrorGrammar {
			if errors.Is(parser.Err(), io.EOF) {
				break
			}
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS parser rejected the stylesheet", false, parser.Err())
		}
		switch grammar {
		case css.AtRuleGrammar, css.BeginAtRuleGrammar:
			name, ok := normalizedCSSName(data, '@', 0)
			if !ok || !allowedUITemplateAtRule(name) {
				return NewError(CodeUITemplateCSSForbidden, "UI template CSS at-rule is forbidden", false, nil)
			}
		case css.DeclarationGrammar:
			name, ok := normalizedCSSName(data, 0, 0)
			if !ok || name == "behavior" || name == "-moz-binding" || name == "src" {
				return NewError(CodeUITemplateCSSForbidden, "UI template CSS declaration is forbidden", false, nil)
			}
		}
		if err := rejectUITemplateCSSToken(tokenType, data); err != nil {
			return err
		}
		for _, value := range parser.Values() {
			if err := rejectUITemplateCSSToken(value.TokenType, value.Data); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateUITemplateCSSDelimiters(stack *[]css.TokenType, tokenType css.TokenType) bool {
	var expected css.TokenType
	switch tokenType {
	case css.FunctionToken, css.LeftParenthesisToken:
		*stack = append(*stack, css.RightParenthesisToken)
		return true
	case css.LeftBracketToken:
		*stack = append(*stack, css.RightBracketToken)
		return true
	case css.LeftBraceToken:
		*stack = append(*stack, css.RightBraceToken)
		return true
	case css.RightParenthesisToken, css.RightBracketToken, css.RightBraceToken:
		expected = tokenType
	default:
		return true
	}
	if len(*stack) == 0 || (*stack)[len(*stack)-1] != expected {
		return false
	}
	*stack = (*stack)[:len(*stack)-1]
	return true
}

func rejectUITemplateCSSToken(tokenType css.TokenType, data []byte) error {
	switch tokenType {
	case css.URLToken, css.BadURLToken:
		return NewError(CodeUITemplateCSSForbidden, "UI template CSS URL syntax is forbidden", false, nil)
	case css.BadStringToken, css.CDOToken, css.CDCToken:
		return NewError(CodeUITemplateCSSForbidden, "UI template CSS contains invalid or HTML-breaking syntax", false, nil)
	case css.FunctionToken:
		name, ok := normalizedCSSName(data, 0, '(')
		if !ok {
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS function name is invalid", false, nil)
		}
		switch name {
		case "url", "expression", "image", "image-set", "-webkit-image-set", "cross-fade", "element", "paint", "progid":
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS function is forbidden", false, nil)
		}
	case css.AtKeywordToken:
		name, ok := normalizedCSSName(data, '@', 0)
		if !ok || !allowedUITemplateAtRule(name) {
			return NewError(CodeUITemplateCSSForbidden, "UI template CSS at-rule is forbidden", false, nil)
		}
	}
	return nil
}

func allowedUITemplateAtRule(name string) bool {
	switch name {
	case "media", "supports", "layer", "keyframes", "-webkit-keyframes":
		return true
	default:
		return false
	}
}

func normalizedCSSName(data []byte, prefix, suffix byte) (string, bool) {
	value := strings.TrimSpace(string(data))
	if prefix != 0 {
		if value == "" || value[0] != prefix {
			return "", false
		}
		value = value[1:]
	}
	if suffix != 0 {
		if value == "" || value[len(value)-1] != suffix {
			return "", false
		}
		value = value[:len(value)-1]
	}
	decoded, err := decodeCSSEscapes(value)
	if err != nil || decoded == "" {
		return "", false
	}
	return strings.ToLower(decoded), true
}

func normalizeCSSForSafetyScan(value string) (string, error) {
	withoutComments, err := stripCSSComments(value)
	if err != nil {
		return "", err
	}
	decoded, err := decodeCSSEscapes(withoutComments)
	if err != nil {
		return "", err
	}
	return strings.ToLower(decoded), nil
}

func stripCSSComments(value string) (string, error) {
	var result strings.Builder
	result.Grow(len(value))
	for index := 0; index < len(value); {
		if index+1 < len(value) && value[index] == '/' && value[index+1] == '*' {
			end := strings.Index(value[index+2:], "*/")
			if end < 0 {
				return "", errors.New("unterminated CSS comment")
			}
			index += end + 4
			continue
		}
		result.WriteByte(value[index])
		index++
	}
	return result.String(), nil
}

func decodeCSSEscapes(value string) (string, error) {
	var result strings.Builder
	result.Grow(len(value))
	for index := 0; index < len(value); {
		if value[index] != '\\' {
			result.WriteByte(value[index])
			index++
			continue
		}
		index++
		if index >= len(value) || value[index] == '\n' || value[index] == '\r' || value[index] == '\f' {
			return "", errors.New("invalid CSS escape")
		}
		start := index
		for index < len(value) && index-start < 6 && isCSSHex(value[index]) {
			index++
		}
		if start != index {
			parsed, err := strconv.ParseUint(value[start:index], 16, 32)
			if err != nil || parsed == 0 || parsed > utf8.MaxRune || parsed >= 0xD800 && parsed <= 0xDFFF {
				return "", errors.New("invalid CSS code point escape")
			}
			result.WriteRune(rune(parsed))
			if index < len(value) && isCSSWhitespace(value[index]) {
				if value[index] == '\r' && index+1 < len(value) && value[index+1] == '\n' {
					index++
				}
				index++
			}
			continue
		}
		result.WriteByte(value[index])
		index++
	}
	return result.String(), nil
}

func isCSSHex(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func isCSSWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f'
}
