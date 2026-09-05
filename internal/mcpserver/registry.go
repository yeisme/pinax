package mcpserver

import (
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/transportcatalog"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
	"github.com/yeisme/pinax/internal/transportmanifest"
)

type toolRegistration struct {
	Tool
	CapabilityID  string
	RequestSchema string
	OutputSchema  string
}

type resourceRegistration struct {
	Resource
	CapabilityID string
}

func toolRegistrations() []toolRegistration {
	return []toolRegistration{
		registeredTool("pinax.search", "Search local notes", "note.search", "pinax.note.search.request.v1"),
		registeredTool("pinax.brain.context", "Read bounded Agent Brain context bundle", "brain.context.bundle", "pinax.agent_brain.context_bundle.request.v1"),
		registeredTool("pinax.brain.answer", "Preview a citation-first bounded answer", "brain.answer.preview", "pinax.agent_brain.answer.request.v1"),
		registeredTool("pinax.brain.sources", "List bounded Agent Brain evidence sources", "brain.sources.list", "pinax.agent_brain.sources.request.v1"),
		registeredTool("pinax.brain.maintenance_plan", "Preview Agent Brain maintenance next steps without writing", "brain.maintenance.plan", "pinax.agent_brain.maintenance_plan.request.v1"),
		registeredTool("pinax.query.run", "Run bounded readonly Pinax SQL query", "query.run", "pinax.query.run.request.v1"),
		registeredTool("pinax.database.view.show", "Show saved readonly database view", "database.view.show", "pinax.database_view.show.request.v1"),
		registeredTool("pinax.database.view.render", "Render saved readonly database view as a bounded tab projection", "database.view.render", "pinax.database_view.render.request.v1"),
		registeredTool("pinax.note.read", "Read one local note", "note.read", "pinax.note.read.request.v1"),
		registeredTool("pinax.note.links", "Read outgoing links for a note", "note.links", "pinax.note.links.request.v1"),
		registeredTool("pinax.note.backlinks", "Read backlinks for a note", "note.backlinks", "pinax.note.backlinks.request.v1"),
		registeredTool("pinax.note.context", "Read bounded graph context around a note", "note.context", "pinax.note.context.request.v1"),
		registeredTool("pinax.vault.graph_summary", "Read vault link graph health summary", "graph.summary", "pinax.graph.summary.request.v1"),
		registeredTool("pinax.project.board", "Read bounded project board facts", "project.board.show", "pinax.project_board.show.request.v1"),
		registeredTool("pinax.task.adopt_plan", "Preview inferred task adoption without writing", "task.adopt.plan", "pinax.task.adopt_plan.request.v1"),
		registeredTool("pinax.organize.plan", "Preview organize operations", "organize.plan", "pinax.organize.plan.request.v1"),
		registeredTool("pinax.git.snapshot_plan", "Show snapshot command", "version.snapshot.plan", "pinax.version.snapshot_plan.request.v1"),
		// Agent memory runtime remains experimental and read-only by default.
		registeredTool("pinax.agent.context", "Read bounded permission-first agent context pack (experimental)", "agent.context", "pinax.agent.context.request.v1"),
		registeredTool("pinax.agent.memory_recall", "Recall bounded agent memories (experimental, readonly)", "agent.memory.recall", "pinax.agent.memory.recall.request.v1"),
		registeredTool("pinax.agent.handoff_read", "Read bounded cross-agent handoff working state (experimental, readonly)", "agent.handoff.read", "pinax.agent.handoff.read.request.v1"),
	}
}

func registeredTool(name, description, capabilityID, requestSchema string) toolRegistration {
	registry := catalogschema.Default()
	input, err := registry.Require(requestSchema)
	if err != nil {
		panic(err)
	}
	if err := validateSchemaKeywordSupport(input); err != nil {
		panic(fmt.Sprintf("pinax MCP tool %s input schema uses keywords the validator does not implement: %v", name, err))
	}
	output, err := registry.Require(catalogschema.MCPToolResultV1)
	if err != nil {
		panic(err)
	}
	return toolRegistration{
		Tool: Tool{
			Name:         name,
			Description:  description,
			InputSchema:  input,
			OutputSchema: output,
			Readonly:     true,
			BodyExposure: "bounded_projection",
			CostClass:    "none",
			Scope:        "local_vault",
		},
		CapabilityID:  capabilityID,
		RequestSchema: requestSchema,
		OutputSchema:  catalogschema.MCPToolResultV1,
	}
}

