package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EnvSnapshot is the immutable, in-memory decrypted dotenv view used at a
// command or sync-run boundary. It is frozen for the lifetime of one run: the
// daemon may reload a NEW snapshot between runs but a single run always sees the
// same values. Plaintext lives only here and is never persisted, logged or
// passed whole to child processes.
type EnvSnapshot struct {
	values map[string]string
	digest string
	source string
}

// NewEnvSnapshot wraps an unlocked value map into an immutable snapshot. The
// caller must not mutate values after hand-off.
func NewEnvSnapshot(values map[string]string, digest, source string) *EnvSnapshot {
	cp := make(map[string]string, len(values))
	for k, v := range values {
		cp[k] = v
	}
	return &EnvSnapshot{values: cp, digest: digest, source: source}
}

// Lookup returns the value for a key and whether it was present. It is the
// typed-secret read path — callers must NOT dump the whole snapshot into a child
// process environment.
func (s *EnvSnapshot) Lookup(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	v, ok := s.values[key]
	return v, ok
}

// AllKeys returns the sorted list of declared keys (no values) for redacted
// reporting in list/doctor output.
func (s *EnvSnapshot) AllKeys() []string {
	if s == nil {
		return nil
	}
	keys := make([]string, 0, len(s.values))
	for k := range s.values {
		keys = append(keys, k)
	}
	return keys
}

// Digest returns the plaintext document digest used for reload identity checks.
func (s *EnvSnapshot) Digest() string {
	if s == nil {
		return ""
	}
	return s.digest
}

// Source returns a redacted source description (e.g. "pinax-sync.env.age").
func (s *EnvSnapshot) Source() string {
	if s == nil {
		return ""
	}
	return s.source
}

// IsEmpty reports whether the snapshot carries no values.
func (s *EnvSnapshot) IsEmpty() bool {
	return s == nil || len(s.values) == 0
}

// ApplyAllowlist returns a NEW environment map suitable for a child process:
// only the keys in the allowlist are copied from the snapshot. The full
// decrypted environment is never handed to a subprocess.
func (s *EnvSnapshot) ApplyAllowlist(allowlist []string) map[string]string {
	out := make(map[string]string, len(allowlist))
	if s == nil {
		return out
	}
	for _, key := range allowlist {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
	}
	return out
}

// OverlayEnv returns a child-process environment built from a base environment
// plus the allowlisted snapshot values, respecting the precedence contract:
//
//	explicit process environment > decrypted env snapshot
//
// Explicit flags are applied by callers BEFORE this function (they choose the
// base). The snapshot only fills keys that are not already set in base and are
// on the allowlist.
func (s *EnvSnapshot) OverlayEnv(base []string, allowlist []string) []string {
	merged := make(map[string]string, len(base))
	order := make([]string, 0, len(base))
	for _, kv := range base {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		k, v := kv[:idx], kv[idx+1:]
		if _, exists := merged[k]; !exists {
			order = append(order, k)
		}
		merged[k] = v
	}
	for _, key := range allowlist {
		if _, set := merged[key]; set {
			continue // explicit process environment wins
		}
		if v, ok := s.Lookup(key); ok {
			merged[key] = v
			order = append(order, key)
		}
	}
	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, k+"="+merged[k])
	}
	return out
}

