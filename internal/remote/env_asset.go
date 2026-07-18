package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvAssetSchemaVersion is the frozen contract version for the encrypted dotenv
// asset. The asset is repository-tracked; its plaintext is never required to be
// committed and lives only in the in-memory EnvSnapshot at runtime.
const EnvAssetSchemaVersion = "pinax.sync.env.v1"

// EnvAssetFileName is the fixed, repository-tracked ciphertext dotenv asset.
// The path is fixed (not user-selectable) to prevent path-escape and protected-
// path bypass. Plaintext runtime files live under a separate managed directory.
const EnvAssetFileName = "pinax-sync.env.age"

// EnvRuntimeDir is the managed, 0600, Git-ignored directory for materialized
// plaintext env files. Only --materialize writes here; default runs stay in memory.
const EnvRuntimeDir = "runtime"

// EnvRuntimeFileName is the single managed materialized plaintext filename.
const EnvRuntimeFileName = "pinax-sync.env"

// EnvAssetPath returns the fixed path to the encrypted dotenv asset.
func EnvAssetPath(root string) string {
	return filepath.Join(root, ".pinax", EnvAssetFileName)
}

// EnvRuntimePath returns the fixed path to the materialized plaintext env file.
func EnvRuntimePath(root string) string {
	return filepath.Join(root, ".pinax", EnvRuntimeDir, EnvRuntimeFileName)
}

// EnvAsset is the repository-tracked encrypted dotenv asset. The schema carries
// ciphertext and redacted metadata only; plaintext key/value pairs live only in
// the in-memory EnvSnapshot. The asset is safe to commit, while plaintext env
// files are Git-ignored and Capsa-protected.
type EnvAsset struct {
	SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	Provider      string `json:"provider" yaml:"provider"`
	// Ciphertext is provider-specific opaque encrypted material for the whole
	// dotenv document (not per-key). Keeping one ciphertext preserves key
	// ordering and avoids leaking key names into the asset metadata.
	Ciphertext string `json:"ciphertext" yaml:"ciphertext"`
	// Digest is a short stable hex digest of the plaintext document, used for
	// daemon reload identity checks. It does NOT reveal plaintext contents.
	Digest string `json:"digest,omitempty" yaml:"digest,omitempty"`
	// KeyNames is the redacted list of declared keys (no values) so list/doctor
	// can report what is present without unlocking.
	KeyNames []string `json:"key_names,omitempty" yaml:"key_names,omitempty"`
}

// ErrEnvAssetMissing is returned when no pinax-sync.env.age exists.
var ErrEnvAssetMissing = errors.New("sync env asset not found")

// LoadEnvAsset reads the encrypted dotenv asset. A missing file is reported via
// ErrEnvAssetMissing so init/doctor can distinguish absent from corrupt.
func LoadEnvAsset(root string) (EnvAsset, error) {
	b, err := os.ReadFile(EnvAssetPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EnvAsset{}, ErrEnvAssetMissing
		}
		return EnvAsset{}, err
	}
	var asset EnvAsset
	if err := yaml.Unmarshal(b, &asset); err != nil {
		return EnvAsset{}, fmt.Errorf("parse env asset: %w", err)
	}
	if asset.SchemaVersion != "" && asset.SchemaVersion != EnvAssetSchemaVersion {
		return EnvAsset{}, &EnvAssetError{Code: "unsupported_env_schema", Message: fmt.Sprintf("unsupported env schema version: %s", asset.SchemaVersion)}
	}
	return asset, nil
}

