package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/transportcatalog"
)

func TestMCPDiscoveryAndCatalogBindingsStayInParity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	service := app.NewService()
	if _, err := service.InitVault(context.Background(), app.InitVaultRequest{VaultPath: root, Title: "MCP registry"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(service, root)

	toolsResponse, err := server.Handle(context.Background(), Request{ID: 1, Method: "tools/list"})
	if err != nil {
		t.Fatal(err)
	}
	resourcesResponse, err := server.Handle(context.Background(), Request{ID: 2, Method: "resources/list"})
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := TransportManifest()
	if err != nil {
		t.Fatalf("TransportManifest() error = %v", err)
	}
	mcpBindingByProtocol := map[string]transportcatalog.TransportBinding{}
	for _, capability := range manifest.Capabilities {
		for _, binding := range capability.Bindings {
			if binding.Transport == transportcatalog.TransportMCPTool || binding.Transport == transportcatalog.TransportMCPResource {
				if _, exists := mcpBindingByProtocol[binding.ProtocolName]; exists {
					t.Fatalf("duplicate MCP protocol binding %q", binding.ProtocolName)
				}
				mcpBindingByProtocol[binding.ProtocolName] = binding
			}
		}
	}

	for _, tool := range toolsResponse.Tools {
		binding, ok := mcpBindingByProtocol[tool.Name]
		if !ok {
			t.Fatalf("discovered tool %q has no manifest binding", tool.Name)
		}
		if binding.Transport != transportcatalog.TransportMCPTool || binding.Availability != transportcatalog.AvailabilityAvailable || binding.BackingRef == "" {
			t.Fatalf("tool binding %q is not available and backed: %#v", tool.Name, binding)
		}
	}

	for _, resource := range resourcesResponse.Resources {
		binding, ok := mcpBindingByProtocol[resource.URI]
		if !ok {
			t.Fatalf("discovered resource %q has no manifest classification", resource.URI)
		}
		if binding.Transport != transportcatalog.TransportMCPResource || binding.Availability != transportcatalog.AvailabilityAvailable {
			t.Fatalf("readable resource %q must be available: %#v", resource.URI, binding)
		}
		if len(binding.Blockers) != 0 || binding.BackingRef == "" {
			t.Fatalf("available resource %q classification is incomplete: %#v", resource.URI, binding)
		}
	}

	for _, forbidden := range []string{"folder.create", "folder.rename", "folder.move", "folder.delete", "inbox.capture", "memory.capture", "organize.apply"} {
		if binding, ok := mcpBindingByProtocol["pinax."+forbidden]; ok && binding.Availability == transportcatalog.AvailabilityAvailable {
			t.Fatalf("direct mutation unexpectedly published through MCP: %#v", binding)
		}
	}
}

func TestMCPManifestConformsToRuntimeRegistries(t *testing.T) {
	t.Parallel()

	manifest, err := TransportManifest()
	if err != nil {
		t.Fatalf("TransportManifest() error = %v", err)
	}
	operations := make([]transportcatalog.AdapterOperation, 0, len(toolRegistrations())+len(resourceRegistrations()))
	for _, registration := range toolRegistrations() {
		operations = append(operations, transportcatalog.AdapterOperation{
			BindingID:    "mcp.tool." + registration.CapabilityID,
			CapabilityID: registration.CapabilityID,
			Transport:    transportcatalog.TransportMCPTool,
			BackingRef:   "internal/mcpserver/tool:" + registration.Name,
			ProtocolName: registration.Name,
		})
	}
	for _, registration := range resourceRegistrations() {
		operations = append(operations, transportcatalog.AdapterOperation{
			BindingID:    "mcp.resource." + registration.CapabilityID,
			CapabilityID: registration.CapabilityID,
			Transport:    transportcatalog.TransportMCPResource,
			BackingRef:   "internal/mcpserver/resource:" + registration.URI,
			ProtocolName: registration.URI,
		})
	}
	if err := transportcatalog.ValidateConformance(
		manifest,
		operations,
		transportcatalog.TransportMCPTool,
		transportcatalog.TransportMCPResource,
	); err != nil {
		t.Fatalf("MCP registry conformance failed: %v", err)
	}
}

func TestEveryDiscoveredMCPToolHasAHandler(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	service := app.NewService()
	if _, err := service.InitVault(context.Background(), app.InitVaultRequest{VaultPath: root, Title: "MCP handlers"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(service, root)
	response, err := server.Handle(context.Background(), Request{ID: 1, Method: "tools/list"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range response.Tools {
		tool := tool
		t.Run(tool.Name, func(t *testing.T) {
			_, callErr := server.callTool(context.Background(), Request{ID: 2, Method: "tools/call", Params: map[string]any{
				"name":      tool.Name,
				"arguments": map[string]any{},
			}})
			if mcpErr, ok := callErr.(*MCPError); ok {
				legacyCode, _ := mcpErr.Data["legacy_code"].(string)
				if legacyCode == "approval_required" || strings.Contains(mcpErr.Message, "只允许只读工具") {
					t.Fatalf("discovered tool %q has no handler", tool.Name)
				}
			}
		})
	}
}
