package promptrepo

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"gopkg.in/yaml.v3"
)

var (
	jsonNumberPattern    = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
	jsonlRecordIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,255}$`)
)

type jsonlRecord struct {
	id    string
	start int
	end   int
}

func DecodeDocument(document TemplateDocumentDescriptor, payload []byte) (LoadedDocument, error) {
	if err := ValidateTemplateDocumentDescriptor(document); err != nil {
		return LoadedDocument{}, err
	}
	if len(payload) > document.Limits.MaxBytes {
		return LoadedDocument{}, NewError(CodeDocumentTooLarge, "document exceeds the declared byte limit", false, nil)
	}
	if !utf8.Valid(payload) {
		return LoadedDocument{}, NewError(CodeDocumentParseFailed, "document must be valid UTF-8", false, nil)
	}
	if digestBytes(payload) != document.SourceDigest {
		return LoadedDocument{}, NewError(CodeDigestMismatch, "document source digest does not match the descriptor", false, nil)
	}

	loaded := LoadedDocument{
		Descriptor:   document,
		SourceDigest: document.SourceDigest,
		SourceBytes:  len(payload),
		Ready:        true,
		Body:         append([]byte(nil), payload...),
	}
	var canonical []byte
	switch document.Format {
	case DocumentFormatMarkdown, DocumentFormatText:
		loaded.Value = string(payload)
		canonical = append([]byte(nil), payload...)
	case DocumentFormatJSON:
		value, err := decodeStrictJSON(payload, document.Limits.MaxDepth)
		if err != nil {
			return LoadedDocument{}, err
		}
		loaded.Value = value
		canonical, err = canonicalJSON(payload)
		if err != nil {
			return LoadedDocument{}, err
		}
	case DocumentFormatYAML:
		value, err := decodeStrictYAML(payload, document.Limits.MaxDepth)
		if err != nil {
			return LoadedDocument{}, err
		}
		loaded.Value = value
		canonical, err = canonicalizeValue(value)
		if err != nil {
			return LoadedDocument{}, err
		}
	case DocumentFormatJSONL:
		index, canonicalDigest, canonicalBytes, recordCount, err := decodeStrictJSONL(payload, document)
		if err != nil {
			return LoadedDocument{}, err
		}
		loaded.jsonlRecords = index
		loaded.RecordCount = recordCount
		loaded.CanonicalDigest = canonicalDigest
		loaded.CanonicalBytes = canonicalBytes
	default:
		return LoadedDocument{}, NewError(CodeDocumentFormatMismatch, "document format is unsupported", false, nil)
	}
	if document.Format == DocumentFormatJSONL {
		return loaded, nil
	}
	loaded.CanonicalDigest = digestBytes(canonical)
	loaded.CanonicalBytes = len(canonical)
	return loaded, nil
}

func decodeStrictJSON(payload []byte, maxDepth int) (any, error) {
	if !utf8.Valid(payload) {
		return nil, NewError(CodeDocumentParseFailed, "JSON document must be valid UTF-8", false, nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	value, err := readJSONValue(decoder, 1, maxDepth)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, NewError(CodeDocumentParseFailed, "JSON document has trailing data", false, err)
	}
	return value, nil
}

func readJSONValue(decoder *json.Decoder, depth, maxDepth int) (any, error) {
	if depth > maxDepth {
		return nil, NewError(CodeDocumentParseFailed, "JSON document exceeds the declared depth limit", false, nil)
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, NewError(CodeDocumentParseFailed, "JSON document is invalid", false, err)
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		if number, ok := token.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
			return nil, NewError(CodeDocumentParseFailed, "JSON number must be finite", false, nil)
		}
		return token, nil
	}
	switch delimiter {
	case '{':
		result := map[string]any{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, NewError(CodeDocumentParseFailed, "JSON object key is invalid", false, err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, NewError(CodeDocumentParseFailed, "JSON object key must be a string", false, nil)
			}
			if _, duplicate := result[key]; duplicate {
				return nil, NewError(CodeDocumentDuplicateKey, "JSON document contains a duplicate object key", false, nil)
			}
			value, err := readJSONValue(decoder, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, NewError(CodeDocumentParseFailed, "JSON object is not terminated", false, err)
		}
		return result, nil
	case '[':
		result := []any{}
		for decoder.More() {
			value, err := readJSONValue(decoder, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, NewError(CodeDocumentParseFailed, "JSON array is not terminated", false, err)
		}
		return result, nil
	default:
		return nil, NewError(CodeDocumentParseFailed, "JSON document contains an unexpected delimiter", false, nil)
	}
}

func decodeStrictYAML(payload []byte, maxDepth int) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, NewError(CodeDocumentParseFailed, "YAML document is invalid", false, err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, NewError(CodeDocumentParseFailed, "YAML input must contain exactly one document", false, nil)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, NewError(CodeDocumentParseFailed, "YAML input must not contain multiple documents", false, err)
	}
	return yamlNodeToJSON(document.Content[0], 1, maxDepth)
}

func yamlNodeToJSON(node *yaml.Node, depth, maxDepth int) (any, error) {
	if node == nil || depth > maxDepth {
		return nil, NewError(CodeDocumentParseFailed, "YAML document exceeds the declared depth limit", false, nil)
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return nil, NewError(CodeDocumentParseFailed, "YAML aliases, anchors, and explicit tags are not supported", false, nil)
	}
	switch node.Kind {
	case yaml.MappingNode:
		result := map[string]any{}
		for index := 0; index < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			if keyNode.Kind != yaml.ScalarNode || keyNode.Tag != "!!str" || keyNode.Anchor != "" || keyNode.Style&yaml.TaggedStyle != 0 || keyNode.Value == "<<" {
				return nil, NewError(CodeDocumentParseFailed, "YAML map keys must be plain strings and merge keys are forbidden", false, nil)
			}
			if _, duplicate := result[keyNode.Value]; duplicate {
				return nil, NewError(CodeDocumentDuplicateKey, "YAML document contains a duplicate map key", false, nil)
			}
			value, err := yamlNodeToJSON(node.Content[index+1], depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			result[keyNode.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := yamlNodeToJSON(child, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			return node.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool":
			if node.Value == "true" {
				return true, nil
			}
			if node.Value == "false" {
				return false, nil
			}
			return nil, NewError(CodeDocumentParseFailed, "YAML boolean must use JSON-compatible syntax", false, nil)
		case "!!int", "!!float":
			if !jsonNumberPattern.MatchString(node.Value) {
				return nil, NewError(CodeDocumentParseFailed, "YAML number must use JSON-compatible finite syntax", false, nil)
			}
			number, err := strconv.ParseFloat(node.Value, 64)
			if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
				return nil, NewError(CodeDocumentParseFailed, "YAML number must be finite IEEE-754", false, err)
			}
			return number, nil
		default:
			return nil, NewError(CodeDocumentParseFailed, "YAML scalar type is outside the JSON-compatible subset", false, nil)
		}
	default:
		return nil, NewError(CodeDocumentParseFailed, "YAML node type is outside the JSON-compatible subset", false, nil)
	}
}

func decodeStrictJSONL(payload []byte, document TemplateDocumentDescriptor) (map[string]jsonlRecord, string, int, int, error) {
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		return nil, "", 0, 0, NewError(CodeDocumentParseFailed, "JSONL document must use LF line endings and end with a newline", false, nil)
	}
	reader := bufio.NewReaderSize(bytes.NewReader(payload), document.Limits.MaxRecordBytes+1)
	index := make(map[string]jsonlRecord)
	canonicalHash := sha256.New()
	canonicalBytes := 0
	recordCount := 0
	offset := 0
	for {
		lineWithNewline, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(lineWithNewline) == 0 {
			break
		}
		if err != nil {
			return nil, "", 0, 0, NewError(CodeDocumentParseFailed, "JSONL record could not be read", false, err)
		}
		line := lineWithNewline[:len(lineWithNewline)-1]
		if len(line) == 0 || bytes.ContainsRune(line, '\r') {
			return nil, "", 0, 0, NewError(CodeDocumentParseFailed, "JSONL document must use LF line endings and must not contain blank records", false, nil)
		}
		if len(line) > document.Limits.MaxRecordBytes {
			return nil, "", 0, 0, NewError(CodeJSONLRecordTooLarge, "JSONL record exceeds the declared byte limit", false, nil)
		}
		if recordCount >= document.Limits.MaxRecords {
			return nil, "", 0, 0, NewError(CodeDocumentTooLarge, "JSONL document exceeds the declared record limit", false, nil)
		}
		value, err := decodeStrictJSON(line, document.Limits.MaxDepth)
		if err != nil {
			return nil, "", 0, 0, err
		}
		record, ok := value.(map[string]any)
		if !ok {
			return nil, "", 0, 0, NewError(CodeDocumentParseFailed, "each JSONL record must be an object", false, nil)
		}
		recordID, ok := record[document.RecordIDField].(string)
		if !ok || !jsonlRecordIDPattern.MatchString(recordID) {
			return nil, "", 0, 0, NewError(CodeJSONLRecordIDInvalid, "JSONL record id is missing or invalid", false, nil)
		}
		if _, duplicate := index[recordID]; duplicate {
			return nil, "", 0, 0, NewError(CodeJSONLRecordDuplicate, "JSONL document contains a duplicate record id", false, nil)
		}
		canonicalRecord, err := canonicalJSON(line)
		if err != nil {
			return nil, "", 0, 0, err
		}
		_, _ = canonicalHash.Write(canonicalRecord)
		_, _ = canonicalHash.Write([]byte{'\n'})
		canonicalBytes += len(canonicalRecord) + 1
		index[recordID] = jsonlRecord{id: recordID, start: offset, end: offset + len(line)}
		offset += len(lineWithNewline)
		recordCount++
	}
	return index, fmt.Sprintf("sha256:%x", canonicalHash.Sum(nil)), canonicalBytes, recordCount, nil
}

func canonicalizeValue(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, NewError(CodeDocumentCanonicalizeFailed, "document value cannot be encoded as JSON", false, err)
	}
	return canonicalJSON(payload)
}

func canonicalJSON(payload []byte) ([]byte, error) {
	canonical, err := jcs.Transform(payload)
	if err != nil {
		return nil, NewError(CodeDocumentCanonicalizeFailed, "document JSON canonicalization failed", false, err)
	}
	return canonical, nil
}
