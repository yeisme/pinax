package app

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type HTTPParameterLocation string

const (
	HTTPParameterPath   HTTPParameterLocation = "path"
	HTTPParameterQuery  HTTPParameterLocation = "query"
	HTTPParameterHeader HTTPParameterLocation = "header"
)

type HTTPParameterSchema struct {
	Name        string                `json:"name"`
	In          HTTPParameterLocation `json:"in"`
	Required    bool                  `json:"required"`
	Schema      map[string]any        `json:"schema"`
	Aliases     []string              `json:"aliases,omitempty"`
	Description string                `json:"description,omitempty"`
}

type HTTPRequestBodySchema struct {
	ContentType string         `json:"content_type"`
	SchemaID    string         `json:"schema_id"`
	Required    bool           `json:"required"`
	MaxBytes    int64          `json:"max_bytes"`
	Schema      map[string]any `json:"-"`
}

type RemoteRESTContract struct {
	RouteID             string                 `json:"route_id"`
	CapabilityID        string                 `json:"capability_id"`
	Command             string                 `json:"command"`
	Method              string                 `json:"method"`
	Path                string                 `json:"path"`
	Parameters          []HTTPParameterSchema  `json:"parameters"`
	RequestBody         *HTTPRequestBodySchema `json:"request_body,omitempty"`
	RequestSchema       string                 `json:"request_schema"`
	SuccessSchema       string                 `json:"success_schema"`
	SuccessDataSchema   string                 `json:"success_data_schema"`
	ErrorSchema         string                 `json:"error_schema"`
	ErrorDetailSchema   string                 `json:"error_detail_schema"`
	BodyExposureDefault string                 `json:"body_exposure_default"`
	ReleaseCore         bool                   `json:"release_core"`
	Readonly            bool                   `json:"readonly"`
	BodyAllowed         bool                   `json:"body_allowed"`
	ApprovalRequired    bool                   `json:"approval_required"`
	SnapshotRequired    bool                   `json:"snapshot_required"`
	UIGroup             string                 `json:"ui_group"`
	WriteGate           string                 `json:"write_gate"`
	Stability           string                 `json:"stability"`
}

type restWireSpec struct {
	Parameters []HTTPParameterSchema
	Body       *HTTPRequestBodySchema
}

var pathParameterPattern = regexp.MustCompile(`\{([^{}]+)\}`)

