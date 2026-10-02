package pinaxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Typed supervised writer surface (pinax-human-reading-write-v1 W05). All
// calls go through the same bounded, redirect-rejecting transport as the
// reader client. Recovery discipline: every mutating call carries a durable
// identity, an ambiguous transport failure is recovered by re-issuing the
// SAME identity (the operation ledger replays the original receipt), and a
// fresh identity is never fabricated for a retry.

const (
	WriterProposalPreviewSchemaV1Alpha1      = catalogschema.WriterProposalPreviewV1Alpha1
	WriterApplyReceiptSchemaV1Alpha1         = catalogschema.WriterApplyReceiptV1Alpha1
	WriterCaptureReceiptSchemaV1Alpha1       = catalogschema.WriterCaptureReceiptV1Alpha1
	WriterOrganizePlanSchemaV1Alpha1         = catalogschema.WriterOrganizePlanV1Alpha1
	WriterOrganizeApplyReceiptSchemaV1Alpha1 = catalogschema.WriterOrganizeApplyReceiptV1Alpha1
)

// WriterChange mirrors the frozen writer change object exactly.
type WriterChange struct {
	OperationID      string   `json:"operation_id,omitempty"`
	Action           string   `json:"action"`
	NoteRef          string   `json:"note_ref,omitempty"`
	ExpectedRevision string   `json:"expected_revision,omitempty"`
	Title            string   `json:"title,omitempty"`
	Body             string   `json:"body,omitempty"`
	Dir              string   `json:"dir,omitempty"`
	TargetPath       string   `json:"target_path,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	TagOperation     string   `json:"tag_operation,omitempty"`
	Status           string   `json:"status,omitempty"`
}

// WriterOperationIdentity is the durable capture identity: re-issuing the
// same identity replays the original receipt instead of writing twice.
type WriterOperationIdentity struct {
	OperationID    string
	IdempotencyKey string
}

// NewWriterOperationIdentity validates the identity shape before any HTTP
// traffic: an invalid identity fails locally, never half-way through a
// write.
func NewWriterOperationIdentity(operationID, idempotencyKey string) (WriterOperationIdentity, error) {
	identity := WriterOperationIdentity{OperationID: strings.TrimSpace(operationID), IdempotencyKey: strings.TrimSpace(idempotencyKey)}
	if !operationIDPattern.MatchString(identity.OperationID) || !operationIDPattern.MatchString(identity.IdempotencyKey) {
		return WriterOperationIdentity{}, newError(CodeRequestInvalid, "Pinax writer identity must be a non-empty operation id and idempotency key", nil)
	}
	return identity, nil
}

// WriterProposalPreview is the exact before/after preview plus the digest
// apply must echo unchanged.
type WriterProposalPreview struct {
	SchemaVersion string       `json:"schema_version"`
	Change        WriterChange `json:"change"`
	PreviewDigest string       `json:"preview_digest"`
	Path          string       `json:"path"`
	Before        string       `json:"before"`
	After         string       `json:"after"`
	State         string       `json:"state"`
}

// WriterReceiptError mirrors the bounded receipt error block.
type WriterReceiptError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// WriterApplyReceipt is the frozen apply receipt (operation view shape).
type WriterApplyReceipt struct {
	SchemaVersion     string              `json:"schema_version"`
	OperationID       string              `json:"operation_id"`
	CapabilityID      string              `json:"capability_id"`
	BindingID         string              `json:"binding_id"`
	RequestDigest     string              `json:"request_digest,omitempty"`
	Status            string              `json:"status"`
	Retryable         bool                `json:"retryable"`
	ReplaySafe        bool                `json:"replay_safe"`
	ReconcileRequired bool                `json:"reconcile_required"`
	RequiredAction    string              `json:"required_action,omitempty"`
	Result            json.RawMessage     `json:"result,omitempty"`
	Receipt           json.RawMessage     `json:"receipt,omitempty"`
	ReceiptRef        string              `json:"receipt_ref,omitempty"`
	ResourceRef       string              `json:"resource_ref,omitempty"`
	RevisionBefore    string              `json:"revision_before,omitempty"`
	RevisionAfter     string              `json:"revision_after,omitempty"`
	Error             *WriterReceiptError `json:"error,omitempty"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
	AcceptedAt        string              `json:"accepted_at"`
	ApplyingAt        string              `json:"applying_at,omitempty"`
	CompletedAt       string              `json:"completed_at,omitempty"`
}

