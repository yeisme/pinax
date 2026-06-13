package templateengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	EngineGoTemplate = "go-template"
	EngineSimple     = "simple"
)

type Engine struct{}

type TemplateDocument struct {
	Name     string
	Engine   string
	Body     string
	Metadata Metadata
	Issues   []Issue
}

type Context struct {
	Title    string
	Date     string
	DateTime string
	Project  string
	Tags     []string
	Vars     map[string]string
	Defaults map[string]string
	Queries  map[string]QueryResult
}

type QueryResult struct {
	Columns []string
	Rows    []map[string]string
}

type RenderResult struct {
	Body   string
	Issues []Issue
}

type Issue struct {
	Code    string
	Message string
}

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func New() *Engine {
	return &Engine{}
}

func ErrorCode(err error) string {
	var templateErr *Error
	if errors.As(err, &templateErr) {
		return templateErr.Code
	}
	return ""
}

func (e *Engine) Render(doc TemplateDocument, ctx Context) (RenderResult, error) {
	engine := strings.TrimSpace(doc.Engine)
	if engine == "" {
		engine = EngineSimple
	}
	if engine != EngineGoTemplate {
		return renderSimple(doc.Body, ctx)
	}

	renderCtx := ctx.withDefaults(doc.Metadata)
	tmpl, err := template.New(doc.Name).Option("missingkey=error").Funcs(safeFuncMap()).Parse(doc.Body)
	if err != nil {
		return RenderResult{}, &Error{Code: "template_parse_failed", Message: "模板解析失败", Err: err}
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, renderCtx); err != nil {
		return RenderResult{}, renderError(err)
	}
	return RenderResult{Body: out.String()}, nil
}

var simpleVariablePattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_:-]*)\s*\}\}`)

func renderSimple(body string, ctx Context) (RenderResult, error) {
	values := simpleContext(ctx)
	missing := make([]string, 0)
	seen := map[string]bool{}
	for _, match := range simpleVariablePattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		key := match[1]
		if _, ok := values[key]; !ok && !seen[key] {
			missing = append(missing, key)
			seen[key] = true
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return RenderResult{}, &Error{Code: "template_variable_missing", Message: "缺少模板变量: " + strings.Join(missing, ",")}
	}
	rendered := simpleVariablePattern.ReplaceAllStringFunc(body, func(token string) string {
		match := simpleVariablePattern.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		return values[match[1]]
	})
	return RenderResult{Body: rendered}, nil
}

func simpleContext(ctx Context) map[string]string {
	values := map[string]string{
		"title":    ctx.Title,
		"date":     ctx.Date,
		"datetime": ctx.DateTime,
		"project":  ctx.Project,
		"tags":     strings.Join(ctx.Tags, ", "),
	}
	for key, value := range ctx.Defaults {
		values[key] = value
	}
	for key, value := range ctx.Vars {
		values[key] = value
	}
	return values
}

func (ctx Context) withDefaults(meta Metadata) Context {
	metadataDefaults := defaultsFromMetadata(meta)
	merged := make(map[string]string, len(metadataDefaults)+len(ctx.Defaults)+len(ctx.Vars))
	for key, value := range metadataDefaults {
		merged[key] = value
	}
	for key, value := range ctx.Defaults {
		merged[key] = value
	}
	for key, value := range ctx.Vars {
		merged[key] = value
	}
	ctx.Vars = merged
	return ctx
}

func defaultsFromMetadata(meta Metadata) map[string]string {
	defaults := make(map[string]string, len(meta.Defaults)+len(meta.Variables))
	for key, value := range meta.Defaults {
		defaults[key] = value
	}
	for key, variable := range meta.Variables {
		if variable.Default != "" {
			defaults[key] = variable.Default
		}
	}
	return defaults
}

func renderError(err error) error {
	if strings.Contains(err.Error(), "map has no entry for key") || strings.Contains(err.Error(), "can't evaluate field") {
		return &Error{Code: "template_variable_missing", Message: "缺少模板变量", Err: err}
	}
	return &Error{Code: "template_render_failed", Message: "模板渲染失败", Err: err}
}

func safeFuncMap() template.FuncMap {
	return template.FuncMap{
		"upper":   strings.ToUpper,
		"lower":   strings.ToLower,
		"title":   title,
		"join":    join,
		"default": defaultValue,
		"slug":    slug,
		"date":    date,
		"yaml":    yamlString,
		"json":    jsonString,
		"quote":   strconv.Quote,
		"table":   renderQueryTable,
		"list":    renderQueryList,
	}
}

func title(value string) string {
	words := strings.Fields(strings.ToLower(value))
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func join(sep string, values []string) string {
	return strings.Join(values, sep)
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func date(layout string) string {
	if strings.TrimSpace(layout) == "" {
		layout = "2006-01-02"
	}
	return time.Now().UTC().Format(layout)
}

func yamlString(value any) string {
	out, err := yaml.Marshal(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func jsonString(value any) string {
	out, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(out)
}

func defaultValue(fallback, value any) any {
	if value == nil {
		return fallback
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
		return fallback
	}
	return value
}

func renderQueryTable(result QueryResult) string {
	if len(result.Columns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("| ")
	b.WriteString(strings.Join(result.Columns, " | "))
	b.WriteString(" |\n| ")
	for i := range result.Columns {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
	for _, row := range result.Rows {
		b.WriteString("| ")
		for i, column := range result.Columns {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(escapeMarkdownTableCell(row[column]))
		}
		b.WriteString(" |\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderQueryList(result QueryResult, column string) string {
	column = strings.TrimSpace(column)
	if column == "" && len(result.Columns) > 0 {
		column = result.Columns[0]
	}
	if column == "" {
		return ""
	}
	items := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		value := strings.TrimSpace(row[column])
		if value == "" {
			continue
		}
		items = append(items, fmt.Sprintf("- %s", value))
	}
	return strings.Join(items, "\n")
}

func escapeMarkdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
