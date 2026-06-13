package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	imagegif "image/gif"
	imagejpeg "image/jpeg"
	imagepng "image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestManifestLoadSaveValidateUsesFixedPath(t *testing.T) {
	root := t.TempDir()
	manifest, err := Load(root)
	if err != nil {
		t.Fatalf("load missing manifest: %v", err)
	}
	if manifest.SchemaVersion != ManifestSchemaVersion || len(manifest.Assets) != 0 {
		t.Fatalf("default manifest = %#v", manifest)
	}

	asset := Asset{ID: "asset_abc", Path: "assets/diagram.png", Filename: "diagram.png", Stem: "diagram", Extension: "png", MediaType: "image/png", Size: 12, SHA256: "abc123", ManagedStatus: domain.ManagedStatusManaged}
	if err := Save(root, Manifest{Assets: []Asset{asset}}); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "assets", "manifest.json")); err != nil {
		t.Fatalf("manifest path missing: %v", err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load saved manifest: %v", err)
	}
	if loaded.SchemaVersion != ManifestSchemaVersion || len(loaded.Assets) != 1 || loaded.Assets[0].Path != "assets/diagram.png" {
		t.Fatalf("loaded manifest = %#v", loaded)
	}

	if err := Validate(Manifest{SchemaVersion: "pinax.assets.v0"}); err == nil {
		t.Fatalf("Validate accepted unsupported schema")
	}
	badPath := Manifest{SchemaVersion: ManifestSchemaVersion, Assets: []Asset{{ID: "asset_bad", Path: "../outside.png", SHA256: "abc", ManagedStatus: domain.ManagedStatusManaged}}}
	if err := Validate(badPath); err == nil {
		t.Fatalf("Validate accepted unsafe asset path")
	}
}

func TestAddWithOptionsDetectsImageDimensions(t *testing.T) {
	root := t.TempDir()
	fixtures := []struct {
		name   string
		encode func(*os.File) error
	}{
		{name: "diagram.png", encode: func(f *os.File) error { return imagepng.Encode(f, image.NewRGBA(image.Rect(0, 0, 3, 2))) }},
		{name: "photo.jpg", encode: func(f *os.File) error { return imagejpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 3)), nil) }},
		{name: "anim.gif", encode: func(f *os.File) error { return imagegif.Encode(f, image.NewRGBA(image.Rect(0, 0, 5, 4)), nil) }},
	}
	for _, fixture := range fixtures {
		source := filepath.Join(t.TempDir(), fixture.name)
		f, err := os.Create(source)
		if err != nil {
			t.Fatalf("create %s: %v", fixture.name, err)
		}
		if err := fixture.encode(f); err != nil {
			_ = f.Close()
			t.Fatalf("encode %s: %v", fixture.name, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close %s: %v", fixture.name, err)
		}
		asset, err := AddWithOptions(root, source, AddOptions{Mode: AddModeCopy})
		if err != nil {
			t.Fatalf("add %s: %v", fixture.name, err)
		}
		if asset.Width == 0 || asset.Height == 0 {
			t.Fatalf("%s dimensions missing: %#v", fixture.name, asset)
		}
	}

	unknown := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(unknown, []byte("not an image"), 0o644); err != nil {
		t.Fatalf("write unknown: %v", err)
	}
	asset, err := AddWithOptions(root, unknown, AddOptions{Mode: AddModeCopy})
	if err != nil {
		t.Fatalf("add unknown: %v", err)
	}
	if asset.Width != 0 || asset.Height != 0 {
		t.Fatalf("unknown dimensions = %#v", asset)
	}
}

func TestAddWithOptionsCopyAndRegisterModes(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "diagram.bin")
	payload := []byte("pinax asset payload")
	if err := os.WriteFile(outside, payload, 0o644); err != nil {
		t.Fatalf("write outside source: %v", err)
	}

	copied, err := AddWithOptions(root, outside, AddOptions{Mode: AddModeCopy})
	if err != nil {
		t.Fatalf("copy add: %v", err)
	}
	if copied.Path != "assets/diagram.bin" || copied.Size != int64(len(payload)) || copied.SHA256 == "" || copied.ModifiedUnix == 0 || copied.MediaType != "application/octet-stream" {
		t.Fatalf("copied asset = %#v", copied)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(copied.Path))); err != nil {
		t.Fatalf("copied target missing: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("copy mode removed source: %v", err)
	}

	inside := filepath.Join(root, "assets", "existing.txt")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir inside assets: %v", err)
	}
	if err := os.WriteFile(inside, []byte("registered"), 0o644); err != nil {
		t.Fatalf("write inside source: %v", err)
	}
	registered, err := AddWithOptions(root, inside, AddOptions{Mode: AddModeRegister})
	if err != nil {
		t.Fatalf("register add: %v", err)
	}
	if registered.Path != "assets/existing.txt" || registered.MediaType != "text/plain" || registered.ModifiedUnix == 0 {
		t.Fatalf("registered asset = %#v", registered)
	}

	if _, err := AddWithOptions(root, outside, AddOptions{Mode: AddModeRegister}); err == nil {
		t.Fatalf("register accepted source outside vault")
	}
	manifest, err := Load(root)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if len(manifest.Assets) != 2 {
		t.Fatalf("manifest changed after rejected register: %#v", manifest.Assets)
	}
}

