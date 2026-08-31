package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/continuitybinding"
	"github.com/yeisme/pinax/internal/vaultregistry"
)

// ContinuityBindingRequest 描述一次 binding 写操作。
// ConfigDir 为空时使用 user-level 默认 config dir（XDG_CONFIG_HOME/pinax）。
type ContinuityBindingRequest struct {
	RepoPath  string
	VaultRef  string
	ScopeKind string
	ScopeID   string
	ConfigDir string
}

// ContinuityBindingResult 是 bind/status 的 bounded 结果。
// 不暴露完整 canonical_repo_root，只暴露 digest 和 basename。
type ContinuityBindingResult struct {
	BindingID        string `json:"binding_id"`
	BindingDigest    string `json:"binding_id_digest"`
	RepoRootBasename string `json:"repo_root_basename"`
	VaultRef         string `json:"vault_ref"`
	Scope            string `json:"scope"`
	Enabled          bool   `json:"enabled"`
	BindingStatus    string `json:"binding_status"`
	Ready            bool   `json:"ready"`
}

func continuityConfigDir(configDir string) string {
	if strings.TrimSpace(configDir) != "" {
		return configDir
	}
	return vaultregistry.DefaultPaths().ConfigDir
}

// resolveRegisteredVault 验证 vault selector 必须解析到已注册 vault。
// 允许 alias（registry locals）或与已注册 vault 绝对路径一致的 path-like selector。
// 缺失时不猜测、不跨 vault 搜索，直接返回 invalid 语义错误。
func resolveRegisteredVault(configDir, selector string) (string, string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", "", fmt.Errorf("vault_ref is required")
	}
	registry, err := vaultregistry.LoadRegistry(vaultregistry.Paths{ConfigDir: configDir})
	if err != nil {
		return "", "", fmt.Errorf("load vault registry: %w", err)
	}
	if local, ok := registry.Locals[selector]; ok {
		return local.Path, selector, nil
	}
	if filepath.IsAbs(selector) {
		abs, err := filepath.Abs(selector)
		if err == nil {
			var aliases []string
			for alias, local := range registry.Locals {
				localAbs, localErr := filepath.Abs(local.Path)
				if localErr == nil && filepath.Clean(localAbs) == filepath.Clean(abs) {
					aliases = append(aliases, alias)
				}
			}
			if len(aliases) > 0 {
				sort.Strings(aliases)
				alias := aliases[0]
				return registry.Locals[alias].Path, alias, nil
			}
		}
	}
	display := selector
	if filepath.IsAbs(selector) || strings.Contains(selector, "://") {
		display = filepath.Base(filepath.Clean(selector))
	}
	return "", "", fmt.Errorf("vault %q is not registered (pinax vault list --json)", display)
}

// scopeExistsInVault 校验 binding scope 在目标 vault 中存在。
// project scope 必须存在于 vault project registry；不隐式创建 project。
func scopeExistsInVault(vaultRoot, scopeKind, scopeID string) error {
	switch agentprotocol.ScopeKind(scopeKind) {
	case agentprotocol.ScopeKindProject:
		registry, err := loadProjectRegistry(vaultRoot)
		if err != nil {
			return fmt.Errorf("load project registry: %w", err)
		}
		for _, project := range registry.Projects {
			if project.Slug == scopeID {
				return nil
			}
		}
		return fmt.Errorf("project scope %q does not exist in vault (pinax project create %s --vault <registered-vault-ref>)", scopeID, scopeID)
	case agentprotocol.ScopeKindWorkspace:
		// workspace:default 是内置 fallback scope；其他 workspace id 是合法命名空间。
		if strings.TrimSpace(scopeID) == "" {
			return fmt.Errorf("workspace scope id is required")
		}
		return nil
	default:
		// 第一 slice 只允许 project/workspace 两类 bounded scope；
		// repository/session/task 绑定延后，避免 scope 生命周期语义漂移。
		return fmt.Errorf("scope kind %q is not bindable in this slice (use project:<slug> or workspace:<id>)", scopeKind)
	}
}

