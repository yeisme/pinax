package app

import (
	"context"
	"crypto/subtle"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

const capsaEncryptionKeyFormat = "capsa_encryption_key.v1"

type heldUnlockSource struct {
	secret     []byte
	descriptor string
}

func rememberKeychainSecret(ctx context.Context, keychain *projectsecrets.KeychainSource, secret []byte) error {
	if keychain == nil {
		return nil
	}
	if err := keychain.Remember(ctx, secret); err != nil {
		return err
	}
	stored, err := keychain.Secret(ctx)
	if err != nil {
		return err
	}
	defer func() {
		for i := range stored {
			stored[i] = 0
		}
	}()
	if subtle.ConstantTimeCompare(stored, secret) != 1 {
		return fmt.Errorf("keychain verification failed")
	}
	return nil
}

func holdUnlockSource(ctx context.Context, source projectsecrets.UnlockSource) (*heldUnlockSource, error) {
	if source == nil {
		return nil, fmt.Errorf("unlock source required")
	}
	secret, err := source.Secret(ctx)
	if err != nil {
		return nil, err
	}
	if len(secret) == 0 {
		return nil, projectsecrets.UnlockRequiredError("empty unlock secret")
	}
	return &heldUnlockSource{secret: secret, descriptor: source.Descriptor()}, nil
}

func (s *heldUnlockSource) Secret(context.Context) ([]byte, error) {
	secret := make([]byte, len(s.secret))
	copy(secret, s.secret)
	return secret, nil
}

func (s *heldUnlockSource) Descriptor() string { return s.descriptor }

func (s *heldUnlockSource) Close() {
	for i := range s.secret {
		s.secret[i] = 0
	}
	s.secret = nil
}

func resolveRepositoryBootstrapSecrets(ctx context.Context, root string, declaration pinaxremote.SyncConfig, source projectsecrets.UnlockSource) (string, error) {
	credentialID := strings.TrimSpace(declaration.Secrets.CredentialID)
	if credentialID == "" {
		credentialID = "default"
	}
	encryptionID := strings.TrimSpace(declaration.Secrets.EncryptionKeyID)
	if encryptionID == "" {
		return "", fmt.Errorf("repository encryption key identity required")
	}
	entries := []string{credentialID, encryptionID}
	policy := projectsecrets.NewAllowlistPolicy(map[string]projectsecrets.ConsumerScope{
		"pinax": {Capabilities: []string{"sync-bootstrap"}, Entries: entries},
	})
	resolver, err := projectsecrets.NewResolver(filepath.Join(root, projectSecretAsset), source, policy)
	if err != nil {
		return "", err
	}
	info, err := resolver.EnvelopeInfo()
	if err != nil {
		return "", err
	}
	credentialInfo, ok := info.Entries[credentialID]
	if !ok || credentialInfo.Kind != "credential" || credentialInfo.Format != pinaxremote.S3CredentialFormat {
		return "", fmt.Errorf("repository credential entry invalid")
	}
	encryptionInfo, ok := info.Entries[encryptionID]
	if !ok || encryptionInfo.Kind != "encryption_key" || encryptionInfo.Format != capsaEncryptionKeyFormat {
		return "", fmt.Errorf("repository encryption key entry invalid")
	}
	snapshot, err := resolver.Resolve(ctx, projectsecrets.Grant{
		Project:    info.Project,
		Repository: info.Repository,
		Consumer:   "pinax",
		Capability: "sync-bootstrap",
		Operation:  "bootstrap",
		Entries:    entries,
		Digest:     info.Digest,
	})
	if err != nil {
		return "", err
	}
	defer func() { _ = snapshot.Close() }()
	credentialPayload, ok := snapshot.Entry(credentialID)
	if !ok {
		return "", fmt.Errorf("repository credential entry missing")
	}
	if _, err := pinaxremote.ParseS3Credentials(credentialPayload); err != nil {
		return "", err
	}
	encryptionKey, ok := snapshot.Entry(encryptionID)
	if !ok || strings.TrimSpace(string(encryptionKey)) == "" {
		return "", fmt.Errorf("repository encryption key entry missing")
	}
	return pinaxprofile.SetStoredSecret("sync-repo-enc-"+encryptionID, string(encryptionKey))
}
