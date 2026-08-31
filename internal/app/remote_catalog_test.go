package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/yeisme/pinax/internal/transportcatalog"
)

func TestRemoteCapabilityDefinitionsPreserveLegacyProjection(t *testing.T) {
	t.Parallel()

	legacyBefore := RemoteCapabilities()
	definitions, declaredSurfaces := RemoteCapabilityDefinitions()
	legacyAfter := RemoteCapabilities()

	if !reflect.DeepEqual(legacyBefore, legacyAfter) {
		t.Fatal("building capability definitions mutated the legacy capability projection")
	}
	if len(definitions) != len(legacyBefore) {
		t.Fatalf("definitions = %d, legacy capabilities = %d", len(definitions), len(legacyBefore))
	}

	definitionByID := make(map[string]transportcatalog.CapabilityDefinition, len(definitions))
	for _, definition := range definitions {
		if _, exists := definitionByID[definition.ID]; exists {
			t.Fatalf("duplicate capability definition %q", definition.ID)
		}
		definitionByID[definition.ID] = definition
	}

	for _, legacy := range legacyBefore {
		definition, ok := definitionByID[legacy.ID]
		if !ok {
			t.Fatalf("missing definition for legacy capability %q", legacy.ID)
		}
		if definition.ID != legacy.ID ||
			definition.Command != legacy.Command ||
			definition.ReleaseCore != legacy.ReleaseCore ||
			definition.Readonly != legacy.Readonly ||
			definition.BodyAllowed != legacy.BodyAllowed ||
			definition.ApprovalRequired != legacy.ApprovalRequired ||
			definition.SnapshotRequired != legacy.SnapshotRequired ||
			definition.UIGroup != legacy.UIGroup ||
			definition.BodyExposureDefault != legacy.BodyExposureDefault ||
			definition.WriteGate != legacy.WriteGate ||
			definition.CopyCommand != legacy.CopyCommand ||
			definition.LocalOnlyReason != legacy.LocalOnlyReason ||
			definition.RequestSchema != legacy.RequestSchema ||
			definition.ResponseSchema != legacy.ResponseSchema ||
			!reflect.DeepEqual(definition.Errors, legacy.Errors) {
			t.Fatalf("definition for %q does not preserve the legacy metadata:\n got %#v\nwant %#v", legacy.ID, definition, legacy)
		}
		if definition.Stability != transportcatalog.StabilityLegacy {
			t.Fatalf("definition %q stability = %q, want legacy", legacy.ID, definition.Stability)
		}
		if !reflect.DeepEqual(declaredSurfaces[legacy.ID], legacy.Surfaces) {
			t.Fatalf("declared surfaces for %q = %#v, want %#v", legacy.ID, declaredSurfaces[legacy.ID], legacy.Surfaces)
		}
	}
}

