package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/redaction"
)

var secretAssignmentPattern = regexp.MustCompile(`(?i)(refresh_token=|client_secret=)[^\s&]+`)

func init() {
	Register("rclone", func(ctx context.Context, endpoint string) (BlobStore, error) {
		backend, err := NewRcloneBackend(endpoint)
		if err != nil {
			return nil, err
		}
		return backend, nil
	})
}

type RcloneBackend struct {
	remote string
	prefix string
	binary string
}

func NewRcloneBackend(endpoint string) (*RcloneBackend, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid rclone URI: %w", err)
	}
	if strings.ToLower(u.Scheme) != "rclone" || strings.TrimSpace(u.Host) == "" {
		return nil, fmt.Errorf("invalid rclone URI: remote name is required")
	}
	binary := strings.TrimSpace(os.Getenv("PINAX_RCLONE_BIN"))
	if binary == "" {
		binary = "rclone"
	}
	return &RcloneBackend{remote: strings.Trim(u.Host, "/"), prefix: strings.Trim(u.Path, "/"), binary: binary}, nil
}

func (b *RcloneBackend) SupportsConditionalWrites() bool { return false }

func (b *RcloneBackend) Get(ctx context.Context, key string) ([]byte, string, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, "", err
	}
	out, err := b.run(ctx, "cat", b.target(key))
	if err != nil {
		if isRcloneMissing(err) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", err
	}
	return out, computeRev(out), nil
}

func (b *RcloneBackend) Put(ctx context.Context, key string, data []byte, _ string) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "pinax-rclone-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if _, err := b.run(ctx, "copyto", tmpName, b.target(key)); err != nil {
		return "", err
	}
	return computeRev(data), nil
}

func (b *RcloneBackend) Stat(ctx context.Context, key string) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	prefix, name := path.Split(strings.Trim(key, "/"))
	entries, err := b.listDir(ctx, strings.Trim(prefix, "/"))
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.name == name {
			return entry.revision(), nil
		}
	}
	return "", ErrObjectNotFound
}

func (b *RcloneBackend) Delete(ctx context.Context, key string) error {
	if err := validateObjectKey(key); err != nil {
		return err
	}
	_, err := b.run(ctx, "deletefile", b.target(key))
	if err != nil && isRcloneMissing(err) {
		return nil
	}
	return err
}

func (b *RcloneBackend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	if prefix != "" {
		if err := validateObjectKey(prefix); err != nil {
			return nil, err
		}
	}
	entries, err := b.listDir(ctx, strings.Trim(prefix, "/"))
	if err != nil {
		return nil, err
	}
	base := strings.Trim(prefix, "/")
	objects := make([]ObjectInfo, 0, len(entries))
	for _, entry := range entries {
		key := entry.name
		if base != "" {
			key = path.Join(base, key)
		}
		objects = append(objects, ObjectInfo{Key: key, Size: entry.size, Revision: entry.revision(), LastModified: entry.modTime})
	}
	return objects, nil
}

func (b *RcloneBackend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.Stat(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrObjectNotFound) {
		return false, nil
	}
	return false, err
}

func (b *RcloneBackend) BatchStat(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		rev, err := b.Stat(ctx, key)
		if err == nil {
			result[key] = rev
		} else if !errors.Is(err, ErrObjectNotFound) {
			return nil, err
		}
	}
	return result, nil
}

type rcloneEntry struct {
	name    string
	size    int64
	modTime time.Time
	line    string
}

func (e rcloneEntry) revision() string {
	h := sha256.Sum256([]byte(e.line))
	return hex.EncodeToString(h[:])
}

func (b *RcloneBackend) listDir(ctx context.Context, prefix string) ([]rcloneEntry, error) {
	target := b.target(prefix)
	out, err := b.run(ctx, "lsf", "--format", "spt", "--files-only", "--recursive", target)
	if err != nil {
		if isRcloneMissing(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]rcloneEntry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ";", 3)
		if len(parts) < 2 {
			continue
		}
		size, _ := strconv.ParseInt(parts[0], 10, 64)
		modTime := time.Time{}
		if len(parts) == 3 {
			modTime, _ = time.Parse(time.RFC3339, parts[2])
		}
		entries = append(entries, rcloneEntry{name: strings.Trim(parts[1], "/"), size: size, modTime: modTime, line: line})
	}
	return entries, nil
}

func (b *RcloneBackend) target(key string) string {
	key = strings.Trim(key, "/")
	remotePath := b.prefix
	if key != "" {
		remotePath = path.Join(remotePath, key)
	}
	if remotePath == "." {
		remotePath = ""
	}
	return b.remote + ":" + remotePath
}

func (b *RcloneBackend) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, rcloneCommandError{op: firstArg(args), err: err, stderr: sanitizeRcloneStderr(stderr.String())}
	}
	return out, nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return "rclone"
	}
	return args[0]
}

type rcloneCommandError struct {
	op     string
	err    error
	stderr string
}

func (e rcloneCommandError) Error() string {
	if strings.TrimSpace(e.stderr) == "" {
		return fmt.Sprintf("rclone %s failed: %v", e.op, e.err)
	}
	return fmt.Sprintf("rclone %s failed: %v: %s", e.op, e.err, e.stderr)
}

func (e rcloneCommandError) Unwrap() error { return e.err }

func isRcloneMissing(err error) bool {
	var cmdErr rcloneCommandError
	if !errors.As(err, &cmdErr) {
		return false
	}
	stderr := strings.ToLower(cmdErr.stderr)
	return strings.Contains(stderr, "not found") || strings.Contains(stderr, "no such") || strings.Contains(stderr, "doesn't exist") || strings.Contains(stderr, "not exist")
}

func sanitizeRcloneStderr(input string) string {
	out := redaction.Cloud(input)
	out = secretAssignmentPattern.ReplaceAllString(out, "${1}[REDACTED_SECRET]")
	out = strings.TrimSpace(out)
	if len(out) > 512 {
		out = out[:512] + "…"
	}
	return out
}

func validateObjectKey(key string) error {
	if strings.ContainsRune(key, '\x00') || filepath.IsAbs(key) {
		return fmt.Errorf("unsafe object key: %q", key)
	}
	slashKey := strings.ReplaceAll(key, "\\", "/")
	if path.IsAbs(slashKey) {
		return fmt.Errorf("unsafe object key: %q", key)
	}
	cleaned := path.Clean(slashKey)
	if cleaned == "." {
		cleaned = ""
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return fmt.Errorf("unsafe object key: %q", key)
	}
	return nil
}
