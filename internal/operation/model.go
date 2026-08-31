package operation

import (
	"encoding/json"
	"time"
)

const (
	SchemaVersion = "pinax.operation.v1"

	MaxResultBytes  = 64 << 10
	MaxReceiptBytes = 16 << 10
)

type Status string

const (
	StatusAccepted          Status = "accepted"
	StatusApplying          Status = "applying"
	StatusSucceeded         Status = "succeeded"
	StatusFailed            Status = "failed"
	StatusReconcileRequired Status = "reconcile_required"
)

// OperationRow is the durable owner-side record for one logical mutation.
// Result and receipt payloads are bounded, redacted JSON projections; request
// bodies, credentials, provider payloads and absolute vault paths do not belong
// in this table.
type OperationRow struct {
	OperationID     string `gorm:"primaryKey;size:128"`
	CapabilityID    string `gorm:"size:128;not null;index"`
	BindingID       string `gorm:"size:160;not null;index"`
	PrincipalDigest string `gorm:"size:80;not null;index:idx_operation_scope,priority:1"`
	ScopeDigest     string `gorm:"size:80;not null;index:idx_operation_scope,priority:2"`
	RequestDigest   string `gorm:"size:80;not null"`
	Status          Status `gorm:"size:32;not null;index"`
	ReplaySafe      bool
	Retryable       bool
	ResultJSON      []byte `gorm:"type:blob"`
	ReceiptJSON     []byte `gorm:"type:blob"`
	ReceiptRef      string `gorm:"size:512"`
	ResourceRef     string `gorm:"size:512"`
	RevisionBefore  string `gorm:"size:160"`
	RevisionAfter   string `gorm:"size:160"`
	ErrorCode       string `gorm:"size:128"`
	ErrorMessage    string `gorm:"size:512"`
	ReconcileCount  uint
	CreatedAt       time.Time
	UpdatedAt       time.Time
	AcceptedAt      time.Time
	ApplyingAt      *time.Time
	CompletedAt     *time.Time
}

func (OperationRow) TableName() string { return "api_operations" }

type IdempotencyRow struct {
	ID              uint   `gorm:"primaryKey"`
	PrincipalDigest string `gorm:"size:80;not null;uniqueIndex:idx_operation_idempotency_binding,priority:1"`
	ScopeDigest     string `gorm:"size:80;not null;uniqueIndex:idx_operation_idempotency_binding,priority:2"`
	IdempotencyKey  string `gorm:"size:128;not null;uniqueIndex:idx_operation_idempotency_binding,priority:3"`
	CapabilityID    string `gorm:"size:128;not null"`
	BindingID       string `gorm:"size:160;not null"`
	OperationID     string `gorm:"size:128;not null;uniqueIndex"`
	RequestDigest   string `gorm:"size:80;not null"`
	CreatedAt       time.Time
}

func (IdempotencyRow) TableName() string { return "api_operation_idempotency" }

type CreateRequest struct {
	OperationID     string
	IdempotencyKey  string
	CapabilityID    string
	BindingID       string
	PrincipalDigest string
	ScopeDigest     string
	RequestDigest   string
	RevisionBefore  string
}

type CreateResult struct {
	Operation OperationRow
	Replay    bool
}

type Outcome struct {
	Result        json.RawMessage
	Receipt       json.RawMessage
	ReceiptRef    string
	ResourceRef   string
	RevisionAfter string
	ErrorCode     string
	ErrorMessage  string
	Retryable     bool
	ReplaySafe    bool
}

type ViewError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type View struct {
	SchemaVersion     string          `json:"schema_version"`
	OperationID       string          `json:"operation_id"`
	CapabilityID      string          `json:"capability_id"`
	BindingID         string          `json:"binding_id"`
	RequestDigest     string          `json:"request_digest,omitempty"`
	Status            Status          `json:"status"`
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
	Error             *ViewError      `json:"error,omitempty"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	AcceptedAt        string          `json:"accepted_at"`
	ApplyingAt        string          `json:"applying_at,omitempty"`
	CompletedAt       string          `json:"completed_at,omitempty"`
}

func ViewFromRow(row OperationRow) View {
	view := View{
		SchemaVersion: SchemaVersion, OperationID: row.OperationID,
		CapabilityID: row.CapabilityID, BindingID: row.BindingID,
		RequestDigest: row.RequestDigest, Status: row.Status,
		Retryable: row.Retryable, ReplaySafe: row.ReplaySafe,
		ReconcileRequired: row.Status == StatusApplying || row.Status == StatusReconcileRequired,
		Result:            append(json.RawMessage(nil), row.ResultJSON...), Receipt: append(json.RawMessage(nil), row.ReceiptJSON...),
		ReceiptRef: row.ReceiptRef, ResourceRef: row.ResourceRef,
		RevisionBefore: row.RevisionBefore, RevisionAfter: row.RevisionAfter,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		AcceptedAt: row.AcceptedAt.UTC().Format(time.RFC3339Nano),
	}
	if view.ReconcileRequired {
		view.RequiredAction = "inspect_and_reconcile"
	}
	if row.ApplyingAt != nil {
		view.ApplyingAt = row.ApplyingAt.UTC().Format(time.RFC3339Nano)
	}
	if row.CompletedAt != nil {
		view.CompletedAt = row.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	if row.ErrorCode != "" || row.ErrorMessage != "" {
		view.Error = &ViewError{Code: row.ErrorCode, Message: row.ErrorMessage}
	}
	return view
}
