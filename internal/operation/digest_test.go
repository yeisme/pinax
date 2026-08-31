package operation

import "testing"

func TestCanonicalDigestIsStableAcrossMapOrder(t *testing.T) {
	t.Parallel()

	left, err := CanonicalDigest(map[string]any{
		"title":  "Capture",
		"nested": map[string]any{"b": 2, "a": 1},
		"tags":   []any{"one", "two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CanonicalDigest(map[string]any{
		"tags":   []any{"one", "two"},
		"nested": map[string]any{"a": 1, "b": 2},
		"title":  "Capture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if left != right || !digestPattern.MatchString(left) {
		t.Fatalf("canonical digests differ: left=%q right=%q", left, right)
	}

	changed, err := CanonicalDigest(map[string]any{"title": "Different"})
	if err != nil {
		t.Fatal(err)
	}
	if changed == left {
		t.Fatalf("different request produced identical digest %q", changed)
	}
}

func TestIdentityDigestUsesUnambiguousPartBoundaries(t *testing.T) {
	t.Parallel()

	if IdentityDigest("ab", "c") == IdentityDigest("a", "bc") {
		t.Fatal("identity digest ignored part boundaries")
	}
}
