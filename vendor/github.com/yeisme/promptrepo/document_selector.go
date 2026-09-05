package promptrepo

import (
	"bytes"
	"strconv"
	"strings"
)

func SelectLoadedDocument(document LoadedDocument, selector string) (SelectedDocument, error) {
	if err := validateLoadedDocumentForSelection(document); err != nil {
		return SelectedDocument{}, err
	}
	kind, argument, err := parseDocumentSelector(selector)
	if err != nil {
		return SelectedDocument{}, err
	}
	if !containsStringValue(document.Descriptor.SelectorKinds, kind) {
		return SelectedDocument{}, NewError(CodeSelectorUnsupported, "document descriptor does not allow this selector kind", false, nil)
	}
	metadata := document
	metadata.Body = nil
	metadata.Value = nil
	metadata.jsonlRecords = nil
	selected := SelectedDocument{Document: metadata, Selector: selector, Ready: true}
	var canonical []byte
	switch kind {
	case "heading":
		if document.Descriptor.Format != DocumentFormatMarkdown {
			return SelectedDocument{}, NewError(CodeSelectorUnsupported, "heading selector requires a Markdown document", false, nil)
		}
		body, err := selectMarkdownHeading(document.Body, argument)
		if err != nil {
			return SelectedDocument{}, err
		}
		selected.Body = body
		selected.Value = string(body)
		canonical = body
	case "json-pointer":
		if document.Descriptor.Format != DocumentFormatJSON {
			return SelectedDocument{}, NewError(CodeSelectorUnsupported, "JSON pointer requires a JSON document", false, nil)
		}
		value, err := selectJSONPointer(document.Value, argument)
		if err != nil {
			return SelectedDocument{}, err
		}
		selected.Value = value
		canonical, err = canonicalizeValue(value)
		if err != nil {
			return SelectedDocument{}, err
		}
	case "yaml-pointer":
		if document.Descriptor.Format != DocumentFormatYAML {
			return SelectedDocument{}, NewError(CodeSelectorUnsupported, "YAML pointer requires a YAML document", false, nil)
		}
		value, err := selectJSONPointer(document.Value, argument)
		if err != nil {
			return SelectedDocument{}, err
		}
		selected.Value = value
		canonical, err = canonicalizeValue(value)
		if err != nil {
			return SelectedDocument{}, err
		}
	case "jsonl-id":
		if document.Descriptor.Format != DocumentFormatJSONL {
			return SelectedDocument{}, NewError(CodeSelectorUnsupported, "JSONL id selector requires a JSONL document", false, nil)
		}
		record, ok := document.jsonlRecords[argument]
		if !ok {
			return SelectedDocument{}, NewError(CodeSelectorNotFound, "JSONL record id was not found", false, nil)
		}
		if record.start < 0 || record.end < record.start || record.end > len(document.Body) {
			return SelectedDocument{}, NewError(CodeDigestMismatch, "loaded JSONL record index no longer matches the source body", false, nil)
		}
		value, err := decodeStrictJSON(document.Body[record.start:record.end], document.Descriptor.Limits.MaxDepth)
		if err != nil {
			return SelectedDocument{}, err
		}
		selected.Value = value
		selected.RecordID = record.id
		canonical, err = canonicalizeValue(value)
		if err != nil {
			return SelectedDocument{}, err
		}
	default:
		return SelectedDocument{}, NewError(CodeSelectorUnsupported, "document selector kind is unsupported", false, nil)
	}
	selected.CanonicalDigest = digestBytes(canonical)
	selected.SelectedBytes = len(canonical)
	return selected, nil
}

func validateLoadedDocumentForSelection(document LoadedDocument) error {
	if !document.Ready {
		return NewError(CodeInvalidRequest, "loaded document is not ready for selection", false, nil)
	}
	if err := ValidateTemplateDocumentDescriptor(document.Descriptor); err != nil {
		return err
	}
	if document.SourceDigest != document.Descriptor.SourceDigest || digestBytes(document.Body) != document.SourceDigest {
		return NewError(CodeDigestMismatch, "loaded document source lineage no longer matches its descriptor", false, nil)
	}
	if document.Descriptor.Format == DocumentFormatJSON || document.Descriptor.Format == DocumentFormatYAML {
		canonical, err := canonicalizeValue(document.Value)
		if err != nil {
			return err
		}
		if digestBytes(canonical) != document.CanonicalDigest {
			return NewError(CodeDigestMismatch, "loaded document parsed node no longer matches its canonical digest", false, nil)
		}
	}
	return nil
}

