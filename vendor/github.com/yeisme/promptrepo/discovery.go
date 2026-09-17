package promptrepo

import "time"

// DiscoveryRequest lists metadata across registered sources. Pagination counts
// solutions; all matching role/locale documents of a solution stay together.
type DiscoveryRequest struct {
	Repository string `json:"repository,omitempty"`
	Category   string `json:"category,omitempty"`
	Media      string `json:"media,omitempty"`
	Consumer   string `json:"consumer,omitempty"`
	Capability string `json:"capability,omitempty"`
	Locale     string `json:"locale,omitempty"`
	Refresh    bool   `json:"refresh,omitempty"`
	Offline    bool   `json:"offline,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

type DiscoveryCounts struct {
	Repositories int `json:"repositories"`
	Solutions    int `json:"solutions"`
	Roles        int `json:"roles"`
	Documents    int `json:"documents"`
}

type DiscoverySource struct {
	Reason            string          `json:"reason,omitempty"`
	ID                string          `json:"id"`
	SourceKind        string          `json:"source_kind"`
	Enabled           bool            `json:"enabled"`
	State             string          `json:"state"`
	ErrorCode         string          `json:"error_code,omitempty"`
	Revision          string          `json:"revision,omitempty"`
	PinnedRevision    string          `json:"pinned_revision,omitempty"`
	FetchedAt         time.Time       `json:"fetched_at,omitempty"`
	Stale             bool            `json:"stale"`
	Counts            DiscoveryCounts `json:"counts"`
	Matched           DiscoveryCounts `json:"matched"`
	FilteredDocuments int             `json:"filtered_documents"`
}

type DiscoveryTemplate struct {
	Role           string   `json:"role"`
	Locale         string   `json:"locale"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	Ref            string   `json:"ref"`
	OwnerRef       string   `json:"owner_ref,omitempty"`
	Digest         string   `json:"digest"`
	Media          []string `json:"media"`
	Consumers      []string `json:"consumers"`
	CompilerStatus string   `json:"compiler_status"`
	DuplicateOf    []string `json:"duplicate_of,omitempty"`
}

type DiscoverySolution struct {
	RepositoryID string              `json:"repository_id"`
	PackageID    string              `json:"package_id"`
	ID           string              `json:"id"`
	Version      string              `json:"version"`
	Title        string              `json:"title"`
	Summary      string              `json:"summary"`
	Category     string              `json:"category"`
	Capabilities []string            `json:"capabilities"`
	Templates    []DiscoveryTemplate `json:"templates"`
}

type DiscoveryFacet struct {
	Value     string `json:"value"`
	Documents int    `json:"documents"`
}

type DiscoveryResult struct {
	Status       string              `json:"status"`
	Sources      []DiscoverySource   `json:"sources"`
	Counts       DiscoveryCounts     `json:"counts"`
	Matched      DiscoveryCounts     `json:"matched"`
	Returned     DiscoveryCounts     `json:"returned"`
	Remaining    int                 `json:"remaining"`
	Offset       int                 `json:"offset"`
	Solutions    []DiscoverySolution `json:"solutions"`
	Categories   []DiscoveryFacet    `json:"categories"`
	Capabilities []DiscoveryFacet    `json:"capabilities"`
	Consumers    []DiscoveryFacet    `json:"consumers"`
}

// RepositoryPatch changes configuration without overwriting omitted fields.
type RepositoryPatch struct {
	Source        *string `json:"source,omitempty"`
	Revision      *string `json:"revision,omitempty"`
	Trust         *string `json:"trust,omitempty"`
	CredentialRef *string `json:"credential_ref,omitempty"`
}

// EikonaCatalog is the metadata-only owner interchange. It never contains text
// bodies or provider options. Unsupported versions fail closed at the adapter.
type EikonaCatalog struct {
	SchemaVersion string           `json:"schema_version"`
	Entries       []EikonaTemplate `json:"entries"`
}
type EikonaTemplate struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Version  string `json:"version"`
	Digest   string `json:"digest"`
	Media    string `json:"media"`
	OwnerRef string `json:"owner_ref"`
}

// TemplateDiscoveryAnnotation is separate from released TemplateRole and
// Solution structs, preserving their positional composite compatibility.
// These descriptive fields do not grant execution authority.
type TemplateDiscoveryAnnotation struct {
	PackageID      string   `json:"package_id"`
	SolutionID     string   `json:"solution_id"`
	Role           string   `json:"role"`
	Locale         string   `json:"locale"`
	Title          string   `json:"title,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	Media          []string `json:"media,omitempty"`
	Consumers      []string `json:"consumers,omitempty"`
	OwnerRef       string   `json:"owner_ref,omitempty"`
	CompilerStatus string   `json:"compiler_status,omitempty"`
}

func DiscoveryAnnotation(c Catalog, pkg, id, role, locale string) TemplateDiscoveryAnnotation {
	for _, a := range c.Discovery {
		if a.PackageID == pkg && a.SolutionID == id && a.Role == role && a.Locale == locale {
			return a
		}
	}
	return TemplateDiscoveryAnnotation{}
}
