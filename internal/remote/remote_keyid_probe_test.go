package remote_test

import (
	"context"
	"os"
	"testing"

	"github.com/yeisme/pinax/internal/cloudsync"
	"github.com/yeisme/pinax/internal/remote"
)

// TestProbeRemoteBlobKeyIDs is a read-only diagnostic against a real
// configured vault (PINAX_PROBE_VAULT=<vault path>): it classifies every
// remote object's envelope KeyID as active-v2 / legacy / other.
func TestProbeRemoteBlobKeyIDs(t *testing.T) {
	t.Parallel()
	root := os.Getenv("PINAX_PROBE_VAULT")
	if root == "" {
		t.Skip("PINAX_PROBE_VAULT not set")
	}
	state, err := remote.Load(root)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	store, err := state.GetStore(context.Background())
	if err != nil {
		t.Fatalf("get store: %v", err)
	}
	transport := cloudsync.NewObjectStoreTransport(store, cloudsync.Layout{WorkspaceID: state.Config.WorkspaceID, VaultID: state.Config.WorkspaceID})
	ctx := context.Background()
	head, err := transport.CurrentHead(ctx, state.Config.WorkspaceID)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	envelope, err := transport.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	active, legacy, err := remote.SyncKeyVersions(remote.EncryptionSecretRef(state.Config))
	if err != nil {
		t.Fatalf("key versions: %v", err)
	}
	t.Logf("manifest key id=%s class=%s", envelope.KeyID, remote.ClassifyKeyID(envelope.KeyID, active, legacy))
	manifest, err := remote.BuildManifest(root)
	if err != nil {
		t.Fatalf("local manifest: %v", err)
	}
	counts := map[string]int{}
	samples := map[string]string{}
	for _, entry := range manifest.Entries {
		blobEnvelope, err := transport.GetBlob(ctx, entry.BlobID)
		if err != nil {
			counts["error"]++
			continue
		}
		class := remote.ClassifyKeyID(blobEnvelope.KeyID, active, legacy)
		if blobEnvelope.KeyID == "" {
			class = "empty-key-id"
		}
		counts[class]++
		if class != "v2" && len(samples[class]) < 200 {
			samples[class] += entry.BlobID + " key_id=" + blobEnvelope.KeyID + "; "
		}
	}
	for _, del := range manifest.Deletes {
		if del.TrashBlobID == "" {
			continue
		}
		blobEnvelope, err := transport.GetBlob(ctx, del.TrashBlobID)
		if err != nil {
			counts["trash-error"]++
			continue
		}
		class := remote.ClassifyKeyID(blobEnvelope.KeyID, active, legacy)
		if blobEnvelope.KeyID == "" {
			class = "empty-key-id"
		}
		counts["trash-"+class]++
	}
	t.Logf("blob key classes: %v", counts)
	for class, s := range samples {
		t.Logf("sample[%s]: %s", class, s)
	}
}