// SaveEnvAsset persists the encrypted dotenv asset with restrictive permissions.
// It never writes plaintext; ciphertext + redacted metadata only.
func SaveEnvAsset(root string, asset EnvAsset) error {
	asset.SchemaVersion = EnvAssetSchemaVersion
	if strings.TrimSpace(asset.Provider) == "" {
		return &EnvAssetError{Code: "missing_provider", Message: "env asset provider is required"}
	}
	if strings.TrimSpace(asset.Ciphertext) == "" {
		return &EnvAssetError{Code: "missing_ciphertext", Message: "env asset ciphertext is required"}
	}
	path := EnvAssetPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create env asset directory: %w", err)
	}
	data, err := yaml.Marshal(asset)
	if err != nil {
		return fmt.Errorf("marshal env asset: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// EnvAssetError is the stable error type for env asset operations. Code is a
// stable English identifier; Message never carries plaintext values.
type EnvAssetError struct {
	Code    string
	Message string
}

func (e *EnvAssetError) Error() string { return e.Message }

// --- strict dotenv parser ---

// ParseDotenv parses a strict, safe dotenv subset. It accepts:
//
//	KEY=value
//	KEY="quoted value"
//	KEY='quoted value'
//
// and rejects shell execution, include directives, recursive ${...} expansion,
// command substitution $() and backticks, NUL / control characters, empty keys,
// duplicate keys and multi-line heredocs. Errors report line number and key name
// only — never the rejected value.
//
// This is intentionally a strict subset of POSIX dotenv so Pinax never evaluates
// attacker-controlled shell syntax stored in the repository.
func ParseDotenv(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	seen := make(map[string]int)
	lines := strings.Split(string(data), "\n")
	for idx, raw := range lines {
		lineNo := idx + 1
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Reject include/source directives before any key parsing.
		if isDotenvDirective(trimmed) {
			return nil, &DotenvError{Code: "dotenv_directive_forbidden", Line: lineNo, Message: fmt.Sprintf("line %d: include/source directives are forbidden in strict dotenv", lineNo)}
		}
		eq := strings.IndexByte(trimmed, '=')
		if eq <= 0 {
			return nil, &DotenvError{Code: "dotenv_missing_equals", Line: lineNo, Message: fmt.Sprintf("line %d: expected KEY=value", lineNo)}
		}
		key := strings.TrimSpace(trimmed[:eq])
		if key == "" {
			return nil, &DotenvError{Code: "dotenv_empty_key", Line: lineNo, Message: fmt.Sprintf("line %d: empty key", lineNo)}
		}
		if err := validateDotenvKey(key, lineNo); err != nil {
			return nil, err
		}
		value := trimmed[eq+1:]
		value, err := unquoteDotenvValue(value, lineNo)
		if err != nil {
			return nil, err
		}
		if err := validateDotenvValue(value, lineNo, key); err != nil {
			return nil, err
		}
		if prev, ok := seen[key]; ok {
			return nil, &DotenvError{Code: "dotenv_duplicate_key", Line: lineNo, Key: key, Message: fmt.Sprintf("line %d: duplicate key %q (first at line %d)", lineNo, key, prev)}
		}
		seen[key] = lineNo
		out[key] = value
	}
	return out, nil
}

// FormatDotenv renders values back into a canonical strict dotenv document with
// stable key ordering. Values are single-quoted when they contain special chars
// so the round-trip survives ParseDotenv without ambiguity.
func FormatDotenv(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(quoteDotenvValue(values[k]))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// DotenvError reports a strict dotenv parse failure with a stable English code
// and line number. Key is included only when known; the rejected value is never
// attached so errors stay safe to log and surface in receipts.
type DotenvError struct {
	Code    string
	Line    int
	Key     string
	Message string
}

func (e *DotenvError) Error() string { return e.Message }

func isDotenvDirective(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "source ") ||
		strings.HasPrefix(lower, "source\t") ||
		strings.HasPrefix(lower, ". ") ||
		strings.HasPrefix(lower, ".\t") ||
		strings.HasPrefix(lower, "include ") ||
		strings.HasPrefix(lower, "include\t") ||
		strings.HasPrefix(lower, "export ")
}

// dotenvKeyPattern is the strict allowlist for keys: ASCII letters, digits and
// underscore, starting with a letter or underscore (POSIX env name).
// var dotenvKeyPattern handled by validateDotenvKey to keep error codes stable.

func validateDotenvKey(key string, lineNo int) error {
	for i, r := range key {
		if r == '_' {
			continue
		}
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' && i > 0 {
			continue
		}
		return &DotenvError{Code: "dotenv_invalid_key", Line: lineNo, Key: key, Message: fmt.Sprintf("line %d: invalid key %q; keys must be [A-Za-z_][A-Za-z0-9_]*", lineNo, key)}
	}
	return nil
}

// validateDotenvValue rejects shell execution vectors and unsafe bytes without
// disclosing the value. It runs BEFORE unquoting (on the raw segment) and AFTER
// (on the final value) so a quoted wrapper cannot smuggle forbidden content.
func validateDotenvValue(value string, lineNo int, key string) error {
	if strings.ContainsRune(value, 0) {
		return &DotenvError{Code: "dotenv_nul_byte", Line: lineNo, Key: key, Message: fmt.Sprintf("line %d: NUL byte forbidden in value for key %q", lineNo, key)}
	}
	for _, r := range value {
		if r < 0x20 && r != '\t' {
			return &DotenvError{Code: "dotenv_control_char", Line: lineNo, Key: key, Message: fmt.Sprintf("line %d: control character forbidden in value for key %q", lineNo, key)}
		}
	}
	// Reject command-substitution / shell-expansion sentinels even inside quotes.
	for _, sentinel := range []string{"$(", "`", "${"} {
		if strings.Contains(value, sentinel) {
			return &DotenvError{Code: "dotenv_shell_substitution", Line: lineNo, Key: key, Message: fmt.Sprintf("line %d: shell substitution %q forbidden in value for key %q", lineNo, sentinel, key)}
		}
	}
	return nil
}

// unquoteDotenvValue strips one layer of matching surrounding quotes. It does
// NOT interpret escape sequences or expand variables — strict subset only.
func unquoteDotenvValue(value string, lineNo int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	raw := value
	// Pre-check forbidden sentinels on the raw token so quoted wrappers fail too.
	if err := validateDotenvValue(raw, lineNo, ""); err != nil {
		return "", err
	}
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			inner := value[1 : len(value)-1]
			if first == '\'' {
				// Single-quoted: support the standard dotenv '\'' escape for an
				// embedded single quote. Any other embedded quote is an error.
				if strings.Contains(inner, "'") && !strings.Contains(inner, `'\''`) {
					return "", &DotenvError{Code: "dotenv_unterminated_quote", Line: lineNo, Message: fmt.Sprintf("line %d: unterminated or embedded quote", lineNo)}
				}
				return strings.ReplaceAll(inner, `'\''`, "'"), nil
			}
			// Double-quoted: an embedded double quote is an error (strict subset
			// performs no escape processing beyond the single-quote '\'' form).
			if strings.ContainsRune(inner, '"') {
				return "", &DotenvError{Code: "dotenv_unterminated_quote", Line: lineNo, Message: fmt.Sprintf("line %d: unterminated or embedded quote", lineNo)}
			}
			return inner, nil
		}
		if first == '"' || first == '\'' || last == '"' || last == '\'' {
			return "", &DotenvError{Code: "dotenv_unterminated_quote", Line: lineNo, Message: fmt.Sprintf("line %d: unterminated quote", lineNo)}
		}
	}
	return value, nil
}

// quoteDotenvValue wraps a value in quotes when it contains characters that
// would otherwise need escaping. It chooses the quote style that avoids escapes
// when possible (double-quote if the value has single quotes, single-quote if it
// has double quotes) and falls back to the standard dotenv '\” escape only when
// both quote types appear.
func quoteDotenvValue(value string) string {
	if value == "" {
		return ""
	}
	if !needsDotenvQuote(value) {
		return value
	}
	hasSingle := strings.ContainsRune(value, '\'')
	hasDouble := strings.ContainsRune(value, '"')
	switch {
	case !hasDouble:
		return `"` + value + `"`
	case !hasSingle:
		return "'" + value + "'"
	default:
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}
}

func needsDotenvQuote(value string) bool {
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '#' || r == '"' || r == '\'' || r == '\\' || r == '$' || r == '`' {
			return true
		}
	}
	return false
}

// EnvDocumentDigest returns a short stable hex digest of a plaintext dotenv
// document, used for daemon reload identity checks. It is computed over the raw
// bytes so it does not reveal structure beyond the digest.
func EnvDocumentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}
