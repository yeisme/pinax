package searchops

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFirstSnippetUnicode(t *testing.T) {
	// 中文及大小写转换可能改变字节长度，片段必须仍对应原文中的完整字符。
	tests := []struct {
		name, body, query, contains string
	}{
		{"multibyte_edges", strings.Repeat("旧资料", 20) + "xPinax" + strings.Repeat("新判断", 30), "pinax", "Pinax"},
		{"prefix_without_match", "x" + strings.Repeat("证据🙂", 50), "missing", "证据🙂"},
		{"prefix_without_query", "x" + strings.Repeat("证据🙂", 50), "", "证据🙂"},
		{"case_mapping_offsets", strings.Repeat("K", 40) + "TARGET" + strings.Repeat("结论", 40), "target", "TARGET"},
		{"case_mapping_query", strings.Repeat("x", 40) + "K" + strings.Repeat("证据", 40), "k", "K"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirstSnippet(tt.body, tt.query)
			if !utf8.ValidString(got) || !strings.Contains(tt.body, got) || !strings.Contains(got, tt.contains) {
				t.Fatalf("snippet must preserve complete original characters and matched text: %q", got)
			}
			if utf8.RuneCountInString(got) > 120+utf8.RuneCountInString(tt.query) {
				t.Fatal("snippet exceeded the bounded character window")
			}
		})
	}
	if got := FirstSnippet("  ordinary evidence  ", "evidence"); got != "ordinary evidence" {
		t.Fatalf("ASCII behavior changed: %q", got)
	}
	if got := FirstSnippet(" \n ", "question"); got != "" {
		t.Fatalf("empty body returned %q", got)
	}
}
