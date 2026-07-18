package assets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/identity"
)

func TestAddAllocatesCanonicalObjectIDAndKeepsLegacyAssetID(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "image.txt")
	if err := os.WriteFile(source, []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	asset, err := Add(root, source)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Classify(asset.ObjectID) != identity.IDClassCanonical {
		t.Fatalf("object_id = %q", asset.ObjectID)
	}
	if len(asset.ID) < len("asset_") || asset.ID[:len("asset_")] != "asset_" {
		t.Fatalf("legacy asset id = %q", asset.ID)
	}
	manifest, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Assets) != 1 || manifest.Assets[0].ObjectID != asset.ObjectID {
		t.Fatalf("manifest = %#v", manifest)
	}
}
