package templateops

import "github.com/yeisme/pinax/internal/templateengine"

func QueryResultMarkdown(result templateengine.QueryResult) string {
	rendered, err := templateengine.New().Render(templateengine.TemplateDocument{Name: "query-result", Engine: templateengine.EngineGoTemplate, Body: "{{ table .Queries.result }}"}, templateengine.Context{Queries: map[string]templateengine.QueryResult{"result": result}})
	if err != nil {
		return ""
	}
	return rendered.Body
}
