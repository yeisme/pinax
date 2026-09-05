package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yeisme/promptrepo"
)

// CredentialGrant is the scoped, secret-free claim under which one credential
// ref is resolved at its use point. It mirrors credentialctl's scoped grant
// shape without importing that module: the (consumer, ref, capability)
// allowlist is enforced by the resolver implementation, not here.
type CredentialGrant struct {
	Consumer   string
	Capability string
	Operation  string
}

// CredentialResolution carries the resolved value for exactly one operation.
// The value lives only in memory; adapters clear it as soon as the outbound
// request has been built and completed.
type CredentialResolution struct {
	Secret []byte
}

// CredentialResolver resolves a credential ref at use time. Implementations
// must enforce the consumer/capability allowlist, fail closed with
// secret-free typed denials, apply revocation on the very next call, and
// never cache resolutions across processes.
type CredentialResolver interface {
	ResolveCredential(ctx context.Context, ref string, grant CredentialGrant) (CredentialResolution, error)
}

// S3CredentialAdapter is the credential-aware S3 source adapter. When a
// profile carries a CredentialRef, the ref is resolved through the resolver
// for each request and the value is used as the Authorization bearer token
// for that single request only. The value never lands in profiles,
// snapshots, receipts, errors or logs; profiles without a CredentialRef
// behave exactly like the unsigned S3Adapter.
type S3CredentialAdapter struct {
	S3Adapter
	Resolver   CredentialResolver
	Consumer   string
	Capability string
}

func (adapter S3CredentialAdapter) Sync(ctx context.Context, profile promptrepo.RepositoryProfile, cache string) (SyncResult, error) {
	if strings.TrimSpace(profile.CredentialRef) == "" {
		return adapter.S3Adapter.Sync(ctx, profile, cache)
	}
	payload, etag, err := adapter.fetchAuthorized(ctx, profile, "catalog.json", maxCatalogBytes, "sync", "S3 catalog fetch failed")
	if err != nil {
		return SyncResult{}, err
	}
	catalog, err := decodeCatalog(payload)
	if err != nil {
		return SyncResult{}, err
	}
	revision := etag
	if revision == "" {
		revision = catalog.Digest
	}
	return SyncResult{Revision: "s3:" + revision, Catalog: catalog}, nil
}

func (adapter S3CredentialAdapter) ReadTemplate(ctx context.Context, profile promptrepo.RepositoryProfile, snapshot promptrepo.SnapshotMetadata, template promptrepo.TemplateRole, cache string) ([]byte, error) {
	if strings.TrimSpace(profile.CredentialRef) == "" {
		return adapter.S3Adapter.ReadTemplate(ctx, profile, snapshot, template, cache)
	}
	payload, _, err := adapter.fetchAuthorized(ctx, profile, template.Path, maxTemplateBytes, "read-template", "S3 prompt template read failed")
	if err != nil {
		return nil, err
	}
	if err := verifyTemplateDigest(payload, template.Digest); err != nil {
		return nil, err
	}
	return payload, nil
}

func (adapter S3CredentialAdapter) ReadCompanion(ctx context.Context, profile promptrepo.RepositoryProfile, snapshot promptrepo.SnapshotMetadata, companionPath string, cache string) ([]byte, error) {
	if strings.TrimSpace(profile.CredentialRef) == "" {
		return adapter.S3Adapter.ReadCompanion(ctx, profile, snapshot, companionPath, cache)
	}
	payload, _, err := adapter.fetchAuthorized(ctx, profile, companionPath, maxCompanionBytes, "read-companion", "S3 template contract companion fetch failed")
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// fetchAuthorized resolves profile.CredentialRef via the resolver and issues
// one authorized GET. The resolved value is zeroed after the request and the
// Authorization header is dropped from the request as soon as the response
// arrives; the transport's internal copy is out of our control and is the
// same in-memory caveat as any local credential use.
func (adapter S3CredentialAdapter) fetchAuthorized(ctx context.Context, profile promptrepo.RepositoryProfile, objectPath string, maxBytes int64, operation, failContext string) ([]byte, string, error) {
	objectURL, err := s3ObjectURL(profile.Source, objectPath)
	if err != nil {
		return nil, "", err
	}
	ref := strings.TrimSpace(profile.CredentialRef)
	resolution, err := adapter.Resolver.ResolveCredential(ctx, ref, CredentialGrant{
		Consumer: adapter.Consumer, Capability: adapter.Capability, Operation: operation,
	})
	if err != nil {
		// Resolver errors are typed, secret-free denials (allowlist or
		// revocation). Fail closed without echoing resolver detail text.
		return nil, "", promptrepo.NewError(promptrepo.CodeAuthRequired, failContext+": credential resolution denied", false, nil)
	}
	defer func() {
		for i := range resolution.Secret {
			resolution.Secret[i] = 0
		}
	}()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return nil, "", promptrepo.NewError(promptrepo.CodeInvalidRequest, failContext+": invalid request", false, err)
	}
	request.Header.Set("Authorization", "Bearer "+string(resolution.Secret))
	client := adapter.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", promptrepo.NewError(promptrepo.CodeSourceFetchFailed, failContext+" failed", true, err)
	}
	defer response.Body.Close()
	request.Header.Del("Authorization")
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, "", promptrepo.NewError(promptrepo.CodeAuthRequired, failContext+" authorization failed", false, nil)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, "", promptrepo.NewError(promptrepo.CodeNotFound, failContext+": object is not present in the S3 prefix", false, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", promptrepo.NewError(promptrepo.CodeSourceFetchFailed, fmt.Sprintf("%s returned HTTP %d", failContext, response.StatusCode), response.StatusCode >= 500, nil)
	}
	if response.ContentLength > maxBytes {
		return nil, "", promptrepo.NewError(promptrepo.CodeInvalidRequest, failContext+": object exceeds size limit", false, nil)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, "", promptrepo.NewError(promptrepo.CodeSourceFetchFailed, failContext+" read failed", true, err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, "", promptrepo.NewError(promptrepo.CodeInvalidRequest, failContext+": object exceeds size limit", false, nil)
	}
	return payload, strings.Trim(response.Header.Get("ETag"), "\""), nil
}
