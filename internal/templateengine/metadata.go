package templateengine

import (
	"bufio"
	pathpkg "path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersionV2 = "pinax.template.v2"

type Metadata struct {
	SchemaVersion      string                              `yaml:"schema_version" json:"schema_version"`
	Name               string                              `yaml:"name" json:"name,omitempty"`
	Title              string                              `yaml:"title" json:"title,omitempty"`
	TemplateKind       string                              `yaml:"template_kind" json:"template_kind,omitempty"`
	ScenarioID         string                              `yaml:"scenario_id" json:"scenario_id,omitempty"`
	Intents            []string                            `yaml:"intents" json:"intents,omitempty"`
	UseCases           []string                            `yaml:"use_cases" json:"use_cases,omitempty"`
	Aliases            []string                            `yaml:"aliases" json:"aliases,omitempty"`
	Difficulty         string                              `yaml:"difficulty" json:"difficulty,omitempty"`
	Starter            *bool                               `yaml:"starter" json:"starter,omitempty"`
	Engine             string                              `yaml:"engine" json:"engine,omitempty"`
	Kind               string                              `yaml:"kind" json:"kind,omitempty"`
	Output             TemplateOutputMetadata              `yaml:"output" json:"output,omitempty"`
	OutputPolicy       TemplateOutputPolicy                `yaml:"output_policy" json:"output_policy,omitempty"`
	Variables          map[string]VariableMetadata         `yaml:"variables" json:"variables,omitempty"`
	VariableSchema     map[string]VariableMetadata         `yaml:"variable_schema" json:"variable_schema,omitempty"`
	Defaults           map[string]string                   `yaml:"defaults" json:"defaults,omitempty"`
	Example            Example                             `yaml:"example" json:"example,omitempty"`
	Queries            map[string]TemplateQueryDeclaration `yaml:"queries" json:"queries,omitempty"`
	AfterCreateActions []TemplateActionMetadata            `yaml:"after_create_actions" json:"after_create_actions,omitempty"`
	Maturity           string                              `yaml:"maturity" json:"maturity,omitempty"`
	ProofGate          TemplateProofGate                   `yaml:"proof_gate" json:"proof_gate,omitempty"`
	Pack               TemplatePackMetadata                `yaml:"pack" json:"pack,omitempty"`
	Lifecycle          string                              `yaml:"lifecycle" json:"lifecycle,omitempty"`
	Replacement        string                              `yaml:"replacement" json:"replacement,omitempty"`
	Metrics            map[string]string                   `yaml:"metrics" json:"metrics,omitempty"`
}

type TemplateOutputMetadata struct {
	PathPattern string `yaml:"path_pattern" json:"path_pattern,omitempty"`
}

type TemplateOutputPolicy struct {
	PathPattern      string `yaml:"path_pattern" json:"path_pattern,omitempty"`
	AllowOverride    bool   `yaml:"allow_override" json:"allow_override,omitempty"`
	WriteBoundary    string `yaml:"write_boundary" json:"write_boundary,omitempty"`
	LegacyCompatible bool   `yaml:"legacy_compatible" json:"legacy_compatible,omitempty"`
}

type VariableMetadata struct {
	Required    bool   `yaml:"required" json:"required,omitempty"`
	Description string `yaml:"description" json:"description,omitempty"`
	Default     string `yaml:"default" json:"default,omitempty"`
}

type Example struct {
	Title   string            `yaml:"title" json:"title,omitempty"`
	Project string            `yaml:"project" json:"project,omitempty"`
	Tags    []string          `yaml:"tags" json:"tags,omitempty"`
	Vars    map[string]string `yaml:"vars" json:"vars,omitempty"`
}

type TemplateQueryDeclaration struct {
	Language string `yaml:"language" json:"language,omitempty"`
	SQL      string `yaml:"sql" json:"sql,omitempty"`
	Kind     string `yaml:"kind" json:"kind,omitempty"`
	MaxRows  int    `yaml:"max_rows" json:"max_rows,omitempty"`
	Required bool   `yaml:"required" json:"required,omitempty"`
}

type TemplateActionMetadata struct {
	Name    string `yaml:"name" json:"name,omitempty"`
	Command string `yaml:"command" json:"command,omitempty"`
}

type TemplateProofGate struct {
	Status           string `yaml:"status" json:"status,omitempty"`
	ManualReview     bool   `yaml:"manual_review" json:"manual_review,omitempty"`
	SnapshotRequired bool   `yaml:"snapshot_required" json:"snapshot_required,omitempty"`
	ReceiptRequired  bool   `yaml:"receipt_required" json:"receipt_required,omitempty"`
	RestoreHint      string `yaml:"restore_hint" json:"restore_hint,omitempty"`
}

type TemplatePackMetadata struct {
	ID        string `yaml:"id" json:"id,omitempty"`
	Source    string `yaml:"source" json:"source,omitempty"`
	Version   string `yaml:"version" json:"version,omitempty"`
	Readiness string `yaml:"readiness" json:"readiness,omitempty"`
}

func ParseDocument(name, content string) (TemplateDocument, error) {
	metaRaw, body, hasFrontmatter, closed := splitYAMLFrontmatter(content)
	body, fencedQueries, err := extractFencedQueries(body)
	if err != nil {
		return TemplateDocument{}, err
	}
	if hasFrontmatter && !closed {
		return TemplateDocument{}, &Error{Code: "template_frontmatter_unclosed", Message: "frontmatter 未闭合"}
	}
	doc := TemplateDocument{Name: name, Engine: EngineSimple, Body: body}
	if len(fencedQueries) > 0 {
		doc.Metadata.Queries = fencedQueries
	}
	if !hasFrontmatter {
		return doc, nil
	}

	var meta Metadata
	if err := yaml.Unmarshal([]byte(metaRaw), &meta); err != nil {
		return TemplateDocument{}, &Error{Code: "template_schema_invalid", Message: "模板 metadata 无法解析", Err: err}
	}
	if strings.TrimSpace(meta.SchemaVersion) == "" {
		return doc, nil
	}
	if meta.SchemaVersion == "pinax.template_design.v1" {
		doc.Issues = append(doc.Issues, Issue{Code: "template_design_legacy", Message: "模板仍是设计稿 metadata"})
		return doc, nil
	}
	if meta.SchemaVersion != SchemaVersionV2 {
		return doc, nil
	}
	if err := validateMetadata(meta); err != nil {
		return TemplateDocument{}, err
	}
	if meta.Engine == "" {
		meta.Engine = EngineSimple
	}
	meta.Queries = mergeQueries(meta.Queries, fencedQueries)
	doc.Engine = meta.Engine
	doc.Metadata = meta
	return doc, nil
}

func splitYAMLFrontmatter(content string) (string, string, bool, bool) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content, false, false
	}
	scanner := bufio.NewScanner(strings.NewReader(content[4:]))
	lines := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			prefix := "---\n" + strings.Join(lines, "\n") + "\n---"
			body := strings.TrimPrefix(content, prefix)
			return strings.Join(lines, "\n"), strings.TrimPrefix(body, "\n"), true, true
		}
		lines = append(lines, line)
	}
	return "", content, true, false
}

