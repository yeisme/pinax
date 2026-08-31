package app

import (
	"fmt"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/transportcatalog"
)

// RemoteCapabilityDefinitions projects the legacy capability inventory into
// transport-neutral definitions. The legacy surfaces remain declarations only;
// they never prove that an adapter is implemented.
func RemoteCapabilityDefinitions() ([]transportcatalog.CapabilityDefinition, map[string][]string) {
	capabilities := RemoteCapabilities()
	definitions := make([]transportcatalog.CapabilityDefinition, 0, len(capabilities))
	declaredSurfaces := make(map[string][]string, len(capabilities))
	for _, capability := range capabilities {
		definitions = append(definitions, transportcatalog.CapabilityDefinition{
			ID:                  capability.ID,
			Command:             capability.Command,
			ReleaseCore:         capability.ReleaseCore,
			Readonly:            capability.Readonly,
			BodyAllowed:         capability.BodyAllowed,
			ApprovalRequired:    capability.ApprovalRequired,
			SnapshotRequired:    capability.SnapshotRequired,
			UIGroup:             capability.UIGroup,
			BodyExposureDefault: capability.BodyExposureDefault,
			WriteGate:           capability.WriteGate,
			CopyCommand:         capability.CopyCommand,
			LocalOnlyReason:     capability.LocalOnlyReason,
			RequestSchema:       capability.RequestSchema,
			ResponseSchema:      capability.ResponseSchema,
			Errors:              append([]string(nil), capability.Errors...),
			Stability:           transportcatalog.StabilityLegacy,
		})
		declaredSurfaces[capability.ID] = append([]string(nil), capability.Surfaces...)
	}
	return definitions, declaredSurfaces
}

// RemoteTransportBindings converts the currently registered REST and RPC
// routes into authoritative available bindings. Other legacy surface labels are
// intentionally excluded until their owning adapter contributes a real backing.
func RemoteTransportBindings() ([]transportcatalog.TransportBinding, error) {
	capabilityByID := make(map[string]domain.RemoteCapability)
	for _, capability := range RemoteCapabilities() {
		capabilityByID[capability.ID] = capability
	}

	routes := RemoteRoutes()
	bindings := make([]transportcatalog.TransportBinding, 0, len(routes))
	for _, route := range routes {
		capability, ok := capabilityByID[route.CapabilityID]
		if !ok {
			return nil, fmt.Errorf("remote route %q references unknown capability %q", route.RouteID, route.CapabilityID)
		}

		binding := transportcatalog.TransportBinding{
			ID:             route.RouteID,
			CapabilityID:   route.CapabilityID,
			Availability:   transportcatalog.AvailabilityAvailable,
			Method:         route.Method,
			Path:           route.Path,
			ProtocolName:   route.RPCMethod,
			Readonly:       route.Readonly,
			WriteGate:      route.WriteGate,
			RequestSchema:  capability.RequestSchema,
			ResponseSchema: capability.ResponseSchema,
		}
		if route.CapabilityID == inboxCaptureCapabilityID {
			binding.Readiness = &transportcatalog.Readiness{
				Status:   transportcatalog.ReadinessReady,
				Maturity: transportcatalog.MaturityFirstSupport,
				EvidenceRefs: []string{
					"test:inbox.capture.dry_run_no_ledger",
					"test:inbox.capture.idempotent_replay",
					"test:inbox.capture.reconcile_without_replay",
				},
			}
		}
		if route.CapabilityID == folderRenameCapabilityID {
			binding.Readiness = &transportcatalog.Readiness{
				Status:   transportcatalog.ReadinessReady,
				Maturity: transportcatalog.MaturityFirstSupport,
				EvidenceRefs: []string{
					"test:folder.rename.expected_revision",
					"test:folder.rename.idempotent_replay",
					"test:folder.rename.reconcile_without_replay",
				},
			}
		}
		switch route.Surface {
		case "rest":
			binding.Transport = transportcatalog.TransportREST
			binding.BackingRef = "internal/api/http:" + route.RouteID
		case "rpc":
			binding.Transport = transportcatalog.TransportRPC
			binding.BackingRef = "internal/api/rpc:" + route.RPCMethod
		default:
			return nil, fmt.Errorf("remote route %q has unsupported surface %q", route.RouteID, route.Surface)
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

// RemoteTransportManifest compiles the additive authoritative transport view.
// Existing RemoteCapabilities and RemoteRoutes remain the compatibility facade.
func RemoteTransportManifest() (transportcatalog.Manifest, error) {
	definitions, declaredSurfaces := RemoteCapabilityDefinitions()
	bindings, err := RemoteTransportBindings()
	if err != nil {
		return transportcatalog.Manifest{}, err
	}
	return transportcatalog.Compile(definitions, bindings, declaredSurfaces)
}