func RemoteRESTContracts() ([]RemoteRESTContract, error) {
	capabilities := make(map[string]domain.RemoteCapability)
	for _, capability := range RemoteCapabilities() {
		capabilities[capability.ID] = capability
	}
	specs := restWireSpecs()
	contracts := make([]RemoteRESTContract, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, route := range RemoteRoutes() {
		if route.Surface != "rest" || route.Path == "" {
			continue
		}
		spec, ok := specs[route.RouteID]
		if !ok {
			return nil, fmt.Errorf("REST route %q has no typed wire contract", route.RouteID)
		}
		capability, ok := capabilities[route.CapabilityID]
		if !ok {
			return nil, fmt.Errorf("REST route %q references unknown capability %q", route.RouteID, route.CapabilityID)
		}
		contract := RemoteRESTContract{
			RouteID:             route.RouteID,
			CapabilityID:        route.CapabilityID,
			Command:             capability.Command,
			Method:              route.Method,
			Path:                route.Path,
			Parameters:          cloneHTTPParameters(spec.Parameters),
			RequestBody:         cloneRequestBody(spec.Body),
			RequestSchema:       capability.RequestSchema,
			SuccessSchema:       catalogschema.ProjectionV1,
			SuccessDataSchema:   capability.ResponseSchema,
			ErrorSchema:         catalogschema.ProjectionV1,
			ErrorDetailSchema:   catalogschema.ErrorV1,
			BodyExposureDefault: capability.BodyExposureDefault,
			ReleaseCore:         capability.ReleaseCore,
			Readonly:            capability.Readonly,
			BodyAllowed:         capability.BodyAllowed,
			ApprovalRequired:    capability.ApprovalRequired,
			SnapshotRequired:    capability.SnapshotRequired,
			UIGroup:             capability.UIGroup,
			WriteGate:           capability.WriteGate,
			Stability:           "legacy",
		}
		if err := contract.Validate(); err != nil {
			return nil, err
		}
		contracts = append(contracts, contract)
		seen[route.RouteID] = true
	}
	for routeID := range specs {
		if !seen[routeID] {
			return nil, fmt.Errorf("typed wire contract %q has no registered REST route", routeID)
		}
	}
	return contracts, nil
}

func (c RemoteRESTContract) Validate() error {
	if strings.TrimSpace(c.RouteID) == "" || strings.TrimSpace(c.CapabilityID) == "" || strings.TrimSpace(c.Command) == "" {
		return fmt.Errorf("REST contract route_id, capability_id and command are required")
	}
	if strings.TrimSpace(c.Method) == "" || strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("REST contract %q method and path are required", c.RouteID)
	}
	if c.RequestSchema == "" || c.SuccessSchema == "" || c.SuccessDataSchema == "" || c.ErrorSchema == "" || c.ErrorDetailSchema == "" {
		return fmt.Errorf("REST contract %q schema refs are incomplete", c.RouteID)
	}
	if c.BodyExposureDefault == "" || c.WriteGate == "" || c.Stability == "" {
		return fmt.Errorf("REST contract %q capability metadata is incomplete", c.RouteID)
	}
	seen := make(map[string]bool, len(c.Parameters))
	pathParameters := make(map[string]bool)
	for _, parameter := range c.Parameters {
		if parameter.Name == "" {
			return fmt.Errorf("REST contract %q has unnamed parameter", c.RouteID)
		}
		switch parameter.In {
		case HTTPParameterPath, HTTPParameterQuery, HTTPParameterHeader:
		default:
			return fmt.Errorf("REST contract %q parameter %q has invalid location %q", c.RouteID, parameter.Name, parameter.In)
		}
		identity := string(parameter.In) + ":" + parameter.Name
		if seen[identity] {
			return fmt.Errorf("REST contract %q has duplicate parameter %q", c.RouteID, identity)
		}
		seen[identity] = true
		if parameter.Schema == nil || parameter.Schema["type"] == nil {
			return fmt.Errorf("REST contract %q parameter %q has no typed schema", c.RouteID, identity)
		}
		if parameter.In == HTTPParameterPath {
			if !parameter.Required {
				return fmt.Errorf("REST contract %q path parameter %q must be required", c.RouteID, parameter.Name)
			}
			pathParameters[parameter.Name] = true
		}
	}
	wantPathParameters := make(map[string]bool)
	for _, match := range pathParameterPattern.FindAllStringSubmatch(c.Path, -1) {
		wantPathParameters[match[1]] = true
	}
	if !equalStringSet(pathParameters, wantPathParameters) {
		return fmt.Errorf("REST contract %q path parameter metadata does not match %q", c.RouteID, c.Path)
	}
	if c.RequestBody != nil {
		if strings.ToUpper(c.Method) == "GET" {
			return fmt.Errorf("REST contract %q GET route cannot declare a request body", c.RouteID)
		}
		if c.RequestBody.ContentType == "" || c.RequestBody.SchemaID == "" || c.RequestBody.MaxBytes <= 0 || c.RequestBody.Schema == nil {
			return fmt.Errorf("REST contract %q request body metadata is incomplete", c.RouteID)
		}
	}
	return nil
}

func RemoteSchemaRegistry() (*catalogschema.Registry, error) {
	contracts, err := RemoteRESTContracts()
	if err != nil {
		return nil, err
	}
	registry := catalogschema.Default()
	definitions := make([]catalogschema.Definition, 0, len(contracts)+1)
	added := make(map[string]bool)
	for _, contract := range contracts {
		if contract.RequestBody != nil && !registry.Has(contract.RequestBody.SchemaID) && !added[contract.RequestBody.SchemaID] {
			definitions = append(definitions, catalogschema.Definition{ID: contract.RequestBody.SchemaID, Schema: contract.RequestBody.Schema})
			added[contract.RequestBody.SchemaID] = true
		}
		if registry.Has(contract.RequestSchema) || added[contract.RequestSchema] {
			continue
		}
		properties := make(map[string]any)
		required := make([]string, 0)
		for _, parameter := range contract.Parameters {
			properties[parameter.Name] = cloneSchemaMap(parameter.Schema)
			if parameter.Required {
				required = append(required, parameter.Name)
			}
		}
		if contract.RequestBody != nil {
			bodyProperties, _ := contract.RequestBody.Schema["properties"].(map[string]any)
			for name, property := range bodyProperties {
				if _, exists := properties[name]; !exists {
					properties[name] = cloneSchemaValue(property)
				}
			}
		}
		sort.Strings(required)
		document := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			document["required"] = required
		}
		definitions = append(definitions, catalogschema.Definition{ID: contract.RequestSchema, Schema: document})
		added[contract.RequestSchema] = true
	}
	if len(definitions) > 0 {
		registry, err = registry.With(definitions...)
		if err != nil {
			return nil, err
		}
	}
	for _, contract := range contracts {
		for _, schemaID := range []string{contract.RequestSchema, contract.SuccessSchema, contract.SuccessDataSchema, contract.ErrorSchema, contract.ErrorDetailSchema} {
			if !registry.Has(schemaID) {
				return nil, fmt.Errorf("REST contract %q references unregistered schema %q", contract.RouteID, schemaID)
			}
		}
		if contract.RequestBody != nil && !registry.Has(contract.RequestBody.SchemaID) {
			return nil, fmt.Errorf("REST contract %q references unregistered body schema %q", contract.RouteID, contract.RequestBody.SchemaID)
		}
	}
	return registry, nil
}

