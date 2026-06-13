package domain

const RemoteCapabilitySchemaVersion = "pinax.remote.capability.v1"

type RemoteCapability struct {
	SchemaVersion    string   `json:"schema_version"`
	ID               string   `json:"id"`
	Surfaces         []string `json:"surfaces"`
	Command          string   `json:"command"`
	Readonly         bool     `json:"readonly"`
	BodyAllowed      bool     `json:"body_allowed"`
	ApprovalRequired bool     `json:"approval_required"`
	SnapshotRequired bool     `json:"snapshot_required"`
	RequestSchema    string   `json:"request_schema"`
	ResponseSchema   string   `json:"response_schema"`
	Errors           []string `json:"errors,omitempty"`
}

type RemoteRoute struct {
	RouteID          string   `json:"route_id"`
	Surface          string   `json:"surface"`
	Method           string   `json:"method"`
	Path             string   `json:"path,omitempty"`
	RPCMethod        string   `json:"rpc_method,omitempty"`
	Command          string   `json:"command"`
	CapabilityID     string   `json:"capability_id"`
	SchemaVersion    string   `json:"schema_version"`
	Readonly         bool     `json:"readonly"`
	BodyAllowed      bool     `json:"body_allowed"`
	ApprovalRequired bool     `json:"approval_required"`
	SnapshotRequired bool     `json:"snapshot_required"`
	Errors           []string `json:"errors,omitempty"`
}