func validateMetadata(meta Metadata) error {
	if meta.Engine != "" && meta.Engine != EngineSimple && meta.Engine != EngineGoTemplate {
		return &Error{Code: "template_schema_invalid", Message: "模板 engine 不受支持"}
	}
	if err := validateOutputPathPattern(meta.Output.PathPattern); err != nil {
		return err
	}
	for key := range meta.Variables {
		if !simpleVariablePattern.MatchString("{{" + key + "}}") {
			return &Error{Code: "template_schema_invalid", Message: "模板变量 key 非法: " + key}
		}
	}
	for key := range meta.Defaults {
		if !simpleVariablePattern.MatchString("{{" + key + "}}") {
			return &Error{Code: "template_schema_invalid", Message: "默认变量 key 非法: " + key}
		}
	}
	for name, query := range meta.Queries {
		if !simpleVariablePattern.MatchString("{{" + name + "}}") {
			return &Error{Code: "template_schema_invalid", Message: "模板查询 key 非法: " + name}
		}
		if strings.TrimSpace(query.Language) != "" && strings.TrimSpace(query.Language) != "sql" {
			return &Error{Code: "template_schema_invalid", Message: "模板查询 language 不受支持: " + name}
		}
		if strings.TrimSpace(query.SQL) == "" {
			return &Error{Code: "template_schema_invalid", Message: "模板查询 SQL 不能为空: " + name}
		}
	}
	return nil
}

// path pattern 只能描述 vault 内的内容路径；绝对路径、上跳路径和 `.pinax/`、`.git/`、`attachments/` 等保留目录都不能由模板写入。
func validateOutputPathPattern(pattern string) error {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	if pattern == "" {
		return nil
	}
	clean := pathpkg.Clean(pattern)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "~/") {
		return &Error{Code: "template_output_path_invalid", Message: "模板输出路径必须是 vault-relative content path"}
	}
	first, _, _ := strings.Cut(clean, "/")
	for _, reserved := range []string{".pinax", ".git", "attachments", "temp", "dist", "node_modules", "vendor"} {
		if first == reserved {
			return &Error{Code: "template_output_path_invalid", Message: "模板输出路径不能指向保留目录: " + reserved}
		}
	}
	return nil
}

var fencedQueryPattern = regexp.MustCompile("(?ms)^```pinax-sql[ \t]+([A-Za-z_][A-Za-z0-9_:-]*)[ \t]*\n(.*?)\n```[ \t]*(?:\n|$)")

func extractFencedQueries(body string) (string, map[string]TemplateQueryDeclaration, error) {
	queries := map[string]TemplateQueryDeclaration{}
	rewritten := fencedQueryPattern.ReplaceAllStringFunc(body, func(block string) string {
		match := fencedQueryPattern.FindStringSubmatch(block)
		if len(match) < 3 {
			return block
		}
		queries[match[1]] = TemplateQueryDeclaration{Language: "sql", SQL: strings.TrimSpace(match[2])}
		return ""
	})
	if len(queries) == 0 {
		return body, nil, nil
	}
	return strings.TrimLeft(rewritten, "\n"), queries, nil
}

func mergeQueries(frontmatter, fenced map[string]TemplateQueryDeclaration) map[string]TemplateQueryDeclaration {
	if len(frontmatter) == 0 && len(fenced) == 0 {
		return nil
	}
	merged := make(map[string]TemplateQueryDeclaration, len(frontmatter)+len(fenced))
	for name, query := range frontmatter {
		if query.Language == "" {
			query.Language = "sql"
		}
		merged[name] = query
	}
	for name, query := range fenced {
		merged[name] = query
	}
	return merged
}
