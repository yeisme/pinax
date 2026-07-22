package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/domain"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// SyncRepoCredentialRequest drives the typed repository-encrypted S3/COS
// credential bundle (s3_credentials.v1). Plaintext is accepted only via the
// Payload field (filled from secure stdin by the CLI) and never persisted
// outside the credentialctl envelope; structured output carries metadata only.
type SyncRepoCredentialRequest struct {
	VaultPath  string
	Action     string // set, list, remove, init
	Name       string
	Kind       string // credential
	Format     string // s3_credentials.v1
	Version    string
	Payload    []byte // for set; transient, wiped after use
	Passphrase []byte // authorizes mutation / proves unlock
	Project    string
	Repository string
}

// projectSecretAsset is the contained path (relative to the vault root) where
// the repository-encrypted credentialctl envelope lives. It is separate from the
// legacy pinax fake/env secrets asset so the two trust boundaries never collide.
const projectSecretAsset = ".pinax/project-secrets.yaml"

// repoCredentialEnvelopePresent reports whether the repository-encrypted
// credentialctl envelope exists at the vault root. It is a non-sensitive
// presence check used by doctor; it never reads or decrypts.
func repoCredentialEnvelopePresent(root string) bool {
	_, err := os.Stat(filepath.Join(root, projectSecretAsset))
	return err == nil
}

// SyncRepoCredential mutates or inspects the typed repository-encrypted S3
// credential bundle through the credentialctl projectsecrets API. Pinax owns
// only the s3_credentials.v1 payload type and the projection; KDF/AEAD/Keychain
// stay in credentialctl.
func (s *Service) SyncRepoCredential(_ context.Context, req SyncRepoCredentialRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.repo.credential", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("sync.repo.credential", err), err
	}
	store := projectsecrets.NewStore()
	switch req.Action {
	case "init":
		return s.syncRepoCredentialInit(root, req, store)
	case "set":
		return s.syncRepoCredentialSet(root, req, store)
	case "list":
		return s.syncRepoCredentialList(root, store)
	case "remove":
		return s.syncRepoCredentialRemove(root, req, store)
	default:
		err := &domain.CommandError{Code: "unsupported_credential_action", Message: fmt.Sprintf("unsupported credential action: %q", req.Action), Hint: "Use init, set, list, or remove"}
		return domain.NewErrorProjection("sync.repo.credential", err), err
	}
}

func (s *Service) syncRepoCredentialInit(root string, req SyncRepoCredentialRequest, store *projectsecrets.Store) (domain.Projection, error) {
	if len(req.Passphrase) == 0 {
		err := &domain.CommandError{Code: "sync_repo_unlock_required", Message: "passphrase is required to initialize the repository credential envelope"}
		return domain.NewErrorProjection("sync.repo.credential", err), err
	}
	project := strings.TrimSpace(req.Project)
	if project == "" {
		project = "pinax"
	}
	repository := strings.TrimSpace(req.Repository)
	if repository == "" {
		repository = filepathBase(root)
	}
	res, err := store.Init(root, projectSecretAsset, project, repository, "passphrase-v1", req.Passphrase)
	if err != nil {
		return mapCredentialStoreError(err)
	}
	proj := domain.NewProjection("sync.repo.credential", "Repository credential envelope initialized.")
	proj.Facts["project"] = res.Project
	proj.Facts["repository"] = res.Repository
	proj.Facts["provider"] = res.Provider
	proj.Facts["entry_count"] = strconv.Itoa(res.EntryCount)
	proj.Facts["digest"] = res.Digest
	return proj, nil
}

