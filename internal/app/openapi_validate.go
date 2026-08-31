package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var openAPIMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

func ValidateOpenAPI(document map[string]any) error {
	if document["openapi"] != "3.1.0" {
		return fmt.Errorf("unsupported OpenAPI version %q", document["openapi"])
	}
	components, ok := document["components"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI components are required")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok || len(schemas) == 0 {
		return fmt.Errorf("OpenAPI component schemas are required")
	}
	securitySchemes, ok := components["securitySchemes"].(map[string]any)
	if !ok || len(securitySchemes) == 0 {
		return fmt.Errorf("OpenAPI security schemes are required")
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		return fmt.Errorf("OpenAPI paths are required")
	}
	operationIDs := make(map[string]string)
	pathNames := make([]string, 0, len(paths))
	for path := range paths {
		pathNames = append(pathNames, path)
	}
	sort.Strings(pathNames)
	for _, path := range pathNames {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			return fmt.Errorf("OpenAPI path %q must be an object", path)
		}
		for method, rawOperation := range pathItem {
			if !openAPIMethods[method] {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				return fmt.Errorf("OpenAPI operation %s %s must be an object", strings.ToUpper(method), path)
			}
			operationID, _ := operation["operationId"].(string)
			if operationID == "" {
				return fmt.Errorf("OpenAPI operation %s %s has no operationId", strings.ToUpper(method), path)
			}
			identity := strings.ToUpper(method) + " " + path
			if owner, exists := operationIDs[operationID]; exists {
				return fmt.Errorf("duplicate OpenAPI operationId %q for %s and %s", operationID, owner, identity)
			}
			operationIDs[operationID] = identity
			if err := validateOpenAPIParameters(path, operationID, operation["parameters"]); err != nil {
				return err
			}
			responses, ok := operation["responses"].(map[string]any)
			if !ok || responses["200"] == nil || responses["default"] == nil {
				return fmt.Errorf("OpenAPI operation %q must define 200 and default responses", operationID)
			}
			if err := validateOpenAPISecurity(operationID, operation["security"], securitySchemes); err != nil {
				return err
			}
		}
	}
	return validateOpenAPIRefs(document, schemas)
}

func OpenAPIDigest(document map[string]any) (string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode OpenAPI document: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateOpenAPIParameters(path, operationID string, value any) error {
	parameters, err := asAnySlice(value)
	if err != nil {
		return fmt.Errorf("OpenAPI operation %q parameters: %w", operationID, err)
	}
	pathParameters := make(map[string]bool)
	seen := make(map[string]bool)
	for _, rawParameter := range parameters {
		parameter, ok := rawParameter.(map[string]any)
		if !ok {
			return fmt.Errorf("OpenAPI operation %q contains a non-object parameter", operationID)
		}
		name, _ := parameter["name"].(string)
		location, _ := parameter["in"].(string)
		if name == "" || location == "" {
			return fmt.Errorf("OpenAPI operation %q contains an unnamed parameter", operationID)
		}
		identity := location + ":" + name
		if seen[identity] {
			return fmt.Errorf("OpenAPI operation %q contains duplicate parameter %q", operationID, identity)
		}
		seen[identity] = true
		if _, ok := parameter["schema"].(map[string]any); !ok {
			return fmt.Errorf("OpenAPI operation %q parameter %q has no schema", operationID, identity)
		}
		if location == "path" {
			if parameter["required"] != true {
				return fmt.Errorf("OpenAPI operation %q path parameter %q is not required", operationID, name)
			}
			pathParameters[name] = true
		}
	}
	want := make(map[string]bool)
	for _, match := range pathParameterPattern.FindAllStringSubmatch(path, -1) {
		want[match[1]] = true
	}
	if !equalStringSet(pathParameters, want) {
		return fmt.Errorf("OpenAPI operation %q path parameters do not match %q", operationID, path)
	}
	return nil
}

func validateOpenAPISecurity(operationID string, value any, schemes map[string]any) error {
	requirements, err := asAnySlice(value)
	if err != nil || len(requirements) == 0 {
		return fmt.Errorf("OpenAPI operation %q has no security requirements", operationID)
	}
	for _, rawRequirement := range requirements {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok {
			return fmt.Errorf("OpenAPI operation %q has invalid security requirement", operationID)
		}
		for name := range requirement {
			if _, exists := schemes[name]; !exists {
				return fmt.Errorf("OpenAPI operation %q references unknown security scheme %q", operationID, name)
			}
		}
	}
	return nil
}

func validateOpenAPIRefs(value any, schemas map[string]any) error {
	switch typed := value.(type) {
	case map[string]any:
		if rawRef, ok := typed["$ref"]; ok {
			ref, ok := rawRef.(string)
			if !ok || !strings.HasPrefix(ref, "#/components/schemas/") {
				return fmt.Errorf("OpenAPI contains unsupported $ref %q", rawRef)
			}
			id := strings.TrimPrefix(ref, "#/components/schemas/")
			if _, exists := schemas[id]; !exists {
				return fmt.Errorf("OpenAPI contains dangling schema ref %q", ref)
			}
		}
		for _, item := range typed {
			if err := validateOpenAPIRefs(item, schemas); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := validateOpenAPIRefs(item, schemas); err != nil {
				return err
			}
		}
	case []map[string]any:
		for _, item := range typed {
			if err := validateOpenAPIRefs(item, schemas); err != nil {
				return err
			}
		}
	}
	return nil
}

func asAnySlice(value any) ([]any, error) {
	switch typed := value.(type) {
	case []any:
		return typed, nil
	case []map[string]any:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = item
		}
		return items, nil
	default:
		return nil, fmt.Errorf("must be an array")
	}
}