func TestRemoteTransportManifestRegistersRESTAndRPCBindings(t *testing.T) {
	t.Parallel()

	manifest, err := RemoteTransportManifest()
	if err != nil {
		t.Fatalf("RemoteTransportManifest() error = %v", err)
	}
	if manifest.SchemaVersion != transportcatalog.ManifestSchemaVersion {
		t.Fatalf("schema version = %q, want %q", manifest.SchemaVersion, transportcatalog.ManifestSchemaVersion)
	}

	bindingByID := map[string]transportcatalog.TransportBinding{}
	capabilityByID := map[string]transportcatalog.ManifestCapability{}
	for _, capability := range manifest.Capabilities {
		capabilityByID[capability.ID] = capability
		for _, binding := range capability.Bindings {
			if _, exists := bindingByID[binding.ID]; exists {
				t.Fatalf("duplicate manifest binding %q", binding.ID)
			}
			bindingByID[binding.ID] = binding
		}
	}

	routes := RemoteRoutes()
	if len(bindingByID) != len(routes) {
		t.Fatalf("manifest bindings = %d, remote routes = %d", len(bindingByID), len(routes))
	}
	for _, route := range routes {
		binding, ok := bindingByID[route.RouteID]
		if !ok {
			t.Fatalf("missing binding for route %q", route.RouteID)
		}
		if binding.CapabilityID != route.CapabilityID || binding.Method != route.Method || binding.Path != route.Path || binding.ProtocolName != route.RPCMethod {
			t.Fatalf("binding %q does not match route:\n got %#v\nwant %#v", route.RouteID, binding, route)
		}
		if binding.Availability != transportcatalog.AvailabilityAvailable || binding.BackingRef == "" {
			t.Fatalf("route binding %q is not backed and available: %#v", route.RouteID, binding)
		}
		wantTransport := transportcatalog.TransportREST
		if route.Surface == "rpc" {
			wantTransport = transportcatalog.TransportRPC
		}
		if binding.Transport != wantTransport {
			t.Fatalf("binding %q transport = %q, want %q", route.RouteID, binding.Transport, wantTransport)
		}
	}

	projectList := capabilityByID["project.list"]
	if manifestContainsString(projectList.AvailableSurfaces, "mcp") {
		t.Fatalf("legacy MCP declaration became a false available binding: %#v", projectList)
	}
	if !manifestContainsString(projectList.DeclaredSurfaces, "mcp") {
		t.Fatalf("legacy MCP declaration was lost: %#v", projectList)
	}
	for _, bindingID := range []string{"rest.inbox.capture", "rpc.inbox.capture"} {
		binding := bindingByID[bindingID]
		if binding.Readiness == nil || binding.Readiness.Status != transportcatalog.ReadinessReady || binding.Readiness.Maturity != transportcatalog.MaturityFirstSupport || len(binding.Readiness.EvidenceRefs) < 3 {
			t.Fatalf("inbox capture recovery readiness is incomplete for %s: %#v", bindingID, binding.Readiness)
		}
	}
	for _, bindingID := range []string{"rest.folder.rename", "rpc.folder.rename"} {
		binding := bindingByID[bindingID]
		if binding.Readiness == nil || binding.Readiness.Status != transportcatalog.ReadinessReady || binding.Readiness.Maturity != transportcatalog.MaturityFirstSupport || len(binding.Readiness.EvidenceRefs) < 3 {
			t.Fatalf("folder rename recovery readiness is incomplete for %s: %#v", bindingID, binding.Readiness)
		}
	}

	first, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	secondManifest, err := RemoteTransportManifest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(secondManifest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("transport manifest projection is not deterministic")
	}
}

func TestRemoteTransportManifestConformsToRouteRegistry(t *testing.T) {
	t.Parallel()

	manifest, err := RemoteTransportManifest()
	if err != nil {
		t.Fatalf("RemoteTransportManifest() error = %v", err)
	}
	operations := make([]transportcatalog.AdapterOperation, 0, len(RemoteRoutes()))
	for _, route := range RemoteRoutes() {
		operation := transportcatalog.AdapterOperation{
			BindingID:    route.RouteID,
			CapabilityID: route.CapabilityID,
			Method:       route.Method,
			Path:         route.Path,
			ProtocolName: route.RPCMethod,
		}
		switch route.Surface {
		case "rest":
			operation.Transport = transportcatalog.TransportREST
			operation.BackingRef = "internal/api/http:" + route.RouteID
		case "rpc":
			operation.Transport = transportcatalog.TransportRPC
			operation.BackingRef = "internal/api/rpc:" + route.RPCMethod
		default:
			t.Fatalf("route %q has unsupported surface %q", route.RouteID, route.Surface)
		}
		operations = append(operations, operation)
	}
	if err := transportcatalog.ValidateConformance(
		manifest,
		operations,
		transportcatalog.TransportREST,
		transportcatalog.TransportRPC,
	); err != nil {
		t.Fatalf("route registry conformance failed: %v", err)
	}
}

func manifestContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
