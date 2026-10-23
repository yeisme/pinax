package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// ValidatePayload checks a decoded JSON value against a registered reader
// schema using the keyword subset the frozen contracts use: type (incl.
// type arrays), properties, required, additionalProperties:false, items,
// enum, const, minLength/maxLength, minimum/maximum, uniqueItems, pattern
// oneOf and $ref. Real HTTP payloads are validated through this gate so the
// generated consumer types never drift from the served wire format.
func ValidatePayload(registry *Registry, schemaID string, value any) error {
	schema, ok := registry.Lookup(schemaID)
	if !ok {
		return fmt.Errorf("schema %q is not registered", schemaID)
	}
	return validateValue(registry, schema, value, "$")
}

func validateValue(registry *Registry, schema any, value any, path string) error {
	node, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := node["$ref"].(string); ok {
		target := strings.TrimPrefix(ref, "pinax://schema/")
		resolved, found := registry.Lookup(target)
		if !found {
			return fmt.Errorf("%s: dangling reference %q", path, ref)
		}
		return validateValue(registry, resolved, value, path)
	}
	if branches, present := node["oneOf"]; present {
		choices, ok := branches.([]any)
		if !ok || len(choices) == 0 {
			return fmt.Errorf("%s: invalid oneOf schema", path)
		}
		matches := 0
		for _, choice := range choices {
			if _, ok := choice.(map[string]any); !ok {
				return fmt.Errorf("%s: invalid oneOf branch", path)
			}
			if validateValue(registry, choice, value, path) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s: oneOf matched %d branches, expected one", path, matches)
		}
	}
	if allowed := stringList(node["enum"]); allowed != nil {
		if !containsValue(allowed, value) {
			return fmt.Errorf("%s: %v is not one of %v", path, value, allowed)
		}
	}
	if expected, ok := node["const"]; ok {
		if !constEqual(expected, value) {
			return fmt.Errorf("%s: %v != const %v", path, value, expected)
		}
	}
	if declared, ok := node["type"]; ok {
		if err := validateType(declared, value, path); err != nil {
			return err
		}
	}
	if pattern, ok := node["pattern"].(string); ok {
		if text, isString := value.(string); isString {
			if matched, err := regexp.MatchString(pattern, text); err == nil && !matched {
				return fmt.Errorf("%s: %q does not match %q", path, text, pattern)
			}
		}
	}
	switch typed := value.(type) {
	case string:
		if min, ok := asInt(node["minLength"]); ok && len([]rune(typed)) < min {
			return fmt.Errorf("%s: shorter than minLength %d", path, min)
		}
		if max, ok := asInt(node["maxLength"]); ok && len([]rune(typed)) > max {
			return fmt.Errorf("%s: longer than maxLength %d", path, max)
		}
	case float64:
		if min, ok := asInt(node["minimum"]); ok && typed < float64(min) {
			return fmt.Errorf("%s: below minimum %d", path, min)
		}
		if max, ok := asInt(node["maximum"]); ok && typed > float64(max) {
			return fmt.Errorf("%s: above maximum %d", path, max)
		}
	case []any:
		if max, ok := asInt(node["maxItems"]); ok && len(typed) > max {
			return fmt.Errorf("%s: more than maxItems %d", path, max)
		}
		if min, ok := asInt(node["minItems"]); ok && len(typed) < min {
			return fmt.Errorf("%s: fewer than minItems %d", path, min)
		}
		if unique, _ := node["uniqueItems"].(bool); unique {
			seen := make(map[string]struct{}, len(typed))
			for _, item := range typed {
				key, _ := json.Marshal(item)
				if _, exists := seen[string(key)]; exists {
					return fmt.Errorf("%s: items are not unique", path)
				}
				seen[string(key)] = struct{}{}
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			for index, item := range typed {
				if err := validateValue(registry, items, item, fmt.Sprintf("%s/%d", path, index)); err != nil {
					return err
				}
			}
		}
	case map[string]any:
		properties, _ := node["properties"].(map[string]any)
		if isClosedObject(node) {
			for key := range typed {
				if _, known := properties[key]; !known {
					return fmt.Errorf("%s: unknown property %q", path, key)
				}
			}
		}
		if required := stringList(node["required"]); required != nil {
			for _, raw := range required {
				name, _ := raw.(string)
				if _, present := typed[name]; !present {
					return fmt.Errorf("%s: missing required property %q", path, name)
				}
			}
		}
		for key, property := range properties {
			if child, present := typed[key]; present {
				if err := validateValue(registry, property, child, path+"/"+key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// isClosedObject reports whether additionalProperties forbids unknown keys.
// Schema constructors store the literal false; decoded documents keep bools.
func isClosedObject(node map[string]any) bool {
	additional, declared := node["additionalProperties"]
	if !declared {
		return false
	}
	allow, isBool := additional.(bool)
	return isBool && !allow
}

func validateType(declared any, value any, path string) error {
	allowed := map[string]bool{}
	switch typed := declared.(type) {
	case string:
		allowed[typed] = true
	case []any:
		for _, raw := range typed {
			if name, ok := raw.(string); ok {
				allowed[name] = true
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	actual := jsonTypeOf(value)
	if actual == "integer" {
		if allowed["integer"] {
			return nil
		}
		if allowed["number"] && !math.IsNaN(value.(float64)) {
			return nil
		}
	}
	if allowed[actual] {
		return nil
	}
	return fmt.Errorf("%s: %s is not of type %v", path, actual, declared)
}

func jsonTypeOf(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if isIntegral(typed) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func isIntegral(value float64) bool {
	return value == math.Trunc(value)
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		if isIntegral(typed) {
			return int(typed), true
		}
	}
	return 0, false
}

func containsValue(values []any, target any) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// stringList normalizes enum/required slices that Go constructors build as
// []string while decoded JSON documents carry []any.
func stringList(value any) []any {
	switch typed := value.(type) {
	case []string:
		converted := make([]any, len(typed))
		for index, item := range typed {
			converted[index] = item
		}
		return converted
	case []any:
		return typed
	default:
		return nil
	}
}

// constEqual compares schema consts across Go int literals and decoded
// float64 JSON values without weakening type separation.
func constEqual(expected, actual any) bool {
	switch typed := expected.(type) {
	case int:
		number, ok := actual.(float64)
		return ok && number == float64(typed)
	case float64:
		number, ok := actual.(float64)
		return ok && number == typed
	default:
		return expected == actual
	}
}
