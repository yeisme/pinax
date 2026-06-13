package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TokenScope defines the permission scope of a token.
type TokenScope string

const (
	ScopeRead  TokenScope = "read"  // All GET routes
	ScopeWrite TokenScope = "write" // All mutation routes
	ScopeAdmin TokenScope = "admin" // Token management itself
)

// ScopeTarget defines which groups and actions a scope covers.
type ScopeTarget struct {
	Groups  []string `json:"groups,omitempty"`
	Actions []string `json:"actions,omitempty"`
}

// TokenRecord represents a stored API token.
type TokenRecord struct {
	ID          string                     `json:"id"`
	SecretHash  string                     `json:"secret_hash"`
	Salt        string                     `json:"salt"`
	Scope       map[TokenScope]ScopeTarget `json:"scope"`
	Label       string                     `json:"label,omitempty"`
	CreatedAt   string                     `json:"created_at"`
	ExpiresAt   string                     `json:"expires_at,omitempty"`
	LastUsedAt  string                     `json:"last_used_at,omitempty"`
	RotatedFrom string                     `json:"rotated_from,omitempty"`
	CreatedBy   string                     `json:"created_by"`
}

// TokenStore is the interface for token persistence and verification.
type TokenStore interface {
	Verify(secret string) (*TokenRecord, error)
	Create(record *TokenRecord) error
	Get(id string) (*TokenRecord, error)
	List() ([]*TokenRecord, error)
	Delete(id string) error
	Update(record *TokenRecord) error
}

// MemoryTokenStore stores tokens in process memory.
type MemoryTokenStore struct {
	sync.RWMutex
	records map[string]*TokenRecord // id -> record
}

// NewMemoryTokenStore creates a new in-memory token store.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{records: make(map[string]*TokenRecord)}
}

// Verify checks a secret against all stored tokens.
func (m *MemoryTokenStore) Verify(secret string) (*TokenRecord, error) {
	m.RLock()
	defer m.RUnlock()
	for _, rec := range m.records {
		if VerifySecret(rec, secret) {
			if IsExpired(rec) {
				return nil, fmt.Errorf("token expired")
			}
			return rec, nil
		}
	}
	return nil, fmt.Errorf("invalid token")
}

// Create stores a new token record.
func (m *MemoryTokenStore) Create(record *TokenRecord) error {
	m.Lock()
	defer m.Unlock()
	m.records[record.ID] = record
	return nil
}

// Get retrieves a token by ID.
func (m *MemoryTokenStore) Get(id string) (*TokenRecord, error) {
	m.RLock()
	defer m.RUnlock()
	rec, ok := m.records[id]
	if !ok {
		return nil, fmt.Errorf("token not found: %s", id)
	}
	return rec, nil
}

// List returns all token records.
func (m *MemoryTokenStore) List() ([]*TokenRecord, error) {
	m.RLock()
	defer m.RUnlock()
	result := make([]*TokenRecord, 0, len(m.records))
	for _, rec := range m.records {
		result = append(result, rec)
	}
	return result, nil
}

// Delete removes a token by ID.
func (m *MemoryTokenStore) Delete(id string) error {
	m.Lock()
	defer m.Unlock()
	delete(m.records, id)
	return nil
}

// Update modifies an existing token record.
func (m *MemoryTokenStore) Update(record *TokenRecord) error {
	m.Lock()
	defer m.Unlock()
	m.records[record.ID] = record
	return nil
}

// FileTokenStore stores tokens in a JSON file.
type FileTokenStore struct {
	mu   sync.Mutex
	path string
}

// NewFileTokenStore creates or opens a file-based token store.
func NewFileTokenStore(path string) (*FileTokenStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create token dir: %w", err)
	}
	// Check file permissions if it exists
	if info, err := os.Stat(path); err == nil {
		if info.Mode().Perm()&0o077 != 0 {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: token file %s has overly broad permissions; recommended mode is 0600\n", path)
		}
	}
	return &FileTokenStore{path: path}, nil
}

