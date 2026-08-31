package transportcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const ManifestSchemaVersion = "pinax.transport_manifest.v1"

type Transport string

const (
	TransportCLI         Transport = "cli"
	TransportREST        Transport = "rest"
	TransportRPC         Transport = "rpc"
	TransportMCPTool     Transport = "mcp_tool"
	TransportMCPResource Transport = "mcp_resource"
	TransportDashboard   Transport = "dashboard"
)

func (t Transport) valid() bool {
	switch t {
	case TransportCLI, TransportREST, TransportRPC, TransportMCPTool, TransportMCPResource, TransportDashboard:
		return true
	default:
		return false
	}
}

func (t Transport) Surface() string {
	switch t {
	case TransportMCPTool, TransportMCPResource:
		return "mcp"
	default:
		return string(t)
	}
}

type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityPlanned     Availability = "planned"
	AvailabilityBlocked     Availability = "blocked"
	AvailabilityFutureOwner Availability = "future_owner"
)

func (a Availability) valid() bool {
	switch a {
	case AvailabilityAvailable, AvailabilityPlanned, AvailabilityBlocked, AvailabilityFutureOwner:
		return true
	default:
		return false
	}
}

type Stability string

const (
	StabilityExperimental Stability = "experimental"
	StabilityPreview      Stability = "preview"
	StabilityStable       Stability = "stable"
	StabilityDeprecated   Stability = "deprecated"
	StabilityLegacy       Stability = "legacy"
)

func (s Stability) valid() bool {
	switch s {
	case StabilityExperimental, StabilityPreview, StabilityStable, StabilityDeprecated, StabilityLegacy:
		return true
	default:
		return false
	}
}

type ReadinessStatus string

const (
	ReadinessReady         ReadinessStatus = "ready"
	ReadinessDegraded      ReadinessStatus = "degraded"
	ReadinessBlocked       ReadinessStatus = "blocked"
	ReadinessNotConfigured ReadinessStatus = "not_configured"
	ReadinessNotApplicable ReadinessStatus = "not_applicable"
)

func (s ReadinessStatus) valid() bool {
	switch s {
	case ReadinessReady, ReadinessDegraded, ReadinessBlocked, ReadinessNotConfigured, ReadinessNotApplicable:
		return true
	default:
		return false
	}
}

type Maturity string

const (
	MaturityExploratory  Maturity = "exploratory"
	MaturityFirstSupport Maturity = "first-support"
	MaturityMature       Maturity = "mature"
)

func (m Maturity) valid() bool {
	switch m {
	case MaturityExploratory, MaturityFirstSupport, MaturityMature:
		return true
	default:
		return false
	}
}

type Readiness struct {
	Status       ReadinessStatus `json:"status"`
	Maturity     Maturity        `json:"maturity"`
	Blockers     []string        `json:"blockers,omitempty"`
	NextActions  []string        `json:"next_actions,omitempty"`
	EvidenceRefs []string        `json:"evidence_refs,omitempty"`
}

func (r Readiness) Validate() error {
	if !r.Status.valid() {
		return fmt.Errorf("invalid readiness status %q", r.Status)
	}
	if !r.Maturity.valid() {
		return fmt.Errorf("invalid readiness maturity %q", r.Maturity)
	}
	return nil
}

type CapabilityDefinition struct {
	ID                  string    `json:"id"`
	Command             string    `json:"command"`
	ReleaseCore         bool      `json:"release_core"`
	Readonly            bool      `json:"readonly"`
	BodyAllowed         bool      `json:"body_allowed"`
	ApprovalRequired    bool      `json:"approval_required"`
	SnapshotRequired    bool      `json:"snapshot_required"`
	UIGroup             string    `json:"ui_group,omitempty"`
	BodyExposureDefault string    `json:"body_exposure_default,omitempty"`
	WriteGate           string    `json:"write_gate,omitempty"`
	CopyCommand         string    `json:"copy_command,omitempty"`
	LocalOnlyReason     string    `json:"local_only_reason,omitempty"`
	RequestSchema       string    `json:"request_schema"`
	ResponseSchema      string    `json:"response_schema"`
	Errors              []string  `json:"errors,omitempty"`
	Stability           Stability `json:"stability"`
}

func (d CapabilityDefinition) Validate() error {
	if err := validateIdentifier("capability id", d.ID); err != nil {
		return err
	}
	if err := validateIdentifier("command", d.Command); err != nil {
		return err
	}
	if !d.Stability.valid() {
		return fmt.Errorf("capability %q has invalid stability %q", d.ID, d.Stability)
	}
	if strings.TrimSpace(d.RequestSchema) == "" {
		return fmt.Errorf("capability %q request_schema is required", d.ID)
	}
	if strings.TrimSpace(d.ResponseSchema) == "" {
		return fmt.Errorf("capability %q response_schema is required", d.ID)
	}
	return nil
}

