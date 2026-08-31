package transportcatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompileBuildsDeterministicManifestWithoutMutatingInputs(t *testing.T) {
	t.Parallel()

	definitions := []CapabilityDefinition{
		{
			ID:             "project.list",
			Command:        "project.list",
			Readonly:       true,
			Stability:      StabilityLegacy,
			RequestSchema:  "pinax.project.list.request.v1",
			ResponseSchema: "pinax.projection.v1",
		},
		{
			ID:             "folder.rename",
			Command:        "folder.rename",
			Stability:      StabilityLegacy,
			RequestSchema:  "pinax.folder.rename.request.v1",
			ResponseSchema: "pinax.projection.v1",
			WriteGate:      "approval_and_snapshot",
		},
	}
	bindings := []TransportBinding{
		{
			ID:             "rpc.project.list",
			CapabilityID:   "project.list",
			Transport:      TransportRPC,
			Availability:   AvailabilityAvailable,
			BackingRef:     "rpc:Pinax.Project.List",
			ProtocolName:   "Pinax.Project.List",
			Readonly:       true,
			RequestSchema:  "pinax.project.list.request.v1",
			ResponseSchema: "pinax.projection.v1",
		},
		{
			ID:             "rest.project.list",
			CapabilityID:   "project.list",
			Transport:      TransportREST,
			Availability:   AvailabilityAvailable,
			BackingRef:     "handler:rest.project.list",
			Method:         "GET",
			Path:           "/v1/projects",
			Readonly:       true,
			RequestSchema:  "pinax.project.list.request.v1",
			ResponseSchema: "pinax.projection.v1",
		},
		{
			ID:             "mcp.folder.rename",
			CapabilityID:   "folder.rename",
			Transport:      TransportMCPTool,
			Availability:   AvailabilityPlanned,
			ProtocolName:   "pinax_folder_rename",
			RequestSchema:  "pinax.folder.rename.request.v1",
			ResponseSchema: "pinax.projection.v1",
			Blockers:       []string{"write_surface_not_approved"},
		},
	}
	declared := map[string][]string{
		"project.list":  {"rpc", "rest", "cli"},
		"folder.rename": {"mcp", "rest", "cli"},
	}

	wantDefinitions := cloneDefinitions(definitions)
	wantBindings := cloneBindings(bindings)
	wantDeclared := cloneDeclared(declared)

	manifest, err := Compile(definitions, bindings, declared)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if manifest.SchemaVersion != ManifestSchemaVersion {
		t.Fatalf("schema version = %q, want %q", manifest.SchemaVersion, ManifestSchemaVersion)
	}
	if !strings.HasPrefix(manifest.Digest, "sha256:") || len(manifest.Digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q, want sha256 digest", manifest.Digest)
	}
	if len(manifest.Capabilities) != 2 {
		t.Fatalf("capabilities = %d, want 2", len(manifest.Capabilities))
	}
	if got := manifest.Capabilities[0].ID; got != "folder.rename" {
		t.Fatalf("first capability = %q, want folder.rename", got)
	}
	if got := manifest.Capabilities[1].ID; got != "project.list" {
		t.Fatalf("second capability = %q, want project.list", got)
	}

	folder := manifest.Capabilities[0]
	if len(folder.AvailableSurfaces) != 0 {
		t.Fatalf("planned MCP binding became available: %#v", folder.AvailableSurfaces)
	}
	if !reflect.DeepEqual(folder.DeclaredSurfaces, []string{"cli", "mcp", "rest"}) {
		t.Fatalf("declared surfaces = %#v", folder.DeclaredSurfaces)
	}
	if len(folder.Bindings) != 1 || folder.Bindings[0].Availability != AvailabilityPlanned {
		t.Fatalf("folder bindings = %#v", folder.Bindings)
	}

	project := manifest.Capabilities[1]
	if !reflect.DeepEqual(project.AvailableSurfaces, []string{"rest", "rpc"}) {
		t.Fatalf("available surfaces = %#v", project.AvailableSurfaces)
	}
	if got := []string{project.Bindings[0].ID, project.Bindings[1].ID}; !reflect.DeepEqual(got, []string{"rest.project.list", "rpc.project.list"}) {
		t.Fatalf("binding order = %#v", got)
	}

	if !reflect.DeepEqual(definitions, wantDefinitions) {
		t.Fatalf("definitions mutated:\n got %#v\nwant %#v", definitions, wantDefinitions)
	}
	if !reflect.DeepEqual(bindings, wantBindings) {
		t.Fatalf("bindings mutated:\n got %#v\nwant %#v", bindings, wantBindings)
	}
	if !reflect.DeepEqual(declared, wantDeclared) {
		t.Fatalf("declared surfaces mutated:\n got %#v\nwant %#v", declared, wantDeclared)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("compiled manifest did not validate: %v", err)
	}
	recompiled, err := Compile(definitions, bindings, declared)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(manifest)
	secondJSON, _ := json.Marshal(recompiled)
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("manifest is not byte-stable:\n%s\n%s", firstJSON, secondJSON)
	}
	tampered := manifest
	tampered.Capabilities = cloneManifestCapabilities(manifest.Capabilities)
	tampered.Capabilities[0].Command = "folder.move"
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered manifest validation error = %v, want digest mismatch", err)
	}
}

func TestCompileRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	validDefinition := CapabilityDefinition{
		ID:             "project.list",
		Command:        "project.list",
		Readonly:       true,
		Stability:      StabilityLegacy,
		RequestSchema:  "pinax.project.list.request.v1",
		ResponseSchema: "pinax.projection.v1",
	}
	validBinding := TransportBinding{
		ID:             "rest.project.list",
		CapabilityID:   "project.list",
		Transport:      TransportREST,
		Availability:   AvailabilityAvailable,
		BackingRef:     "handler:rest.project.list",
		Method:         "GET",
		Path:           "/v1/projects",
		Readonly:       true,
		RequestSchema:  "pinax.project.list.request.v1",
		ResponseSchema: "pinax.projection.v1",
	}

	tests := []struct {
		name        string
		definitions []CapabilityDefinition
		bindings    []TransportBinding
		wantError   string
	}{
		{
			name:        "duplicate capability",
			definitions: []CapabilityDefinition{validDefinition, validDefinition},
			bindings:    []TransportBinding{validBinding},
			wantError:   "duplicate capability",
		},
		{
			name:        "duplicate binding",
			definitions: []CapabilityDefinition{validDefinition},
			bindings:    []TransportBinding{validBinding, validBinding},
			wantError:   "duplicate binding",
		},
		{
			name:        "unknown capability",
			definitions: []CapabilityDefinition{validDefinition},
			bindings: []TransportBinding{{
				ID:             "rest.missing",
				CapabilityID:   "missing",
				Transport:      TransportREST,
				Availability:   AvailabilityAvailable,
				BackingRef:     "handler:rest.missing",
				RequestSchema:  "pinax.missing.request.v1",
				ResponseSchema: "pinax.projection.v1",
			}},
			wantError: "unknown capability",
		},
		{
			name:        "available binding without backing",
			definitions: []CapabilityDefinition{validDefinition},
			bindings: []TransportBinding{{
				ID:             "rest.project.list",
				CapabilityID:   "project.list",
				Transport:      TransportREST,
				Availability:   AvailabilityAvailable,
				RequestSchema:  "pinax.project.list.request.v1",
				ResponseSchema: "pinax.projection.v1",
			}},
			wantError: "backing_ref",
		},
		{
			name:        "unknown availability",
			definitions: []CapabilityDefinition{validDefinition},
			bindings: []TransportBinding{{
				ID:             "rest.project.list",
				CapabilityID:   "project.list",
				Transport:      TransportREST,
				Availability:   Availability("surprise"),
				RequestSchema:  "pinax.project.list.request.v1",
				ResponseSchema: "pinax.projection.v1",
			}},
			wantError: "availability",
		},
		{
			name:        "unknown transport",
			definitions: []CapabilityDefinition{validDefinition},
			bindings: []TransportBinding{{
				ID:             "rest.project.list",
				CapabilityID:   "project.list",
				Transport:      Transport("carrier-pigeon"),
				Availability:   AvailabilityPlanned,
				RequestSchema:  "pinax.project.list.request.v1",
				ResponseSchema: "pinax.projection.v1",
			}},
			wantError: "transport",
		},
		{
			name: "unknown stability",
			definitions: []CapabilityDefinition{{
				ID:             "project.list",
				Command:        "project.list",
				Stability:      Stability("forever"),
				RequestSchema:  "pinax.project.list.request.v1",
				ResponseSchema: "pinax.projection.v1",
			}},
			wantError: "stability",
		},
		{
			name:        "unknown readiness status",
			definitions: []CapabilityDefinition{validDefinition},
			bindings: []TransportBinding{{
				ID:             "rest.project.list",
				CapabilityID:   "project.list",
				Transport:      TransportREST,
				Availability:   AvailabilityPlanned,
				RequestSchema:  "pinax.project.list.request.v1",
				ResponseSchema: "pinax.projection.v1",
				Readiness: &Readiness{
					Status:   ReadinessStatus("maybe"),
					Maturity: MaturityFirstSupport,
				},
			}},
			wantError: "readiness status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Compile(tt.definitions, tt.bindings, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Compile() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestCompileKeepsBlockedAndFutureBindingsUnavailable(t *testing.T) {
	t.Parallel()

	definition := CapabilityDefinition{
		ID:             "project.list",
		Command:        "project.list",
		Readonly:       true,
		Stability:      StabilityLegacy,
		RequestSchema:  "pinax.project.list.request.v1",
		ResponseSchema: "pinax.projection.v1",
	}
	bindings := []TransportBinding{
		{
			ID:             "mcp.project.list",
			CapabilityID:   "project.list",
			Transport:      TransportMCPTool,
			Availability:   AvailabilityBlocked,
			RequestSchema:  "pinax.project.list.request.v1",
			ResponseSchema: "pinax.projection.v1",
			Blockers:       []string{"scope_not_configured"},
		},
		{
			ID:             "http-mcp.project.list",
			CapabilityID:   "project.list",
			Transport:      TransportMCPResource,
			Availability:   AvailabilityFutureOwner,
			RequestSchema:  "pinax.project.list.request.v1",
			ResponseSchema: "pinax.projection.v1",
		},
	}

	manifest, err := Compile([]CapabilityDefinition{definition}, bindings, nil)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got := manifest.Capabilities[0].AvailableSurfaces; len(got) != 0 {
		t.Fatalf("unavailable bindings produced surfaces: %#v", got)
	}
}

func cloneDefinitions(in []CapabilityDefinition) []CapabilityDefinition {
	out := append([]CapabilityDefinition(nil), in...)
	for i := range out {
		out[i].Errors = append([]string(nil), out[i].Errors...)
	}
	return out
}

func cloneBindings(in []TransportBinding) []TransportBinding {
	out := append([]TransportBinding(nil), in...)
	for i := range out {
		out[i].Blockers = append([]string(nil), out[i].Blockers...)
		if out[i].Readiness != nil {
			readiness := *out[i].Readiness
			readiness.Blockers = append([]string(nil), readiness.Blockers...)
			readiness.NextActions = append([]string(nil), readiness.NextActions...)
			readiness.EvidenceRefs = append([]string(nil), readiness.EvidenceRefs...)
			out[i].Readiness = &readiness
		}
	}
	return out
}

func cloneDeclared(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func cloneManifestCapabilities(in []ManifestCapability) []ManifestCapability {
	out := append([]ManifestCapability(nil), in...)
	for index := range out {
		out[index].Errors = append([]string(nil), out[index].Errors...)
		out[index].DeclaredSurfaces = append([]string(nil), out[index].DeclaredSurfaces...)
		out[index].AvailableSurfaces = append([]string(nil), out[index].AvailableSurfaces...)
		out[index].Bindings = cloneBindings(out[index].Bindings)
	}
	return out
}
