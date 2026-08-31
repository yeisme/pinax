package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/yeisme/promptrepo"
)

// S3Adapter reads the immutable catalog projection from an S3-compatible
// bucket. The built-in adapter is intentionally unsigned. Private stores can
// register a credential-aware Adapter through engine.Options.Sources without
// putting secret values in RepositoryProfile.
type S3Adapter struct {
	HTTPClient *http.Client
}

func (S3Adapter) Kind() string { return "s3" }

func (adapter S3Adapter) Sync(ctx context.Context, profile promptrepo.RepositoryProfile, _ string) (SyncResult, error) {
	if strings.TrimSpace(profile.CredentialRef) != "" {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeAuthRequired, "S3 credential reference requires a credential-aware source adapter", false, nil)
	}
	objectURL, err := s3ObjectURL(profile.Source, "catalog.json")
	if err != nil {
		return SyncResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid S3 catalog request", false, err)
	}
	client := adapter.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "S3 catalog fetch failed", true, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeAuthRequired, "S3 catalog authorization failed", false, nil)
	}
	if response.StatusCode == http.StatusNotFound {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeNotFound, "catalog.json is not present in the S3 prefix", false, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, fmt.Sprintf("S3 catalog fetch returned HTTP %d", response.StatusCode), response.StatusCode >= 500, nil)
	}
	if response.ContentLength > maxCatalogBytes {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog exceeds size limit", false, nil)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBytes+1))
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "S3 catalog read failed", true, err)
	}
	if len(payload) > maxCatalogBytes {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog exceeds size limit", false, nil)
	}
	catalog, err := decodeCatalog(payload)
	if err != nil {
		return SyncResult{}, err
	}
	revision := strings.Trim(response.Header.Get("ETag"), "\"")
	if revision == "" {
		revision = catalog.Digest
	}
	return SyncResult{Revision: "s3:" + revision, Catalog: catalog}, nil
}

func (adapter S3Adapter) ReadTemplate(ctx context.Context, profile promptrepo.RepositoryProfile, _ promptrepo.SnapshotMetadata, template promptrepo.TemplateRole, _ string) ([]byte, error) {
	if strings.TrimSpace(profile.CredentialRef) != "" {
		return nil, promptrepo.NewError(promptrepo.CodeAuthRequired, "S3 credential reference requires a credential-aware source adapter", false, nil)
	}
	objectURL, err := s3ObjectURL(profile.Source, template.Path)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid S3 prompt template request", false, err)
	}
	client := adapter.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "S3 prompt template fetch failed", true, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, promptrepo.NewError(promptrepo.CodeAuthRequired, "S3 prompt template authorization failed", false, nil)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "prompt template is not present in the S3 prefix", false, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, fmt.Sprintf("S3 prompt template fetch returned HTTP %d", response.StatusCode), response.StatusCode >= 500, nil)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxTemplateBytes+1))
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "S3 prompt template read failed", true, err)
	}
	if err := verifyTemplateDigest(payload, template.Digest); err != nil {
		return nil, err
	}
	return payload, nil
}

func (adapter S3Adapter) ReadCompanion(ctx context.Context, profile promptrepo.RepositoryProfile, _ promptrepo.SnapshotMetadata, companionPath string, _ string) ([]byte, error) {
	if strings.TrimSpace(profile.CredentialRef) != "" {
		return nil, promptrepo.NewError(promptrepo.CodeAuthRequired, "S3 credential reference requires a credential-aware source adapter", false, nil)
	}
	objectURL, err := s3ObjectURL(profile.Source, companionPath)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid S3 template contract companion request", false, err)
	}
	client := adapter.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "S3 template contract companion fetch failed", true, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, promptrepo.NewError(promptrepo.CodeAuthRequired, "S3 template contract companion authorization failed", false, nil)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "template contract companion is not present in the S3 prefix", false, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, fmt.Sprintf("S3 template contract companion fetch returned HTTP %d", response.StatusCode), response.StatusCode >= 500, nil)
	}
	if response.ContentLength > maxCompanionBytes {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template contract companion exceeds size limit", false, nil)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxCompanionBytes+1))
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "S3 template contract companion read failed", true, err)
	}
	if len(payload) > maxCompanionBytes {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template contract companion exceeds size limit", false, nil)
	}
	return payload, nil
}

func s3ObjectURL(raw, objectPath string) (string, error) {
	if !safeObjectPath(objectPath) {
		return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "S3 object path must stay inside the repository prefix", false, nil)
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "S3 source must be s3://bucket/prefix", false, err)
	}
	bucket := parsed.Host
	prefix := strings.Trim(parsed.Path, "/")
	query := parsed.Query()
	region := strings.TrimSpace(query.Get("region"))
	if region == "" {
		region = "us-east-1"
	}
	pathStyle, err := strconv.ParseBool(defaultString(query.Get("path_style"), "false"))
	if err != nil {
		return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "S3 path_style must be true or false", false, err)
	}
	endpoint := strings.TrimSpace(query.Get("endpoint"))
	if endpoint == "" {
		if pathStyle {
			return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "S3 path_style requires an endpoint", false, nil)
		}
		host := bucket + ".s3.amazonaws.com"
		if region != "us-east-1" {
			host = bucket + ".s3." + region + ".amazonaws.com"
		}
		return (&url.URL{Scheme: "https", Host: host, Path: "/" + path.Join(prefix, objectPath)}).String(), nil
	}
	base, err := url.Parse(endpoint)
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "S3 endpoint must be an HTTP(S) origin without credentials or query parameters", false, err)
	}
	if pathStyle {
		base.Path = "/" + path.Join(strings.Trim(base.Path, "/"), bucket, prefix, objectPath)
	} else {
		base.Host = bucket + "." + base.Host
		base.Path = "/" + path.Join(strings.Trim(base.Path, "/"), prefix, objectPath)
	}
	return base.String(), nil
}

func safeObjectPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsAny(value, "\x00\n\r") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
