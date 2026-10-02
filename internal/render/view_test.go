package render

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRenderedEnvelopeAnchorDisplayRedlineAndFenceFailures(t *testing.T) {
	body := "# 同名\n\n" + strings.Repeat("正文", 600) + "\n\n# 同名\n\n[[Known]] [[Missing]]\n\n```pinax-board\nmissing\n```\n\n```unknown\nplain code\n```\n\n/home/private/file\n"
	in := Inputs{Body: body, Display: "context", Ref: Reference{ObjectID: "note-1", Status: "resolved"}, Badge: Badge{ObjectID: "note-1", Revision: "sha256:abc"}, Resolve: func(target, _ string) Reference {
		if target == "Known" {
			return Reference{ObjectID: "note-2", URI: "pinax://note/note-2", Status: "resolved"}
		}
		return Reference{Status: "broken"}
	}, Fences: map[string]func(string) FenceResult{"pinax-board": func(string) FenceResult {
		return FenceResult{ErrorCode: "board_not_found", NextAction: "pinax project list"}
	}}}
	a, b := Build(in), Build(in)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("tree is nondeterministic")
	}
	if a.SchemaVersion != SchemaVersion || a.Tree[0].Anchor == a.Tree[2].Anchor || !a.Truncated || a.DroppedUnsafe == 0 {
		t.Fatal("envelope, anchors or redline missing")
	}
	data, _ := json.Marshal(a)
	if strings.Contains(string(data), "/home/private") || strings.Contains(string(data), strings.Repeat("正文", 600)) || !strings.Contains(string(data), "fence_failed") || !strings.Contains(string(data), "broken") {
		t.Fatal("bounded or failure semantics violated")
	}
	in.Display = "card"
	card, _ := json.Marshal(Build(in))
	if strings.Contains(string(card), "正文") || strings.Contains(string(card), "plain code") {
		t.Fatal("card includes body content")
	}
	in.Display = "body"
	full, _ := json.Marshal(Build(in))
	if !strings.Contains(string(full), strings.Repeat("正文", 600)) || !strings.Contains(string(full), "plain code") {
		t.Fatal("explicit body or unknown fence was lost")
	}
	if strings.Contains(HTML(Build(Inputs{Body: "<script>alert(1)</script>", Display: "body"})), "<script>") {
		t.Fatal("HTML export permits script injection")
	}
}

func TestRenderedBacklinksBoundedAndFullUnsafeScan(t *testing.T) {
	refs := []Reference{}
	for i := 0; i < 70; i++ {
		refs = append(refs, Reference{ObjectID: "note-safe", Title: "title", Status: "resolved"})
	}
	refs = append(refs, Reference{ObjectID: "token-value", Title: "hidden"})
	v := Build(Inputs{Body: "# Title", Backlinks: refs})
	p := v.Panels["backlinks"]
	if len(p.Entities) != 50 || !p.Truncated || p.DroppedUnsafe != 1 {
		t.Fatalf("bounds: %#v", p)
	}
	dropped, truncated := 0, false
	vv := BoundPayload(map[string]any{"path": "notes/secret.md", "body": "hidden", "unsafe": "Bearer fixture-value", "rows": make([]int, 120)}, &dropped, &truncated)
	data, _ := json.Marshal(vv)
	if strings.Contains(string(data), "secret.md") || strings.Contains(string(data), "fixture-value") || dropped != 1 || !truncated {
		t.Fatal("payload redaction failed")
	}
}

func TestRenderedRedlineCoversProofFactsAndSignedReferences(t *testing.T) {
	v := Build(Inputs{Body: "# Safe", Ref: Reference{ObjectID: "/home/private", URI: "https://example.invalid/file?X-Amz-Signature=fixture-value"}, Badge: Badge{ObjectID: "cookie-id", Revision: "token=fixture-value", RenderedAt: "/tmp/private"}})
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "fixture-value") || strings.Contains(string(b), "/home/") || strings.Contains(string(b), "/tmp/") || strings.Contains(string(b), "cookie-id") || v.DroppedUnsafe < 4 {
		t.Fatal("proof facts or signed reference leaked")
	}
}
