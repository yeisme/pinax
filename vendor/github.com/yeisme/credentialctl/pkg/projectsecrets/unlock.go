package projectsecrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// UnlockSource produces the unlock secret bytes for one resolution and reports
// a stable descriptor (prompt | keychain | file | env) for redacted metadata.
// Implementations must fail closed on missing/wrong input and must never print
// the secret to stdout/stderr.
type UnlockSource interface {
	Secret(ctx context.Context) ([]byte, error)
	Descriptor() string
}

// StaticSource returns a fixed secret, primarily for tests and CI where the
// passphrase arrives out-of-band. The caller is responsible for wiping the
// input slice after construction; this source copies internally.
func StaticSource(secret []byte) UnlockSource {
	dup := make([]byte, len(secret))
	copy(dup, secret)
	return &staticSource{secret: dup}
}

type staticSource struct {
	secret []byte
}

func (s *staticSource) Secret(context.Context) ([]byte, error) {
	out := make([]byte, len(s.secret))
	copy(out, s.secret)
	return out, nil
}
func (s *staticSource) Descriptor() string { return "static" }

// EnvSource reads the unlock secret from a named environment variable. Status
// surfaces only the variable name and configured=true|false, never the value.
func EnvSource(name string) UnlockSource { return &envSource{name: name} }

type envSource struct{ name string }

func (e *envSource) Secret(context.Context) ([]byte, error) {
	v, ok := os.LookupEnv(e.name)
	if !ok || v == "" {
		return nil, UnlockRequiredError("env " + e.name + " not set")
	}
	return []byte(v), nil
}
func (e *envSource) Descriptor() string { return "env" }

// FileSource reads the unlock secret from a regular file with owner-only
// permissions. The path must be explicit (no ~ expansion by the caller's
// shell); non-regular files, group/world-readable files and missing files fail
// closed.
func FileSource(path string) (UnlockSource, error) {
	if path == "" {
		return nil, fmt.Errorf("projectsecrets: empty file source path")
	}
	clean, err := validateSecretFile(path)
	if err != nil {
		return nil, err
	}
	return &fileSource{path: clean}, nil
}

type fileSource struct{ path string }

func (f *fileSource) Secret(context.Context) ([]byte, error) {
	if _, err := validateSecretFile(f.path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.path)
	if err != nil {
		return nil, UnlockFailedError("read secret file failed")
	}
	return trimTrailingNewline(raw), nil
}
func (f *fileSource) Descriptor() string { return "file" }

// validateSecretFile confirms the path is a regular file and, on POSIX, that
// its permission bits do not grant group/other read access. A symlink target
// is resolved and the same check applies to the target.
func validateSecretFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", UnlockRequiredError("secret file missing")
		}
		return "", UnlockFailedError("secret file resolve failed")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", UnlockRequiredError("secret file missing")
	}
	if !info.Mode().IsRegular() {
		return "", UnlockFailedError("secret file not regular")
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm()&0o077 != 0 {
			return "", UnlockFailedError("secret file group/world readable")
		}
	}
	return resolved, nil
}

func trimTrailingNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// PromptSource reads the passphrase from the controlling TTY with echo
// disabled. It never reads from the standard stdin that may carry a secret
// payload. When no controlling TTY is available it fails with
// UnlockRequiredError so the caller can suggest a keychain/file/env source
// instead of guessing stdin.
type PromptSource struct {
	prompt string
	out    *os.File
}

// PromptOption configures a PromptSource.
type PromptOption func(*PromptSource)

// WithPromptText overrides the default prompt label.
func WithPromptText(text string) PromptOption {
	return func(p *PromptSource) { p.prompt = text }
}

// NewPromptSource builds a TTY prompt source. The default input is /dev/tty
// (or os.Stdin on Windows) and output is os.Stderr so prompts never pollute
// machine stdout.
func NewPromptSource(opts ...PromptOption) *PromptSource {
	p := &PromptSource{prompt: "Passphrase: ", out: os.Stderr}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *PromptSource) Descriptor() string { return "prompt" }

// Secret reads a single passphrase line from the controlling TTY with echo
// disabled. Prompt text goes to stderr; the secret never touches stdout.
func (p *PromptSource) Secret(ctx context.Context) ([]byte, error) {
	tty, err := openControllingTTY()
	if err != nil {
		return nil, UnlockRequiredError("no controlling tty for prompt unlock")
	}
	defer tty.Close()
	out := p.out
	if out == nil {
		out = os.Stderr
	}
	if p.prompt != "" {
		if _, err := out.Write([]byte(p.prompt)); err != nil {
			return nil, UnlockFailedError("prompt write failed")
		}
	}
	secret, err := readPassword(tty)
	if err != nil {
		return nil, UnlockFailedError("prompt read failed")
	}
	return secret, nil
}

// Remember stores the repository passphrase in the scoped macOS Keychain
// item. The secret is provided through the child process stdin and is never
// placed in the process argument vector.
func (k *KeychainSource) Remember(ctx context.Context, secret []byte) error {
	if len(secret) == 0 {
		return UnlockRequiredError("empty keychain secret")
	}
	execPath, err := k.executablePath()
	if err != nil {
		return err
	}
	cmdFn := k.execCommand
	if cmdFn == nil {
		cmdFn = exec.Command
	}
	cmd := cmdFn(execPath,
		"add-generic-password",
		"-U",
		"-s", k.service,
		"-a", k.account,
		"-w",
	)
	payload := make([]byte, len(secret)+1)
	copy(payload, secret)
	payload[len(payload)-1] = '\n'
	defer func() {
		for i := range payload {
			payload[i] = 0
		}
	}()
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		classified := classifyKeychainError(err, out)
		for i := range out {
			out[i] = 0
		}
		return classified
	}
	for i := range out {
		out[i] = 0
	}
	return nil
}

