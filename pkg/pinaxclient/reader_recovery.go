package pinaxclient

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// ReaderRecoveryHandle is the original operation identity retained by the
// caller, not authority to read it. Only OperationID goes over HTTP. Private
// host approval, current session authentication and grant expiry stay owner-side.
type ReaderRecoveryHandle struct {
	OperationID   string
	CapabilityID  string
	BindingID     string
	RequestDigest string
}

// ReaderRecoveryLookup reads original evidence without retrying, applying or
// reconciling. A failed read does not establish that a mutation never happened.
// Result and Receipt remain inert bounded JSON, not executable instructions.
func (c *Client) ReaderRecoveryLookup(ctx context.Context, original ReaderRecoveryHandle) (Operation, error) {
	empty := Operation{}
	payload := map[string]any{"operation_id": original.OperationID}
	validCapability := false
	switch original.CapabilityID {
	case "writer.apply", "writer.daily.apply", "writer.daily.edit", "writer.tags.apply":
		validCapability = true
	}
	if !validCapability || original.BindingID != "rest."+original.CapabilityID || !validSHA256Digest(original.RequestDigest) || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderRecoveryLookupRequestV1Alpha1, payload) != nil {
		return empty, newError(CodeRequestInvalid, "Recovery requires the original fixed operation handle", nil)
	}
	bounded := *c
	if bounded.maxResponseBytes > 128*1024 {
		bounded.maxResponseBytes = 128 * 1024
	}
	projection, err := bounded.getReaderProjection(ctx, "/v1/reader/recovery/operation", url.Values{"operation_id": {original.OperationID}})
	if err != nil {
		return empty, err
	}
	bad := func() (Operation, error) {
		return empty, newError(CodeUpstreamInvalidResponse, "Recovery evidence identity, state or budget is inconsistent", nil)
	}
	var raw map[string]any
	if projection.Command != "reader.recovery.lookup" || json.Unmarshal(projection.Data, &raw) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.OperationV1, raw) != nil {
		return bad()
	}
	// The wider legacy operation schema permits this field; the recovery owner
	// View never emits it and must not leak an old idempotency credential.
	if _, exists := raw["idempotency_key"]; exists {
		return bad()
	}
	var view Operation
	if json.Unmarshal(projection.Data, &view) != nil || view.OperationID != original.OperationID || view.CapabilityID != original.CapabilityID || view.BindingID != original.BindingID || view.RequestDigest != original.RequestDigest || !validRecoveryView(view) {
		return bad()
	}
	return view, nil
}

func validRecoveryView(v Operation) bool {
	if len(v.Result) > 64*1024 || len(v.Receipt) > 16*1024 || len(v.ReceiptRef) > 512 || len(v.ResourceRef) > 512 {
		return false
	}
	if v.Error != nil && (len(v.Error.Code) > 128 || len(v.Error.Message) > 512 || v.Error.Hint != "") {
		return false
	}
	for _, rev := range []string{v.RevisionBefore, v.RevisionAfter} {
		if rev != "" && !validSHA256Digest(rev) {
			return false
		}
	}
	for _, stamp := range []string{v.CreatedAt, v.UpdatedAt, v.AcceptedAt} {
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			return false
		}
	}
	for _, stamp := range []string{v.ApplyingAt, v.CompletedAt} {
		if stamp != "" {
			if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
				return false
			}
		}
	}
	needsReconcile := v.Status == "applying" || v.Status == "reconcile_required"
	if v.ReconcileRequired != needsReconcile {
		return false
	}
	if needsReconcile {
		if v.RequiredAction != "inspect_and_reconcile" {
			return false
		}
	} else if v.RequiredAction != "" {
		return false
	}
	switch v.Status {
	case "accepted":
		return v.ApplyingAt == "" && v.CompletedAt == "" && !v.Retryable && !v.ReplaySafe
	case "applying", "reconcile_required":
		return v.ApplyingAt != "" && v.CompletedAt == "" && !v.Retryable && !v.ReplaySafe
	case "succeeded":
		return v.ApplyingAt != "" && v.CompletedAt != "" && v.Error == nil && !v.Retryable && !v.ReplaySafe
	case "failed":
		return v.ApplyingAt != "" && v.CompletedAt != ""
	default:
		return false
	}
}
