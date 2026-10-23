package templateengine

import (
	"context"
	"strings"
	"testing"
)

func TestDailyBoundedTemplate(t *testing.T) {
	t.Parallel()
	engine := New()
	values := Context{Date: "2026-02-03", Title: "Daily", Vars: map[string]string{"many": strings.Repeat("字", 40)}}
	for _, test := range []struct {
		name, body, engine string
		budget             int
		valid              bool
	}{
		{"fixed date", `{{date "2006-01-02"}} {{.Date}}`, EngineGoTemplate, 64, true},
		{"output cap", strings.Repeat("{{.Vars.many}}", 10), EngineGoTemplate, 256, false},
		{"input cap", strings.Repeat("a", 300), EngineSimple, 256, false},
		{"simple output cap", strings.Repeat("{{many}}", 10), EngineSimple, 256, false},
		{"simple valid", "# {{date}}", EngineSimple, 256, true},
		{"huge printf", `{{printf "%1000000000s" ""}}`, EngineGoTemplate, 256, false},
		{"recursive", `{{define "loop"}}{{template "loop"}}{{end}}{{template "loop"}}`, EngineGoTemplate, 256, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := engine.RenderBounded(context.Background(), TemplateDocument{Name: "daily", Body: test.body, Engine: test.engine}, values, test.budget)
			if test.valid {
				if err != nil || len(result.Body) > test.budget {
					t.Fatalf("bounded render: %v", err)
				}
				if test.name == "fixed date" && result.Body != "2026-02-03 2026-02-03" {
					t.Fatal("machine clock influenced preview")
				}
			} else if err == nil {
				t.Fatal("unsafe template accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.RenderBounded(ctx, TemplateDocument{Name: "daily", Engine: EngineGoTemplate, Body: "hello"}, values, 64); err == nil {
		t.Fatal("cancelled render accepted")
	}
}
