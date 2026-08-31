package transportmanifest

import (
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/transportcatalog"
)

func TestCompileCombinesBaseAndAdapterContributions(t *testing.T) {
	t.Parallel()

	manifest, err := Compile(Contribution{Bindings: []transportcatalog.TransportBinding{{
		ID:             "cli.remote.project.board.show",
		CapabilityID:   "project.board.show",
		Transport:      transportcatalog.TransportCLI,
		Availability:   transportcatalog.AvailabilityAvailable,
		BackingRef:     "internal/cli/remote:project board show",
		Method:         "Pinax.ProjectBoard.Show",
		ProtocolName:   "project board show",
		Readonly:       true,
		WriteGate:      "readonly",
		RequestSchema:  "pinax.project_board.show.request.v1",
		ResponseSchema: "pinax.projection.v1",
	}}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if manifest.Digest == "" || manifest.SchemaVersion != transportcatalog.ManifestSchemaVersion {
		t.Fatalf("manifest identity = %#v", manifest)
	}

	found := false
	for _, capability := range manifest.Capabilities {
		for _, binding := range capability.Bindings {
			if binding.ID == "cli.remote.project.board.show" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("adapter contribution was not compiled")
	}

	projection := Projection(manifest)
	if projection.Command != "api.manifest" || projection.Facts["digest"] != manifest.Digest {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestCompileRejectsDuplicateAdapterBinding(t *testing.T) {
	t.Parallel()

	duplicate := transportcatalog.TransportBinding{
		ID:             "rest.project.list",
		CapabilityID:   "project.list",
		Transport:      transportcatalog.TransportREST,
		Availability:   transportcatalog.AvailabilityAvailable,
		BackingRef:     "duplicate",
		Method:         "GET",
		Path:           "/v1/projects",
		Readonly:       true,
		RequestSchema:  "pinax.project.list.request.v1",
		ResponseSchema: "pinax.projection.v1",
	}
	_, err := Compile(Contribution{Bindings: []transportcatalog.TransportBinding{duplicate}})
	if err == nil || !strings.Contains(err.Error(), "duplicate binding") {
		t.Fatalf("Compile() error = %v, want duplicate binding", err)
	}
}