// ContinuityBind 创建或更新一条 user-level binding（唯一由 service 写 registry）。
// 校验顺序：repository → registered vault → scope 存在；任一失败不写部分状态。
func (s *AgentMemoryService) ContinuityBind(ctx context.Context, req ContinuityBindingRequest) (ContinuityBindingResult, error) {
	ctx = ensureCtx(ctx)
	configDir := continuityConfigDir(req.ConfigDir)

	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKind(req.ScopeKind), ID: req.ScopeID}
	if err := scope.Validate(); err != nil {
		return ContinuityBindingResult{}, err
	}
	canonicalRoot, err := continuitybinding.DetectWorktreeRoot(ctx, req.RepoPath)
	if err != nil {
		return ContinuityBindingResult{}, agentprotocol.NewStableError(continuitybinding.ErrCodeBindingInvalid,
			"repository is not a resolvable git worktree")
	}
	vaultRoot, canonicalVaultRef, err := resolveRegisteredVault(configDir, req.VaultRef)
	if err != nil {
		return ContinuityBindingResult{}, agentprotocol.NewStableError(continuitybinding.ErrCodeBindingInvalid, err.Error())
	}
	if err := scopeExistsInVault(vaultRoot, req.ScopeKind, req.ScopeID); err != nil {
		return ContinuityBindingResult{}, agentprotocol.NewStableError(continuitybinding.ErrCodeBindingInvalid, err.Error())
	}

	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		return ContinuityBindingResult{}, err
	}
	saved := registry.Upsert(continuitybinding.Binding{
		CanonicalRepoRoot: canonicalRoot,
		VaultRef:          canonicalVaultRef,
		ScopeKind:         req.ScopeKind,
		ScopeID:           req.ScopeID,
		Enabled:           true,
	}, time.Now().UTC())
	if err := continuitybinding.SaveRegistry(configDir, registry); err != nil {
		return ContinuityBindingResult{}, err
	}
	return ContinuityBindingResult{
		BindingID:        saved.BindingID,
		BindingDigest:    saved.RepoRootDigest,
		RepoRootBasename: continuitybinding.RedactedRoot(canonicalRoot),
		VaultRef:         saved.VaultRef,
		Scope:            fmt.Sprintf("%s:%s", saved.ScopeKind, saved.ScopeID),
		Enabled:          saved.Enabled,
		BindingStatus:    string(continuitybinding.StatusReady),
		Ready:            true,
	}, nil
}

// ContinuityBindingStatusRequest 描述一次 read-only binding status 查询。
type ContinuityBindingStatusRequest struct {
	RepoPath  string
	ConfigDir string
}

// ContinuityBindingStatusResult 是 status projection 的 bounded 结果。
// 查询全程 read-only：不创建 run receipt、handoff、proposal 或 vault mutation。
type ContinuityBindingStatusResult struct {
	BindingStatus continuitybinding.BindingStatus `json:"binding_status"`
	Ready         bool                            `json:"ready"`
	IsRepository  bool                            `json:"is_repository"`
	BindingDigest string                          `json:"binding_id_digest,omitempty"`
	RepoBasename  string                          `json:"repo_root_basename,omitempty"`
	VaultRef      string                          `json:"vault_ref,omitempty"`
	VaultResolved bool                            `json:"vault_resolved"`
	Scope         string                          `json:"scope,omitempty"`
	ScopeValid    bool                            `json:"scope_valid"`
	Registry      string                          `json:"registry"`
}