func TestVerifyClassifiesMissingChangedAndUnmanagedAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	verifiedPath := filepath.Join(root, "assets", "verified.txt")
	changedPath := filepath.Join(root, "assets", "changed.txt")
	unmanagedPath := filepath.Join(root, "assets", "unmanaged.txt")
	if err := os.WriteFile(verifiedPath, []byte("verified"), 0o644); err != nil {
		t.Fatalf("write verified: %v", err)
	}
	if err := os.WriteFile(changedPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write changed old: %v", err)
	}
	verifiedSHA, verifiedSize, err := hashFile(verifiedPath)
	if err != nil {
		t.Fatalf("hash verified: %v", err)
	}
	changedSHA, changedSize, err := hashFile(changedPath)
	if err != nil {
		t.Fatalf("hash changed: %v", err)
	}
	if err := os.WriteFile(changedPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write changed new: %v", err)
	}
	if err := os.WriteFile(unmanagedPath, []byte("unmanaged"), 0o644); err != nil {
		t.Fatalf("write unmanaged: %v", err)
	}
	manifest := Manifest{Assets: []Asset{
		{ID: "asset_verified", Path: "assets/verified.txt", Filename: "verified.txt", Stem: "verified", Extension: "txt", MediaType: "text/plain", Size: verifiedSize, SHA256: verifiedSHA, ManagedStatus: domain.ManagedStatusManaged},
		{ID: "asset_changed", Path: "assets/changed.txt", Filename: "changed.txt", Stem: "changed", Extension: "txt", MediaType: "text/plain", Size: changedSize, SHA256: changedSHA, ManagedStatus: domain.ManagedStatusManaged},
		{ID: "asset_missing", Path: "assets/missing.txt", Filename: "missing.txt", Stem: "missing", Extension: "txt", MediaType: "text/plain", Size: 7, SHA256: "missing-sha", ManagedStatus: domain.ManagedStatusManaged},
	}}
	if err := Save(root, manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	result, err := Verify(root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Verified != 1 || result.Changed != 1 || result.Missing != 1 || result.Unmanaged != 1 || result.Failed != 0 {
		t.Fatalf("verify result = %#v", result)
	}
	if !hasVerificationStatus(result.Results, "unmanaged", "assets/unmanaged.txt") {
		t.Fatalf("unmanaged result missing: %#v", result.Results)
	}
}

func TestHashReaderStreamsWithoutLargeSingleRead(t *testing.T) {
	const size = int64(2 << 20)
	reader := &boundedChunkReader{remaining: size, maxReadSize: 64 << 10}
	sum, gotSize, err := hashReader(reader)
	if err != nil {
		t.Fatalf("hash reader: %v", err)
	}
	if gotSize != size {
		t.Fatalf("size = %d, want %d", gotSize, size)
	}
	if sum != repeatedByteSHA256('x', size) {
		t.Fatalf("sum = %s", sum)
	}
	if reader.reads < 2 {
		t.Fatalf("reader was not streamed: reads=%d", reader.reads)
	}
}

func BenchmarkVerifyLargeAssetStreaming(b *testing.B) {
	const size = int64(8 << 20)
	root := b.TempDir()
	assetPath := filepath.Join(root, "assets", "large.bin")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		b.Fatalf("mkdir assets: %v", err)
	}
	if err := writeRepeatedFile(assetPath, 'v', size); err != nil {
		b.Fatalf("write asset: %v", err)
	}
	sha, actualSize, err := hashFile(assetPath)
	if err != nil {
		b.Fatalf("hash asset: %v", err)
	}
	manifest := Manifest{Assets: []Asset{{ID: "asset_large", Path: "assets/large.bin", Filename: "large.bin", Stem: "large", Extension: "bin", MediaType: "application/octet-stream", Size: actualSize, SHA256: sha, ManagedStatus: domain.ManagedStatusManaged}}}
	if err := Save(root, manifest); err != nil {
		b.Fatalf("save manifest: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := Verify(root)
		if err != nil {
			b.Fatalf("verify: %v", err)
		}
		if result.Verified != 1 || result.Failed != 0 || result.Changed != 0 || result.Missing != 0 {
			b.Fatalf("verify result = %#v", result)
		}
	}
}

func writeRepeatedFile(path string, b byte, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	chunk := bytes.Repeat([]byte{b}, 64<<10)
	for size > 0 {
		write := int64(len(chunk))
		if write > size {
			write = size
		}
		if _, err := f.Write(chunk[:write]); err != nil {
			return err
		}
		size -= write
	}
	return nil
}

type boundedChunkReader struct {
	remaining   int64
	maxReadSize int
	reads       int
}

func (r *boundedChunkReader) Read(p []byte) (int, error) {
	if len(p) > r.maxReadSize {
		return 0, fmt.Errorf("read buffer too large: %d", len(p))
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= int64(len(p))
	r.reads++
	return len(p), nil
}

func repeatedByteSHA256(b byte, size int64) string {
	h := sha256.New()
	chunk := bytes.Repeat([]byte{b}, 32<<10)
	for size > 0 {
		write := int64(len(chunk))
		if write > size {
			write = size
		}
		_, _ = h.Write(chunk[:write])
		size -= write
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hasVerificationStatus(results []Verification, status, path string) bool {
	for _, result := range results {
		if result.Status == status && result.Asset.Path == path {
			return true
		}
	}
	return false
}
