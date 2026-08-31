package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestInboxCaptureRemoteIdempotentReplayCreatesOneNote(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	identity := testInboxOperationIdentity(root, "op-inbox-idempotent-1", "rpc.inbox.capture")
	request := CreateNoteRequest{VaultPath: root, Title: "Idempotent capture", Body: "private body", Tags: []string{"capture"}}

	first, err := service.InboxCaptureRemote(ctx, request, identity)
	if err != nil || first.Facts["operation_status"] != "succeeded" || first.Facts["idempotent_replay"] != "false" {
		t.Fatalf("first capture = %#v err=%v", first, err)
	}
	second, err := service.InboxCaptureRemote(ctx, request, identity)
	if err != nil || second.Facts["operation_id"] != identity.OperationID || second.Facts["operation_status"] != "succeeded" || second.Facts["idempotent_replay"] != "true" {
		t.Fatalf("replayed capture = %#v err=%v", second, err)
	}
	files, err := filepath.Glob(filepath.Join(root, "inbox", "*.md"))
	if err != nil || len(files) != 1 {
		t.Fatalf("inbox files = %#v err=%v", files, err)
	}
	store, err := operation.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	row, err := store.Get(ctx, identity.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(row)
	if strings.Contains(string(encoded), request.Body) || strings.Contains(string(encoded), root) || strings.Contains(string(encoded), identity.IdempotencyKey) {
		t.Fatalf("operation ledger leaked request or owner material: %s", encoded)
	}

	conflict := request
	conflict.Body = "different body"
	projection, err := service.InboxCaptureRemote(ctx, conflict, identity)
	if err == nil || projection.Error == nil || projection.Error.Code != "idempotency_conflict" {
		t.Fatalf("digest conflict = %#v err=%v", projection, err)
	}
}

func TestInboxCaptureRemoteDryRunCreatesNoLedgerOrNote(t *testing.T) {
	root := t.TempDir()
	identity := testInboxOperationIdentity(root, "op-inbox-dry-run-1", "rpc.inbox.capture")
	projection, err := NewService().InboxCaptureRemote(context.Background(), CreateNoteRequest{VaultPath: root, Title: "Preview", DryRun: true}, identity)
	if err != nil || projection.Facts["operation_status"] != "planned" || projection.Facts["remote_write"] != "false" {
		t.Fatalf("dry run = %#v err=%v", projection, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("dry run created operation ledger: %v", err)
	}
	if files, _ := filepath.Glob(filepath.Join(root, "inbox", "*.md")); len(files) != 0 {
		t.Fatalf("dry run created notes: %#v", files)
	}
}

func TestInboxCaptureOperationReconcileUsesRecordProofWithoutReplay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	identity := testInboxOperationIdentity(root, "op-inbox-reconcile-1", "rpc.inbox.capture")
	request := CreateNoteRequest{VaultPath: root, Title: "Response lost", Body: "written once"}

	planRequest := request
	planRequest.DryRun = true
	plan, err := service.InboxCapture(ctx, planRequest)
	if err != nil {
		t.Fatal(err)
	}
	prepared := inboxResultFromProjection(plan)
	preparedJSON, _ := json.Marshal(prepared)
	digest, err := operation.CanonicalDigest(map[string]any{
		"capability_id": inboxCaptureCapabilityID, "binding_id": identity.BindingID,
		"title": request.Title, "body": request.Body, "tags": []string(nil), "slug": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccepted(ctx, operation.CreateRequest{
		OperationID: identity.OperationID, IdempotencyKey: identity.IdempotencyKey,
		CapabilityID: inboxCaptureCapabilityID, BindingID: identity.BindingID,
		PrincipalDigest: identity.Access.PrincipalDigest, ScopeDigest: identity.Access.ScopeDigest, RequestDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplyingWithOutcome(ctx, identity.OperationID, operation.Outcome{Result: preparedJSON}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	applyRequest := request
	applyRequest.PlannedPath = prepared.PlannedPath
	applyRequest.ObjectID = prepared.NoteID
	applyRequest.RecordIdempotencyKey = inboxRecordIdempotency(identity.OperationID)
	applyRequest.RecordEvidence = []string{"operation_id=" + identity.OperationID}
	if _, err := service.InboxCapture(ctx, applyRequest); err != nil {
		t.Fatal(err)
	}

	reconciled, err := service.OperationReconcile(ctx, OperationRequest{VaultPath: root, OperationID: identity.OperationID, Access: OperationAccess{OwnerLocal: true}})
	if err != nil || reconciled.Facts["operation_status"] != "succeeded" || reconciled.Facts["receipt_ref"] == "" {
		t.Fatalf("reconciled = %#v err=%v", reconciled, err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "inbox", "*.md"))
	if len(files) != 1 {
		t.Fatalf("reconcile replayed mutation: files=%#v", files)
	}
}

func TestInboxCaptureRemotePartialIdentityDoesNotCreateLedger(t *testing.T) {
	root := t.TempDir()
	identity := testInboxOperationIdentity(root, "op-inbox-partial-1", "rpc.inbox.capture")
	identity.IdempotencyKey = ""
	projection, err := NewService().InboxCaptureRemote(context.Background(), CreateNoteRequest{VaultPath: root, Title: "Blocked"}, identity)
	if err == nil || projection.Error == nil || projection.Error.Code != "operation_identity_required" {
		t.Fatalf("partial identity = %#v err=%v", projection, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("partial identity created ledger: %v", err)
	}
}

func testInboxOperationIdentity(root, operationID, bindingID string) RemoteOperationIdentity {
	return RemoteOperationIdentity{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID, BindingID: bindingID,
		Access: OperationAccess{
			PrincipalDigest: operation.IdentityDigest("principal", "test"),
			ScopeDigest:     operation.IdentityDigest("pinax-vault", filepath.Clean(root)),
		},
	}
}
