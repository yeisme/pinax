package continuitybinding

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"gopkg.in/yaml.v3"
)

// registryFileName 是 user-level binding registry 文件名。
const registryFileName = "continuity_bindings.yaml"

// Registry 是 continuity binding registry 的 versioned 文件形态。
type Registry struct {
	SchemaVersion string    `yaml:"schema_version" json:"schema_version"`
	Bindings      []Binding `yaml:"bindings,omitempty" json:"bindings,omitempty"`
}

// RegistryPath 返回 registry 文件路径（user-level config dir 下）。
// configDir 为空时使用默认 config dir 调用方提供的路径。
func RegistryPath(configDir string) string {
	return filepath.Join(configDir, registryFileName)
}

// LoadRegistry 读取 registry；文件不存在时返回空 registry（不是错误）。
// YAML 损坏时返回 continuity_binding_invalid，调用方必须 fail closed。
func LoadRegistry(configDir string) (Registry, error) {
	path := RegistryPath(configDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Registry{SchemaVersion: BindingSchemaVersion}, nil
		}
		return Registry{}, fmt.Errorf("read continuity binding registry: %w", err)
	}
	var registry Registry
	if err := yaml.Unmarshal(b, &registry); err != nil {
		return Registry{}, agentprotocol.NewStableError(ErrCodeBindingInvalid,
			"continuity binding registry is corrupt: fix or remove "+registryFileName)
	}
	if registry.SchemaVersion != BindingSchemaVersion {
		return Registry{}, agentprotocol.NewStableError(ErrCodeBindingInvalid,
			fmt.Sprintf("unsupported registry schema_version %q", registry.SchemaVersion))
	}
	for i, binding := range registry.Bindings {
		if err := binding.Validate(); err != nil {
			return Registry{}, agentprotocol.NewStableError(ErrCodeBindingInvalid,
				fmt.Sprintf("continuity binding registry record %d is invalid", i))
		}
	}
	return registry, nil
}

// SaveRegistry 原子写入 registry（temp + rename），权限 owner-only 0600。
// 原子写保证并发/崩溃时不会留下半截 YAML 触发损坏分支。
func SaveRegistry(configDir string, registry Registry) error {
	registry.SchemaVersion = BindingSchemaVersion
	enabledRoots := map[string]struct{}{}
	for i, binding := range registry.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("binding[%d]: %w", i, err)
		}
		if binding.Enabled {
			if _, exists := enabledRoots[binding.CanonicalRepoRoot]; exists {
				return newInvalid("multiple enabled bindings for the same canonical_repo_root")
			}
			enabledRoots[binding.CanonicalRepoRoot] = struct{}{}
		}
	}
	path := RegistryPath(configDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(registry)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(path), registryFileName+".tmp-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// Upsert 创建或更新一条 binding。同一 canonical_repo_root 的既有 enabled
// 记录会被替换（重新绑定同一 worktree 是显式用户操作），不同 root 的
// 记录保持不变。返回最终写入的记录。
func (r *Registry) Upsert(binding Binding, now time.Time) Binding {
	binding.SchemaVersion = BindingSchemaVersion
	binding.RepoRootDigest = Digest(binding.CanonicalRepoRoot)
	if binding.BindingID == "" {
		binding.BindingID = "bind_" + binding.RepoRootDigest
	}
	// 同一 worktree 重新绑定时收敛所有重复记录。这样即使旧版本或损坏
	// registry 留下多条 exact binding，显式 bind 也能成为安全恢复路径。
	kept := make([]Binding, 0, len(r.Bindings)+1)
	for _, existing := range r.Bindings {
		if existing.CanonicalRepoRoot == binding.CanonicalRepoRoot {
			if binding.CreatedAt.IsZero() || (!existing.CreatedAt.IsZero() && existing.CreatedAt.Before(binding.CreatedAt)) {
				binding.CreatedAt = existing.CreatedAt
			}
			continue
		}
		kept = append(kept, existing)
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	r.Bindings = append(kept, binding)
	return binding
}

// Disable 禁用指定 canonical root 的所有 binding（rollback 不删除数据）。
func (r *Registry) Disable(canonicalRoot string, now time.Time) int {
	disabled := 0
	for i := range r.Bindings {
		if r.Bindings[i].CanonicalRepoRoot == canonicalRoot && r.Bindings[i].Enabled {
			r.Bindings[i].Enabled = false
			r.Bindings[i].UpdatedAt = now
			disabled++
		}
	}
	return disabled
}

// EnabledFor 返回某 canonical root 的全部 enabled binding。
// 多条结果即 ambiguous——registry 写路径保证唯一，读到多条说明状态损坏。
func (r Registry) EnabledFor(canonicalRoot string) []Binding {
	var matches []Binding
	for _, binding := range r.Bindings {
		if binding.CanonicalRepoRoot == canonicalRoot && binding.Enabled {
			matches = append(matches, binding)
		}
	}
	return matches
}

// AnyFor 返回某 canonical root 的全部 binding（含 disabled）。
func (r Registry) AnyFor(canonicalRoot string) []Binding {
	var matches []Binding
	for _, binding := range r.Bindings {
		if binding.CanonicalRepoRoot == canonicalRoot {
			matches = append(matches, binding)
		}
	}
	return matches
}
