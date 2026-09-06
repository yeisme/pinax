package mcpserver

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
)

// supportedSchemaKeywords is the locked JSON Schema subset that the
// hand-written runtime validator implements. Assertion keywords are enforced
// by matchesToolArgumentSchema; annotation keywords are accepted in
// descriptors but carry no runtime enforcement. The set was locked by
// scanning the shared transport catalog schema descriptors — any other
// keyword must fail closed through validateSchemaKeywordSupport before a
// tool is published.
var supportedSchemaKeywords = map[string]bool{
	// Assertion keywords enforced by the runtime validator.
	"type": true, "properties": true, "required": true, "additionalProperties": true,
	"items": true, "enum": true, "const": true, "pattern": true,
	"minLength": true, "maxLength": true, "minimum": true, "maximum": true,
	"exclusiveMinimum": true, "exclusiveMaximum": true, "minItems": true, "maxItems": true,
	// Annotation keywords accepted in descriptors, never asserted.
	"description": true, "title": true, "default": true, "examples": true, "format": true,
	// Document identity keywords, allowed at the schema root only.
	"$schema": true, "$id": true,
}

// validateSchemaKeywordSupport rejects descriptors that use keywords outside
// the supported subset so a schema can never promise validation the runtime
// does not perform.
func validateSchemaKeywordSupport(document map[string]any) error {
	if document == nil {
		return fmt.Errorf("schema document is required")
	}
	return validateSchemaNodeKeywords("$", document, true)
}

