package inferrum

import (
	"context"
	"sort"
)

// Store is the domain-multiplexed vector store entry point. It is the
// architecture-contract surface that the retrieval pipeline and CLI commands
// consume. A concrete implementation lives in store.go.
type Store interface {
	// Doctor checks backend/sidecar health.
	Doctor(ctx context.Context) (Status, error)
	// Rebuild (re)creates the vector index from the given records.
	Rebuild(ctx context.Context, records []Record, opts RebuildOpts) (int, error)
	// Search runs a vector search constrained to allowedIDs.
	Search(ctx context.Context, queryVec []float64, allowedIDs []string, limit int) ([]Hit, int, error)
}

// BackendInfo describes a vector backend for listing and doctor output.
type BackendInfo struct {
	Name            string `json:"name"`
	LocalOnly       bool   `json:"local_only"`
	RequiresSidecar bool   `json:"requires_sidecar"`
	Description     string `json:"description,omitempty"`
}

const (
	DefaultBackend = "lancedb"
	FakeBackend    = "fake"
)

var backendRegistry = map[string]BackendInfo{
	DefaultBackend: {Name: DefaultBackend, LocalOnly: true, RequiresSidecar: true, Description: "LanceDB sidecar projection"},
	FakeBackend:    {Name: FakeBackend, LocalOnly: true, Description: "Deterministic in-memory test projection"},
}

// ListBackends returns info for every registered backend sorted by name.
func ListBackends() []BackendInfo {
	infos := make([]BackendInfo, 0, len(backendRegistry))
	for _, info := range backendRegistry {
		infos = append(infos, info)
	}
	sort.SliceStable(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}
