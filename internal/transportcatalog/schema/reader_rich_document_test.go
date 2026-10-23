package schema

import (
	"strings"
	"testing"
)

func TestReaderContractRichClosedShapes(t *testing.T) {
	check := func(t *testing.T, id string, value any, valid bool) {
		t.Helper()
		err := ValidatePayload(Default(), id, value)
		if (err == nil) != valid {
			t.Fatalf("valid=%v err=%v", valid, err)
		}
	}
	check(t, ReaderRichDocumentRequestV1Alpha1, map[string]any{"object_id": "note_one", "revision": "sha256:" + strings.Repeat("a", 64)}, true)
	check(t, ReaderRichDocumentRequestV1Alpha1, map[string]any{"object_id": "note_one"}, false)
	check(t, ReaderRichDocumentRequestV1Alpha1, map[string]any{"object_id": "note_one", "revision": "sha256:" + strings.Repeat("a", 64), "session_id": "injected"}, false)
	code := func() map[string]any {
		return map[string]any{"anchor": "code-1", "page_anchors": []any{"code-1"}, "kind": "math", "source": "x+y"}
	}
	callout := map[string]any{"anchor": "quote-1", "page_anchors": []any{"paragraph-2"}, "kind": "callout", "callout_kind": "note", "fold": "fixed"}
	check(t, ReaderRichAnnotationV1Alpha1, callout, true)
	check(t, ReaderRichAnnotationV1Alpha1, code(), true)
	empty := code()
	delete(empty, "source")
	check(t, ReaderRichAnnotationV1Alpha1, empty, true)
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"kind", func(v map[string]any) { v["kind"] = "html" }},
		{"mixed-branch", func(v map[string]any) { v["fold"] = "collapsed" }},
		{"empty-targets", func(v map[string]any) { v["page_anchors"] = []any{} }},
		{"duplicate-targets", func(v map[string]any) { v["page_anchors"] = []any{"a", "a"} }},
		{"null", func(v map[string]any) { v["source"] = nil }},
		{"script", func(v map[string]any) { v["execute"] = true }},
		{"oversized", func(v map[string]any) { v["source"] = strings.Repeat("x", ReaderRichMaxPayloadBytes+1) }},
	} {
		t.Run(test.name, func(t *testing.T) { v := code(); test.mutate(v); check(t, ReaderRichAnnotationV1Alpha1, v, false) })
	}
}
