package templateengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
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

// RenderBounded is the interactive owner path: output is byte-bounded and
// date() is pinned to the requested calendar context, not the machine clock.
// Existing Render callers retain their established behavior.
func (e *Engine) RenderBounded(ctx context.Context, doc TemplateDocument, values Context, maxBytes int) (RenderResult, error) {
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}
	if maxBytes < 1 || len(doc.Body) > maxBytes {
		return RenderResult{}, &Error{Code: "template_budget_exceeded", Message: "Template exceeds the interactive budget"}
	}
	if doc.Engine != EngineGoTemplate {
		out := &boundedTemplateWriter{ctx: ctx, remaining: maxBytes}
		variables := simpleContext(values)
		position := 0
		for _, match := range simpleVariablePattern.FindAllStringSubmatchIndex(doc.Body, -1) {
			value, ok := variables[doc.Body[match[2]:match[3]]]
			if !ok {
				return RenderResult{}, &Error{Code: "template_variable_missing", Message: "Template variable is unavailable"}
			}
			if _, err := out.Write([]byte(doc.Body[position:match[0]])); err != nil {
				return RenderResult{}, err
			}
			if _, err := out.Write([]byte(value)); err != nil {
				return RenderResult{}, err
			}
			position = match[1]
		}
		if _, err := out.Write([]byte(doc.Body[position:])); err != nil {
			return RenderResult{}, err
		}
		return RenderResult{Body: out.String()}, nil
	}
	dateValue, err := time.Parse("2006-01-02", values.Date)
	if err != nil {
		return RenderResult{}, &Error{Code: "template_variable_missing", Message: "Bounded template requires a calendar date"}
	}
	functions := safeFuncMap()
	functions["date"] = func(layout string) string {
		if layout == "" {
			layout = "2006-01-02"
		}
		return dateValue.Format(layout)
	}
	tmpl, err := template.New(doc.Name).Option("missingkey=error").Funcs(functions).Parse(doc.Body)
	if err != nil {
		return RenderResult{}, &Error{Code: "template_parse_failed", Message: "Template could not be parsed"}
	}
	// Output caps alone do not bound printf allocations or empty recursive
	// templates. Interactive proposals admit only scalar fields and date().
	if !boundedTemplateSyntax(tmpl) {
		return RenderResult{}, &Error{Code: "template_unsupported", Message: "Interactive templates support scalar fields and date() only"}
	}
	out := &boundedTemplateWriter{ctx: ctx, remaining: maxBytes}
	if err := tmpl.Execute(out, values.withDefaults(doc.Metadata)); err != nil {
		return RenderResult{}, &Error{Code: "template_render_failed", Message: "Template exceeded its budget or could not be rendered", Err: err}
	}
	return RenderResult{Body: out.String()}, nil
}

func boundedTemplateSyntax(tmpl *template.Template) bool {
	if len(tmpl.Templates()) != 1 {
		return false
	}
	for _, node := range tmpl.Root.Nodes {
		switch node := node.(type) {
		case *parse.TextNode:
		case *parse.ActionNode:
			if len(node.Pipe.Decl) != 0 || len(node.Pipe.Cmds) != 1 {
				return false
			}
			args := node.Pipe.Cmds[0].Args
			if len(args) == 1 {
				field, ok := args[0].(*parse.FieldNode)
				if !ok {
					return false
				}
				if len(field.Ident) == 1 {
					switch field.Ident[0] {
					case "Title", "Date", "DateTime", "Project":
					default:
						return false
					}
				} else if len(field.Ident) != 2 || field.Ident[0] != "Vars" && field.Ident[0] != "Defaults" {
					return false
				}
			} else if len(args) == 2 {
				fn, ok := args[0].(*parse.IdentifierNode)
				layout, literal := args[1].(*parse.StringNode)
				if !ok || fn.Ident != "date" || !literal || len(layout.Text) > 64 {
					return false
				}
			} else {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type boundedTemplateWriter struct {
	bytes.Buffer
	ctx       context.Context
	remaining int
}

func (w *boundedTemplateWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > w.remaining {
		return 0, &Error{Code: "template_budget_exceeded", Message: "Template result exceeds the interactive budget"}
	}
	w.remaining -= len(p)
	return w.Buffer.Write(p)
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