func resourceRegistrations() []resourceRegistration {
	return []resourceRegistration{
		{Resource: Resource{URI: "pinax://manifest", Name: "transport manifest", Description: "authoritative bounded transport manifest"}, CapabilityID: "transport.manifest"},
		{Resource: Resource{URI: "pinax://readiness", Name: "connection readiness", Description: "six-layer local stdio readiness"}, CapabilityID: "connection.readiness"},
		{Resource: Resource{URI: "pinax://vault/current", Name: "current vault"}, CapabilityID: "vault.stats"},
		{Resource: Resource{URI: "pinax://note/{note_id}", Name: "note by id"}, CapabilityID: "note.read"},
		{Resource: Resource{URI: "pinax://search/{query}", Name: "search notes"}, CapabilityID: "note.search"},
		{Resource: Resource{URI: "pinax://organize/plan", Name: "organize plan"}, CapabilityID: "organize.plan"},
		{Resource: Resource{URI: "pinax://vault/graph", Name: "vault link graph"}, CapabilityID: "graph.summary"},
		{Resource: Resource{URI: "pinax://project/{slug}/board", Name: "project board", Description: "bounded readonly project board"}, CapabilityID: "project.board.show"},
	}
}

// ToolInventory is the single discovery projection for tools/list.
func ToolInventory() []Tool {
	registrations := toolRegistrations()
	tools := make([]Tool, 0, len(registrations))
	for _, registration := range registrations {
		tools = append(tools, registration.Tool)
	}
	return tools
}

// ResourceInventory is the single discovery projection for resources/list.
func ResourceInventory() []Resource {
	registrations := resourceRegistrations()
	resources := make([]Resource, 0, len(registrations))
	for _, registration := range registrations {
		resources = append(resources, registration.Resource)
	}
	return resources
}

// ConcreteResourceInventory is the modern MCP resources/list projection.
// Parameterized entries are published separately through
// resources/templates/list as required by the current protocol.
func ConcreteResourceInventory() []Resource {
	resources := make([]Resource, 0, len(resourceRegistrations()))
	for _, registration := range resourceRegistrations() {
		if !strings.Contains(registration.URI, "{") {
			resources = append(resources, registration.Resource)
		}
	}
	return resources
}

func ResourceTemplateInventory() []map[string]any {
	templates := make([]map[string]any, 0, len(resourceRegistrations()))
	for _, registration := range resourceRegistrations() {
		if !strings.Contains(registration.URI, "{") {
			continue
		}
		template := map[string]any{
			"uriTemplate": registration.URI,
			"name":        registration.Name,
		}
		if registration.Description != "" {
			template["description"] = registration.Description
		}
		templates = append(templates, template)
	}
	return templates
}

// MCPAdditionalCapabilityDefinitions contains MCP-backed capabilities that did
// not exist in the legacy RemoteCapabilities projection.
func MCPAdditionalCapabilityDefinitions() ([]transportcatalog.CapabilityDefinition, map[string][]string) {
	definitions := []transportcatalog.CapabilityDefinition{
		mcpReadonlyDefinition("query.run", "query.run", "pinax.query.run.request.v1"),
		mcpReadonlyDefinition("database.view.show", "database.view.show", "pinax.database_view.show.request.v1"),
		mcpReadonlyDefinition("note.links", "note.links", "pinax.note.links.request.v1"),
		mcpReadonlyDefinition("note.backlinks", "note.backlinks", "pinax.note.backlinks.request.v1"),
		mcpReadonlyDefinition("note.context", "note.context", "pinax.note.context.request.v1"),
		mcpReadonlyDefinition("version.snapshot.plan", "version.snapshot.plan", "pinax.version.snapshot_plan.request.v1"),
		mcpReadonlyDefinition("agent.handoff.read", "agent.handoff.read", "pinax.agent.handoff.read.request.v1"),
	}
	declared := make(map[string][]string, len(definitions))
	for _, definition := range definitions {
		declared[definition.ID] = []string{"mcp"}
	}
	return definitions, declared
}

