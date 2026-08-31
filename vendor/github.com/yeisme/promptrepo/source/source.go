package source

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yeisme/promptrepo"
)

type SyncResult struct {
	Revision string
	Catalog  promptrepo.Catalog
}

type Adapter interface {
	Kind() string
	Sync(context.Context, promptrepo.RepositoryProfile, string) (SyncResult, error)
	ReadTemplate(context.Context, promptrepo.RepositoryProfile, promptrepo.SnapshotMetadata, promptrepo.TemplateRole, string) ([]byte, error)
}

// CompanionReader is optional so third-party Adapter implementations remain
// source compatible. Built-in adapters implement it for bounded metadata
// sidecars; callers must not use it to expose prompt bodies.
type CompanionReader interface {
	ReadCompanion(context.Context, promptrepo.RepositoryProfile, promptrepo.SnapshotMetadata, string, string) ([]byte, error)
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		registry.adapters[adapter.Kind()] = adapter
	}
	return registry
}

func DefaultRegistry() *Registry {
	return NewRegistry(LocalAdapter{}, GitAdapter{}, S3Adapter{HTTPClient: &http.Client{Timeout: 30 * time.Second}})
}

func DetectKind(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid repository source URI", false, err)
	}
	switch parsed.Scheme {
	case "file":
		return "file", nil
	case "git+file", "git+https", "git+ssh", "github":
		return "git", nil
	case "s3":
		return "s3", nil
	default:
		return "", promptrepo.NewError(promptrepo.CodeUnsupportedSourceScheme, "unsupported repository source scheme", false, nil)
	}
}

func (r *Registry) Resolve(raw string) (Adapter, error) {
	kind, err := DetectKind(raw)
	if err != nil {
		return nil, err
	}
	adapter, ok := r.adapters[kind]
	if !ok {
		return nil, promptrepo.NewError(promptrepo.CodeUnsupportedSourceScheme, "repository source capability is not available in this build", false, nil)
	}
	return adapter, nil
}
