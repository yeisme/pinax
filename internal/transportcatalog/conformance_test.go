package transportcatalog

import (
	"strings"
	"testing"
)

func TestValidateConformanceDetectsParityFailures(t *testing.T) {
	t.Parallel()

	definition := CapabilityDefinition{
		ID:             "project.list",
		Command:        "project.list",
		Readonly:       true,
		Stability:      StabilityLegacy,
		RequestSchema:  "pinax.project.list.request.v1",
		ResponseSchema: "pinax.projection.v1",
	}
	binding := TransportBinding{
		ID:             "rest.project.list",
		CapabilityID:   definition.ID,
		Transport:      TransportREST,
		Availability:   AvailabilityAvailable,
		BackingRef:     "internal/api/http:rest.project.list",
		Method:         "GET",
		Path:           "/v1/projects",
		Readonly:       true,
		RequestSchema:  definition.RequestSchema,
		ResponseSchema: definition.ResponseSchema,
	}
	adapter := AdapterOperation{
		BindingID:    binding.ID,
		CapabilityID: binding.CapabilityID,
		Transport:    binding.Transport,
		BackingRef:   binding.BackingRef,
		Method:       binding.Method,
		Path:         binding.Path,
	}

	manifest, err := Compile([]CapabilityDefinition{definition}, []TransportBinding{binding}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConformance(manifest, []AdapterOperation{adapter}, TransportREST); err != nil {
		t.Fatalf("ValidateConformance() error = %v", err)
	}

	tests := []struct {
		name      string
		manifest  Manifest
		adapters  []AdapterOperation
		wantError string
	}{
		{
			name:      "route missing manifest binding",
			manifest:  mustCompileConformanceManifest(t, []CapabilityDefinition{definition}, nil),
			adapters:  []AdapterOperation{adapter},
			wantError: `orphan adapter operation "rest.project.list"`,
		},
		{
			name:      "manifest falsely marks missing route available",
			manifest:  manifest,
			adapters:  nil,
			wantError: `false available binding "rest.project.list"`,
		},
		{
			name: "duplicate operation identity",
			manifest: mustCompileConformanceManifest(t, []CapabilityDefinition{definition}, []TransportBinding{
				binding,
				{
					ID:             "rest.project.list.alias",
					CapabilityID:   definition.ID,
					Transport:      TransportREST,
					Availability:   AvailabilityAvailable,
					BackingRef:     "internal/api/http:rest.project.list.alias",
					Method:         "GET",
					Path:           "/v1/projects",
					Readonly:       true,
					RequestSchema:  definition.RequestSchema,
					ResponseSchema: definition.ResponseSchema,
				},
			}),
			wantError: "duplicate manifest operation identity",
		},
		{
			name:      "adapter fields drift",
			manifest:  manifest,
			adapters:  []AdapterOperation{func() AdapterOperation { changed := adapter; changed.Path = "/v1/project-list"; return changed }()},
			wantError: `adapter operation "rest.project.list" does not match manifest binding`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConformance(tt.manifest, tt.adapters, TransportREST)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ValidateConformance() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func mustCompileConformanceManifest(t *testing.T, definitions []CapabilityDefinition, bindings []TransportBinding) Manifest {
	t.Helper()
	manifest, err := Compile(definitions, bindings, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