func restWireSpecs() map[string]restWireSpec {
	return map[string]restWireSpec{
		"rest.transport.manifest":        noWireInput(),
		"rest.connection.readiness":      noWireInput(),
		"rest.operation.show":            wire(pathString("operation_id")),
		"rest.operation.reconcile":       wire(pathString("operation_id")),
		"rest.workbench.status":          noWireInput(),
		"rest.workbench.activity.list":   wire(queryString("source"), queryString("query"), queryString("status"), queryString("object"), queryString("since"), queryString("until"), queryInteger("limit")),
		"rest.workbench.activity.show":   wire(pathString("event_id")),
		"rest.monitor.list":              wire(queryString("command"), queryString("query"), queryString("status"), queryString("since"), queryString("until"), queryInteger("limit")),
		"rest.monitor.show":              wire(pathString("run_id")),
		"rest.monitor.summary":           wire(queryString("command"), queryString("query"), queryString("status"), queryString("since"), queryString("until"), queryInteger("limit")),
		"rest.project.list":              noWireInput(),
		"rest.project.show":              wire(pathString("project")),
		"rest.project.board.show":        wire(pathString("slug"), queryString("subproject"), queryEnum("note_display", "card", "metadata", "body")),
		"rest.project.subproject.list":   wire(pathString("project")),
		"rest.project.subproject.show":   wire(pathString("project"), pathString("subproject")),
		"rest.project.subproject.create": wire(pathString("project"), queryString("subproject"), queryString("title"), queryString("template"), queryBoolean("dry_run"), queryBoolean("yes")),
		"rest.note.read":                 wire(pathString("ref"), queryEnum("display", "card", "metadata", "body")),
		"rest.project.item.show":         wire(pathString("ref")),
		"rest.project.item.plan":         wire(pathString("ref"), pathString("action"), queryString("column"), queryBoolean("yes")),
		"rest.task.adopt.plan":           wire(pathString("item")),
		"rest.database.view.render":      wire(pathString("name")),
		"rest.graph.summary":             noWireInput(),
		"rest.memory.list":               wire(queryString("type"), queryString("entity"), queryBooleanAliases("include_draft", "include-draft"), queryBooleanAliases("include_superseded", "include-superseded"), queryBooleanAliases("include_expired", "include-expired"), queryBooleanAliases("include_rejected", "include-rejected"), queryInteger("limit")),
		"rest.memory.capture":            memoryCaptureWireSpec(),
		"rest.memory.recall":             wire(queryString("query"), queryString("entity"), queryString("type"), queryInteger("limit")),
		"rest.memory.context":            wire(queryStringAliases("task", "query"), queryString("entity"), queryString("type"), queryInteger("limit")),
		"rest.memory.stats":              noWireInput(),
		"rest.folder.list":               wire(queryString("purpose"), queryString("under"), queryBoolean("include_empty"), queryInteger("depth")),
		"rest.folder.show":               wire(pathString("path")),
		"rest.folder.create":             wire(queryString("path"), queryString("purpose"), queryBoolean("dry_run"), queryBoolean("yes")),
		"rest.folder.rename":             wire(pathString("path"), queryString("target_path"), queryString("expected_revision"), queryBoolean("dry_run"), queryBoolean("yes"), headerString("X-Pinax-Operation-ID"), headerString("Idempotency-Key")),
		"rest.folder.move":               wire(pathString("path"), queryString("target_parent"), queryBoolean("dry_run"), queryBoolean("yes")),
		"rest.folder.delete":             wire(pathString("path"), queryBoolean("empty_only"), queryBoolean("dry_run"), queryBoolean("yes")),
		"rest.folder.adopt":              wire(pathString("path"), queryString("purpose"), queryBoolean("dry_run"), queryBoolean("yes")),
		"rest.folder.repair":             noWireInput(),
		"rest.inbox.list":                noWireInput(),
		"rest.inbox.show":                wire(pathString("ref")),
		"rest.inbox.capture":             wire(queryString("title"), queryString("body"), queryString("tags"), queryString("slug"), queryBoolean("dry_run"), queryBoolean("yes"), headerString("X-Pinax-Operation-ID"), headerString("Idempotency-Key")),
		"rest.inbox.promote":             wire(pathString("ref"), queryString("to"), queryString("group"), queryString("folder"), queryString("kind"), requiredQueryBoolean("yes"), queryBoolean("dry_run")),
		"rest.inbox.discard":             wire(pathString("ref"), requiredQueryBoolean("yes"), queryBoolean("dry_run")),
		"rest.draft.list":                noWireInput(),
		"rest.draft.show":                wire(pathString("ref")),
		"rest.draft.create":              wire(queryString("title"), queryString("body"), requiredQueryBoolean("yes")),
		"rest.draft.promote":             wire(pathString("ref"), queryString("status"), queryString("folder"), queryString("kind"), requiredQueryBoolean("yes"), queryBoolean("dry_run")),
		"rest.draft.archive":             wire(pathString("ref"), requiredQueryBoolean("yes"), queryBoolean("dry_run")),
		"rest.draft.discard":             wire(pathString("ref"), requiredQueryBoolean("yes"), queryBoolean("dry_run")),
	}
}

