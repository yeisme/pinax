package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yeisme/promptrepo"
)

// EikonaAdapter reads only the owner's explicit metadata export. Local sources
// name an output root, never executable commands. HTTP sources use the owner's
// credential-filtered MCP action discovery and execution bridge.
type EikonaAdapter struct {
	HTTPClient *http.Client
	Resolver   CredentialResolver
}

func (EikonaAdapter) Kind() string { return "eikona" }
func (a EikonaAdapter) Sync(ctx context.Context, p promptrepo.RepositoryProfile, _ string) (SyncResult, error) {
	var catalog promptrepo.EikonaCatalog
	u, err := url.Parse(p.Source)
	if err != nil {
		return SyncResult{}, eikonaError(promptrepo.CodeInvalidRequest)
	}
	if u.Scheme == "eikona+file" {
		if !filepath.IsAbs(u.Path) || u.Host != "" {
			return SyncResult{}, eikonaError(promptrepo.CodeInvalidRequest)
		}
		if info, err := os.Stat(u.Path); err != nil || !info.IsDir() {
			return SyncResult{}, eikonaError("EIKONA_SOURCE_NOT_FOUND")
		}
		cmd := exec.CommandContext(ctx, "eikona", "--output-root", u.Path, "prompts", "catalog", "list", "--json", "--full")
		var out boundedBuffer
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return SyncResult{}, eikonaError("EIKONA_CLI_UNAVAILABLE_OR_UNSUPPORTED")
		}
		var envelope struct {
			Status string                   `json:"status"`
			Data   promptrepo.EikonaCatalog `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			return SyncResult{}, eikonaError("EIKONA_CATALOG_INVALID")
		}
		if envelope.Status != "success" {
			return SyncResult{}, eikonaError("EIKONA_CATALOG_INCOMPLETE")
		}
		catalog = envelope.Data
	} else {
		u.Scheme = strings.TrimPrefix(u.Scheme, "eikona+")
		if u.Scheme != "http" && u.Scheme != "https" {
			return SyncResult{}, eikonaError(promptrepo.CodeUnsupportedSourceScheme)
		}
		var discovery struct {
			Data struct {
				Actions []struct {
					Action      string         `json:"action"`
					Kind        string         `json:"kind"`
					InputSchema map[string]any `json:"input_schema"`
				} `json:"actions"`
			} `json:"data"`
		}
		if err := a.request(ctx, p, u, "GET", "/api/v1/mcp/actions", nil, &discovery); err != nil {
			return SyncResult{}, err
		}
		found := false
		for _, action := range discovery.Data.Actions {
			if action.Action == "prompts.catalog.list" && action.Kind == "readonly" && action.InputSchema != nil {
				found = true
			}
		}
		if !found {
			return SyncResult{}, eikonaError("EIKONA_CATALOG_UNSUPPORTED")
		}
		var response struct {
			Data struct {
				Status    string                   `json:"status"`
				Truncated bool                     `json:"truncated"`
				Data      promptrepo.EikonaCatalog `json:"data"`
			} `json:"data"`
		}
		body := []byte(`{"action":"prompts.catalog.list","args":{}}`)
		if err := a.request(ctx, p, u, "POST", "/api/v1/mcp/execute", body, &response); err != nil {
			return SyncResult{}, err
		}
		if response.Data.Status != "success" || response.Data.Truncated {
			return SyncResult{}, eikonaError("EIKONA_CATALOG_INCOMPLETE")
		}
		catalog = response.Data.Data
	}
	return eikonaSnapshot(p, catalog)
}
func eikonaSnapshot(p promptrepo.RepositoryProfile, c promptrepo.EikonaCatalog) (SyncResult, error) {
	if c.SchemaVersion != "eikona.template_catalog.v1" {
		return SyncResult{}, eikonaError("EIKONA_CATALOG_UNSUPPORTED")
	}
	catalog := promptrepo.Catalog{SchemaVersion: promptrepo.CatalogSchemaVersion, Repository: promptrepo.RepositoryMetadata{ID: p.ID, DefaultLocale: "en"}, Solutions: []promptrepo.Solution{}}
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(e.ID) || seen[e.ID] || e.Title == "" || e.Version == "" || !strings.HasPrefix(e.OwnerRef, "eikona://") || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(e.Digest) {
			return SyncResult{}, eikonaError("EIKONA_CATALOG_INVALID")
		}
		seen[e.ID] = true
		summary := e.Summary
		if summary == "" {
			summary = "Reusable Eikona " + e.Kind
		}
		media := e.Media
		if media == "" {
			media = "unknown"
		}
		category := e.Category
		if category == "" {
			category = "unknown"
		}
		s := promptrepo.Solution{PackageID: "eikona", ID: e.ID, Version: e.Version, Digest: e.Digest, Category: category, Rights: "owner_managed", Maturity: "unknown", Locales: map[string]promptrepo.LocalizedText{"en": {Title: e.Title, Summary: summary}}, Templates: []promptrepo.TemplateRole{{Role: "main", Locale: "en", Digest: e.Digest}}}
		catalog.Discovery = append(catalog.Discovery, promptrepo.TemplateDiscoveryAnnotation{PackageID: "eikona", SolutionID: e.ID, Role: "main", Locale: "en", Title: e.Title, Summary: summary, Media: []string{media}, Consumers: []string{"eikona"}, OwnerRef: e.OwnerRef, CompilerStatus: "owner_only"})

		catalog.Solutions = append(catalog.Solutions, s)
	}
	digest, err := promptrepo.CanonicalCatalogDigest(catalog)
	if err != nil {
		return SyncResult{}, eikonaError("EIKONA_CATALOG_INVALID")
	}
	catalog.Digest = digest
	return SyncResult{Revision: digest, Catalog: catalog}, nil
}
func (a EikonaAdapter) request(ctx context.Context, p promptrepo.RepositoryProfile, base *url.URL, method, path string, body []byte, out any) error {
	u := *base
	u.Path = strings.TrimRight(base.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return eikonaError(promptrepo.CodeInvalidRequest)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.CredentialRef != "" {
		if a.Resolver == nil {
			return eikonaError(promptrepo.CodeAuthRequired)
		}
		grant, err := a.Resolver.ResolveCredential(ctx, p.CredentialRef, CredentialGrant{Consumer: "template-registry", Capability: "template_discovery", Operation: "read"})
		if err != nil {
			return eikonaError(promptrepo.CodeAuthRequired)
		}
		req.Header.Set("Authorization", "Bearer "+string(grant.Secret))
		clear(grant.Secret)
	}
	client := http.Client{Timeout: 30 * time.Second}
	if a.HTTPClient != nil {
		client = *a.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	req.Header.Del("Authorization")
	if err != nil {
		return eikonaError(promptrepo.CodeSourceFetchFailed)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 {
		return eikonaError(promptrepo.CodeAuthRequired)
	}
	if response.StatusCode == 403 {
		return eikonaError(promptrepo.CodeAuthorizationFailed)
	}
	if response.StatusCode != 200 {
		return eikonaError(promptrepo.CodeSourceFetchFailed)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBytes+1))
	if err != nil || len(raw) > maxCatalogBytes {
		return eikonaError("EIKONA_CATALOG_TOO_LARGE")
	}
	if json.Unmarshal(raw, out) != nil {
		return eikonaError("EIKONA_CATALOG_INVALID")
	}
	return nil
}
func (EikonaAdapter) ReadTemplate(context.Context, promptrepo.RepositoryProfile, promptrepo.SnapshotMetadata, promptrepo.TemplateRole, string) ([]byte, error) {
	return nil, eikonaError("EIKONA_OWNER_READ_REQUIRED")
}
func eikonaError(code string) error {
	return promptrepo.NewError(code, "Eikona metadata discovery did not complete; check owner capability and source configuration", false, nil)
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > maxCatalogBytes-b.Len() {
		return 0, fmt.Errorf("metadata exceeds size limit")
	}
	return b.Buffer.Write(p)
}
