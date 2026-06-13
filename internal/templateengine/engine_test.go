package templateengine

import (
	"strings"
	"testing"
)

func TestRenderGoTemplateFeatures(t *testing.T) {
	t.Parallel()

	engine := New()
	result, err := engine.Render(TemplateDocument{
		Name:   "briefing",
		Engine: EngineGoTemplate,
		Body: strings.Join([]string{
			"# {{ .Title | upper }}",
			"{{ if .Vars.url }}link: {{ .Vars.url }}{{ end }}",
			"{{ range .Tags }}- {{ . }}{{ end }}",
		}, "\n"),
	}, Context{
		Title: "weekly",
		Tags:  []string{"pinax", "sync"},
		Vars:  map[string]string{"url": "https://example.test"},
	})
	if err != nil {
		t.Fatalf("render go template: %v", err)
	}

	body := result.Body
	for _, want := range []string{"# WEEKLY", "link: https://example.test", "- pinax", "- sync"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, body)
		}
	}
}

func TestRenderMissingKeyReturnsStableIssue(t *testing.T) {
	t.Parallel()

	_, err := New().Render(TemplateDocument{
		Name:   "missing",
		Engine: EngineGoTemplate,
		Body:   "{{ .Vars.client }}",
	}, Context{Vars: map[string]string{}})
	if err == nil {
		t.Fatal("expected missing key error")
	}
	if code := ErrorCode(err); code != "template_variable_missing" {
		t.Fatalf("error code = %q, want template_variable_missing; err=%v", code, err)
	}
}

func TestUnsupportedFuncIsRejected(t *testing.T) {
	t.Parallel()

	_, err := New().Render(TemplateDocument{
		Name:   "unsafe",
		Engine: EngineGoTemplate,
		Body:   `{{ env "HOME" }}`,
	}, Context{})
	if err == nil {
		t.Fatal("expected unsupported function error")
	}
	if code := ErrorCode(err); code != "template_parse_failed" {
		t.Fatalf("error code = %q, want template_parse_failed; err=%v", code, err)
	}
}

func TestSafePureFunctionsRender(t *testing.T) {
	t.Parallel()

	result, err := New().Render(TemplateDocument{
		Name:   "helpers",
		Engine: EngineGoTemplate,
		Body: strings.Join([]string{
			`slug={{ slug .Title }}`,
			`date={{ date "2006-01-02" }}`,
			`yaml={{ yaml .Vars }}`,
			`json={{ json .Vars }}`,
			`quote={{ quote .Title }}`,
		}, "\n"),
	}, Context{Title: "Go 模板学习", Vars: map[string]string{"client": "Acme"}})
	if err != nil {
		t.Fatalf("render helper functions: %v", err)
	}
	for _, want := range []string{`slug=go-模板学习`, `client: Acme`, `"client":"Acme"`, `quote="Go 模板学习"`} {
		if !strings.Contains(result.Body, want) {
			t.Fatalf("helper output missing %q:\n%s", want, result.Body)
		}
	}
	if !strings.Contains(result.Body, "date=20") {
		t.Fatalf("date helper output missing ISO-like date:\n%s", result.Body)
	}
}

func TestLegacySimpleTemplateKeepsTokenSyntax(t *testing.T) {
	t.Parallel()

	result, err := New().Render(TemplateDocument{
		Name:   "legacy",
		Engine: EngineSimple,
		Body:   "# {{title}}\n日期: {{date}}\n项目: {{project}}\n标签: {{tags}}\n客户: {{client}}\n",
	}, Context{
		Title:   "客户会议",
		Date:    "2026-06-08",
		Project: "research",
		Tags:    []string{"pinax", "sync"},
		Vars:    map[string]string{"client": "Acme"},
	})
	if err != nil {
		t.Fatalf("render legacy template: %v", err)
	}
	for _, want := range []string{"# 客户会议", "日期: 2026-06-08", "项目: research", "标签: pinax, sync", "客户: Acme"} {
		if !strings.Contains(result.Body, want) {
			t.Fatalf("legacy rendered body missing %q:\n%s", want, result.Body)
		}
	}
}

func TestSimpleTemplateReportsMissingVariable(t *testing.T) {
	t.Parallel()

	_, err := New().Render(TemplateDocument{
		Name:   "legacy-missing",
		Engine: EngineSimple,
		Body:   "客户: {{client}}",
	}, Context{Vars: map[string]string{}})
	if err == nil {
		t.Fatal("expected missing variable error")
	}
	if code := ErrorCode(err); code != "template_variable_missing" {
		t.Fatalf("error code = %q, want template_variable_missing; err=%v", code, err)
	}
}