// Delete removes the scoped repository passphrase from macOS Keychain.
func (k *KeychainSource) Delete(ctx context.Context) error {
	execPath, err := k.executablePath()
	if err != nil {
		return err
	}
	cmdFn := k.execCommand
	if cmdFn == nil {
		cmdFn = exec.Command
	}
	cmd := cmdFn(execPath,
		"delete-generic-password",
		"-s", k.service,
		"-a", k.account,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		classified := classifyKeychainError(err, out)
		for i := range out {
			out[i] = 0
		}
		return classified
	}
	for i := range out {
		out[i] = 0
	}
	return nil
}

func (k *KeychainSource) executablePath() (string, error) {
	if runtime.GOOS != "darwin" && !k.fakeExec {
		return "", KeychainUnavailableError("keychain only available on macOS")
	}
	execPath := k.execPath
	if execPath == "" {
		execPath = defaultSecurityPath()
	}
	if _, err := os.Stat(execPath); err != nil {
		return "", KeychainUnavailableError("security executable unavailable")
	}
	return execPath, nil
}

// KeychainSource reads the unlock secret from the macOS Keychain via the fixed
// /usr/bin/security executable. Service/account metadata may be surfaced
// redacted; the value is never printed. On non-darwin hosts or when the
// Keychain is locked/missing/denied, it fails with KeychainUnavailableError.
//
// The executable path is fixed by the library; the caller cannot override it
// (no shell, no PATH lookup) to prevent hijacking.
type KeychainSource struct {
	service     string
	account     string
	execPath    string
	fakeExec    bool // test injected a fake executable; skip darwin guard
	execCommand func(name string, args ...string) *exec.Cmd
}

// KeychainOption configures a KeychainSource.
type KeychainOption func(*KeychainSource)

// WithKeychainExecutable overrides the security executable path. This is
// intended for tests using a fake executable; production callers must not use
// it so the fixed system path cannot be bypassed. Injecting a fake executable
// also relaxes the macOS-only guard so the adapter logic is testable on Linux CI.
func WithKeychainExecutable(path string) KeychainOption {
	return func(k *KeychainSource) { k.execPath = path; k.fakeExec = true }
}

// WithKeychainCommand injects an exec function (tests only).
func WithKeychainCommand(fn func(name string, args ...string) *exec.Cmd) KeychainOption {
	return func(k *KeychainSource) { k.execCommand = fn }
}

// NewKeychainSource builds a macOS Keychain source for a repository-scoped
// service/account.
func NewKeychainSource(service, account string, opts ...KeychainOption) (*KeychainSource, error) {
	if service == "" || account == "" {
		return nil, fmt.Errorf("projectsecrets: keychain service and account required")
	}
	k := &KeychainSource{service: service, account: account, execPath: defaultSecurityPath()}
	for _, o := range opts {
		o(k)
	}
	return k, nil
}

func (k *KeychainSource) Descriptor() string { return "keychain" }

func (k *KeychainSource) Secret(ctx context.Context) ([]byte, error) {
	execPath, err := k.executablePath()
	if err != nil {
		return nil, err
	}
	cmdFn := k.execCommand
	if cmdFn == nil {
		cmdFn = exec.Command
	}
	cmd := cmdFn(execPath,
		"find-generic-password",
		"-s", k.service,
		"-a", k.account,
		"-w",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, classifyKeychainError(err, out)
	}
	trimmed := trimTrailingNewline(out)
	// Copy into a fresh buffer so wiping the captured exec output does not also
	// zero the returned secret (trimTrailingNewline aliases the input slice).
	secret := make([]byte, len(trimmed))
	copy(secret, trimmed)
	for i := range out {
		out[i] = 0
	}
	return secret, nil
}

// AccountDigest returns a non-sensitive short digest of the account name so a
// doctor/envelope can record "a keychain item exists" without storing the value.
func (k *KeychainSource) AccountDigest() string {
	return shortDigest(k.service + "/" + k.account)
}

// Service and Account expose the non-sensitive identifiers.
func (k *KeychainSource) Service() string { return k.service }
func (k *KeychainSource) Account() string { return k.account }

func defaultSecurityPath() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return "/usr/bin/security"
}

func classifyKeychainError(err error, out []byte) error {
	msg := strings.ToLower(err.Error() + " " + string(out))
	switch {
	case strings.Contains(msg, "could not be found") || strings.Contains(msg, "item not found"):
		return KeychainUnavailableError("keychain item missing")
	case strings.Contains(msg, "user canceled") || strings.Contains(msg, "denied") || strings.Contains(msg, "interaction"):
		return KeychainUnavailableError("keychain access denied")
	case strings.Contains(msg, "locked"):
		return KeychainUnavailableError("keychain locked")
	default:
		return KeychainUnavailableError("keychain unavailable")
	}
}

// mapSourceError normalizes an unlock-source error to a projectsecrets Error.
func mapSourceError(err error) error {
	var perr *Error
	if errors.As(err, &perr) {
		return perr
	}
	return UnlockFailedError("unlock source failed")
}

var readPasswordMu sync.Mutex