func validateSchemaNodeKeywords(path string, node map[string]any, root bool) error {
	for _, key := range sortedSchemaKeys(node) {
		if !supportedSchemaKeywords[key] {
			return fmt.Errorf("unsupported schema keyword %q at %s", key, path)
		}
		if key == "$schema" || key == "$id" {
			if !root {
				return fmt.Errorf("keyword %q is only allowed at the schema root (found at %s)", key, path)
			}
			continue
		}
		switch key {
		case "properties":
			properties, _ := node[key].(map[string]any)
			for _, name := range sortedSchemaKeys(properties) {
				child, _ := properties[name].(map[string]any)
				if child == nil {
					return fmt.Errorf("property %q at %s must be a schema object", name, path)
				}
				if err := validateSchemaNodeKeywords(path+"/"+name, child, false); err != nil {
					return err
				}
			}
		case "items":
			child, _ := node[key].(map[string]any)
			if child == nil {
				return fmt.Errorf("items at %s must be a schema object", path)
			}
			if err := validateSchemaNodeKeywords(path+"/items", child, false); err != nil {
				return err
			}
		case "additionalProperties":
			switch typed := node[key].(type) {
			case bool:
			case map[string]any:
				if err := validateSchemaNodeKeywords(path+"/additionalProperties", typed, false); err != nil {
					return err
				}
			default:
				return fmt.Errorf("additionalProperties at %s must be a boolean or schema object", path)
			}
		default:
			if err := validateSchemaKeywordShape(path, key, node[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSchemaKeywordShape rejects leaf keywords whose value cannot be
// interpreted by the runtime validator, so a malformed bound can never be
// silently treated as absent.
func validateSchemaKeywordShape(path, key string, value any) error {
	switch key {
	case "type", "pattern", "description", "title", "format":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("keyword %q at %s must be a string", key, path)
		}
	case "minLength", "maxLength", "minItems", "maxItems",
		"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
		if _, ok := numericValue(value); !ok {
			return fmt.Errorf("keyword %q at %s must be a number", key, path)
		}
	case "enum":
		if values := schemaEnumList(value); len(values) == 0 {
			return fmt.Errorf("keyword %q at %s must be a non-empty array", key, path)
		}
	case "required":
		if !schemaAllStrings(value) {
			return fmt.Errorf("keyword %q at %s must be an array of strings", key, path)
		}
	}
	return nil
}

func sortedSchemaKeys(node map[string]any) []string {
	keys := make([]string, 0, len(node))
	for key := range node {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// schemaAllStrings reports whether value is a list whose members are all
// strings. schemaStringList silently drops non-string members, so the guard
// must reject them at publication or a malformed required list would
// silently lose entries at validation time.
func schemaAllStrings(value any) bool {
	switch typed := value.(type) {
	case []string:
		return true
	case []any:
		for _, item := range typed {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// matchesToolArgumentSchema reports whether value satisfies the locked JSON
// Schema subset. An empty schema asserts nothing, matching draft 2020-12.
func matchesToolArgumentSchema(value any, schema map[string]any) bool {
	if len(schema) == 0 {
		return true
	}
	if !matchesEnumOrConst(value, schema) {
		return false
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "":
		return true
	case "string":
		return matchesStringSchema(value, schema)
	case "integer":
		return matchesNumberSchema(value, schema, true)
	case "number":
		return matchesNumberSchema(value, schema, false)
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		return matchesArraySchema(value, schema)
	case "object":
		return matchesObjectSchema(value, schema)
	default:
		return false
	}
}

func matchesStringSchema(value any, schema map[string]any) bool {
	stringValue, ok := value.(string)
	if !ok {
		return false
	}
	if pattern, ok := schema["pattern"].(string); ok && pattern != "" {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		if !expression.MatchString(stringValue) {
			return false
		}
	}
	length := len([]rune(stringValue))
	if minLength, ok := schemaBound(schema, "minLength"); ok && float64(length) < minLength {
		return false
	}
	if maxLength, ok := schemaBound(schema, "maxLength"); ok && float64(length) > maxLength {
		return false
	}
	return true
}

func matchesNumberSchema(value any, schema map[string]any, integral bool) bool {
	number, ok := numericValue(value)
	if !ok {
		return false
	}
	if integral && number != math.Trunc(number) {
		return false
	}
	if bound, ok := schemaBound(schema, "minimum"); ok && number < bound {
		return false
	}
	if bound, ok := schemaBound(schema, "maximum"); ok && number > bound {
		return false
	}
	if bound, ok := schemaBound(schema, "exclusiveMinimum"); ok && number <= bound {
		return false
	}
	if bound, ok := schemaBound(schema, "exclusiveMaximum"); ok && number >= bound {
		return false
	}
	return true
}

func matchesArraySchema(value any, schema map[string]any) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	if minItems, ok := schemaBound(schema, "minItems"); ok && float64(len(values)) < minItems {
		return false
	}
	if maxItems, ok := schemaBound(schema, "maxItems"); ok && float64(len(values)) > maxItems {
		return false
	}
	itemSchema, _ := schema["items"].(map[string]any)
	for _, item := range values {
		if !matchesToolArgumentSchema(item, itemSchema) {
			return false
		}
	}
	return true
}

func matchesObjectSchema(value any, schema map[string]any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, required := range schemaStringList(schema["required"]) {
		if _, present := object[required]; !present {
			return false
		}
	}
	for key, rawSchema := range properties {
		propertySchema, _ := rawSchema.(map[string]any)
		if fieldValue, present := object[key]; present {
			if !matchesToolArgumentSchema(fieldValue, propertySchema) {
				return false
			}
		}
	}
	switch additional := schema["additionalProperties"].(type) {
	case bool:
		if !additional {
			for key := range object {
				if _, known := properties[key]; !known {
					return false
				}
			}
		}
	case map[string]any:
		for key, fieldValue := range object {
			if _, known := properties[key]; known {
				continue
			}
			if !matchesToolArgumentSchema(fieldValue, additional) {
				return false
			}
		}
	}
	return true
}

func matchesEnumOrConst(value any, schema map[string]any) bool {
	if enumValues := schemaEnumList(schema["enum"]); len(enumValues) > 0 {
		matched := false
		for _, candidate := range enumValues {
			if reflect.DeepEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if constValue, present := schema["const"]; present {
		if !reflect.DeepEqual(value, constValue) {
			return false
		}
	}
	return true
}

func schemaEnumList(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		values := make([]any, len(typed))
		for index, item := range typed {
			values[index] = item
		}
		return values
	default:
		return nil
	}
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func schemaBound(schema map[string]any, key string) (float64, bool) {
	switch typed := schema[key].(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		// Malformed bounds never reach the runtime: validateSchemaKeywordShape
		// rejects them at publication time, so absence here is trusted.
		return 0, false
	}
}
