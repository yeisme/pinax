package app

import (
	"context"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Supervised writer projection (pinax-human-reading-write-v1 W02).
//
// Writer scopes live in the same supervised session registry as reader
// scopes but are granted independently: writer.capture, writer.editor and
// writer.organizer never widen a legacy read/write/admin token, and request
// fields never upgrade a session. The capture projection reuses the owner
// inbox.capture operation semantics — approval plus operation identity and
// idempotency — without a second write path.

// Writer scope vocabulary mirrors the frozen catalog constants.
const (
	WriterScopeCapture   = catalogschema.WriterScopeCapture
	WriterScopeEditor    = catalogschema.WriterScopeEditor
	WriterScopeOrganizer = catalogschema.WriterScopeOrganizer
)

// IsWriterScope reports whether a scope belongs to the supervised writer
// vocabulary (the session bootstrap allowlist extension).
func IsWriterScope(scope string) bool {
	switch scope {
	case WriterScopeCapture, WriterScopeEditor, WriterScopeOrganizer:
		return true
	}
	return false
}

// WriterCaptureRequest is the frozen writer.capture payload
// (pinax.writer.capture.request.v1alpha1): bounded capture content only.
// Approval is not a request field — the host grants the writer.capture
// scope and the caller supplies the operation identity.
type WriterCaptureRequest struct {
	VaultPath string
	Title     string
	Body      string
	Tags      []string
	Slug      string
	Dir       string
}

// writerCaptureBindingID is the REST binding the projection runs under.
const writerCaptureBindingID = "rest.writer.capture"

// WriterCapture projects inbox.capture under the supervised writer gate.
// Operation identity is REQUIRED (unlike the legacy REST surface): a
// capture without X-Pinax-Operation-ID plus Idempotency-Key never enters
// the ledger and fails closed with operation_identity_required.
func (s *Service) WriterCapture(ctx context.Context, req WriterCaptureRequest, identity RemoteOperationIdentity) (domain.Projection, error) {
	operationID := strings.TrimSpace(identity.OperationID)
	idempotencyKey := strings.TrimSpace(identity.IdempotencyKey)
	if operationID == "" || idempotencyKey == "" {
		err := &domain.CommandError{
			Code:    "operation_identity_required",
			Message: "Writer capture requires X-Pinax-Operation-ID and Idempotency-Key",
			Hint:    "Reuse the same identity to recover the receipt after a timeout; never retry with a fresh identity",
		}
		return domain.NewErrorProjection("writer.capture", err), err
	}
	identity.OperationID = operationID
	identity.IdempotencyKey = idempotencyKey
	if strings.TrimSpace(identity.BindingID) == "" {
		identity.BindingID = writerCaptureBindingID
	}
	projection, err := s.InboxCaptureRemote(ctx, CreateNoteRequest{
		VaultPath: req.VaultPath, Title: req.Title, Body: req.Body,
		Tags: req.Tags, Slug: req.Slug, Dir: req.Dir,
	}, identity)
	if projection.Command == "inbox.capture" {
		projection.Command = "writer.capture"
	}
	return projection, err
}

// WriterOperationAccess derives the operation principal/scope digests for a
// supervised writer session. The session (not the owner-local process) is
// the principal; the granted writer scopes are the scope identity, so
// operation receipts stay attributable and recoverable per session.
func WriterOperationAccess(sessionID, bindingID string, scopes []string) RemoteOperationIdentity {
	granted := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if IsWriterScope(scope) {
			granted = append(granted, scope)
		}
	}
	return RemoteOperationIdentity{
		BindingID: writerCaptureBindingID,
		Access: OperationAccess{
			PrincipalDigest: operation.IdentityDigest("pinax-writer-session", sessionID, bindingID),
			ScopeDigest:     operation.IdentityDigest(append([]string{"pinax-writer-scopes"}, granted...)...),
		},
	}
}
