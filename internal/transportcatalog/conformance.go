package transportcatalog

import (
	"fmt"
	"sort"
	"strings"
)

// AdapterOperation describes one operation that a public adapter actually
// registers. It intentionally contains only transport identity and backing
// metadata, so conformance stays independent from HTTP, Cobra and MCP framing.
type AdapterOperation struct {
	BindingID    string
	CapabilityID string
	Transport    Transport
	BackingRef   string
	Method       string
	Path         string
	ProtocolName string
}

func (o AdapterOperation) validate() error {
	if err := validateIdentifier("adapter binding id", o.BindingID); err != nil {
		return err
	}
	if err := validateIdentifier("adapter capability id", o.CapabilityID); err != nil {
		return err
	}
	if !o.Transport.valid() {
		return fmt.Errorf("adapter operation %q has invalid transport %q", o.BindingID, o.Transport)
	}
	if strings.TrimSpace(o.BackingRef) == "" {
		return fmt.Errorf("adapter operation %q backing_ref is required", o.BindingID)
	}
	if _, err := operationIdentity(o.Transport, o.Method, o.Path, o.ProtocolName); err != nil {
		return fmt.Errorf("adapter operation %q: %w", o.BindingID, err)
	}
	return nil
}

// ValidateConformance verifies adapter->manifest and manifest->adapter parity
// for the selected transports. With no transport filter it validates every
// known transport. Only available bindings require executable adapter backing;
// planned, blocked and future-owner bindings remain declarations.
func ValidateConformance(manifest Manifest, adapters []AdapterOperation, transports ...Transport) error {
	if err := manifest.Validate(); err != nil {
		return err
	}

	scope, err := conformanceScope(transports)
	if err != nil {
		return err
	}

	adapterByID := make(map[string]AdapterOperation, len(adapters))
	adapterIdentityOwner := make(map[string]string, len(adapters))
	for _, adapter := range adapters {
		if err := adapter.validate(); err != nil {
			return err
		}
		if !scope[adapter.Transport] {
			continue
		}
		if _, exists := adapterByID[adapter.BindingID]; exists {
			return fmt.Errorf("duplicate adapter binding %q", adapter.BindingID)
		}
		identity, _ := operationIdentity(adapter.Transport, adapter.Method, adapter.Path, adapter.ProtocolName)
		if owner, exists := adapterIdentityOwner[identity]; exists {
			return fmt.Errorf("duplicate adapter operation identity %q for %q and %q", identity, owner, adapter.BindingID)
		}
		adapterIdentityOwner[identity] = adapter.BindingID
		adapterByID[adapter.BindingID] = adapter
	}

	availableByID := make(map[string]TransportBinding)
	manifestIdentityOwner := make(map[string]string)
	for _, capability := range manifest.Capabilities {
		for _, binding := range capability.Bindings {
			if binding.Availability != AvailabilityAvailable || !scope[binding.Transport] {
				continue
			}
			identity, identityErr := operationIdentity(binding.Transport, binding.Method, binding.Path, binding.ProtocolName)
			if identityErr != nil {
				return fmt.Errorf("available binding %q: %w", binding.ID, identityErr)
			}
			if owner, exists := manifestIdentityOwner[identity]; exists {
				return fmt.Errorf("duplicate manifest operation identity %q for %q and %q", identity, owner, binding.ID)
			}
			manifestIdentityOwner[identity] = binding.ID
			availableByID[binding.ID] = binding
		}
	}

	adapterIDs := sortedOperationIDs(adapterByID)
	for _, bindingID := range adapterIDs {
		adapter := adapterByID[bindingID]
		binding, exists := availableByID[bindingID]
		if !exists {
			return fmt.Errorf("orphan adapter operation %q has no available manifest binding", bindingID)
		}
		if err := compareAdapterBinding(adapter, binding); err != nil {
			return err
		}
	}

	manifestIDs := sortedBindingIDs(availableByID)
	for _, bindingID := range manifestIDs {
		if _, exists := adapterByID[bindingID]; !exists {
			return fmt.Errorf("false available binding %q has no registered adapter operation", bindingID)
		}
	}
	return nil
}

func conformanceScope(transports []Transport) (map[Transport]bool, error) {
	if len(transports) == 0 {
		return map[Transport]bool{
			TransportCLI: true, TransportREST: true, TransportRPC: true,
			TransportMCPTool: true, TransportMCPResource: true, TransportDashboard: true,
		}, nil
	}
	scope := make(map[Transport]bool, len(transports))
	for _, transport := range transports {
		if !transport.valid() {
			return nil, fmt.Errorf("invalid conformance transport %q", transport)
		}
		scope[transport] = true
	}
	return scope, nil
}

func operationIdentity(transport Transport, method, path, protocolName string) (string, error) {
	switch transport {
	case TransportREST:
		method = strings.ToUpper(strings.TrimSpace(method))
		path = strings.TrimSpace(path)
		if method == "" || path == "" {
			return "", fmt.Errorf("REST method and path are required")
		}
		return string(transport) + ":" + method + " " + path, nil
	case TransportCLI, TransportRPC, TransportMCPTool, TransportMCPResource, TransportDashboard:
		protocolName = strings.TrimSpace(protocolName)
		if protocolName == "" {
			return "", fmt.Errorf("protocol_name is required for %s", transport)
		}
		return string(transport) + ":" + protocolName, nil
	default:
		return "", fmt.Errorf("invalid transport %q", transport)
	}
}

func compareAdapterBinding(adapter AdapterOperation, binding TransportBinding) error {
	if adapter.CapabilityID != binding.CapabilityID ||
		adapter.Transport != binding.Transport ||
		adapter.BackingRef != binding.BackingRef ||
		!strings.EqualFold(strings.TrimSpace(adapter.Method), strings.TrimSpace(binding.Method)) ||
		adapter.Path != binding.Path ||
		adapter.ProtocolName != binding.ProtocolName {
		return fmt.Errorf("adapter operation %q does not match manifest binding", adapter.BindingID)
	}
	return nil
}

func sortedOperationIDs(values map[string]AdapterOperation) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedBindingIDs(values map[string]TransportBinding) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
