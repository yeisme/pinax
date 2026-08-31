package pinaxclient

import "encoding/json"

const (
	ProjectionSpecVersion       = "1.0"
	TransportManifestSchemaV1   = "pinax.transport_manifest.v1"
	ConnectionReadinessSchemaV1 = "pinax.connection_readiness.v1"
	OperationSchemaV1           = "pinax.operation.v1"

	ReadinessLayerContract         = "contract"
	ReadinessLayerTransport        = "transport"
	ReadinessLayerAuth             = "auth"
	ReadinessLayerOwner            = "owner"
	ReadinessLayerMutationRecovery = "mutation_recovery"
	ReadinessLayerProduction       = "production"
)

var requiredReadinessLayers = []string{
	ReadinessLayerContract,
	ReadinessLayerTransport,
	ReadinessLayerAuth,
	ReadinessLayerOwner,
	ReadinessLayerMutationRecovery,
	ReadinessLayerProduction,
}

// ReadinessLayerNames returns the required v1 layer names without exposing
// mutable package state.
func ReadinessLayerNames() []string {
	return append([]string(nil), requiredReadinessLayers...)
}

type Action struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type CommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Projection struct {
	SpecVersion string            `json:"spec_version"`
	Mode        string            `json:"mode,omitempty"`
	Command     string            `json:"command"`
	Status      string            `json:"status"`
	Summary     string            `json:"summary,omitempty"`
	Facts       map[string]string `json:"facts,omitempty"`
	Actions     []Action          `json:"actions,omitempty"`
	Evidence    []string          `json:"evidence,omitempty"`
	Data        json.RawMessage   `json:"data,omitempty"`
	Warnings    []Warning         `json:"warnings,omitempty"`
	Error       *CommandError     `json:"error,omitempty"`
}

type Manifest struct {
	SchemaVersion string               `json:"schema_version"`
	Digest        string               `json:"digest"`
	Capabilities  []ManifestCapability `json:"capabilities"`
}

type ManifestCapability struct {
	ID                  string             `json:"id"`
	Command             string             `json:"command"`
	ReleaseCore         bool               `json:"release_core"`
	Readonly            bool               `json:"readonly"`
	BodyAllowed         bool               `json:"body_allowed"`
	ApprovalRequired    bool               `json:"approval_required"`
	SnapshotRequired    bool               `json:"snapshot_required"`
	UIGroup             string             `json:"ui_group,omitempty"`
	BodyExposureDefault string             `json:"body_exposure_default,omitempty"`
	WriteGate           string             `json:"write_gate,omitempty"`
	CopyCommand         string             `json:"copy_command,omitempty"`
	LocalOnlyReason     string             `json:"local_only_reason,omitempty"`
	RequestSchema       string             `json:"request_schema"`
	ResponseSchema      string             `json:"response_schema"`
	Errors              []string           `json:"errors,omitempty"`
	Stability           string             `json:"stability"`
	DeclaredSurfaces    []string           `json:"declared_surfaces,omitempty"`
	AvailableSurfaces   []string           `json:"available_surfaces,omitempty"`
	Bindings            []TransportBinding `json:"bindings,omitempty"`
}

type TransportBinding struct {
	ID             string     `json:"id"`
	CapabilityID   string     `json:"capability_id"`
	Transport      string     `json:"transport"`
	Availability   string     `json:"availability"`
	BackingRef     string     `json:"backing_ref,omitempty"`
	Method         string     `json:"method,omitempty"`
	Path           string     `json:"path,omitempty"`
	ProtocolName   string     `json:"protocol_name,omitempty"`
	Readonly       bool       `json:"readonly"`
	WriteGate      string     `json:"write_gate,omitempty"`
	RequestSchema  string     `json:"request_schema"`
	ResponseSchema string     `json:"response_schema"`
	Blockers       []string   `json:"blockers,omitempty"`
	Readiness      *Readiness `json:"readiness,omitempty"`
}

type Readiness struct {
	Status       string   `json:"status"`
	Maturity     string   `json:"maturity"`
	Blockers     []string `json:"blockers,omitempty"`
	NextActions  []string `json:"next_actions,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

// IsReady returns true only for the stable ready status. Unknown future status
// values remain available in Status but are deliberately treated as non-ready.
func (r Readiness) IsReady() bool {
	return r.Status == "ready"
}

func (r Readiness) StatusKnown() bool {
	switch r.Status {
	case "ready", "degraded", "blocked", "not_configured", "not_applicable":
		return true
	default:
		return false
	}
}

type ConnectionReadiness struct {
	SchemaVersion string               `json:"schema_version"`
	Overall       Readiness            `json:"overall"`
	Layers        map[string]Readiness `json:"layers"`
}

type Operation struct {
	SchemaVersion     string          `json:"schema_version"`
	OperationID       string          `json:"operation_id"`
	CapabilityID      string          `json:"capability_id"`
	BindingID         string          `json:"binding_id"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	RequestDigest     string          `json:"request_digest,omitempty"`
	Status            string          `json:"status"`
	Retryable         bool            `json:"retryable"`
	ReplaySafe        bool            `json:"replay_safe"`
	ReconcileRequired bool            `json:"reconcile_required"`
	RequiredAction    string          `json:"required_action,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	Receipt           json.RawMessage `json:"receipt,omitempty"`
	ReceiptRef        string          `json:"receipt_ref,omitempty"`
	ResourceRef       string          `json:"resource_ref,omitempty"`
	RevisionBefore    string          `json:"revision_before,omitempty"`
	RevisionAfter     string          `json:"revision_after,omitempty"`
	Error             *CommandError   `json:"error,omitempty"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	AcceptedAt        string          `json:"accepted_at"`
	ApplyingAt        string          `json:"applying_at,omitempty"`
	CompletedAt       string          `json:"completed_at,omitempty"`
}

type RPCRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type MutationIdentity struct {
	OperationID    string `json:"operation_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