// ResolveEnvSnapshot unlocks and parses the encrypted dotenv asset into an
// immutable snapshot. It returns (nil, ErrEnvAssetMissing) when no asset exists
// so callers can distinguish "not configured" from "failed to unlock".
func ResolveEnvSnapshot(root string, provider UnlockProvider) (*EnvSnapshot, error) {
	asset, err := LoadEnvAsset(root)
	if err != nil {
		if errors.Is(err, ErrEnvAssetMissing) {
			return nil, ErrEnvAssetMissing
		}
		return nil, err
	}
	p := provider
	if p == nil {
		resolved, perr := ResolveUnlockProvider(asset.Provider)
		if perr != nil {
			return nil, perr
		}
		p = resolved
	}
	plaintext, err := p.Unlock(SecretEnvelope{
		SchemaVersion: SyncSecretsSchemaVersion,
		Provider:      asset.Provider,
		Secrets: map[string]SecretEntry{
			"env": {
				Identity:   "pinax-sync-env",
				Ciphertext: asset.Ciphertext,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	doc := plaintext["env"]
	values, err := ParseDotenv([]byte(doc))
	if err != nil {
		return nil, err
	}
	return NewEnvSnapshot(values, EnvDocumentDigest([]byte(doc)), asset.Provider), nil
}

// EnvReloader is the daemon-side reload coordinator. It checks the encrypted env
// asset identity (content digest) between sync runs; on change it unlocks and
// parses a new snapshot, then atomically swaps it in for the NEXT run. The
// current run retains its original snapshot. On failure it keeps the last
// successful snapshot and reports a structured degraded code.
type EnvReloader struct {
	root     string
	provider UnlockProvider

	mu           sync.Mutex
	current      *EnvSnapshot
	lastIdentity envAssetIdentity
	lastErrCode  string
}

// envAssetIdentity is the content-based file identity used to detect changes
// between daemon runs. We hash the on-disk ciphertext bytes rather than relying
// on mtime+size, because two distinct plaintexts can produce equal-length
// ciphertexts that would otherwise be missed by a size-only check, and tmpfs
// mtime resolution can be too coarse to distinguish rapid rewrites.
type envAssetIdentity struct {
	digest string
}

// NewEnvReloader creates a reloader. The initial snapshot is loaded lazily on
// the first RunSnapshot call so a missing asset does not block daemon startup.
func NewEnvReloader(root string, provider UnlockProvider) *EnvReloader {
	return &EnvReloader{root: root, provider: provider}
}

// RunSnapshot returns the snapshot a sync run should use, reloading first if the
// encrypted asset changed since the last check. The returned snapshot is frozen
// for the run even if a subsequent reload succeeds. When no asset exists it
// returns (nil, nil). When reload fails it returns the last successful snapshot
// (which may be nil) and a structured ReloadStatus describing the failure.
type ReloadStatus struct {
	Changed  bool
	Loaded   bool
	Degraded bool
	Code     string
}

func (r *EnvReloader) RunSnapshot() (*EnvSnapshot, ReloadStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := ReloadStatus{}
	identity, exists := r.assetIdentity()
	if !exists {
		// No asset configured: not an error, just no snapshot.
		r.lastErrCode = ""
		return nil, status
	}
	if identity == r.lastIdentity {
		// Unchanged: reuse current snapshot for this run boundary.
		return r.current, status
	}
	status.Changed = true
	snapshot, err := ResolveEnvSnapshot(r.root, r.provider)
	if err != nil {
		// Reload failed: retain the last successful snapshot, mark degraded.
		r.lastErrCode = reloadErrorCode(err)
		status.Degraded = true
		status.Code = r.lastErrCode
		return r.current, status
	}
	r.lastIdentity = identity
	r.current = snapshot
	r.lastErrCode = ""
	status.Loaded = true
	return snapshot, status
}

// DegradedCode returns the last reload error code ("" when healthy).
func (r *EnvReloader) DegradedCode() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErrCode
}

// CurrentDigest returns the digest of the currently-active snapshot (for doctor).
func (r *EnvReloader) CurrentDigest() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return ""
	}
	return r.current.Digest()
}

func (r *EnvReloader) assetIdentity() (envAssetIdentity, bool) {
	b, err := os.ReadFile(EnvAssetPath(r.root))
	if err != nil {
		return envAssetIdentity{}, false
	}
	sum := sha256.Sum256(b)
	return envAssetIdentity{digest: hex.EncodeToString(sum[:])}, true
}

func reloadErrorCode(err error) string {
	var dotenvErr *DotenvError
	if errors.As(err, &dotenvErr) {
		return "sync_env_parse_failed"
	}
	var assetErr *EnvAssetError
	if errors.As(err, &assetErr) {
		return "sync_env_unlock_failed"
	}
	var secretErr *SecretEnvelopeError
	if errors.As(err, &secretErr) {
		switch secretErr.Code {
		case "sync_repo_unlock_required":
			return "sync_env_unlock_required"
		case "decrypt_failed":
			return "sync_env_decrypt_failed"
		}
		return "sync_env_unlock_failed"
	}
	return "sync_env_reload_failed"
}

// MaterializeEnv writes the snapshot's plaintext to the managed runtime path
// with 0600 permissions. It is the compatibility exit for external tools that
// cannot consume in-memory injection. Callers MUST pass an allowlist so only
// declared keys are written; the full snapshot is never dumped.
func MaterializeEnv(root string, snapshot *EnvSnapshot, allowlist []string) (string, error) {
	if snapshot == nil {
		return "", &EnvAssetError{Code: "sync_env_empty_snapshot", Message: "cannot materialize an empty env snapshot"}
	}
	values := snapshot.ApplyAllowlist(allowlist)
	if len(values) == 0 {
		return "", &EnvAssetError{Code: "sync_env_empty_allowlist", Message: "no allowlisted keys to materialize"}
	}
	dir := filepath.Join(root, ".pinax", EnvRuntimeDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create env runtime directory: %w", err)
	}
	path := EnvRuntimePath(root)
	// Refuse to overwrite a path that is not our managed file (symlink escape).
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", &EnvAssetError{Code: "sync_env_unsafe_path", Message: "refusing to materialize through a symlink"}
		}
	}
	if err := os.WriteFile(path, FormatDotenv(values), 0o600); err != nil {
		return "", fmt.Errorf("write materialized env: %w", err)
	}
	return path, nil
}

// CleanMaterializedEnv removes only the Pinax-managed materialized env file. It
// never removes arbitrary user-selected files and refuses symlink targets.
func CleanMaterializedEnv(root string) (bool, error) {
	path := EnvRuntimePath(root)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, &EnvAssetError{Code: "sync_env_unsafe_path", Message: "refusing to clean a symlinked env file"}
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