func (f *FileTokenStore) load() (map[string]*TokenRecord, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*TokenRecord), nil
		}
		return nil, err
	}
	var records map[string]*TokenRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}
	return records, nil
}

func (f *FileTokenStore) save(records map[string]*TokenRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(f.path, data, 0o600); err != nil {
		return err
	}
	return nil
}

// Verify checks a secret against all stored tokens in the file.
func (f *FileTokenStore) Verify(secret string) (*TokenRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		if VerifySecret(rec, secret) {
			if IsExpired(rec) {
				return nil, fmt.Errorf("token expired")
			}
			rec.LastUsedAt = time.Now().UTC().Format(time.RFC3339)
			records[rec.ID] = rec
			_ = f.save(records)
			return rec, nil
		}
	}
	return nil, fmt.Errorf("invalid token")
}

// Create stores a new token record to the file.
func (f *FileTokenStore) Create(record *TokenRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, err := f.load()
	if err != nil {
		return err
	}
	records[record.ID] = record
	return f.save(records)
}

// Get retrieves a token by ID from the file.
func (f *FileTokenStore) Get(id string) (*TokenRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	rec, ok := records[id]
	if !ok {
		return nil, fmt.Errorf("token not found: %s", id)
	}
	return rec, nil
}

// List returns all token records from the file.
func (f *FileTokenStore) List() ([]*TokenRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	result := make([]*TokenRecord, 0, len(records))
	for _, rec := range records {
		result = append(result, rec)
	}
	return result, nil
}

// Delete removes a token by ID from the file.
func (f *FileTokenStore) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, err := f.load()
	if err != nil {
		return err
	}
	delete(records, id)
	return f.save(records)
}

// Update modifies an existing token record in the file.
func (f *FileTokenStore) Update(record *TokenRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, err := f.load()
	if err != nil {
		return err
	}
	records[record.ID] = record
	return f.save(records)
}

// generateTokenID creates a new token ID with prefix "pt_".
func generateTokenID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "pt_" + hex.EncodeToString(b)
}

// GenerateSecret creates a random secret string.
func GenerateSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSalt() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hashSecret computes SHA256(salt + secret) as hex.
func hashSecret(salt, secret string) string {
	h := sha256.Sum256([]byte(salt + secret))
	return hex.EncodeToString(h[:])
}

// GenerateTokenRecord creates a new token record and returns it along with the plaintext secret.
func GenerateTokenRecord(label string, scope map[TokenScope]ScopeTarget, expiresAt string, createdBy string) (*TokenRecord, string) {
	secret := GenerateSecret()
	salt := generateSalt()
	return &TokenRecord{
		ID:         generateTokenID(),
		SecretHash: hashSecret(salt, secret),
		Salt:       salt,
		Scope:      scope,
		Label:      label,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:  expiresAt,
		CreatedBy:  createdBy,
	}, secret
}

// VerifySecret checks if a plaintext secret matches the stored hash.
func VerifySecret(record *TokenRecord, secret string) bool {
	return hashSecret(record.Salt, secret) == record.SecretHash
}

// IsExpired checks if a token has expired.
func IsExpired(record *TokenRecord) bool {
	if record.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, record.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(t)
}

// HasScope checks if a token record has the required scope for a given group.
func HasScope(record *TokenRecord, required TokenScope, group string) bool {
	return HasScopeForAction(record, required, group, "")
}

// HasScopeForAction checks scope group and, when present, the action allowlist.
func HasScopeForAction(record *TokenRecord, required TokenScope, group string, action string) bool {
	target, ok := record.Scope[required]
	if !ok {
		return false
	}
	if len(target.Groups) > 0 {
		matched := false
		for _, g := range target.Groups {
			if g == group {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(target.Actions) == 0 || action == "" {
		return true
	}
	for _, allowed := range target.Actions {
		if allowed == action {
			return true
		}
	}
	return false
}
