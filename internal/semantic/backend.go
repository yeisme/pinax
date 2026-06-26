package semantic

import (
	"context"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	DefaultBackend = "lancedb"
	FakeBackend    = "fake"
	SidecarSchema  = "pinax.kb.sidecar.v1"
)

type BackendInfo struct {
	Name            string `json:"name"`
	LocalOnly       bool   `json:"local_only"`
	RequiresSidecar bool   `json:"requires_sidecar"`
	Description     string `json:"description,omitempty"`
}

var backendRegistry = map[string]BackendInfo{
	DefaultBackend: {Name: DefaultBackend, LocalOnly: true, RequiresSidecar: true, Description: "LanceDB sidecar projection"},
	FakeBackend:    {Name: FakeBackend, LocalOnly: true, Description: "Deterministic file-backed test projection"},
}

func ListBackends() []BackendInfo {
	infos := make([]BackendInfo, 0, len(backendRegistry))
	for _, info := range backendRegistry {
		infos = append(infos, info)
	}
	sort.SliceStable(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

func Save(ctx context.Context, root string, chunks []Chunk, backend string, sidecar SidecarConfig, documents int) (string, error) {
	backend = normalizedBackend(backend)
	switch backend {
	case FakeBackend:
		store := NewFileStore(root, backend)
		if err := store.Save(chunks); err != nil {
			return "", err
		}
		_ = writeStoreMetadata(root, backend, chunks)
		return store.Path(), nil
	case DefaultBackend:
		if err := runSidecarRebuild(ctx, root, chunks, backend, sidecar, documents); err != nil {
			return "", err
		}
		_ = writeStoreMetadata(root, backend, chunks)
		return sidecarStorePath(root, backend), nil
	default:
		return "", invalidBackendError()
	}
}

func Doctor(ctx context.Context, root, backend string, sidecar SidecarConfig) (map[string]any, error) {
	backend = normalizedBackend(backend)
	if backend == FakeBackend {
		return map[string]any{"backend": FakeBackend, "available": true, "dependency": "built-in fake store"}, nil
	}
	if backend != DefaultBackend {
		return nil, invalidBackendError()
	}
	return runSidecarDoctor(ctx, root, backend, sidecar)
}

func normalizedBackend(backend string) string {
	if strings.TrimSpace(backend) == "" {
		return DefaultBackend
	}
	return strings.ToLower(strings.TrimSpace(backend))
}

func invalidBackendError() *domain.CommandError {
	return &domain.CommandError{Code: "backend_invalid", Message: "Semantic backend is not supported", Hint: "Use --backend lancedb or --backend fake"}
}
