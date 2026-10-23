package schema

import "testing"

func TestWriterContractOneOfExactMatch(t *testing.T) {
	id := "pinax.test.union.v1"
	registry, err := New(define(id, map[string]any{"oneOf": []any{closedObject(map[string]any{"mode": constString("a")}, "mode"), closedObject(map[string]any{"mode": constString("b")}, "mode")}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"a", "b"} {
		if err := ValidatePayload(registry, id, map[string]any{"mode": mode}); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []any{map[string]any{"mode": "c"}, map[string]any{"mode": "a", "extra": true}, nil, "a"} {
		if ValidatePayload(registry, id, v) == nil {
			t.Fatal("union admitted no matching branch")
		}
	}
	ambiguous, err := New(define(id, map[string]any{"oneOf": []any{stringValue(), stringValue()}}))
	if err != nil {
		t.Fatal(err)
	}
	if ValidatePayload(ambiguous, id, "both") == nil {
		t.Fatal("union admitted multiple matching branches")
	}
}
