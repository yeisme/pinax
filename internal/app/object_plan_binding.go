package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
	"github.com/yeisme/pinax/internal/records"
)

type managedObjectPlanBinding struct {
	ObjectID                string
	ObjectKind              string
	ObservedPath            string
	ExpectedRecordVersion   uint64
	ExpectedContentRevision domain.ContentRevision
}

func bindRepairPlanObjects(ctx context.Context, root string, plan *domain.RepairPlan) error {
	if plan == nil {
		return nil
	}
	bindings, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return err
	}
	for index := range plan.Operations {
		operation := &plan.Operations[index]
		binding, ok := bindings[filepath.ToSlash(strings.TrimSpace(operation.Path))]
		if !ok {
			continue
		}
		operation.ObjectID = binding.ObjectID
		operation.ObjectKind = binding.ObjectKind
		operation.ObservedPath = binding.ObservedPath
		operation.ExpectedRecordVersion = binding.ExpectedRecordVersion
		operation.ExpectedContentRevision = binding.ExpectedContentRevision
		if operation.NoteID == "" {
			operation.NoteID = binding.ObjectID
		}
	}
	return nil
}

func bindOrganizePlanObjects(ctx context.Context, root string, plan *domain.OrganizePlan) error {
	if plan == nil {
		return nil
	}
	bindings, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return err
	}
	for index := range plan.Operations {
		operation := &plan.Operations[index]
		binding, ok := bindings[filepath.ToSlash(strings.TrimSpace(operation.Path))]
		if !ok {
			continue
		}
		operation.ObjectID = binding.ObjectID
		operation.ObjectKind = binding.ObjectKind
		operation.ObservedPath = binding.ObservedPath
		operation.ExpectedRecordVersion = binding.ExpectedRecordVersion
		operation.ExpectedContentRevision = binding.ExpectedContentRevision
	}
	return nil
}

func managedNotePlanBindings(ctx context.Context, root string) (map[string]managedObjectPlanBinding, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return nil, err
	}
	ledger, ledgerErr := records.NewService(root).ReplayReadOnly(ctx)
	if ledgerErr != nil && !os.IsNotExist(ledgerErr) {
		return nil, ledgerErr
	}
	bindings := make(map[string]managedObjectPlanBinding, len(notes))
	for _, note := range notes {
		if identity.Classify(note.ID) != identity.IDClassCanonical {
			continue
		}
		revision, err := fileContentRevision(root, note.Path)
		if err != nil {
			return nil, err
		}
		binding := managedObjectPlanBinding{ObjectID: note.ID, ObjectKind: "note", ObservedPath: note.Path, ExpectedContentRevision: revision}
		if record, ok := ledger.Records[note.ID]; ok {
			binding.ExpectedRecordVersion = record.RecordVersion
		}
		bindings[note.Path] = binding
	}
	return bindings, nil
}

func rebaseRepairPlanObjects(ctx context.Context, root string, plan *domain.RepairPlan) (bool, error) {
	if plan == nil {
		return false, nil
	}
	hasBindings := false
	for index := range plan.Operations {
		operation := &plan.Operations[index]
		if strings.TrimSpace(operation.ObjectID) == "" {
			continue
		}
		hasBindings = true
		currentPath, err := resolveBoundPlanObject(ctx, root, operation.ObjectID, operation.ObservedPath, operation.ExpectedContentRevision)
		if err != nil {
			return true, err
		}
		operation.Path = currentPath
	}
	return hasBindings, nil
}

func rebaseOrganizePlanObjects(ctx context.Context, root string, plan *domain.OrganizePlan) (bool, error) {
	if plan == nil {
		return false, nil
	}
	hasBindings := false
	for index := range plan.Operations {
		operation := &plan.Operations[index]
		if strings.TrimSpace(operation.ObjectID) == "" {
			continue
		}
		hasBindings = true
		currentPath, err := resolveBoundPlanObject(ctx, root, operation.ObjectID, operation.ObservedPath, operation.ExpectedContentRevision)
		if err != nil {
			return true, err
		}
		operation.Path = currentPath
		if operation.Kind == "move" && operation.Before != nil {
			operation.Before["path"] = currentPath
		}
	}
	return hasBindings, nil
}

func resolveBoundPlanObject(_ context.Context, root, objectID, observedPath string, expected domain.ContentRevision) (string, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return "", err
	}
	var current *domain.Note
	var observed *domain.Note
	for index := range notes {
		note := &notes[index]
		if note.ID == objectID {
			current = note
		}
		if note.Path == observedPath {
			observed = note
		}
	}
	if current == nil {
		return "", staleObjectPlanError("managed object no longer exists")
	}
	if observed != nil && observed.ID != objectID {
		return "", staleObjectPlanError("observed path is now owned by another object")
	}
	currentRevision, err := fileContentRevision(root, current.Path)
	if err != nil {
		return "", err
	}
	if expected.Hash != "" && (currentRevision.Hash != expected.Hash || currentRevision.Size != expected.Size) {
		return "", staleObjectPlanError("managed object content revision changed")
	}
	return current.Path, nil
}

func fileContentRevision(root, rel string) (domain.ContentRevision, error) {
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.ContentRevision{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.ContentRevision{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return domain.ContentRevision{}, err
	}
	digest := sha256.Sum256(payload)
	return domain.ContentRevision{Hash: hex.EncodeToString(digest[:]), Size: int64(len(payload)), ModifiedUnix: info.ModTime().Unix()}, nil
}

func staleObjectPlanError(message string) error {
	return &domain.CommandError{Code: "plan_stale", Message: fmt.Sprintf("object-bound plan is stale: %s", message), Hint: "Generate and review a fresh plan before applying"}
}

func bindPlanOperations(ctx context.Context, root string, operations []domain.PlanOperation) ([]domain.PlanOperation, error) {
	bindings, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return nil, err
	}
	result := append([]domain.PlanOperation(nil), operations...)
	for index := range result {
		operation := &result[index]
		binding, ok := bindings[filepath.ToSlash(strings.TrimSpace(operation.Path))]
		if !ok {
			continue
		}
		operation.ObjectID = binding.ObjectID
		operation.ObjectKind = binding.ObjectKind
		operation.ObservedPath = binding.ObservedPath
		operation.ExpectedRecordVersion = binding.ExpectedRecordVersion
		operation.ExpectedContentRevision = binding.ExpectedContentRevision
	}
	return result, nil
}
