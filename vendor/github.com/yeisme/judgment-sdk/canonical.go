package judgment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode/utf16"
)

// ErrNonFinite is returned when canonicalization meets NaN or Infinity.
var ErrNonFinite = errors.New("nonfinite_number")

// ParseTree decodes JSON into the package tree shape: nil, bool, string,
// json.Number, []any, or map[string]any. Numbers stay as json.Number so the
// original lexeme survives until canonical formatting. Trailing content is
// rejected.
func ParseTree(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing_content")
	}
	return v, nil
}

// MarshalTree re-encodes a tree produced by ParseTree using plain (not
// canonical) JSON. json.Number lexemes are preserved verbatim.
func MarshalTree(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// CanonicalJSON serializes v with RFC 8785 / JCS equivalent rules: object
// keys sorted by UTF-16 code unit order, minimal string escaping,
// ECMAScript number formatting, no NaN/Infinity, no insignificant
// whitespace. Map iteration order never influences the output.
func CanonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Digest returns "sha256:<64 lowercase hex>" over the canonical JSON of v.
func Digest(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		writeCanonicalString(buf, t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return fmt.Errorf("number_out_of_range: %w", err)
		}
		return writeCanonicalFloat(buf, f)
	case float64:
		return writeCanonicalFloat(buf, t)
	case int:
		return writeCanonicalFloat(buf, float64(t))
	case int64:
		if !exactlyRepresentable(int64(t)) {
			return errors.New("number_not_canonical")
		}
		return writeCanonicalFloat(buf, float64(t))
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonicalString(buf, k)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return errors.New("unsupported_json_type")
	}
	return nil
}

func exactlyRepresentable(n int64) bool {
	return int64(float64(n)) == n
}

func writeCanonicalString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

func writeCanonicalFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ErrNonFinite
	}
	if f == 0 {
		buf.WriteString("0")
		return nil
	}
	a := math.Abs(f)
	if a >= 1e21 || a < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		ei := bytes.IndexByte([]byte(s), 'e')
		mant := s[:ei]
		expNum, err := strconv.ParseInt(s[ei+1:], 10, 32)
		if err != nil {
			return err
		}
		sign := "+"
		if expNum < 0 {
			sign = "-"
			expNum = -expNum
		}
		buf.WriteString(mant)
		buf.WriteByte('e')
		buf.WriteString(sign)
		buf.WriteString(strconv.FormatInt(expNum, 10))
		return nil
	}
	buf.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
	return nil
}

// utf16Less orders strings by UTF-16 code unit sequence, the JCS key order.
func utf16Less(a, b string) bool {
	ua := utf16.Encode([]rune(a))
	ub := utf16.Encode([]rune(b))
	n := len(ua)
	if len(ub) < n {
		n = len(ub)
	}
	for i := 0; i < n; i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}
