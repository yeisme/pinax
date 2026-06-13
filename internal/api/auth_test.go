package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateTokenRecordCreatesValidRecord(t *testing.T) {
	scope := map[TokenScope]ScopeTarget{
		ScopeRead:  {},
		ScopeWrite: {},
	}
	rec, secret := GenerateTokenRecord("test", scope, "", "auto")

	if rec.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if len(rec.ID) < 4 {
		t.Fatalf("ID too short: %s", rec.ID)
	}
	if rec.SecretHash == "" {
		t.Fatal("expected non-empty SecretHash")
	}
	if rec.Salt == "" {
		t.Fatal("expected non-empty Salt")
	}
	if secret == "" {
		t.Fatal("expected non-empty plaintext secret")
	}
	if rec.CreatedAt == "" {
		t.Fatal("expected non-empty CreatedAt")
	}
	if rec.CreatedBy != "auto" {
		t.Fatalf("expected createdBy=auto, got %s", rec.CreatedBy)
	}
}

func TestVerifySecretMatchesPlaintext(t *testing.T) {
	rec, secret := GenerateTokenRecord("test", nil, "", "auto")
	if !VerifySecret(rec, secret) {
		t.Fatal("expected secret to verify")
	}
	if VerifySecret(rec, "wrong-secret") {
		t.Fatal("expected wrong secret to fail")
	}
}

func TestIsExpired_NoExpiry(t *testing.T) {
	rec := &TokenRecord{}
	if IsExpired(rec) {
		t.Fatal("token without expiry should not be expired")
	}
}