// Terminal reports whether the receipt reached a terminal status.
func (r WriterApplyReceipt) Terminal() bool {
	switch r.Status {
	case "succeeded", "failed", "reconcile_required":
		return true
	}
	return false
}

// WriterCaptureReceipt is the frozen capture receipt.
type WriterCaptureReceipt struct {
	SchemaVersion     string              `json:"schema_version"`
	OperationID       string              `json:"operation_id"`
	CapabilityID      string              `json:"capability_id"`
	BindingID         string              `json:"binding_id"`
	Status            string              `json:"status"`
	Retryable         bool                `json:"retryable"`
	ReplaySafe        bool                `json:"replay_safe"`
	ReconcileRequired bool                `json:"reconcile_required"`
	NoteID            string              `json:"note_id,omitempty"`
	Path              string              `json:"path,omitempty"`
	RecordEventID     string              `json:"record_event_id,omitempty"`
	RevisionAfter     string              `json:"revision_after,omitempty"`
	ResourceRef       string              `json:"resource_ref,omitempty"`
	Error             *WriterReceiptError `json:"error,omitempty"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
	AcceptedAt        string              `json:"accepted_at"`
}

// WriterOrganizeOperation is one planned organize operation.
type WriterOrganizeOperation struct {
	OperationID string `json:"operation_id"`
	ObjectID    string `json:"object_id,omitempty"`
	Kind        string `json:"kind"`
	Mode        string `json:"mode"`
	Risk        string `json:"risk"`
	Path        string `json:"path,omitempty"`
}

// WriterOrganizePlan is the frozen saved-plan summary.
type WriterOrganizePlan struct {
	SchemaVersion  string                    `json:"schema_version"`
	PlanID         string                    `json:"plan_id"`
	Status         string                    `json:"status"`
	CreatedAt      string                    `json:"created_at"`
	ExpiresAt      string                    `json:"expires_at"`
	OperationCount int                       `json:"operation_count"`
	Operations     []WriterOrganizeOperation `json:"operations"`
}

// WriterOrganizeApplyReceipt is the frozen organize apply receipt; the plan
// id doubles as the recovery identity.
type WriterOrganizeApplyReceipt struct {
	SchemaVersion     string `json:"schema_version"`
	OperationID       string `json:"operation_id"`
	PlanID            string `json:"plan_id"`
	CapabilityID      string `json:"capability_id"`
	BindingID         string `json:"binding_id"`
	Status            string `json:"status"`
	Retryable         bool   `json:"retryable"`
	ReplaySafe        bool   `json:"replay_safe"`
	ReconcileRequired bool   `json:"reconcile_required"`
	SnapshotID        string `json:"snapshot_id,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	AcceptedAt        string `json:"accepted_at"`
}

// WriterCaptureRequest is the bounded capture payload.
type WriterCaptureRequest struct {
	Title string
	Body  string
	Tags  []string
	Slug  string
	Dir   string
}

// WriterApplyRequest is the apply payload: the resolved change, its
// unchanged preview digest, and the caller-asserted authorization.
type WriterApplyRequest struct {
	Change        WriterChange
	PreviewDigest string
	Authorization string
}

// WriterOrganizeApplyRequest applies a saved plan id as-is.
type WriterOrganizeApplyRequest struct {
	PlanID          string
	SnapshotMessage string
	AllowStale      bool
}

func writerSchemaError(expected string) error {
	return newError(CodeUpstreamInvalidResponse, fmt.Sprintf("Pinax writer receipt schema version is unsupported (want %s)", expected), nil)
}

// postWriterJSON issues a bounded POST with writer headers through the
// shared projection transport.
func (c *Client) postWriterJSON(ctx context.Context, path string, payload any, headers map[string]string) (Projection, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Projection{}, newError(CodeRequestInvalid, "Pinax writer request could not be encoded", err)
	}
	return c.doProjectionHeaders(ctx, http.MethodPost, path, strings.NewReader(string(encoded)), "application/json", headers)
}

// writerOutcomeUnknown reports an ambiguous write outcome after the
// same-identity recovery attempt failed too: the host must keep the
// original identity and recover it (re-issue or owner-local inspection);
// a fresh identity would be a blind retry.
func writerOutcomeUnknown(operationID string, ambiguousErr error) error {
	return &Error{
		Code: CodeMutationOutcomeUnknown, Message: "Pinax writer outcome could not be proven",
		RequiredAction: "Re-issue with the same operation identity (the ledger replays the receipt); never retry with a fresh identity",
		OperationRef:   operationID, ReconcileRequired: true, cause: ambiguousErr,
	}
}