func (s *Service) syncRepoCredentialSet(root string, req SyncRepoCredentialRequest, store *projectsecrets.Store) (domain.Projection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "missing_credential_name", Message: "credential name is required"}
		return domain.NewErrorProjection("sync.repo.credential", err), err
	}
	if len(req.Payload) == 0 {
		err := &domain.CommandError{Code: "missing_credential_payload", Message: "credential payload is required (use --stdin)", Hint: "Pipe a s3_credentials.v1 JSON object via --stdin; value flags are rejected"}
		return domain.NewErrorProjection("sync.repo.credential", err), err
	}
	// Validate the typed payload BEFORE encrypting so a malformed bundle never
	// reaches the envelope and field-level errors never echo the value.
	if _, err := pinaxremote.ParseS3Credentials(req.Payload); err != nil {
		return domain.NewErrorProjection("sync.repo.credential", &domain.CommandError{Code: "invalid_credential_payload", Message: err.Error()}), err
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "credential"
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = pinaxremote.S3CredentialFormat
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = "1"
	}
	res, err := store.SetEntry(root, projectSecretAsset, name, kind, format, version, req.Payload, req.Passphrase)
	if err != nil {
		return mapCredentialStoreError(err)
	}
	proj := domain.NewProjection("sync.repo.credential", "Repository credential bundle encrypted.")
	proj.Facts["name"] = name
	proj.Facts["kind"] = kind
	proj.Facts["format"] = format
	proj.Facts["version"] = version
	proj.Facts["entry_count"] = strconv.Itoa(res.EntryCount)
	proj.Facts["digest"] = res.Digest
	return proj, nil
}

func (s *Service) syncRepoCredentialList(root string, store *projectsecrets.Store) (domain.Projection, error) {
	res, err := store.List(root, projectSecretAsset)
	if err != nil {
		return mapCredentialStoreError(err)
	}
	proj := domain.NewProjection("sync.repo.credential", fmt.Sprintf("%d repository credential bundle(s).", res.EntryCount))
	proj.Facts["project"] = res.Project
	proj.Facts["repository"] = res.Repository
	proj.Facts["digest"] = res.Digest
	proj.Facts["entry_count"] = strconv.Itoa(res.EntryCount)
	entries := make([]map[string]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		entries = append(entries, map[string]string{
			"name": e.Name, "identity": e.Identity, "kind": e.Kind, "format": e.Format, "version": e.Version,
		})
	}
	proj.Data = entries
	return proj, nil
}

func (s *Service) syncRepoCredentialRemove(root string, req SyncRepoCredentialRequest, store *projectsecrets.Store) (domain.Projection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "missing_credential_name", Message: "credential name is required"}
		return domain.NewErrorProjection("sync.repo.credential", err), err
	}
	res, err := store.RemoveEntry(root, projectSecretAsset, name, req.Passphrase)
	if err != nil {
		return mapCredentialStoreError(err)
	}
	proj := domain.NewProjection("sync.repo.credential", "Repository credential bundle removed.")
	proj.Facts["name"] = name
	proj.Facts["remaining"] = strconv.Itoa(res.EntryCount)
	return proj, nil
}

// mapCredentialStoreError converts a credentialctl projectsecrets error into a
// pinax projection + stable CommandError code, preserving the fail-closed
// semantics (no value ever leaks).
func mapCredentialStoreError(err error) (domain.Projection, error) {
	code := "credential_store_failed"
	var perr *projectsecrets.Error
	if e, ok := err.(*projectsecrets.Error); ok {
		perr = e
	}
	if perr != nil {
		switch perr.Code {
		case projectsecrets.CodeUnlockFailed:
			code = "sync_repo_unlock_failed"
		case projectsecrets.CodeUnlockRequired:
			code = "sync_repo_unlock_required"
		case projectsecrets.CodeAssetMissing:
			code = "credential_envelope_missing"
		case projectsecrets.CodeAssetInvalid:
			code = "credential_envelope_invalid"
		case projectsecrets.CodePathUnsafe:
			code = "credential_path_unsafe"
		case projectsecrets.CodeKeychainUnavailable:
			code = "keychain_unavailable"
		}
	}
	cerr := &domain.CommandError{Code: code, Message: code}
	return domain.NewErrorProjection("sync.repo.credential", cerr), err
}

func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
