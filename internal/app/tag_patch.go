package app

import (
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// patchTagMetadata edits only proven single-line fields. It never serializes
// the document tree: unknown metadata and the body retain their exact bytes.
func patchTagMetadata(content, tags, updatedAt string) (string, error) {
	reject := func() (string, error) {
		return "", collaborationError("frontmatter_unsupported", "Tag metadata cannot be changed losslessly; repair it with the owner first")
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) || !strings.HasPrefix(content, "---\n") {
		return reject()
	}
	lines := strings.SplitAfter(content, "\n")
	end := -1
	bytes := 0
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---\n" || lines[i] == "---" {
			end = i
			break
		}
		if strings.HasPrefix(lines[i], "---") || strings.HasPrefix(lines[i], "...") {
			return reject()
		}
		bytes += len(lines[i])
		if bytes > 64*1024 {
			return reject()
		}
	}
	if end < 0 {
		return reject()
	}
	front := strings.Join(lines[1:end], "")
	if strings.ContainsAny(front, "\r\u0085\u2028\u2029") {
		return reject()
	}
	var doc yaml.Node
	if yaml.Unmarshal([]byte(front), &doc) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Style&yaml.FlowStyle != 0 {
		return reject()
	}
	budget := 4096
	if !safeTagYAML(&doc, 0, &budget) {
		return reject()
	}
	fields := map[string]string{"tags": tags, "updated_at": updatedAt}
	seen := map[string]bool{}
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		replacement, ok := fields[key.Value]
		if !ok {
			continue
		}
		if key.Tag != "!!str" || key.Style != 0 || key.Column != 1 || key.Line != value.Line || key.Line < 1 || key.Line >= end || value.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return reject()
		}
		var single yaml.Node
		if yaml.Unmarshal([]byte(lines[key.Line]), &single) != nil || len(single.Content) != 1 || single.Content[0].Kind != yaml.MappingNode || len(single.Content[0].Content) != 2 || !sameTagYAML(single.Content[0].Content[1], value) {
			return reject()
		}
		comment := ""
		if value.LineComment != "" {
			comment = " " + value.LineComment
		} else if key.LineComment != "" {
			comment = " " + key.LineComment
		}
		lines[key.Line] = key.Value + ": " + replacement + comment + "\n"
		seen[key.Value] = true
	}
	var additions strings.Builder
	for _, key := range []string{"tags", "updated_at"} {
		if !seen[key] {
			additions.WriteString(key + ": " + fields[key] + "\n")
		}
	}
	return strings.Join(lines[:end], "") + additions.String() + strings.Join(lines[end:], ""), nil
}

func safeTagYAML(node *yaml.Node, depth int, budget *int) bool {
	*budget--
	if *budget < 0 || depth > 32 || node.Anchor != "" || node.Kind == yaml.AliasNode || node.Tag == "!!merge" {
		return false
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return false
		}
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || seen[key.Value] {
				return false
			}
			seen[key.Value] = true
		}
	}
	for _, child := range node.Content {
		if !safeTagYAML(child, depth+1, budget) {
			return false
		}
	}
	return true
}

func sameTagYAML(a, b *yaml.Node) bool {
	if a.Kind != b.Kind || a.Tag != b.Tag || a.Value != b.Value || a.Style != b.Style || len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !sameTagYAML(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}