// ContinuityBindingStatus 返回 binding 诊断状态（missing/disabled/invalid/ambiguous/ready）。
func (s *AgentMemoryService) ContinuityBindingStatus(ctx context.Context, req ContinuityBindingStatusRequest) (ContinuityBindingStatusResult, error) {
	ctx = ensureCtx(ctx)
	configDir := continuityConfigDir(req.ConfigDir)
	result := ContinuityBindingStatusResult{
		BindingStatus: continuitybinding.StatusNotARepository,
		Registry:      continuitybinding.BindingSchemaVersion,
	}

	canonicalRoot, err := continuitybinding.DetectWorktreeRoot(ctx, req.RepoPath)
	if err != nil {
		// 非 Git worktree：不向上搜索、不跨 vault 扫描，直接报告 not_a_repository。
		return result, nil
	}
	result.IsRepository = true
	result.RepoBasename = continuitybinding.RedactedRoot(canonicalRoot)

	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		return result, err
	}
	enabled := registry.EnabledFor(canonicalRoot)
	switch len(enabled) {
	case 0:
		if len(registry.AnyFor(canonicalRoot)) > 0 {
			result.BindingStatus = continuitybinding.StatusDisabled
		} else {
			result.BindingStatus = continuitybinding.StatusMissing
		}
		return result, nil
	case 1:
		// 唯一 enabled exact binding；继续验证目标是否仍可解析。
	default:
		// 多条 enabled exact binding = 损坏状态，fail closed。
		result.BindingStatus = continuitybinding.StatusAmbiguous
		return result, nil
	}

	binding := enabled[0]
	result.BindingDigest = binding.RepoRootDigest
	result.VaultRef = binding.VaultRef
	result.Scope = fmt.Sprintf("%s:%s", binding.ScopeKind, binding.ScopeID)

	vaultRoot, canonicalVaultRef, err := resolveRegisteredVault(configDir, binding.VaultRef)
	result.VaultResolved = err == nil
	if err == nil {
		result.VaultRef = canonicalVaultRef
	}
	if err == nil {
		result.ScopeValid = scopeExistsInVault(vaultRoot, binding.ScopeKind, binding.ScopeID) == nil
	}
	if result.VaultResolved && result.ScopeValid {
		result.BindingStatus = continuitybinding.StatusReady
		result.Ready = true
	} else {
		// vault/scope 在绑定后被删除：invalid，禁止回退到其他 vault 掩盖错误。
		result.BindingStatus = continuitybinding.StatusInvalid
	}
	return result, nil
}

// ContinuityResolveRequest 描述一次 binding 自动解析请求。
type ContinuityResolveRequest struct {
	RepoPath  string
	ConfigDir string
}

// ContinuityResolveResult 描述解析优先级结果。
// 显式参数永远优先于 binding（由调用方先判断），本函数只负责 binding 层。
type ContinuityResolveResult struct {
	Status    continuitybinding.BindingStatus `json:"binding_status"`
	Binding   continuitybinding.Binding       `json:"binding,omitempty"`
	VaultPath string                          `json:"vault_path,omitempty"`
}

// ContinuityResolveBinding 按固定顺序解析当前 worktree 的 binding：
// 唯一 enabled exact binding → 解析；缺失 → missing；多条 → ambiguous 报错。
// 返回 missing 不是错误（legacy default fallback），ambiguous/invalid 是错误。
func (s *AgentMemoryService) ContinuityResolveBinding(ctx context.Context, req ContinuityResolveRequest) (ContinuityResolveResult, error) {
	ctx = ensureCtx(ctx)
	configDir := continuityConfigDir(req.ConfigDir)
	canonicalRoot, err := continuitybinding.DetectWorktreeRoot(ctx, req.RepoPath)
	if err != nil {
		return ContinuityResolveResult{Status: continuitybinding.StatusNotARepository}, nil
	}
	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		return ContinuityResolveResult{}, err
	}
	enabled := registry.EnabledFor(canonicalRoot)
	switch len(enabled) {
	case 0:
		return ContinuityResolveResult{Status: continuitybinding.StatusMissing}, nil
	case 1:
		// fallthrough
	default:
		return ContinuityResolveResult{Status: continuitybinding.StatusAmbiguous},
			agentprotocol.NewStableError(continuitybinding.ErrCodeBindingAmbiguous,
				"multiple enabled continuity bindings for this worktree").
				WithDetail("recovery", "pinax continue status --repo "+req.RepoPath)
	}
	binding := enabled[0]
	vaultRoot, canonicalVaultRef, err := resolveRegisteredVault(configDir, binding.VaultRef)
	if err != nil {
		return ContinuityResolveResult{Status: continuitybinding.StatusInvalid},
			agentprotocol.NewStableError(continuitybinding.ErrCodeBindingInvalid, err.Error())
	}
	if err := scopeExistsInVault(vaultRoot, binding.ScopeKind, binding.ScopeID); err != nil {
		return ContinuityResolveResult{Status: continuitybinding.StatusInvalid},
			agentprotocol.NewStableError(continuitybinding.ErrCodeBindingInvalid, err.Error())
	}
	binding.VaultRef = canonicalVaultRef
	return ContinuityResolveResult{
		Status:    continuitybinding.StatusReady,
		Binding:   binding,
		VaultPath: vaultRoot,
	}, nil
}