func memoryCaptureWireSpec() restWireSpec {
	bodySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type":       stringSchema(),
			"subject":    stringSchema(),
			"predicate":  stringSchema(),
			"object":     stringSchema(),
			"body":       stringSchema(),
			"status":     stringSchema(),
			"confidence": stringSchema(),
			"source":     stringSchema(),
			"sourceSpan": stringSchema(),
			"entities":   arraySchema(stringSchema()),
		},
		"additionalProperties": false,
	}
	return restWireSpec{
		Parameters: []HTTPParameterSchema{
			queryString("type"), queryString("subject"), queryString("predicate"), queryString("object"), queryString("body"), queryString("status"), queryString("confidence"), queryString("source"), queryStringAliases("source_span", "source-span"), queryStringAliases("entity", "entities"), queryBooleanAliases("dry_run", "dry-run"), queryBoolean("yes"),
		},
		Body: &HTTPRequestBodySchema{ContentType: "application/json", SchemaID: "pinax.memory.capture.body.v1", Required: false, MaxBytes: 1 << 20, Schema: bodySchema},
	}
}

func noWireInput() restWireSpec {
	return restWireSpec{Parameters: []HTTPParameterSchema{}}
}

func wire(parameters ...HTTPParameterSchema) restWireSpec {
	return restWireSpec{Parameters: parameters}
}

func pathString(name string) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterPath, Required: true, Schema: stringSchema()}
}

func queryString(name string) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterQuery, Schema: stringSchema()}
}

func headerString(name string) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterHeader, Schema: stringSchema()}
}

func queryStringAliases(name string, aliases ...string) HTTPParameterSchema {
	parameter := queryString(name)
	parameter.Aliases = append([]string(nil), aliases...)
	return parameter
}

func queryBoolean(name string) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterQuery, Schema: boolSchema()}
}

func requiredQueryBoolean(name string) HTTPParameterSchema {
	parameter := queryBoolean(name)
	parameter.Required = true
	return parameter
}

func queryBooleanAliases(name string, aliases ...string) HTTPParameterSchema {
	parameter := queryBoolean(name)
	parameter.Aliases = append([]string(nil), aliases...)
	return parameter
}

func queryInteger(name string) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterQuery, Schema: map[string]any{"type": "integer"}}
}

func queryEnum(name string, values ...string) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterQuery, Schema: map[string]any{"type": "string", "enum": append([]string(nil), values...)}}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func boolSchema() map[string]any {
	return map[string]any{"type": "boolean"}
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func cloneHTTPParameters(parameters []HTTPParameterSchema) []HTTPParameterSchema {
	cloned := make([]HTTPParameterSchema, len(parameters))
	for index, parameter := range parameters {
		cloned[index] = parameter
		cloned[index].Schema = cloneSchemaMap(parameter.Schema)
		cloned[index].Aliases = append([]string(nil), parameter.Aliases...)
	}
	return cloned
}

func cloneRequestBody(body *HTTPRequestBodySchema) *HTTPRequestBodySchema {
	if body == nil {
		return nil
	}
	copy := *body
	copy.Schema = cloneSchemaMap(body.Schema)
	return &copy
}

func cloneSchemaMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return cloneSchemaValue(value).(map[string]any)
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, item := range typed {
			copy[key] = cloneSchemaValue(item)
		}
		return copy
	case []any:
		copy := make([]any, len(typed))
		for index, item := range typed {
			copy[index] = cloneSchemaValue(item)
		}
		return copy
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func equalStringSet(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}