func TestIsExpired_FutureExpiry(t *testing.T) {
	rec := &TokenRecord{
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	if IsExpired(rec) {
		t.Fatal("token with future expiry should not be expired")
	}
}

func TestIsExpired_PastExpiry(t *testing.T) {
	rec := &TokenRecord{
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	if !IsExpired(rec) {
		t.Fatal("token with past expiry should be expired")
	}
}

func TestHasScope_EmptyGroups(t *testing.T) {
	rec := &TokenRecord{
		Scope: map[TokenScope]ScopeTarget{
			ScopeRead: {Groups: nil},
		},
	}
	if !HasScope(rec, ScopeRead, "notes") {
		t.Fatal("empty groups should match any group")
	}
	if !HasScope(rec, ScopeRead, "folders") {
		t.Fatal("empty groups should match any group")
	}
}

func TestHasScope_SpecificGroups(t *testing.T) {
	rec := &TokenRecord{
		Scope: map[TokenScope]ScopeTarget{
			ScopeRead: {Groups: []string{"notes", "folders"}},
		},
	}
	if !HasScope(rec, ScopeRead, "notes") {
		t.Fatal("should match listed group")
	}
	if HasScope(rec, ScopeRead, "inbox") {
		t.Fatal("should not match unlisted group")
	}
}

func TestHasScope_MissingScope(t *testing.T) {
	rec := &TokenRecord{
		Scope: map[TokenScope]ScopeTarget{
			ScopeRead: {},
		},
	}
	if HasScope(rec, ScopeWrite, "notes") {
		t.Fatal("should not match missing scope")
	}
}
func TestHasScope_RestrictsActionsWhenPresent(t *testing.T) {
	rec := &TokenRecord{
		Scope: map[TokenScope]ScopeTarget{
			ScopeWrite: {Groups: []string{"folders"}, Actions: []string{"folder.create"}},
		},
	}
	if !HasScopeForAction(rec, ScopeWrite, "folders", "folder.create") {
		t.Fatal("should match listed action")
	}
	if HasScopeForAction(rec, ScopeWrite, "folders", "folder.delete") {
		t.Fatal("should not match unlisted action")
	}
}

// --- MemoryTokenStore ---

func TestMemoryTokenStore_CRUD(t *testing.T) {
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("test", map[TokenScope]ScopeTarget{
		ScopeRead: {},
	}, "", "manual")

	// Create
	if err := store.Create(rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get
	got, err := store.Get(rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != rec.ID {
		t.Fatalf("Get: expected ID %s, got %s", rec.ID, got.ID)
	}

	// List
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: expected 1, got %d", len(list))
	}

	// Verify
	verified, err := store.Verify(secret)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.ID != rec.ID {
		t.Fatalf("Verify: expected ID %s, got %s", rec.ID, verified.ID)
	}

	// Verify wrong secret
	_, err = store.Verify("wrong")
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}

	// Update
	rec.Label = "updated"
	if err := store.Update(rec); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = store.Get(rec.ID)
	if got.Label != "updated" {
		t.Fatalf("Update: expected label=updated, got %s", got.Label)
	}

	// Delete
	if err := store.Delete(rec.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = store.Get(rec.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestMemoryTokenStore_VerifyExpired(t *testing.T) {
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("expired", nil,
		time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339), "auto")
	_ = store.Create(rec)

	_, err := store.Verify(secret)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestMemoryTokenStore_GetNotFound(t *testing.T) {
	store := NewMemoryTokenStore()
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

// --- FileTokenStore ---

func TestFileTokenStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	store, err := NewFileTokenStore(path)
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}

	rec, secret := GenerateTokenRecord("file-test", map[TokenScope]ScopeTarget{
		ScopeRead:  {},
		ScopeWrite: {},
	}, "", "manual")

	// Create
	if err := store.Create(rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Verify file permissions
	info, _ := os.Stat(path)
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("file permissions too open: %o", info.Mode().Perm())
	}

	// Get
	got, err := store.Get(rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != rec.ID {
		t.Fatalf("Get: expected ID %s, got %s", rec.ID, got.ID)
	}

	// List
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: expected 1, got %d", len(list))
	}

	// Verify
	verified, err := store.Verify(secret)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.ID != rec.ID {
		t.Fatalf("Verify: expected ID %s, got %s", rec.ID, verified.ID)
	}

	// Delete
	if err := store.Delete(rec.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = store.List()
	if len(list) != 0 {
		t.Fatalf("List after delete: expected 0, got %d", len(list))
	}
}

func TestFileTokenStore_VerifyExpired(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileTokenStore(filepath.Join(dir, "tokens.json"))
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}
	rec, secret := GenerateTokenRecord("expired", nil,
		time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339), "auto")
	_ = store.Create(rec)

	_, err = store.Verify(secret)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestFileTokenStore_MultipleTokens(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileTokenStore(filepath.Join(dir, "tokens.json"))

	rec1, secret1 := GenerateTokenRecord("first", nil, "", "auto")
	rec2, secret2 := GenerateTokenRecord("second", nil, "", "auto")
	_ = store.Create(rec1)
	_ = store.Create(rec2)

	// Verify both
	v1, err := store.Verify(secret1)
	if err != nil || v1.ID != rec1.ID {
		t.Fatalf("Verify first: err=%v id=%s", err, v1)
	}
	v2, err := store.Verify(secret2)
	if err != nil || v2.ID != rec2.ID {
		t.Fatalf("Verify second: err=%v id=%s", err, v2)
	}

	// Cross-verify should fail
	_, err = store.Verify(secret1)
	if err != nil {
		t.Fatal("same secret should still verify")
	}
}

func TestFileTokenStore_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store1, _ := NewFileTokenStore(path)
	rec, secret := GenerateTokenRecord("persist", nil, "", "auto")
	_ = store1.Create(rec)

	// New instance should see the same token
	store2, _ := NewFileTokenStore(path)
	verified, err := store2.Verify(secret)
	if err != nil {
		t.Fatalf("Verify from new instance: %v", err)
	}
	if verified.ID != rec.ID {
		t.Fatalf("wrong ID: expected %s got %s", rec.ID, verified.ID)
	}
}

func TestFileTokenStore_EmptyOnNewFile(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileTokenStore(filepath.Join(dir, "tokens.json"))
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}

func TestGenerateSecret_Uniqueness(t *testing.T) {
	s1 := GenerateSecret()
	s2 := GenerateSecret()
	if s1 == s2 {
		t.Fatal("two generated secrets should not be equal")
	}
}
