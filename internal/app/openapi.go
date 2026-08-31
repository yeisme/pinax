package app

import (
	"fmt"
	"strings"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func BuildOpenAPI() (map[string]any, error) {
	contracts, err := RemoteRESTContracts()
	if err != nil {
		return nil, err
	}
	registry, err := RemoteSchemaRegistry()
	if err != nil {
		return nil, err
	}
	components := openAPIComponents(registry)
	paths := make(map[string]any)
	for _, contract := range contracts {
		pathItem, _ := paths[contract.Path].(map[string]any)
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[contract.Path] = pathItem
		}
		method := strings.ToLower(contract.Method)
		if _, exists := pathItem[method]; exists {
			return nil, fmt.Errorf("duplicate OpenAPI operation %s %s", contract.Method, contract.Path)
		}
		pathItem[method] = openAPIOperation(contract)
	}
	document := map[string]any{
		"openapi":           "3.1.0",
		"jsonSchemaDialect": catalogschema.Draft202012,
		"info": map[string]any{
			"title":   "Pinax Local API",
			"version": "v1",
		},
		"paths":      paths,
		"components": components,
	}
	if err := ValidateOpenAPI(document); err != nil {
		return nil, err
	}
	return document, nil
}

func openAPIComponents(registry *catalogschema.Registry) map[string]any {
	snapshot := registry.Snapshot()
	schemas := make(map[string]any, len(snapshot.Schemas))
	for id, document := range snapshot.Schemas {
		schemas[id] = openAPISchemaValue(document)
	}
	return map[string]any{
		"schemas": schemas,
		"securitySchemes": map[string]any{
			"BearerAuth": map[string]any{
				"type":         "http",
				"scheme":       "bearer",
				"bearerFormat": "Pinax token",
				"description":  "Required when the local API server enables token authentication.",
			},
		},
	}
}

func openAPIOperation(contract RemoteRESTContract) map[string]any {
	parameters := make([]map[string]any, 0, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		entry := map[string]any{
			"name":     parameter.Name,
			"in":       string(parameter.In),
			"required": parameter.Required,
			"schema":   openAPISchemaValue(parameter.Schema),
		}
		if parameter.Description != "" {
			entry["description"] = parameter.Description
		}
		if len(parameter.Aliases) > 0 {
			entry["x-pinax-aliases"] = append([]string(nil), parameter.Aliases...)
		}
		parameters = append(parameters, entry)
	}
	operation := map[string]any{
		"operationId":               contract.RouteID,
		"parameters":                parameters,
		"security":                  []map[string]any{{"BearerAuth": []string{}}},
		"responses":                 openAPIResponses(contract),
		"x-pinax-command":           contract.Command,
		"x-pinax-capability":        contract.CapabilityID,
		"x-pinax-capability-id":     contract.CapabilityID,
		"x-pinax-release-core":      contract.ReleaseCore,
		"x-pinax-readonly":          contract.Readonly,
		"x-pinax-body-allowed":      contract.BodyAllowed,
		"x-pinax-approval-required": contract.ApprovalRequired,
		"x-pinax-snapshot-required": contract.SnapshotRequired,
		"x-pinax-ui-group":          contract.UIGroup,
		"x-pinax-body-exposure":     contract.BodyExposureDefault,
		"x-pinax-write-gate":        contract.WriteGate,
		"x-pinax-stability":         contract.Stability,
		"x-pinax-request-schema":    componentRef(contract.RequestSchema),
		"x-pinax-readiness": map[string]any{
			"status":     "degraded",
			"maturity":   "exploratory",
			"blockers":   []string{"runtime_readiness_not_evaluated"},
			"schema_ref": componentRef(catalogschema.ReadinessV1),
		},
	}
	if contract.RequestBody != nil {
		operation["requestBody"] = map[string]any{
			"required": contract.RequestBody.Required,
			"content": map[string]any{
				contract.RequestBody.ContentType: map[string]any{
					"schema": map[string]any{"$ref": componentRef(contract.RequestBody.SchemaID)},
				},
			},
			"x-pinax-max-bytes": contract.RequestBody.MaxBytes,
		}
	}
	return operation
}

func openAPIResponses(contract RemoteRESTContract) map[string]any {
	return map[string]any{
		"200": map[string]any{
			"description": "Pinax projection response.",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": componentRef(contract.SuccessSchema)},
				},
			},
			"x-pinax-data-schema": componentRef(contract.SuccessDataSchema),
		},
		"default": map[string]any{
			"description": "Pinax error projection.",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": componentRef(contract.ErrorSchema)},
				},
			},
			"x-pinax-error-detail-schema": componentRef(contract.ErrorDetailSchema),
		},
	}
}

func componentRef(id string) string {
	return "#/components/schemas/" + id
}

func openAPISchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "$ref" {
				if ref, ok := item.(string); ok && strings.HasPrefix(ref, "pinax://schema/") {
					copy[key] = componentRef(strings.TrimPrefix(ref, "pinax://schema/"))
					continue
				}
			}
			copy[key] = openAPISchemaValue(item)
		}
		return copy
	case []any:
		copy := make([]any, len(typed))
		for index, item := range typed {
			copy[index] = openAPISchemaValue(item)
		}
		return copy
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}
