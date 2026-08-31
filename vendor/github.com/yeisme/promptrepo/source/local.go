package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yeisme/promptrepo"
)

const maxCatalogBytes = 8 << 20
const maxTemplateBytes = 2 << 20
const maxCompanionBytes = 4 << 20

type LocalAdapter struct{}

func (LocalAdapter) Kind() string { return "file" }

func (LocalAdapter) Sync(_ context.Context, profile promptrepo.RepositoryProfile, _ string) (SyncResult, error) {
	parsed, err := url.Parse(profile.Source)
	if err != nil || parsed.Scheme != "file" {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid file repository URI", false, err)
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil || !filepath.IsAbs(path) {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "file repository path must be absolute", false, err)
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeNotFound, "repository directory is not available", false, err)
	}
	catalogPath := filepath.Join(root, "catalog.json")
	info, err := os.Lstat(catalogPath)
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeNotFound, "catalog.json is not available", false, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxCatalogBytes {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog.json must be a bounded regular file", false, nil)
	}
	resolvedCatalog, err := filepath.EvalSymlinks(catalogPath)
	if err != nil || !withinRoot(root, resolvedCatalog) {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog path escapes repository root", false, err)
	}
	payload, err := os.ReadFile(resolvedCatalog)
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "failed to read catalog.json", true, err)
	}
	catalog, err := decodeCatalog(payload)
	if err != nil {
		return SyncResult{}, err
	}
	revision := "file:" + catalog.Digest
	return SyncResult{Revision: revision, Catalog: catalog}, nil
}

func (LocalAdapter) ReadTemplate(_ context.Context, profile promptrepo.RepositoryProfile, _ promptrepo.SnapshotMetadata, template promptrepo.TemplateRole, _ string) ([]byte, error) {
	parsed, err := url.Parse(profile.Source)
	if err != nil || parsed.Scheme != "file" {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid file repository URI", false, err)
	}
	rootPath, err := url.PathUnescape(parsed.Path)
	if err != nil || !filepath.IsAbs(rootPath) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "file repository path must be absolute", false, err)
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(rootPath))
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "repository directory is not available", false, err)
	}
	contentPath := filepath.Join(root, filepath.FromSlash(template.Path))
	info, err := os.Lstat(contentPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxTemplateBytes {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "prompt template is not an available bounded regular file", false, err)
	}
	resolved, err := filepath.EvalSymlinks(contentPath)
	if err != nil || !withinRoot(root, resolved) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "prompt template path escapes repository root", false, err)
	}
	payload, err := os.ReadFile(resolved)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "failed to read prompt template", true, err)
	}
	if err := verifyTemplateDigest(payload, template.Digest); err != nil {
		return nil, err
	}
	return payload, nil
}

func (LocalAdapter) ReadCompanion(_ context.Context, profile promptrepo.RepositoryProfile, _ promptrepo.SnapshotMetadata, companionPath string, _ string) ([]byte, error) {
	parsed, err := url.Parse(profile.Source)
	if err != nil || parsed.Scheme != "file" {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid file repository URI", false, err)
	}
	rootPath, err := url.PathUnescape(parsed.Path)
	if err != nil || !filepath.IsAbs(rootPath) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "file repository path must be absolute", false, err)
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(rootPath))
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "repository directory is not available", false, err)
	}
	if !safeObjectPath(companionPath) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid template contract companion path", false, nil)
	}
	contentPath := filepath.Join(root, filepath.FromSlash(companionPath))
	info, err := os.Lstat(contentPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxCompanionBytes {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "template contract companion is not an available bounded regular file", false, err)
	}
	resolved, err := filepath.EvalSymlinks(contentPath)
	if err != nil || !withinRoot(root, resolved) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template contract companion path escapes repository root", false, err)
	}
	payload, err := os.ReadFile(resolved)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "failed to read template contract companion", true, err)
	}
	return payload, nil
}

func decodeCatalog(payload []byte) (promptrepo.Catalog, error) {
	if len(payload) > maxCatalogBytes {
		return promptrepo.Catalog{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog exceeds size limit", false, nil)
	}
	var catalog promptrepo.Catalog
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return promptrepo.Catalog{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog.json is invalid", false, err)
	}
	if err := promptrepo.ValidateCatalog(catalog); err != nil {
		return promptrepo.Catalog{}, err
	}
	digest, err := promptrepo.CanonicalCatalogDigest(catalog)
	if err != nil {
		return promptrepo.Catalog{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog digest failed", false, err)
	}
	if catalog.Digest != "" && catalog.Digest != digest {
		return promptrepo.Catalog{}, promptrepo.NewError(promptrepo.CodeDigestMismatch, "catalog digest does not match content", false, nil)
	}
	catalog.Digest = digest
	return catalog, nil
}

func withinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "."
}

func sourceError(kind string, err error) error {
	return promptrepo.NewError(promptrepo.CodeSourceFetchFailed, fmt.Sprintf("%s repository sync failed", kind), true, err)
}

func verifyTemplateDigest(payload []byte, expected string) error {
	if len(payload) > maxTemplateBytes {
		return promptrepo.NewError(promptrepo.CodeInvalidRequest, "prompt template exceeds size limit", false, nil)
	}
	if !utf8.Valid(payload) {
		return promptrepo.NewError(promptrepo.CodeInvalidRequest, "prompt template must be valid UTF-8 text", false, nil)
	}
	digest := sha256.Sum256(payload)
	actual := "sha256:" + hex.EncodeToString(digest[:])
	if expected == "" || actual != expected {
		return promptrepo.NewError(promptrepo.CodeDigestMismatch, "prompt template digest does not match catalog", false, nil)
	}
	return nil
}
