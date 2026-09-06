package operation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestOperationMigrationCreatesAdditiveStoreAndIndexes(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing-note.md")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if !store.db.Migrator().HasTable(&OperationRow{}) || !store.db.Migrator().HasTable(&IdempotencyRow{}) {
		t.Fatal("operation migration did not create both additive tables")
	}
	for _, index := range []struct {
		model any
		name  string
	}{
		{&OperationRow{}, "idx_operation_scope"},
		{&OperationRow{}, "idx_api_operations_status"},
		{&IdempotencyRow{}, "idx_operation_idempotency_binding"},
		{&IdempotencyRow{}, "idx_api_operation_idempotency_operation_id"},
	} {
		if !store.db.Migrator().HasIndex(index.model, index.name) {
			t.Fatalf("migration missing index %s", index.name)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(existing)
	if err != nil || string(content) != "keep" {
		t.Fatalf("migration modified existing vault content: content=%q err=%v", content, err)
	}
	databasePath := filepath.Join(root, ".pinax", "api", "operations.sqlite")
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("operation database mode = %o, want 600", info.Mode().Perm())
	}
}

func TestOperationRepositoryPersistsReopensAndReplaysSameBinding(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	request := testCreateRequest(t, "op-reopen-1", "idem-reopen-1", map[string]any{"title": "Capture"})
	created, err := store.CreateAccepted(ctx, request)
	if err != nil || created.Replay || created.Operation.Status != StatusAccepted {
		t.Fatalf("create accepted = %#v err=%v", created, err)
	}
	replayed, err := store.CreateAccepted(ctx, request)
	if err != nil || !replayed.Replay || replayed.Operation.OperationID != created.Operation.OperationID {
		t.Fatalf("same binding replay = %#v err=%v", replayed, err)
	}
	var operationCount, bindingCount int64
	if err := store.db.Model(&OperationRow{}).Count(&operationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&IdempotencyRow{}).Count(&bindingCount).Error; err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 || bindingCount != 1 {
		t.Fatalf("duplicate rows after replay: operations=%d bindings=%d", operationCount, bindingCount)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	operation, err := reopened.Get(ctx, request.OperationID)
	if err != nil || operation.RequestDigest != request.RequestDigest {
		t.Fatalf("reopened operation = %#v err=%v", operation, err)
	}
}

func TestOperationIdempotencyConflictKeepsOriginalRecord(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	original := testCreateRequest(t, "op-conflict-1", "idem-conflict-1", map[string]any{"title": "Original"})
	if _, err := store.CreateAccepted(ctx, original); err != nil {
		t.Fatal(err)
	}

	conflicts := []CreateRequest{
		func() CreateRequest { value := original; value.OperationID = "op-conflict-2"; return value }(),
		func() CreateRequest { value := original; value.BindingID = "rpc.inbox.capture"; return value }(),
		func() CreateRequest { value := original; value.CapabilityID = "folder.rename"; return value }(),
		func() CreateRequest {
			value := original
			value.RequestDigest = IdentityDigest("different-request")
			return value
		}(),
	}
	for _, conflict := range conflicts {
		if _, err := store.CreateAccepted(ctx, conflict); !IsCode(err, CodeIdempotencyConflict) {
			t.Fatalf("conflicting binding returned %v", err)
		}
	}
	stored, err := store.Get(ctx, original.OperationID)
	if err != nil || stored.CapabilityID != original.CapabilityID || stored.BindingID != original.BindingID || stored.RequestDigest != original.RequestDigest {
		t.Fatalf("original operation changed: %#v err=%v", stored, err)
	}
}

func TestFindReplayDoesNotCreateMissingOperation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-find-replay-missing", "key-find-replay-missing", map[string]any{"title": "missing"})
	result, found, err := store.FindReplay(ctx, request)
	if err != nil || found || result.Operation.OperationID != "" {
		t.Fatalf("missing replay = %#v found=%v err=%v", result, found, err)
	}
	if _, err := store.Get(ctx, request.OperationID); !IsCode(err, CodeNotFound) {
		t.Fatalf("read-only lookup created operation: %v", err)
	}
	created, err := store.CreateAccepted(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	result, found, err = store.FindReplay(ctx, request)
	if err != nil || !found || !result.Replay || result.Operation.OperationID != created.Operation.OperationID {
		t.Fatalf("existing replay = %#v found=%v err=%v", result, found, err)
	}
}

func TestOperationIdempotencyConcurrentReplayCreatesSingleRecord(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-concurrent-1", "idem-concurrent-1", map[string]any{"title": "Concurrent"})

	const callers = 8
	results := make(chan CreateResult, callers)
	errors := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, createErr := store.CreateAccepted(ctx, request)
			results <- result
			errors <- createErr
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for createErr := range errors {
		if createErr != nil {
			t.Fatalf("concurrent create returned %v", createErr)
		}
	}
	nonReplay := 0
	for result := range results {
		if !result.Replay {
			nonReplay++
		}
		if result.Operation.OperationID != request.OperationID {
			t.Fatalf("concurrent result operation = %#v", result.Operation)
		}
	}
	if nonReplay != 1 {
		t.Fatalf("concurrent non-replay results = %d, want 1", nonReplay)
	}
	var operationCount, bindingCount int64
	if err := store.db.Model(&OperationRow{}).Count(&operationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&IdempotencyRow{}).Count(&bindingCount).Error; err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 || bindingCount != 1 {
		t.Fatalf("concurrent rows: operations=%d bindings=%d", operationCount, bindingCount)
	}
}

func TestOperationScopeAuthorizationDoesNotRevealExistence(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-scope-1", "idem-scope-1", map[string]any{"title": "Scoped"})
	if _, err := store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	for _, identity := range [][2]string{
		{IdentityDigest("other-principal"), request.ScopeDigest},
		{request.PrincipalDigest, IdentityDigest("other-scope")},
	} {
		if _, err := store.GetAuthorized(ctx, request.OperationID, identity[0], identity[1]); !IsCode(err, CodeNotFound) {
			t.Fatalf("scope mismatch leaked operation: %v", err)
		}
	}
}

func TestOperationRepositoryEnforcesTransitionsAndBoundedPayloads(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-transition-1", "idem-transition-1", map[string]any{"title": "Bounded"})
	if _, err := store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteSucceeded(ctx, request.OperationID, Outcome{}); !IsCode(err, CodeIllegalTransition) {
		t.Fatalf("accepted skipped applying: %v", err)
	}
	if _, err := store.StartApplying(ctx, request.OperationID); err != nil {
		t.Fatal(err)
	}
	tooLarge := json.RawMessage(`{"value":"` + strings.Repeat("x", MaxResultBytes) + `"}`)
	if _, err := store.CompleteSucceeded(ctx, request.OperationID, Outcome{Result: tooLarge}); !IsCode(err, CodePayloadTooLarge) {
		t.Fatalf("oversized result returned %v", err)
	}
	stillApplying, err := store.Get(ctx, request.OperationID)
	if err != nil || stillApplying.Status != StatusApplying {
		t.Fatalf("oversized result changed operation: %#v err=%v", stillApplying, err)
	}
	succeeded, err := store.CompleteSucceeded(ctx, request.OperationID, Outcome{
		Result: json.RawMessage(`{"note_ref":"note_1"}`), Receipt: json.RawMessage(`{"receipt_ref":"receipt_1"}`),
		ReceiptRef: "receipts/receipt_1.json", ResourceRef: "pinax://note/note_1", RevisionAfter: "rev-after-1",
	})
	if err != nil || succeeded.Status != StatusSucceeded || string(succeeded.ResultJSON) != `{"note_ref":"note_1"}` {
		t.Fatalf("complete succeeded = %#v err=%v", succeeded, err)
	}
	if _, err := store.RetryApplying(ctx, request.OperationID); !IsCode(err, CodeIllegalTransition) {
		t.Fatalf("succeeded operation retried: %v", err)
	}
}

func TestOperationRepositoryRejectsUnsafeRefsBeforeStateWrite(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-ref-1", "idem-ref-1", map[string]any{"title": "Safe refs"})
	request.RevisionBefore = "/home/user/private-vault"
	if _, err := store.CreateAccepted(ctx, request); !IsCode(err, CodePayloadInvalid) {
		t.Fatalf("absolute revision ref returned %v", err)
	}
	request.RevisionBefore = ""
	if _, err := store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplying(ctx, request.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteFailed(ctx, request.OperationID, Outcome{ErrorMessage: "failed at C:\\Users\\name\\vault"}); !IsCode(err, CodePayloadInvalid) {
		t.Fatalf("absolute error path returned %v", err)
	}
	operation, err := store.Get(ctx, request.OperationID)
	if err != nil || operation.Status != StatusApplying {
		t.Fatalf("unsafe ref changed state: %#v err=%v", operation, err)
	}
}

func TestOperationMigrationFailureIsStoreUnavailable(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, ".pinax")
	if err := os.WriteFile(blockingFile, []byte("not-a-directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if store != nil || !IsCode(err, CodeStoreUnavailable) {
		t.Fatalf("Open() = %#v, %v; want operation_store_unavailable", store, err)
	}
}

// TestCreateAcceptedClaimsRetryableReplay 覆盖重试认领：
// failed+retryable+replay_safe 的 replay 事务内迁移到 applying 并获得执行权；
// 未认领（不可重试或已被并发认领）时保持普通 Replay 短路。
func TestCreateAcceptedClaimsRetryableReplay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-retry-1", "idem-retry-1", map[string]any{"title": "Retry"})

	if _, err = store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = store.StartApplying(ctx, request.OperationID); err != nil {
		t.Fatal(err)
	}
	failed, err := store.CompleteFailed(ctx, request.OperationID, Outcome{
		ErrorCode: "revision_conflict", ErrorMessage: "retry me", Retryable: true, ReplaySafe: true,
	})
	if err != nil || failed.Status != StatusFailed || !failed.Retryable || !failed.ReplaySafe {
		t.Fatalf("failed row = %#v err=%v", failed, err)
	}

	// 可重试 replay：认领并迁移到 applying。
	claimed, err := store.CreateAccepted(ctx, request)
	if err != nil || !claimed.Replay || !claimed.RetryClaimed {
		t.Fatalf("retry claim = %#v err=%v", claimed, err)
	}
	if claimed.Operation.Status != StatusApplying {
		t.Fatalf("claimed status = %s", claimed.Operation.Status)
	}
	// 认领后可正常推进到终态。
	succeeded, err := store.CompleteSucceeded(ctx, request.OperationID, Outcome{Result: json.RawMessage(`{"ok":true}`)})
	if err != nil || succeeded.Status != StatusSucceeded {
		t.Fatalf("succeeded after claim = %#v err=%v", succeeded, err)
	}

	// 终态 succeeded 的 replay：不认领，普通短路。
	final, err := store.CreateAccepted(ctx, request)
	if err != nil || !final.Replay || final.RetryClaimed || final.Operation.Status != StatusSucceeded {
		t.Fatalf("terminal replay = %#v err=%v", final, err)
	}

	// 不可重试 failed：不认领。
	request2 := testCreateRequest(t, "op-retry-2", "idem-retry-2", map[string]any{"title": "NoRetry"})
	if _, err = store.CreateAccepted(ctx, request2); err != nil {
		t.Fatal(err)
	}
	if _, err = store.StartApplying(ctx, request2.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteFailed(ctx, request2.OperationID, Outcome{
		ErrorCode: "validation_failed", ErrorMessage: "do not retry", Retryable: false, ReplaySafe: true,
	}); err != nil {
		t.Fatal(err)
	}
	nonRetryable, err := store.CreateAccepted(ctx, request2)
	if err != nil || !nonRetryable.Replay || nonRetryable.RetryClaimed || nonRetryable.Operation.Status != StatusFailed {
		t.Fatalf("non-retryable replay = %#v err=%v", nonRetryable, err)
	}
}