type TransportBinding struct {
	ID             string       `json:"id"`
	CapabilityID   string       `json:"capability_id"`
	Transport      Transport    `json:"transport"`
	Availability   Availability `json:"availability"`
	BackingRef     string       `json:"backing_ref,omitempty"`
	Method         string       `json:"method,omitempty"`
	Path           string       `json:"path,omitempty"`
	ProtocolName   string       `json:"protocol_name,omitempty"`
	Readonly       bool         `json:"readonly"`
	WriteGate      string       `json:"write_gate,omitempty"`
	RequestSchema  string       `json:"request_schema"`
	ResponseSchema string       `json:"response_schema"`
	Blockers       []string     `json:"blockers,omitempty"`
	Readiness      *Readiness   `json:"readiness,omitempty"`
}

func (b TransportBinding) Validate() error {
	if err := validateIdentifier("binding id", b.ID); err != nil {
		return err
	}
	if err := validateIdentifier("binding capability id", b.CapabilityID); err != nil {
		return err
	}
	if !b.Transport.valid() {
		return fmt.Errorf("binding %q has invalid transport %q", b.ID, b.Transport)
	}
	if !b.Availability.valid() {
		return fmt.Errorf("binding %q has invalid availability %q", b.ID, b.Availability)
	}
	if b.Availability == AvailabilityAvailable && strings.TrimSpace(b.BackingRef) == "" {
		return fmt.Errorf("binding %q backing_ref is required when availability is available", b.ID)
	}
	if strings.TrimSpace(b.RequestSchema) == "" {
		return fmt.Errorf("binding %q request_schema is required", b.ID)
	}
	if strings.TrimSpace(b.ResponseSchema) == "" {
		return fmt.Errorf("binding %q response_schema is required", b.ID)
	}
	if b.Readiness != nil {
		if err := b.Readiness.Validate(); err != nil {
			return fmt.Errorf("binding %q: %w", b.ID, err)
		}
	}
	return nil
}

type Manifest struct {
	SchemaVersion string               `json:"schema_version"`
	Digest        string               `json:"digest"`
	Capabilities  []ManifestCapability `json:"capabilities"`
}

type ManifestCapability struct {
	CapabilityDefinition
	DeclaredSurfaces  []string           `json:"declared_surfaces,omitempty"`
	AvailableSurfaces []string           `json:"available_surfaces,omitempty"`
	Bindings          []TransportBinding `json:"bindings,omitempty"`
}