func parseDocumentSelector(selector string) (string, string, error) {
	selector = strings.TrimSpace(selector)
	separator := strings.IndexByte(selector, ':')
	if separator <= 0 || separator == len(selector)-1 || strings.ContainsAny(selector, "\x00\n\r") {
		return "", "", NewError(CodeInvalidRequest, "document selector is invalid", false, nil)
	}
	kind, argument := selector[:separator], selector[separator+1:]
	switch kind {
	case "heading", "json-pointer", "yaml-pointer", "jsonl-id":
	default:
		return "", "", NewError(CodeSelectorUnsupported, "document selector kind is unsupported", false, nil)
	}
	if (kind == "json-pointer" || kind == "yaml-pointer") && !strings.HasPrefix(argument, "/") {
		return "", "", NewError(CodeInvalidRequest, "document pointer selector must start with a slash", false, nil)
	}
	return kind, argument, nil
}

func selectJSONPointer(root any, pointer string) (any, error) {
	current := root
	for _, encoded := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		token, ok := decodeJSONPointerToken(encoded)
		if !ok {
			return nil, NewError(CodeInvalidRequest, "JSON pointer escape is invalid", false, nil)
		}
		switch value := current.(type) {
		case map[string]any:
			next, exists := value[token]
			if !exists {
				return nil, NewError(CodeSelectorNotFound, "document pointer target was not found", false, nil)
			}
			current = next
		case []any:
			if token == "" || token == "-" || len(token) > 1 && token[0] == '0' {
				return nil, NewError(CodeSelectorNotFound, "document pointer array index is invalid", false, nil)
			}
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(value) {
				return nil, NewError(CodeSelectorNotFound, "document pointer array index is out of range", false, err)
			}
			current = value[index]
		default:
			return nil, NewError(CodeSelectorNotFound, "document pointer traverses a non-container value", false, nil)
		}
	}
	return current, nil
}

func decodeJSONPointerToken(value string) (string, bool) {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			result.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			return "", false
		}
		index++
		switch value[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", false
		}
	}
	return result.String(), true
}

func selectMarkdownHeading(payload []byte, heading string) ([]byte, error) {
	type match struct {
		start int
		end   int
		level int
	}
	lines := bytes.SplitAfter(payload, []byte{'\n'})
	offset := 0
	matches := []match{}
	for _, line := range lines {
		level, text, ok := parseMarkdownHeading(line)
		if ok && text == heading {
			matches = append(matches, match{start: offset, end: len(payload), level: level})
		}
		offset += len(line)
	}
	if len(matches) == 0 {
		return nil, NewError(CodeSelectorNotFound, "Markdown heading was not found", false, nil)
	}
	if len(matches) > 1 {
		return nil, NewError(CodeSelectorAmbiguous, "Markdown heading selector is ambiguous", false, nil)
	}
	target := matches[0]
	offset = 0
	started := false
	for _, line := range lines {
		if offset == target.start {
			started = true
			offset += len(line)
			continue
		}
		if started {
			level, _, ok := parseMarkdownHeading(line)
			if ok && level <= target.level {
				target.end = offset
				break
			}
		}
		offset += len(line)
	}
	return append([]byte(nil), payload[target.start:target.end]...), nil
}

func parseMarkdownHeading(line []byte) (int, string, bool) {
	text := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
	spaces := 0
	for spaces < len(text) && spaces < 4 && text[spaces] == ' ' {
		spaces++
	}
	if spaces > 3 {
		return 0, "", false
	}
	text = text[spaces:]
	level := 0
	for level < len(text) && level < 6 && text[level] == '#' {
		level++
	}
	if level == 0 || level < len(text) && text[level] != ' ' && text[level] != '\t' {
		return 0, "", false
	}
	content := strings.TrimSpace(text[level:])
	if closing := strings.LastIndex(content, " #"); closing >= 0 && strings.Trim(content[closing+1:], "#") == "" {
		content = strings.TrimSpace(content[:closing])
	}
	return level, content, true
}

func containsStringValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
