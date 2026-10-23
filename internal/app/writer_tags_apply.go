package app

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/operation"
)

const writerTagsApplyCapability = "writer.tags.apply"

type writerTagsItemResult struct {
	ObjectID      string `json:"object_id"`
	OperationID   string `json:"operation_id"`
	Status        string `json:"status"`
	RevisionAfter string `json:"revision_after,omitempty"`
}

type writerTagsResult struct {
	Items []writerTagsItemResult `json:"items"`
}

func writerTagsOperation(req WriterTagsRequest, digest string) operation.CreateRequest {
	access := writerTagsOperationAccess(req.Session)
	return operation.CreateRequest{OperationID: req.OperationID, IdempotencyKey: req.OperationID, CapabilityID: writerTagsApplyCapability, BindingID: "rest.writer.tags.apply", PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest, RequestDigest: digest}
}

func writerTagsChildID(parent, objectID string) string {
	return "tag-item-" + strings.TrimPrefix(collaborationRevision(parent+"\x00"+objectID), "sha256:")
}

// WriterTagsApply is not a multi-file transaction. The parent binds the plan;
// each child uses the existing snapshot/TagNote/receipt engine. Interrupted
// parents are inspect-only on replay, never implicit resume commands.
func (s *Service) WriterTagsApply(ctx context.Context, req WriterTagsRequest, plan WriterTagsPlan, authorization string) (operation.View, error) {
	var empty operation.View
	if err := writerTagsGate(req); err != nil {
		return empty, err
	}
	if authorization != "reviewed_proposal" && authorization != "explicit_instruction" {
		return empty, collaborationError("authorization_required", "Tag apply requires a caller assertion; host approval still applies")
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, err
	}
	req.VaultPath = root
	if plan.OperationID != req.OperationID || len(plan.Items) < 1 || len(plan.Items) > 32 || !hmac.Equal([]byte(plan.Digest), []byte(writerTagsPlanDigest(root, req, plan))) {
		return empty, collaborationError("preview_mismatch", "Apply requires the unchanged tag plan and supervised session")
	}
	parent := writerTagsOperation(req, plan.Digest)
	// Inspect original identity before acquiring a persistent lock or opening
	// a migrating store. Replay must not create sidecars or owner metadata.
	readonly, cleanup, readErr := operation.OpenReadSnapshot(root)
	if readErr == nil {
		replay, found, err := readonly.FindReplay(ctx, parent)
		cleanup()
		if err != nil {
			return empty, err
		}
		if found {
			return operation.ViewFromRow(replay.Operation), nil
		}
	} else {
		var opErr *operation.Error
		if !errors.As(readErr, &opErr) || opErr.Code != operation.CodeNotFound {
			return empty, readErr
		}
	}
	if err := dailyStorageSafe(ctx, root); err != nil {
		return empty, err
	}
	if err := collaborationSafePath(root, ".pinax/sync/operation.lock"); err != nil {
		return empty, err
	}
	lock, err := syncdaemon.AcquireOperationLock(root, writerTagsApplyCapability)
	if err != nil {
		return empty, err
	}
	defer lock.Release()
	store, err := operation.Open(root)
	if err != nil {
		return empty, err
	}
	defer func() { _ = store.Close() }()
	if replay, found, err := store.FindReplay(ctx, parent); err != nil {
		return empty, err
	} else if found {
		// Accepted/applying may mean a process stopped; do not infer zero writes.
		return operation.ViewFromRow(replay.Operation), nil
	}
	targets := make([]WriterTagTarget, 0, len(plan.Items))
	for _, item := range plan.Items {
		targets = append(targets, WriterTagTarget{ObjectID: item.ObjectID, Revision: item.Revision})
	}
	checked, err := s.WriterTagsPlan(ctx, WriterTagsPlanRequest{Writer: req, Operation: plan.Operation, OldTag: plan.OldTag, NewTag: plan.NewTag, Targets: targets})
	if err != nil {
		return empty, err
	}
	if !hmac.Equal([]byte(plan.Digest), []byte(checked.Digest)) {
		return empty, collaborationError("preview_mismatch", "Selected tag plan changed before acceptance")
	}
	created, err := store.CreateAccepted(ctx, parent)
	if err != nil {
		return empty, err
	}
	if created.Replay {
		return operation.ViewFromRow(created.Operation), nil
	}
	result := writerTagsResult{Items: make([]writerTagsItemResult, 0, len(plan.Items))}
	for _, item := range plan.Items {
		result.Items = append(result.Items, writerTagsItemResult{ObjectID: item.ObjectID, OperationID: writerTagsChildID(req.OperationID, item.ObjectID), Status: "not_started"})
	}
	outcome := operation.Outcome{}
	checkpoint := func() error {
		outcome.Result, _ = json.Marshal(result)
		return store.CheckpointApplying(ctx, req.OperationID, outcome)
	}
	if _, err = store.StartApplying(ctx, req.OperationID); err != nil {
		return s.collaborationFailed(store, req.OperationID, outcome, err)
	}
	if err = checkpoint(); err != nil {
		return s.collaborationFailed(store, req.OperationID, outcome, err)
	}
	for i, item := range plan.Items {
		if err = ctx.Err(); err != nil {
			return s.collaborationFailed(store, req.OperationID, outcome, err)
		}
		// Persist possible dispatch BEFORE calling the child. A crash cannot leave
		// a misleading not_started marker for a child that may already have run.
		result.Items[i].Status = "unknown"
		if err = checkpoint(); err != nil {
			return s.collaborationFailed(store, req.OperationID, outcome, err)
		}
		change := CollaborationChange{OperationID: result.Items[i].OperationID, Action: "tags", NoteRef: item.Path, ExpectedRevision: item.Revision, TagOperation: "set", Tags: item.After}
		digest := collaborationDigest(root, change)
		child := parent
		child.OperationID = change.OperationID
		child.IdempotencyKey = change.OperationID
		child.CapabilityID = "writer.tags.item"
		child.BindingID = "rest.writer.tags.item"
		child.RequestDigest = readerNavigationDigest([]string{plan.Digest, digest})
		child.RevisionBefore = item.Revision
		if err = validateCollaborationChange(change); err != nil {
			return s.collaborationFailed(store, req.OperationID, outcome, err)
		}
		if err = operation.ValidateCreateRequest(child); err != nil {
			return s.collaborationFailed(store, req.OperationID, outcome, err)
		}
		view, applyErr := s.collaborationApplyStored(ctx, root, change, digest, authorization, writerPolicy(), child, func() error { return s.writerTagsItemGuard(ctx, req, item) }, store)
		if applyErr != nil {
			return s.collaborationFailed(store, req.OperationID, outcome, applyErr)
		}
		result.Items[i].Status = string(view.Status)
		result.Items[i].RevisionAfter = view.RevisionAfter
		if err = checkpoint(); err != nil {
			return s.collaborationFailed(store, req.OperationID, outcome, err)
		}
		if view.Status != operation.StatusSucceeded {
			return s.collaborationFailed(store, req.OperationID, outcome, collaborationError("outcome_unknown", "Inspect the original tag item operation"))
		}
	}
	outcome.Receipt, _ = json.Marshal(result)
	row, err := store.CompleteSucceeded(ctx, req.OperationID, outcome)
	if err != nil {
		return s.collaborationFailed(store, req.OperationID, outcome, err)
	}
	return operation.ViewFromRow(row), nil
}

func (s *Service) writerTagsItemGuard(ctx context.Context, req WriterTagsRequest, item WriterTagPlanItem) error {
	source, err := s.WriterEditorSource(ctx, WriterEditorSourceRequest{VaultPath: req.VaultPath, Session: req.Session, ObjectID: item.ObjectID, Revision: item.Revision})
	if err != nil {
		return err
	}
	if !source.Editable || source.ProtectedMetadata.Kind == "daily" {
		return collaborationError("frontmatter_unsupported", "Selected tag source is not editable")
	}
	opened, err := os.OpenRoot(req.VaultPath)
	if err != nil {
		return err
	}
	defer func() { _ = opened.Close() }()
	raw, err := readWriterEditorCandidate(opened, item.Path)
	if err != nil {
		return err
	}
	meta, _ := splitFrontmatter(string(raw))
	if meta["note_id"] != item.ObjectID || collaborationRevision(string(raw)) != item.Revision {
		return collaborationError("revision_conflict", "Selected tag source changed")
	}
	_, err = patchTagMetadata(string(raw), formatTags(item.After), "2000-01-01T00:00:00Z")
	return err
}
