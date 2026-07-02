package domain

const RemoteCapabilitySchemaVersion = "pinax.remote.capability.v1"

type RemoteCapability struct {
	SchemaVersion       string   `json:"schema_version"`
	ID                  string   `json:"id"`
	Surfaces            []string `json:"surfaces"`
	Command             string   `json:"command"`
	ReleaseCore         bool     `json:"release_core"`
	Readonly            bool     `json:"readonly"`
	BodyAllowed         bool     `json:"body_allowed"`
	ApprovalRequired    bool     `json:"approval_required"`
	SnapshotRequired    bool     `json:"snapshot_required"`
	UIGroup             string   `json:"ui_group,omitempty"`
	BodyExposureDefault string   `json:"body_exposure_default,omitempty"`
	WriteGate           string   `json:"write_gate,omitempty"`
	CopyCommand         string   `json:"copy_command,omitempty"`
	LocalOnlyReason     string   `json:"local_only_reason,omitempty"`
	RequestSchema       string   `json:"request_schema"`
	ResponseSchema      string   `json:"response_schema"`
	Errors              []string `json:"errors,omitempty"`
}

type RemoteRoute struct {
	RouteID             string   `json:"route_id"`
	Surface             string   `json:"surface"`
	Method              string   `json:"method"`
	Path                string   `json:"path,omitempty"`
	RPCMethod           string   `json:"rpc_method,omitempty"`
	Command             string   `json:"command"`
	CapabilityID        string   `json:"capability_id"`
	SchemaVersion       string   `json:"schema_version"`
	ReleaseCore         bool     `json:"release_core"`
	Readonly            bool     `json:"readonly"`
	BodyAllowed         bool     `json:"body_allowed"`
	ApprovalRequired    bool     `json:"approval_required"`
	SnapshotRequired    bool     `json:"snapshot_required"`
	UIGroup             string   `json:"ui_group,omitempty"`
	BodyExposureDefault string   `json:"body_exposure_default,omitempty"`
	WriteGate           string   `json:"write_gate,omitempty"`
	CopyCommand         string   `json:"copy_command,omitempty"`
	LocalOnlyReason     string   `json:"local_only_reason,omitempty"`
	Errors              []string `json:"errors,omitempty"`
}