// WriterCapture captures a note under a durable identity. On an ambiguous
// transport failure the SAME identity is re-issued once — the operation
// ledger replays the original receipt instead of writing twice.
func (c *Client) WriterCapture(ctx context.Context, request WriterCaptureRequest, identity WriterOperationIdentity) (WriterCaptureReceipt, error) {
	if !operationIDPattern.MatchString(identity.OperationID) || !operationIDPattern.MatchString(identity.IdempotencyKey) {
		return WriterCaptureReceipt{}, newError(CodeRequestInvalid, "Pinax writer capture requires a valid operation identity", nil)
	}
	payload := map[string]any{"title": request.Title}
	if request.Body != "" {
		payload["body"] = request.Body
	}
	if request.Tags != nil {
		payload["tags"] = request.Tags
	}
	if request.Slug != "" {
		payload["slug"] = request.Slug
	}
	if request.Dir != "" {
		payload["dir"] = request.Dir
	}
	headers := map[string]string{
		"X-Pinax-Operation-ID": identity.OperationID,
		"Idempotency-Key":      identity.IdempotencyKey,
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/capture", payload, headers)
	if err != nil {
		if !ambiguousMutationError(err) {
			return WriterCaptureReceipt{}, err
		}
		projection, err = c.postWriterJSON(ctx, "/v1/writer/capture", payload, headers)
		if err != nil {
			return WriterCaptureReceipt{}, writerOutcomeUnknown(identity.OperationID, err)
		}
	}
	var receipt WriterCaptureReceipt
	if err := decodeProjectionData(projection, &receipt); err != nil {
		return WriterCaptureReceipt{}, err
	}
	if receipt.SchemaVersion != WriterCaptureReceiptSchemaV1Alpha1 || receipt.OperationID != identity.OperationID {
		return WriterCaptureReceipt{}, writerSchemaError(WriterCaptureReceiptSchemaV1Alpha1)
	}
	return receipt, nil
}

// WriterPropose prepares an edit proposal without writing.
func (c *Client) WriterPropose(ctx context.Context, change WriterChange) (WriterProposalPreview, error) {
	if strings.TrimSpace(change.Action) == "" {
		return WriterProposalPreview{}, newError(CodeRequestInvalid, "Pinax writer propose requires a change action", nil)
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/proposals", map[string]any{"change": change}, nil)
	if err != nil {
		return WriterProposalPreview{}, err
	}
	var preview WriterProposalPreview
	if err := decodeProjectionData(projection, &preview); err != nil {
		return WriterProposalPreview{}, err
	}
	if preview.SchemaVersion != WriterProposalPreviewSchemaV1Alpha1 {
		return WriterProposalPreview{}, writerSchemaError(WriterProposalPreviewSchemaV1Alpha1)
	}
	return preview, nil
}

// WriterApply saves an unchanged preview. The operation identity is the
// change's operation id: an ambiguous failure is recovered by re-issuing
// the SAME apply (the ledger replays), never a fresh identity.
func (c *Client) WriterApply(ctx context.Context, request WriterApplyRequest) (WriterApplyReceipt, error) {
	if strings.TrimSpace(request.Change.OperationID) == "" {
		return WriterApplyReceipt{}, newError(CodeRequestInvalid, "Pinax writer apply requires change.operation_id as the durable identity", nil)
	}
	if request.Authorization != "explicit_instruction" && request.Authorization != "reviewed_proposal" {
		return WriterApplyReceipt{}, newError(CodeRequestInvalid, "Pinax writer apply authorization must be an explicit caller assertion", nil)
	}
	payload := map[string]any{"change": request.Change, "preview_digest": request.PreviewDigest, "authorization": request.Authorization}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/proposals:apply", payload, nil)
	if err != nil {
		if !ambiguousMutationError(err) {
			return WriterApplyReceipt{}, err
		}
		projection, err = c.postWriterJSON(ctx, "/v1/writer/proposals:apply", payload, nil)
		if err != nil {
			return WriterApplyReceipt{}, writerOutcomeUnknown(request.Change.OperationID, err)
		}
	}
	var receipt WriterApplyReceipt
	if err := decodeProjectionData(projection, &receipt); err != nil {
		return WriterApplyReceipt{}, err
	}
	if receipt.SchemaVersion != WriterApplyReceiptSchemaV1Alpha1 || receipt.OperationID != request.Change.OperationID {
		return WriterApplyReceipt{}, writerSchemaError(WriterApplyReceiptSchemaV1Alpha1)
	}
	return receipt, nil
}

// isWriterOutcomeUnknown reports an ambiguous write whose same-identity
// recovery attempt also failed ambiguously: the identity stays durable, so
// further polls of the SAME identity remain safe.
func isWriterOutcomeUnknown(err error) bool {
	writerErr, ok := err.(*Error)
	return ok && writerErr.Code == CodeMutationOutcomeUnknown
}

// WriterAwaitApply polls the SAME apply identity until a terminal receipt,
// the attempt budget is exhausted, or the context is cancelled. Polling
// never fabricates a fresh identity. A non-terminal receipt (accepted /
// applying) keeps polling under the SAME identity until it reaches a
// terminal status, the budget is exhausted, or the context is cancelled.
func (c *Client) WriterAwaitApply(ctx context.Context, request WriterApplyRequest, attempts int, interval time.Duration) (WriterApplyReceipt, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	var lastReceipt WriterApplyReceipt
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return lastReceipt, ctx.Err()
			case <-time.After(interval):
			}
		}
		receipt, err := c.WriterApply(ctx, request)
		if err == nil {
			if receipt.Terminal() {
				return receipt, nil
			}
			// Accepted/applying: the apply is in flight under the same
			// identity — poll again instead of returning early.
			lastReceipt = receipt
			lastErr = newError(CodeMutationOutcomeUnknown, "Pinax writer apply is still in flight", nil)
			continue
		}
		lastErr = err
		if !ambiguousMutationError(err) && !isWriterOutcomeUnknown(err) {
			return receipt, err
		}
	}
	return lastReceipt, lastErr
}

