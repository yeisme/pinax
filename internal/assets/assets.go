package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const ManifestSchemaVersion = "pinax.assets.v1"

type Asset = domain.Asset

type Manifest = domain.AssetManifest

type Verification = domain.AssetVerification

type VerifyResult = domain.AssetVerifyResult

type AddMode string

const (
	AddModeCopy     AddMode = "copy"
	AddModeRegister AddMode = "register"
)

type AddOptions struct {
	Mode AddMode
}

func Add(root, source string) (Asset, error) {
	return AddWithOptions(root, source, AddOptions{Mode: AddModeCopy})
}

func AddWithOptions(root, source string, opts AddOptions) (Asset, error) {
	mode := opts.Mode
	if mode == "" {
		mode = AddModeCopy
	}
	if mode != AddModeCopy && mode != AddModeRegister {
		return Asset{}, fmt.Errorf("unsupported asset add mode %q", mode)
	}
	manifest, err := Load(root)
	if err != nil {
		return Asset{}, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return Asset{}, err
	}
	if info.IsDir() {
		return Asset{}, fmt.Errorf("asset source is a directory")
	}
	var rel string
	var target string
	if mode == AddModeRegister {
		var inside bool
		rel, inside, err = vaultRelativePath(root, source)
		if err != nil {
			return Asset{}, err
		}
		if !inside || strings.HasPrefix(rel, ".pinax/") {
			return Asset{}, fmt.Errorf("asset source must be inside vault for register mode")
		}
		target = filepath.Join(root, filepath.FromSlash(rel))
	} else {
		filename := filepath.Base(source)
		rel = filepath.ToSlash(filepath.Join("assets", filename))
		rel = uniqueAssetPath(root, manifest, rel)
		target = filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return Asset{}, err
		}
		if err := copyFile(target, source); err != nil {
			return Asset{}, err
		}
		if info, err = os.Stat(target); err != nil {
			return Asset{}, err
		}
	}
	sha, size, err := hashFile(target)
	if err != nil {
		return Asset{}, err
	}
	width, height, err := detectImageDimensions(target)
	if err != nil {
		return Asset{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	asset := Asset{ID: "asset_" + sha[:12], Path: rel, Filename: filepath.Base(rel), Stem: strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)), Extension: strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), "."), MediaType: mediaType(rel), Size: size, ModifiedUnix: info.ModTime().Unix(), Width: width, Height: height, SHA256: sha, ManagedStatus: "managed", CreatedAt: now, UpdatedAt: now}
	manifest.Assets = upsertAsset(manifest.Assets, asset)
	if err := Save(root, manifest); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func Load(root string) (Manifest, error) {
	path := manifestPath(root)
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{SchemaVersion: ManifestSchemaVersion, Assets: []Asset{}}, nil
		}
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = ManifestSchemaVersion
	}
	if manifest.Assets == nil {
		manifest.Assets = []Asset{}
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Save(root string, manifest Manifest) error {
	manifest.SchemaVersion = ManifestSchemaVersion
	if manifest.Assets == nil {
		manifest.Assets = []Asset{}
	}
	if err := Validate(manifest); err != nil {
		return err
	}
	path := manifestPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func Validate(manifest Manifest) error {
	if manifest.SchemaVersion != "" && manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported asset manifest schema %q", manifest.SchemaVersion)
	}
	for i, asset := range manifest.Assets {
		if strings.TrimSpace(asset.ID) == "" {
			return fmt.Errorf("asset manifest entry %d missing id", i)
		}
		cleanPath := pathpkg.Clean(strings.TrimSpace(filepath.ToSlash(asset.Path)))
		if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || strings.HasPrefix(cleanPath, ".pinax/") || filepath.IsAbs(cleanPath) {
			return fmt.Errorf("asset manifest entry %d has unsafe path %q", i, asset.Path)
		}
		if strings.TrimSpace(asset.SHA256) == "" {
			return fmt.Errorf("asset manifest entry %d missing sha256", i)
		}
	}
	return nil
}

func Find(root, ref string) (Asset, error) {
	manifest, err := Load(root)
	if err != nil {
		return Asset{}, err
	}
	ref = strings.TrimSpace(filepath.ToSlash(ref))
	for _, asset := range manifest.Assets {
		if asset.ID == ref || asset.Path == ref || asset.Filename == ref || asset.Stem == ref {
			return asset, nil
		}
	}
	return Asset{}, os.ErrNotExist
}

func Verify(root string) (VerifyResult, error) {
	manifest, err := Load(root)
	if err != nil {
		return VerifyResult{}, err
	}
	result := VerifyResult{Results: []Verification{}}
	managed := map[string]bool{}
	for _, asset := range manifest.Assets {
		managed[asset.Path] = true
		sha, _, err := hashFile(filepath.Join(root, filepath.FromSlash(asset.Path)))
		if err != nil {
			if os.IsNotExist(err) {
				result.Missing++
				result.Orphan++
				result.Results = append(result.Results, Verification{Asset: asset, Status: "missing"})
				continue
			}
			result.Failed++
			result.Results = append(result.Results, Verification{Asset: asset, Status: "failed"})
			continue
		}
		if sha != asset.SHA256 {
			result.Changed++
			result.Results = append(result.Results, Verification{Asset: asset, Status: "changed", SHA256: sha})
			continue
		}
		result.Verified++
		result.Results = append(result.Results, Verification{Asset: asset, Status: "verified", SHA256: sha})
	}
	if err := scanUnmanagedAssets(root, managed, &result); err != nil {
		return VerifyResult{}, err
	}
	return result, nil
}

func scanUnmanagedAssets(root string, managed map[string]bool, result *VerifyResult) error {
	for _, base := range []string{"assets", "attachments"} {
		start := filepath.Join(root, base)
		if _, err := os.Stat(start); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := filepath.WalkDir(start, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if managed[rel] {
				return nil
			}
			asset := unmanagedAsset(rel)
			sha, size, err := hashFile(path)
			if err != nil {
				result.Failed++
				result.Results = append(result.Results, Verification{Asset: asset, Status: "failed"})
				return nil
			}
			asset.Size = size
			asset.SHA256 = sha
			result.Unmanaged++
			result.Results = append(result.Results, Verification{Asset: asset, Status: "unmanaged", SHA256: sha})
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func unmanagedAsset(rel string) Asset {
	return Asset{Path: rel, Filename: filepath.Base(rel), Stem: strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)), Extension: strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), "."), MediaType: mediaType(rel), ManagedStatus: domain.ManagedStatusUnmanaged}
}

