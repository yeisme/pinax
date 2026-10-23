package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReaderContractComposedShapes(t *testing.T) {
	check := func(id string, v any, valid bool) {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		err = ValidatePayload(Default(), id, decoded)
		if (err == nil) != valid {
			t.Fatalf("schema=%s valid=%v err=%v", id, valid, err)
		}
	}
	target := func() map[string]any {
		return map[string]any{"object_id": "note_target", "revision": "sha256:" + strings.Repeat("a", 64)}
	}
	edge := func() map[string]any {
		return map[string]any{"embed_id": "embed-1", "parent_embed_id": "", "source_anchor": "block-1", "page_anchors": []any{"embed-1-block-1"}, "depth": 1, "status": "expanded", "target": target()}
	}
	check(ReaderComposedRequestV1Alpha1, target(), true)
	for _, key := range []string{"session_id", "vault_path", "max_depth", "execute"} {
		v := target()
		v[key] = "injected"
		check(ReaderComposedRequestV1Alpha1, v, false)
	}
	check(ReaderEmbedTargetV1Alpha1, target(), true)
	check(ReaderEmbedV1Alpha1, edge(), true)
	cycle := edge()
	cycle["status"] = "cycle"
	cycle["depth"] = 2
	cycle["parent_embed_id"] = "embed-parent"
	check(ReaderEmbedV1Alpha1, cycle, true)
	for _, status := range []string{"unavailable", "unsupported", "depth_limit"} {
		v := edge()
		v["status"] = status
		delete(v, "target")
		if status == "depth_limit" {
			v["depth"] = 3
		}
		check(ReaderEmbedV1Alpha1, v, true)
		v["target"] = target()
		check(ReaderEmbedV1Alpha1, v, false)
	}
	for _, mode := range []string{"missing-target", "unknown", "depth0", "depth3", "null-target", "duplicate-page", "empty-page", "raw", "path", "execute"} {
		t.Run(mode, func(t *testing.T) {
			v := edge()
			switch mode {
			case "missing-target":
				delete(v, "target")
			case "unknown":
				v["status"] = "resolved"
			case "depth0":
				v["depth"] = 0
			case "depth3":
				v["depth"] = 3
			case "null-target":
				v["target"] = nil
			case "duplicate-page":
				v["page_anchors"] = []any{"a", "a"}
			case "empty-page":
				v["page_anchors"] = []any{}
			default:
				v[mode] = "forbidden"
			}
			check(ReaderEmbedV1Alpha1, v, false)
		})
	}
	origin := map[string]any{"page_anchor": "embed-1-block-1-f2", "embed_id": "embed-1", "source_anchor": "block-1"}
	check(ReaderNodeOriginV1Alpha1, origin, true)
	origin["body"] = "no"
	check(ReaderNodeOriginV1Alpha1, origin, false)
	document := map[string]any{"schema_version": ReaderDocumentV1Alpha1, "object_id": "note_root", "revision": "sha256:" + strings.Repeat("b", 64), "current_revision": "sha256:" + strings.Repeat("b", 64), "uri": "pinax://note/note_root", "nodes": []any{}, "outline": []any{}, "complete": true, "content_bytes": 0, "observed_at": "2026-10-06T00:00:00Z"}
	wrapper := map[string]any{"schema_version": ReaderComposedDocumentV1Alpha1, "rich_document": map[string]any{"schema_version": ReaderRichDocumentV1Alpha1, "document": document, "annotations": []any{}}, "embeds": []any{}, "origins": []any{}}
	check(ReaderComposedDocumentV1Alpha1, wrapper, true)
	delete(wrapper, "origins")
	check(ReaderComposedDocumentV1Alpha1, wrapper, false)
	wrapper["origins"] = []any{}
	tooMany := make([]any, 33)
	for i := range tooMany {
		tooMany[i] = edge()
	}
	wrapper["embeds"] = tooMany
	check(ReaderComposedDocumentV1Alpha1, wrapper, false)
	wrapper["embeds"] = []any{}
	wrapper["raw_markdown"] = "no"
	check(ReaderComposedDocumentV1Alpha1, wrapper, false)
	// An embed is not a new variant of the old closed rich annotation contract.
	check(ReaderRichAnnotationV1Alpha1, edge(), false)
}
