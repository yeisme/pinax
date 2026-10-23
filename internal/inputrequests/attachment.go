package inputrequests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/inputcontract"
	intake "github.com/yeisme/runtime-plane/pkg/inputintake"
)

func (s *Service) stagedAttachment(ctx context.Context, who intake.Identity, id string) (app.StagedAttachment, error) {
	var empty app.StagedAttachment
	view, err := s.Status(ctx, who, id)
	if err != nil {
		return empty, err
	}
	if view.State != "ready" || view.Purpose != "attachment" || view.File == nil || view.Receipt == nil {
		return empty, intake.ErrConflict
	}
	var previous adoption
	if err := s.repo.db.WithContext(ctx).Where(&adoption{ID: id}).First(&previous).Error; err == nil {
		return empty, intake.ErrConflict
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return empty, err
	}
	root, err := os.OpenRoot(filepath.Join(s.root, ".pinax", "input-files"))
	if err != nil {
		return empty, err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(id)
	if err != nil {
		return empty, err
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return empty, err
	}
	if int64(len(body)) != view.File.Size {
		return empty, intake.ErrInvalid
	}
	return app.StagedAttachment{Name: view.File.Name, MIME: view.File.MIME, SHA256: view.File.SHA256, Bytes: body}, nil
}

func (s *Service) AttachmentPreview(ctx context.Context, who intake.Identity, request inputcontract.AttachmentRequest) (inputcontract.AttachmentProposal, error) {
	file, err := s.stagedAttachment(ctx, who, request.InputRequestID)
	if err != nil {
		return inputcontract.AttachmentProposal{}, err
	}
	return s.app.InputAttachmentPreview(ctx, s.root, request, file)
}

func (s *Service) AttachmentApply(ctx context.Context, who intake.Identity, request inputcontract.AttachmentApply) (operation.View, error) {
	file, err := s.stagedAttachment(ctx, who, request.Proposal.Request.InputRequestID)
	if err != nil {
		return operation.View{}, err
	}
	return s.app.InputAttachmentApply(ctx, s.root, request, file, app.OperationAccess{
		PrincipalDigest: operation.IdentityDigest("input-actor", who.Actor),
		ScopeDigest:     operation.IdentityDigest("input-project", s.root, who.Project),
	})
}

// AttachmentOperation works after the upload ticket expires. It checks the
// original actor/project scope and never replays the adoption or upload.
func (s *Service) AttachmentOperation(ctx context.Context, who intake.Identity, id string, reconcile bool) (operation.View, error) {
	req := app.OperationRequest{VaultPath: s.root, OperationID: id, Access: app.OperationAccess{
		PrincipalDigest: operation.IdentityDigest("input-actor", who.Actor), ScopeDigest: operation.IdentityDigest("input-project", s.root, who.Project),
	}}
	projection, err := s.app.OperationShow(ctx, req)
	if reconcile && err == nil {
		projection, err = s.app.OperationReconcile(ctx, req)
	}
	if err != nil {
		return operation.View{}, err
	}
	data, _ := projection.Data.(map[string]any)
	view, _ := data["operation"].(operation.View)
	return view, nil
}

// Keep the original input.preview/apply surface on the same adoption engine.
// The legacy digest now binds full source metadata, not only the body.
func (s *Service) legacyAttachmentPreview(ctx context.Context, who intake.Identity, id, note string, confirm bool, expected string) (Preview, error) {
	opID := "input-" + id
	resolved, revision, err := s.app.CollaborationRead(ctx, s.root, note, "edit", app.CollaborationPolicy{AllowBody: true, AllowWrite: true})
	if err != nil {
		return Preview{}, err
	}
	if view, err := s.AttachmentOperation(ctx, who, opID, false); err == nil {
		var result struct {
			PreviewDigest string `json:"preview_digest"`
			NoteID        string `json:"note_id"`
		}
		_ = json.Unmarshal(view.Result, &result)
		if result.NoteID != resolved.ID || (confirm && expected != result.PreviewDigest) {
			return Preview{}, intake.ErrConflict
		}
		state := "unconfirmed"
		if view.Status == operation.StatusSucceeded {
			state = "adopted"
		}
		return Preview{InputRef: "pinax://input/" + id, Digest: result.PreviewDigest, State: state}, nil
	}
	proposal, err := s.AttachmentPreview(ctx, who, inputcontract.AttachmentRequest{InputRequestID: id, OperationID: opID, ObjectID: resolved.ID, Revision: revision, Placement: "per-note"})
	if err != nil {
		return Preview{}, err
	}
	preview := Preview{InputRef: "pinax://input/" + id, Digest: proposal.PreviewDigest, State: "preview", Plan: proposal}
	if !confirm {
		return preview, nil
	}
	if expected != proposal.PreviewDigest {
		return Preview{}, intake.ErrConflict
	}
	view, err := s.AttachmentApply(ctx, who, inputcontract.AttachmentApply{Proposal: proposal, Confirm: true})
	if err != nil {
		return Preview{}, err
	}
	preview.State = "unconfirmed"
	if view.Status == operation.StatusSucceeded {
		preview.State = "adopted"
	}
	preview.Plan = nil
	return preview, nil
}
