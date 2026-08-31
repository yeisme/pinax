package promptrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

type templatePlaceholder struct {
	start int
	end   int
	name  string
}

// RenderTemplate strictly substitutes declared placeholders in caller-provided
// memory. It neither reads sources nor performs provider, state, or usage work.
func RenderTemplate(request RenderRequest) (RenderResult, error) {
	if err := ValidateTemplateInputs(request.Inputs); err != nil {
		return RenderResult{}, err
	}
	validation := ValidateInputValues(request.Inputs, request.Values)
	result := RenderResult{Inputs: validation.Inputs, Ready: validation.Ready, Issues: append([]InputIssue{}, validation.Issues...)}
	if !result.Ready {
		return result, nil
	}
	placeholders, valid := scanTemplatePlaceholders(request.Template)
	if !valid {
		result.Issues = append(result.Issues, InputIssue{Code: CodeTemplateSyntax, Message: "template placeholder syntax is invalid"})
		result.Ready = false
		return result, nil
	}
	for _, placeholder := range placeholders {
		if _, ok := validation.Value(placeholder.name); !ok {
			result.Issues = append(result.Issues, InputIssue{Code: CodeTemplatePlaceholder, Field: placeholder.name, Message: "template placeholder is not declared or ready"})
		}
	}
	if len(result.Issues) != 0 {
		result.Ready = false
		return result, nil
	}
	var builder strings.Builder
	for index, placeholder := range placeholders {
		previous := 0
		if index > 0 {
			previous = placeholders[index-1].end
		}
		builder.WriteString(request.Template[previous:placeholder.start])
		value, _ := validation.Value(placeholder.name)
		builder.WriteString(fmt.Sprint(value))
		if index == len(placeholders)-1 {
			builder.WriteString(request.Template[placeholder.end:])
		}
	}
	rendered := builder.String()
	if len(placeholders) == 0 {
		rendered = request.Template
	}
	digest := sha256.Sum256([]byte(rendered))
	result.RenderedBody = rendered
	result.RenderedDigest = "sha256:" + hex.EncodeToString(digest[:])
	result.RenderedBytes = len(rendered)
	result.RenderedRunes = utf8.RuneCountInString(rendered)
	return result, nil
}

func scanTemplatePlaceholders(body string) ([]templatePlaceholder, bool) {
	placeholders := make([]templatePlaceholder, 0)
	for index := 0; index < len(body); {
		switch body[index] {
		case '{':
			if index+1 == len(body) || body[index+1] != '{' {
				index++
				continue
			}
			nameStart := index + 2
			if nameStart >= len(body) || !isPlaceholderNameStart(body[nameStart]) {
				return nil, false
			}
			cursor := nameStart + 1
			for cursor < len(body) && isPlaceholderNamePart(body[cursor]) {
				cursor++
			}
			if cursor+1 >= len(body) || body[cursor] != '}' || body[cursor+1] != '}' || cursor+2 < len(body) && body[cursor+2] == '}' {
				return nil, false
			}
			placeholders = append(placeholders, templatePlaceholder{start: index, end: cursor + 2, name: body[nameStart:cursor]})
			index = cursor + 2
		case '}':
			if index+1 < len(body) && body[index+1] == '}' {
				return nil, false
			}
			index++
		default:
			index++
		}
	}
	return placeholders, true
}

func isPlaceholderNameStart(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isPlaceholderNamePart(value byte) bool {
	return isPlaceholderNameStart(value) || value >= '0' && value <= '9' || value == '_'
}
