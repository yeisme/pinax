package transportmanifest

import (
	"fmt"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/transportcatalog"
)

type Contribution struct {
	Definitions      []transportcatalog.CapabilityDefinition
	DeclaredSurfaces map[string][]string
	Bindings         []transportcatalog.TransportBinding
}

type Provider func() (transportcatalog.Manifest, error)

// Compile combines the stable application capability/route projection with
// adapter-owned contributions. Adapters provide descriptors only; the compiler
// remains independent from HTTP, Cobra and MCP framing packages.
func Compile(contributions ...Contribution) (transportcatalog.Manifest, error) {
	definitions, declaredSurfaces := app.RemoteCapabilityDefinitions()
	bindings, err := app.RemoteTransportBindings()
	if err != nil {
		return transportcatalog.Manifest{}, err
	}

	for _, contribution := range contributions {
		definitions = append(definitions, contribution.Definitions...)
		bindings = append(bindings, contribution.Bindings...)
		for capabilityID, surfaces := range contribution.DeclaredSurfaces {
			declaredSurfaces[capabilityID] = append(declaredSurfaces[capabilityID], surfaces...)
		}
	}
	return transportcatalog.Compile(definitions, bindings, declaredSurfaces)
}

func Projection(manifest transportcatalog.Manifest) domain.Projection {
	bindings := 0
	for _, capability := range manifest.Capabilities {
		bindings += len(capability.Bindings)
	}
	projection := domain.NewProjection("api.manifest", "Transport manifest generated.")
	projection.Facts["schema_version"] = manifest.SchemaVersion
	projection.Facts["digest"] = manifest.Digest
	projection.Facts["capabilities"] = fmt.Sprint(len(manifest.Capabilities))
	projection.Facts["bindings"] = fmt.Sprint(bindings)
	projection.Data = map[string]any{"manifest": manifest}
	return projection
}

func ProjectionProvider(provider Provider) func() (domain.Projection, error) {
	return func() (domain.Projection, error) {
		manifest, err := provider()
		if err != nil {
			commandErr := &domain.CommandError{Code: "transport_manifest_invalid", Message: "Transport manifest could not be compiled"}
			return domain.NewErrorProjection("api.manifest", commandErr), err
		}
		return Projection(manifest), nil
	}
}

func DefaultProjection() (domain.Projection, error) {
	manifest, err := Compile()
	if err != nil {
		commandErr := &domain.CommandError{Code: "transport_manifest_invalid", Message: "Transport manifest could not be compiled"}
		return domain.NewErrorProjection("api.manifest", commandErr), err
	}
	return Projection(manifest), nil
}
