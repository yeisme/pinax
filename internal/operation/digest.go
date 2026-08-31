package operation

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// CanonicalDigest produces a stable digest for JSON-shaped request DTOs. It
// sorts every object level and preserves array order, so Go map iteration order
// cannot create a second logical idempotency binding.
func CanonicalDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", operationError(CodePayloadInvalid, "Operation request cannot be canonicalized", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", operationError(CodePayloadInvalid, "Operation request cannot be canonicalized", err)
	}
	var canonical bytes.Buffer
	if err := writeCanonicalJSON(&canonical, decoded); err != nil {
		return "", operationError(CodePayloadInvalid, "Operation request cannot be canonicalized", err)
	}
	sum := sha256.Sum256(canonical.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// IdentityDigest hashes principal and scope material with length prefixes, so
// raw identity values never need to enter the ledger.
func IdentityDigest(parts ...string) string {
	hash := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func writeCanonicalJSON(buffer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
	case bool:
		buffer.WriteString(strconv.FormatBool(typed))
	case string:
		encoded, _ := json.Marshal(typed)
		buffer.Write(encoded)
	case json.Number:
		if _, err := typed.Int64(); err != nil {
			if _, floatErr := typed.Float64(); floatErr != nil {
				return fmt.Errorf("invalid JSON number")
			}
		}
		buffer.WriteString(typed.String())
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			encoded, _ := json.Marshal(key)
			buffer.Write(encoded)
			buffer.WriteByte(':')
			if err := writeCanonicalJSON(buffer, typed[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical JSON value %T", value)
	}
	return nil
}
