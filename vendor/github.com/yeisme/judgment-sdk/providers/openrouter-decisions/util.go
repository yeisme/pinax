package openrouterdecisions

import (
	"encoding/json"
	"math"
)

func mustMarshal(v any) []byte {
	out, err := json.Marshal(v)
	if err != nil {
		panic("openrouterdecisions: constant document not marshalable: " + err.Error())
	}
	return out
}

// num converts a wire JSON value (json.Number from judgment.ParseTree, or a
// native float64 built in-process) to a finite float64.
func num(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, false
		}
		return t, true
	default:
		return 0, false
	}
}

// integral reports whether f is a non-negative integer within int64 range.
func integral(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) && f >= 0 && f <= 1e15
}

// str asserts a wire string value.
func str(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}
