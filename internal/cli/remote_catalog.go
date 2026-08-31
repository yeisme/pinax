package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/mcpserver"
	"github.com/yeisme/pinax/internal/transportcatalog"
	"github.com/yeisme/pinax/internal/transportmanifest"
)

// RemoteCLITransportBindings projects the actual command dispatch registry.
// Commands absent from remoteCommandRegistry remain local-only or unsupported
// and continue to fail before any local business handler can run.
func RemoteCLITransportBindings() ([]transportcatalog.TransportBinding, error) {
	registry := remoteCommandRegistrySnapshot()
	definitionByID := make(map[string]transportcatalog.CapabilityDefinition)
	definitions, _ := app.RemoteCapabilityDefinitions()
	for _, definition := range definitions {
		definitionByID[definition.ID] = definition
	}

	routeByRPCMethod := make(map[string]struct {
		capabilityID string
		readonly     bool
		writeGate    string
	})
	for _, route := range app.RemoteRoutes() {
		if route.Surface != "rpc" {
			continue
		}
		routeByRPCMethod[route.RPCMethod] = struct {
			capabilityID string
			readonly     bool
			writeGate    string
		}{capabilityID: route.CapabilityID, readonly: route.Readonly, writeGate: route.WriteGate}
	}

	commandPaths := make([]string, 0, len(registry))
	for commandPath := range registry {
		commandPaths = append(commandPaths, commandPath)
	}
	sort.Strings(commandPaths)

	bindings := make([]transportcatalog.TransportBinding, 0, len(commandPaths))
	for _, commandPath := range commandPaths {
		spec := registry[commandPath]
		route, ok := routeByRPCMethod[spec.Method]
		if !ok {
			return nil, fmt.Errorf("remote CLI command %q references unregistered RPC method %q", commandPath, spec.Method)
		}
		definition, ok := definitionByID[route.capabilityID]
		if !ok {
			return nil, fmt.Errorf("remote CLI command %q references unknown capability %q", commandPath, route.capabilityID)
		}
		binding := transportcatalog.TransportBinding{
			ID:             "cli.remote." + strings.ReplaceAll(commandPath, " ", "."),
			CapabilityID:   route.capabilityID,
			Transport:      transportcatalog.TransportCLI,
			Availability:   transportcatalog.AvailabilityAvailable,
			BackingRef:     "internal/cli/remote:" + commandPath,
			Method:         spec.Method,
			ProtocolName:   commandPath,
			Readonly:       route.readonly,
			WriteGate:      route.writeGate,
			RequestSchema:  definition.RequestSchema,
			ResponseSchema: definition.ResponseSchema,
		}
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

// TransportManifest composes every adapter registry loaded by the Pinax CLI.
// API and RPC servers receive this function as a provider so all three public
// query surfaces return the same immutable manifest value.
func TransportManifest() (transportcatalog.Manifest, error) {
	cliBindings, err := RemoteCLITransportBindings()
	if err != nil {
		return transportcatalog.Manifest{}, err
	}
	mcpDefinitions, mcpDeclared := mcpserver.MCPAdditionalCapabilityDefinitions()
	mcpBindings, err := mcpserver.MCPTransportBindings()
	if err != nil {
		return transportcatalog.Manifest{}, err
	}
	return transportmanifest.Compile(
		transportmanifest.Contribution{Bindings: cliBindings},
		transportmanifest.Contribution{
			Definitions:      mcpDefinitions,
			DeclaredSurfaces: mcpDeclared,
			Bindings:         mcpBindings,
		},
	)
}

func TransportManifestProjection() (domain.Projection, error) {
	manifest, err := TransportManifest()
	if err != nil {
		commandErr := &domain.CommandError{Code: "transport_manifest_invalid", Message: "Transport manifest could not be compiled"}
		return domain.NewErrorProjection("api.manifest", commandErr), err
	}
	return transportmanifest.Projection(manifest), nil
}
