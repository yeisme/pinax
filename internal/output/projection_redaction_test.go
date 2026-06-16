package output

import (
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

// TestApplyProjectionRedactionScansNestedPayloads 证明共享脱敏门禁递归扫描 projection
// 全部渲染面，把受保护凭证/prompt/webhook 替换成有界占位符，覆盖 facts/actions/evidence/data/error。
// note 正文（body）由各命令的有界投影控制，门禁不做全局清空，只拦截凭证与 prompt。
func TestApplyProjectionRedactionScansNestedPayloads(t *testing.T) {
	p := domain.Projection{
		Summary: "Authorization: Bearer s3cr3t leak",
		Facts:   map[string]string{"auth": "Bearer abc123", "endpoint": "token=xyz456"},
		Actions: []domain.Action{{Name: "cookie=session-leak", Command: "https://hooks.example.local/webhook/abc"}},
		Evidence: []string{
			"secret_ref=op://vault/cloud",
		},
		Data: map[string]any{
			"config": []map[string]any{
				{"endpoint": "ok", "authorization": "Bearer nested-leak"},
			},
			"results": []any{
				map[string]any{"note": map[string]any{"title": "ok", "raw_prompt": "system: you are"}},
			},
			"api_key": "sk-live-1234567890",
		},
		Error: &domain.CommandError{Code: "x", Message: "Bearer tok failed", Hint: "cookie=monster"},
	}

	ApplyProjectionRedaction(&p)

	assertNotContains(t, p.Summary, "s3cr3t")
	assertNotContains(t, p.Facts["auth"], "abc123")
	assertNotContains(t, p.Facts["endpoint"], "xyz456")
	assertNotContains(t, p.Actions[0].Name, "session-leak")
	assertNotContains(t, p.Actions[0].Command, "webhook/abc")
	if !strings.Contains(p.Actions[0].Command, "[REDACTED") {
		t.Fatalf("webhook URL should be redacted: %q", p.Actions[0].Command)
	}
	config := p.Data.(map[string]any)["config"].([]map[string]any)[0]
	if config["authorization"] != "[REDACTED]" {
		t.Fatalf("nested authorization should be fully redacted, got %q", config["authorization"])
	}
	results := p.Data.(map[string]any)["results"].([]any)[0].(map[string]any)["note"].(map[string]any)
	if results["raw_prompt"] != "[REDACTED]" {
		t.Fatalf("raw_prompt should be fully redacted: %q", results["raw_prompt"])
	}
	apiKey := p.Data.(map[string]any)["api_key"]
	if apiKey != "[REDACTED]" {
		t.Fatalf("api_key should be fully redacted: %q", apiKey)
	}
	if strings.Contains(p.Error.Message, "tok") {
		t.Fatalf("error message should redact bearer token: %q", p.Error.Message)
	}
	if strings.Contains(p.Error.Hint, "monster") {
		t.Fatalf("error hint should redact cookie: %q", p.Error.Hint)
	}
}

// TestApplyProjectionRedactionPreservesSafeContent 证明门禁不误伤正常事实、路径与摘要。
func TestApplyProjectionRedactionPreservesSafeContent(t *testing.T) {
	p := domain.Projection{
		Summary: "Version restore applied to local Markdown.",
		Facts:   map[string]string{"path": "notes/alpha.md", "plan_id": "restore_20260615", "local_write": "true"},
		Data:    map[string]any{"files_changed": 2, "receipt": ".pinax/receipts/restore-x.json"},
	}
	ApplyProjectionRedaction(&p)
	if p.Summary != "Version restore applied to local Markdown." {
		t.Fatalf("safe summary altered: %q", p.Summary)
	}
	if p.Facts["path"] != "notes/alpha.md" || p.Facts["plan_id"] != "restore_20260615" {
		t.Fatalf("safe facts altered: %#v", p.Facts)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected %q to be redacted, but found in: %q", needle, haystack)
	}
}
