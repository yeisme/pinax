package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

// reencryptRemoteManifestLegacy rewrites the manifest envelope in a file
// backend's object root under the legacy derivation, simulating a remote last
// pushed before the v2 key derivation. Uses only exported crypto, so the
// production keychain must classify it as an authentic legacy envelope.
func reencryptRemoteManifestLegacy(t *testing.T, root, objectRoot, workspace string) {
	t.Helper()
	// The embedded file backend nests the vault namespace under
	// workspaces/<ws>/vaults/<ws>/ and shards manifest objects by content hash;
	// a fresh test remote holds exactly one manifest envelope there.
	var manifests []string
	err := filepath.WalkDir(objectRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/manifests/") && strings.HasSuffix(entry.Name(), ".json") {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk manifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected exactly one manifest envelope, got %d: %v", len(manifests), manifests)
	}
	manifestPath := manifests[0]
	envelopeBody, readErr := os.ReadFile(manifestPath)
	err = readErr
	if err != nil {
		t.Fatalf("read manifest envelope: %v", err)
	}
	var envelope pinaxcloud.EncryptedEnvelope
	if err := json.Unmarshal(envelopeBody, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	// The encryption secret ref recorded by this test's login is the explicit
	// plain: ref (kept out of the user-level stored:// secrets file).
	configBody, err := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "config.yaml"))
	if err != nil {
		t.Fatalf("read cloud config: %v", err)
	}
	encryptionRef := ""
	for _, line := range strings.Split(string(configBody), "\n") {
		if value, ok := strings.CutPrefix(line, "encryption_secret_ref:"); ok {
			encryptionRef = strings.TrimSpace(value)
		}
	}
	if encryptionRef == "" {
		t.Fatalf("no encryption_secret_ref in config:\n%s", configBody)
	}
	keys, err := pinaxcloud.DeriveKeychain(encryptionRef)
	if err != nil {
		t.Fatalf("derive keychain: %v", err)
	}
	if envelope.KeyID != keys.Active.KeyID {
		t.Fatalf("fixture expects a v2 remote manifest, got key id %s", envelope.KeyID)
	}
	plain, err := pinaxcloud.DecryptBlob(keys, envelope, []byte("pinax.cloud.manifest"))
	if err != nil {
		t.Fatalf("decrypt manifest: %v", err)
	}
	var manifest pinaxcloud.Manifest
	if err := json.Unmarshal(plain, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	legacyEnvelope, err := pinaxcloud.EncryptManifest(keys.Legacy[0], manifest)
	if err != nil {
		t.Fatalf("encrypt legacy manifest: %v", err)
	}
	legacyBody, err := json.Marshal(legacyEnvelope)
	if err != nil {
		t.Fatalf("marshal legacy envelope: %v", err)
	}
	if err := os.WriteFile(manifestPath, legacyBody, 0o600); err != nil {
		t.Fatalf("write legacy envelope: %v", err)
	}
	_ = errors.New
	_ = strings.TrimSpace
}