// WriterOrganizePlan generates a saved organize plan; noteRefs scopes the
// plan to exactly those objects, empty means the whole vault.
func (c *Client) WriterOrganizePlan(ctx context.Context, noteRefs []string) (WriterOrganizePlan, error) {
	payload := map[string]any{}
	if len(noteRefs) > 0 {
		payload["note_refs"] = noteRefs
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/organize/plan", payload, nil)
	if err != nil {
		return WriterOrganizePlan{}, err
	}
	var plan WriterOrganizePlan
	if err := decodeProjectionData(projection, &plan); err != nil {
		return WriterOrganizePlan{}, err
	}
	if plan.SchemaVersion != WriterOrganizePlanSchemaV1Alpha1 || plan.PlanID == "" {
		return WriterOrganizePlan{}, writerSchemaError(WriterOrganizePlanSchemaV1Alpha1)
	}
	return plan, nil
}

// WriterOrganizeApply applies a saved plan id as-is. Organize plans are
// consumed by apply, so an ambiguous outcome is NOT auto-retried: the
// receipt/error tells the host to inspect the plan id instead.
func (c *Client) WriterOrganizeApply(ctx context.Context, request WriterOrganizeApplyRequest) (WriterOrganizeApplyReceipt, error) {
	if strings.TrimSpace(request.PlanID) == "" {
		return WriterOrganizeApplyReceipt{}, newError(CodeRequestInvalid, "Pinax writer organize apply requires a plan id", nil)
	}
	payload := map[string]any{"plan_id": request.PlanID}
	if strings.TrimSpace(request.SnapshotMessage) != "" {
		payload["snapshot_message"] = request.SnapshotMessage
	}
	if request.AllowStale {
		payload["allow_stale"] = true
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/organize/apply", payload, nil)
	if err != nil {
		if ambiguousMutationError(err) {
			return WriterOrganizeApplyReceipt{}, &Error{
				Code: CodeMutationOutcomeUnknown, Message: "Pinax writer organize apply outcome could not be proven",
				RequiredAction: "Inspect the plan id (pinax pipeline show) before any re-apply; the plan is consumed by apply",
				OperationRef:   request.PlanID, ReconcileRequired: true, cause: err,
			}
		}
		return WriterOrganizeApplyReceipt{}, err
	}
	var receipt WriterOrganizeApplyReceipt
	if err := decodeProjectionData(projection, &receipt); err != nil {
		return WriterOrganizeApplyReceipt{}, err
	}
	if receipt.SchemaVersion != WriterOrganizeApplyReceiptSchemaV1Alpha1 || receipt.PlanID != request.PlanID {
		return WriterOrganizeApplyReceipt{}, writerSchemaError(WriterOrganizeApplyReceiptSchemaV1Alpha1)
	}
	return receipt, nil
}