func manifestPath(root string) string {
	return filepath.Join(root, ".pinax", "assets", "manifest.json")
}

func uniqueAssetPath(root string, manifest Manifest, rel string) string {
	used := map[string]bool{}
	for _, asset := range manifest.Assets {
		used[asset.Path] = true
	}
	ext := filepath.Ext(rel)
	base := strings.TrimSuffix(rel, ext)
	candidate := rel
	for i := 2; fileExists(filepath.Join(root, filepath.FromSlash(candidate))) || used[candidate]; i++ {
		candidate = filepath.ToSlash(fmt.Sprintf("%s-%d%s", base, i, ext))
	}
	return candidate
}

func vaultRelativePath(root, target string) (string, bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return rel, false, nil
	}
	return rel, true, nil
}

func upsertAsset(assets []Asset, asset Asset) []Asset {
	out := make([]Asset, 0, len(assets)+1)
	for _, existing := range assets {
		if existing.Path != asset.Path {
			out = append(out, existing)
		}
	}
	return append(out, asset)
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	return hashReader(f)
}

func hashReader(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func detectImageDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, nil
	}
	return config.Width, config.Height, nil
}

func mediaType(path string) string {
	if mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); mt != "" {
		return strings.Split(mt, ";")[0]
	}
	return "application/octet-stream"
}

func copyFile(target, source string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
