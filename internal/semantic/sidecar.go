package semantic

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/redaction"
)

// SidecarConfig selects the Inferrum sidecar executable and generation store.
type SidecarConfig struct {
	Executable string
	Timeout    time.Duration
	// StoreURI pins a rebuild/search/doctor operation to a generation store.
	// Empty preserves the vault-local default for callers that have not opted
	// into staged generations.
	StoreURI string
}

func sanitizeSidecarStderr(input string) string {
	out := redaction.Cloud(strings.TrimSpace(input))
	out = strings.ReplaceAll(out, "\n", " ")
	if len(out) > 512 {
		out = out[:512] + "..."
	}
	if out == "" {
		return "Run pinax kb doctor --json to inspect the sidecar"
	}
	return out
}

func sidecarStorePath(root, backend string) string {
	return filepath.Join(root, ".pinax", "kb", normalizedBackend(backend))
}

func sidecarStoreURI(root, backend string, cfg SidecarConfig) string {
	if strings.TrimSpace(cfg.StoreURI) != "" {
		return filepath.Clean(cfg.StoreURI)
	}
	return sidecarStorePath(root, backend)
}
