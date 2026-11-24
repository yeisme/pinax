package inboxjudgment

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	judgment "github.com/yeisme/judgment-sdk"
	"github.com/yeisme/judgment-sdk/providers"
	"github.com/yeisme/pinax/internal/sqlitedsn"
	"gorm.io/gorm"
)

// receipts_test.go 覆盖本 owner 的持久执行回执：原子 claim、身份绑定、
// 终态更新，以及重启后未知结果仍然粘滞（不自动重提）。

func openReceiptTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := sqlitedsn.Open(filepath.Join(t.TempDir(), "judgment.sqlite"))
	if err != nil {
		t.Fatalf("open receipt test db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func receiptFixture(state string) providers.Receipt {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	return providers.Receipt{
		SchemaVersion: "1.0",
		Owner:         "pinax",
		Project:       "inbox-judgment",
		RequestID:     "judgment-request-1",
		AttemptID:     "judgment-attempt-1",
		InputDigest:   "sha256:" + strings64("a"),
		ProfileDigest: "sha256:" + strings64("b"),
		State:         state,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func strings64(value string) string {
	out := ""
	for i := 0; i < 64; i++ {
		out += value
	}
	return out
}

func TestReceiptStoreClaimIsAtomicAndKeepsIdentity(t *testing.T) {
	store, err := OpenReceiptStoreDB(openReceiptTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	claimed, ok, err := store.Claim(ctx, receiptFixture("dispatching"))
	if err != nil || !ok {
		t.Fatalf("first claim must insert: ok=%t err=%v", ok, err)
	}
	if claimed.RequestID != "judgment-request-1" || claimed.AttemptID != "judgment-attempt-1" {
		t.Fatalf("claim must preserve the owner operation identity: %+v", claimed)
	}
	existing, ok, err := store.Claim(ctx, receiptFixture("dispatching"))
	if err != nil || ok {
		t.Fatalf("second claim must return the existing receipt: ok=%t err=%v", ok, err)
	}
	if existing.InputDigest != claimed.InputDigest || existing.ProfileDigest != claimed.ProfileDigest {
		t.Fatalf("existing receipt lost its digest binding: %+v", existing)
	}
	if _, _, err := store.Claim(ctx, providers.Receipt{Owner: "pinax"}); err == nil {
		t.Fatal("claim without a complete identity must fail closed")
	}
}

func TestReceiptStoreFinishPreservesBinding(t *testing.T) {
	store, err := OpenReceiptStoreDB(openReceiptTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := store.Claim(ctx, receiptFixture("dispatching")); err != nil {
		t.Fatal(err)
	}
	finished := receiptFixture("unknown")
	finished.SubmissionState = judgment.SubmissionUnknown
	finished.ErrorCode = judgment.CodeOutcomeUnknown
	finished.UpdatedAt = finished.CreatedAt.Add(time.Second)
	if err := store.Finish(ctx, finished); err != nil {
		t.Fatalf("finish must update the claimed receipt: %v", err)
	}
	stored, found, err := store.Get(ctx, "pinax", "inbox-judgment", "judgment-request-1", "judgment-attempt-1")
	if err != nil || !found {
		t.Fatalf("stored receipt must be readable: found=%t err=%v", found, err)
	}
	if stored.State != "unknown" || stored.ErrorCode != judgment.CodeOutcomeUnknown || stored.SubmissionState != judgment.SubmissionUnknown {
		t.Fatalf("finish must record the terminal observation: %+v", stored)
	}
	if stored.InputDigest != finished.InputDigest || stored.ProfileDigest != finished.ProfileDigest {
		t.Fatalf("finish must not rewrite the digest binding: %+v", stored)
	}
	unclaimed := receiptFixture("completed")
	unclaimed.AttemptID = "judgment-attempt-never-claimed"
	if err := store.Finish(ctx, unclaimed); err == nil {
		t.Fatal("finish must not insert a receipt that was never claimed")
	}
}

type receiptTestRoundTrip func(*http.Request) (*http.Response, error)

func (f receiptTestRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func managedBridgeWithStore(t *testing.T, path string, calls *int) *JudgmentSDKBridge {
	t.Helper()
	db, err := sqlitedsn.Open(path)
	if err != nil {
		t.Fatalf("open judgment store: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	store, err := OpenReceiptStoreDB(db)
	if err != nil {
		t.Fatal(err)
	}
	options := providers.Options{
		Profile: providers.ExecutionProfile{
			ID: "review", Revision: 1, Provider: "openai_decisions", Model: "gpt-6-luna",
			Enabled: true, Credential: providers.CredentialBinding{Source: "env", Env: "OPENAI_API_KEY"},
		},
		Authorize:   func(context.Context, *judgment.Request) error { return nil },
		Credentials: providers.Resolver{Environment: func(string) (string, bool) { return "synthetic-key", true }},
		RoundTripper: receiptTestRoundTrip(func(*http.Request) (*http.Response, error) {
			*calls++
			return nil, errors.New("synthetic interruption")
		}),
	}
	bridge, err := NewManagedJudgmentSDKBridge(options, store)
	if err != nil {
		t.Fatal(err)
	}
	if bridge == nil {
		t.Fatal("enabled profile must assemble a bridge")
	}
	return bridge
}

func managedReplayRequest() JudgmentWireRequest {
	request := judgmentWireFixtureRequest()
	request.RequestID = "judgment-request-persist"
	request.AttemptID = "judgment-attempt-persist"
	request.Scope.Project = "inbox-judgment"
	request.Model.Provider = "openai"
	request.Model.Model = "gpt-6-luna"
	return request
}

func TestReceiptStoreKeepsUnknownStickyAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "judgment.sqlite")
	ctx := context.Background()
	calls := 0
	first := managedBridgeWithStore(t, path, &calls)
	if _, err := first.Evaluate(ctx, managedReplayRequest()); err == nil {
		t.Fatal("synthetic interruption must surface as a judgment error")
	}
	var transportError *JudgmentTransportError
	if _, err := first.Evaluate(ctx, managedReplayRequest()); !errors.As(err, &transportError) || transportError.Code != JudgmentCodeOutcomeUnknown {
		t.Fatalf("second call must reconcile the original attempt: %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider must be called exactly once, got %d", calls)
	}
	// 新连接/新进程等价路径：回执必须从磁盘恢复，未知结果依旧粘滞。
	restarted := 0
	second := managedBridgeWithStore(t, path, &restarted)
	if _, err := second.Evaluate(ctx, managedReplayRequest()); !errors.As(err, &transportError) || transportError.Code != JudgmentCodeOutcomeUnknown {
		t.Fatalf("restart must not resubmit: %v", err)
	}
	if restarted != 0 {
		t.Fatalf("restart dispatched %d provider calls, want 0", restarted)
	}
}

func TestReceiptStoreRejectsConflictingReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "judgment.sqlite")
	ctx := context.Background()
	calls := 0
	bridge := managedBridgeWithStore(t, path, &calls)
	if _, err := bridge.Evaluate(ctx, managedReplayRequest()); err == nil {
		t.Fatal("first attempt must record an unknown outcome")
	}
	tampered := managedReplayRequest()
	tampered.Sources[0].InlineText = "a different excerpt"
	if _, err := bridge.Evaluate(ctx, tampered); err == nil {
		t.Fatal("changed input under the same attempt identity must fail closed")
	}
	if calls != 1 {
		t.Fatalf("conflicting replay must not dispatch, got %d calls", calls)
	}
}