func mcpReadonlyDefinition(id, command, requestSchema string) transportcatalog.CapabilityDefinition {
	return transportcatalog.CapabilityDefinition{
		ID:                  id,
		Command:             command,
		Readonly:            true,
		BodyExposureDefault: "none",
		WriteGate:           "readonly",
		RequestSchema:       requestSchema,
		ResponseSchema:      "pinax.projection.v1",
		Stability:           transportcatalog.StabilityExperimental,
	}
}

// MCPTransportBindings classifies tools and resources as available only when
// they are present in the real discovery registry and have callable backing.
func MCPTransportBindings() ([]transportcatalog.TransportBinding, error) {
	baseDefinitions, _ := app.RemoteCapabilityDefinitions()
	additionalDefinitions, _ := MCPAdditionalCapabilityDefinitions()
	definitionByID := make(map[string]transportcatalog.CapabilityDefinition, len(baseDefinitions)+len(additionalDefinitions))
	for _, definition := range append(baseDefinitions, additionalDefinitions...) {
		definitionByID[definition.ID] = definition
	}

	registrations := toolRegistrations()
	bindings := make([]transportcatalog.TransportBinding, 0, len(registrations)+len(resourceRegistrations()))
	for _, registration := range registrations {
		definition, ok := definitionByID[registration.CapabilityID]
		if !ok {
			return nil, fmt.Errorf("MCP tool %q references unknown capability %q", registration.Name, registration.CapabilityID)
		}
		if definition.RequestSchema != registration.RequestSchema {
			return nil, fmt.Errorf("MCP tool %q schema %q does not match capability schema %q", registration.Name, registration.RequestSchema, definition.RequestSchema)
		}
		bindings = append(bindings, transportcatalog.TransportBinding{
			ID:             "mcp.tool." + registration.CapabilityID,
			CapabilityID:   registration.CapabilityID,
			Transport:      transportcatalog.TransportMCPTool,
			Availability:   transportcatalog.AvailabilityAvailable,
			BackingRef:     "internal/mcpserver/tool:" + registration.Name,
			ProtocolName:   registration.Name,
			Readonly:       true,
			WriteGate:      "readonly",
			RequestSchema:  definition.RequestSchema,
			ResponseSchema: definition.ResponseSchema,
		})
	}
	for _, registration := range resourceRegistrations() {
		definition, ok := definitionByID[registration.CapabilityID]
		if !ok {
			return nil, fmt.Errorf("MCP resource %q references unknown capability %q", registration.URI, registration.CapabilityID)
		}
		bindings = append(bindings, transportcatalog.TransportBinding{
			ID:             "mcp.resource." + registration.CapabilityID,
			CapabilityID:   registration.CapabilityID,
			Transport:      transportcatalog.TransportMCPResource,
			Availability:   transportcatalog.AvailabilityAvailable,
			BackingRef:     "internal/mcpserver/resource:" + registration.URI,
			ProtocolName:   registration.URI,
			Readonly:       true,
			WriteGate:      "readonly",
			RequestSchema:  definition.RequestSchema,
			ResponseSchema: definition.ResponseSchema,
		})
	}
	return bindings, nil
}

// TransportManifest assembles the current REST, RPC and local MCP truth for
// internal conformance. Public query surfaces are added by the manifest task.
func TransportManifest() (transportcatalog.Manifest, error) {
	additionalDefinitions, additionalDeclared := MCPAdditionalCapabilityDefinitions()
	mcpBindings, err := MCPTransportBindings()
	if err != nil {
		return transportcatalog.Manifest{}, err
	}
	return transportmanifest.Compile(transportmanifest.Contribution{
		Definitions:      additionalDefinitions,
		DeclaredSurfaces: additionalDeclared,
		Bindings:         mcpBindings,
	})
}