func Compile(definitions []CapabilityDefinition, bindings []TransportBinding, declaredSurfaces map[string][]string) (Manifest, error) {
	definitionByID := make(map[string]CapabilityDefinition, len(definitions))
	for _, raw := range definitions {
		definition := cloneDefinition(raw)
		if err := definition.Validate(); err != nil {
			return Manifest{}, err
		}
		if _, exists := definitionByID[definition.ID]; exists {
			return Manifest{}, fmt.Errorf("duplicate capability %q", definition.ID)
		}
		definitionByID[definition.ID] = definition
	}

	declaredByID := make(map[string][]string, len(declaredSurfaces))
	for capabilityID, surfaces := range declaredSurfaces {
		if _, exists := definitionByID[capabilityID]; !exists {
			return Manifest{}, fmt.Errorf("declared surfaces reference unknown capability %q", capabilityID)
		}
		declaredByID[capabilityID] = sortedUnique(surfaces)
	}

	bindingByID := make(map[string]struct{}, len(bindings))
	bindingsByCapability := make(map[string][]TransportBinding, len(definitions))
	for _, raw := range bindings {
		binding := cloneBinding(raw)
		if err := binding.Validate(); err != nil {
			return Manifest{}, err
		}
		if _, exists := bindingByID[binding.ID]; exists {
			return Manifest{}, fmt.Errorf("duplicate binding %q", binding.ID)
		}
		if _, exists := definitionByID[binding.CapabilityID]; !exists {
			return Manifest{}, fmt.Errorf("binding %q references unknown capability %q", binding.ID, binding.CapabilityID)
		}
		bindingByID[binding.ID] = struct{}{}
		bindingsByCapability[binding.CapabilityID] = append(bindingsByCapability[binding.CapabilityID], binding)
	}

	capabilityIDs := make([]string, 0, len(definitionByID))
	for capabilityID := range definitionByID {
		capabilityIDs = append(capabilityIDs, capabilityID)
	}
	sort.Strings(capabilityIDs)

	manifest := Manifest{SchemaVersion: ManifestSchemaVersion, Capabilities: make([]ManifestCapability, 0, len(capabilityIDs))}
	for _, capabilityID := range capabilityIDs {
		capabilityBindings := bindingsByCapability[capabilityID]
		sort.Slice(capabilityBindings, func(i, j int) bool { return capabilityBindings[i].ID < capabilityBindings[j].ID })

		// available_surfaces 只能从真实 available binding 推导，不能继承 legacy
		// surfaces。这样 planned/blocked/future-owner 不会被误报为当前可调用。
		availableSurfaces := make([]string, 0, len(capabilityBindings))
		for _, binding := range capabilityBindings {
			if binding.Availability == AvailabilityAvailable {
				availableSurfaces = append(availableSurfaces, binding.Transport.Surface())
			}
		}

		manifest.Capabilities = append(manifest.Capabilities, ManifestCapability{
			CapabilityDefinition: definitionByID[capabilityID],
			DeclaredSurfaces:     append([]string(nil), declaredByID[capabilityID]...),
			AvailableSurfaces:    sortedUnique(availableSurfaces),
			Bindings:             append([]TransportBinding(nil), capabilityBindings...),
		})
	}

	digest, err := manifest.computeDigest()
	if err != nil {
		return Manifest{}, err
	}
	manifest.Digest = digest
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("invalid manifest schema version %q", m.SchemaVersion)
	}

	capabilityIDs := make(map[string]struct{}, len(m.Capabilities))
	bindingIDs := make(map[string]struct{})
	previousCapabilityID := ""
	for _, capability := range m.Capabilities {
		if err := capability.Validate(); err != nil {
			return err
		}
		if _, exists := capabilityIDs[capability.ID]; exists {
			return fmt.Errorf("duplicate manifest capability %q", capability.ID)
		}
		if previousCapabilityID != "" && capability.ID < previousCapabilityID {
			return fmt.Errorf("manifest capabilities are not sorted")
		}
		previousCapabilityID = capability.ID
		capabilityIDs[capability.ID] = struct{}{}

		if !isSortedUnique(capability.DeclaredSurfaces) {
			return fmt.Errorf("capability %q declared_surfaces are not sorted and unique", capability.ID)
		}
		if !isSortedUnique(capability.AvailableSurfaces) {
			return fmt.Errorf("capability %q available_surfaces are not sorted and unique", capability.ID)
		}

		availableSurfaces := make([]string, 0, len(capability.Bindings))
		previousBindingID := ""
		for _, binding := range capability.Bindings {
			if err := binding.Validate(); err != nil {
				return err
			}
			if binding.CapabilityID != capability.ID {
				return fmt.Errorf("binding %q belongs to capability %q, not %q", binding.ID, binding.CapabilityID, capability.ID)
			}
			if _, exists := bindingIDs[binding.ID]; exists {
				return fmt.Errorf("duplicate manifest binding %q", binding.ID)
			}
			if previousBindingID != "" && binding.ID < previousBindingID {
				return fmt.Errorf("capability %q bindings are not sorted", capability.ID)
			}
			previousBindingID = binding.ID
			bindingIDs[binding.ID] = struct{}{}
			if binding.Availability == AvailabilityAvailable {
				availableSurfaces = append(availableSurfaces, binding.Transport.Surface())
			}
		}
		if got, want := strings.Join(capability.AvailableSurfaces, "\x00"), strings.Join(sortedUnique(availableSurfaces), "\x00"); got != want {
			return fmt.Errorf("capability %q available_surfaces do not match bindings", capability.ID)
		}
	}
	wantDigest, err := m.computeDigest()
	if err != nil {
		return err
	}
	if m.Digest != wantDigest {
		return fmt.Errorf("manifest digest mismatch: got %q, want %q", m.Digest, wantDigest)
	}
	return nil
}

func (m Manifest) computeDigest() (string, error) {
	payload := struct {
		SchemaVersion string               `json:"schema_version"`
		Capabilities  []ManifestCapability `json:"capabilities"`
	}{SchemaVersion: m.SchemaVersion, Capabilities: m.Capabilities}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode manifest digest payload: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func cloneDefinition(in CapabilityDefinition) CapabilityDefinition {
	out := in
	out.Errors = sortedUnique(in.Errors)
	return out
}

func cloneBinding(in TransportBinding) TransportBinding {
	out := in
	out.Blockers = sortedUnique(in.Blockers)
	if in.Readiness != nil {
		readiness := *in.Readiness
		readiness.Blockers = sortedUnique(in.Readiness.Blockers)
		readiness.NextActions = sortedUnique(in.Readiness.NextActions)
		readiness.EvidenceRefs = sortedUnique(in.Readiness.EvidenceRefs)
		out.Readiness = &readiness
	}
	return out
}

func validateIdentifier(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	for index, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == ':' || char == '-'
		if !valid || index == 0 && (char == '.' || char == '_' || char == ':' || char == '-') {
			return fmt.Errorf("%s %q is invalid", field, value)
		}
	}
	return nil
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isSortedUnique(values []string) bool {
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
		if index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}
