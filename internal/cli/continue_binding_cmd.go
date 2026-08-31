package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/continuitybinding"
	"github.com/yeisme/pinax/internal/domain"
)

// continuityScopeResolution 是 continue/checkpoint 的 scope+vault 解析结果。
type continuityScopeResolution struct {
	Scope         agentprotocol.Scope
	VaultPath     string
	BindingStatus continuitybinding.BindingStatus
	BindingDigest string
}

// resolveContinueScope 按固定优先级解析 scope 与 vault：
//
//  1. 显式 --scope 或全局 --vault 已设置 → 完全走旧路径（binding 不参与，
//     显式参数永远胜出，现有脚本行为不被 binding 改写）。
//  2. 无任何显式参数 → 尝试当前 Git worktree 的唯一 enabled exact binding。
//     resolved → 使用 binding 的 vault 与 scope。
//  3. 无 binding → 保留 workspace:default legacy fallback，
//     binding_status=missing（调用方附一个 copyable bind action）。
//
// ambiguous/invalid 状态 fail closed，返回 stable error 且不选任意 vault。
func resolveContinueScope(cmd *cobra.Command, ctx commandBuildContext, scopeFlag string) (continuityScopeResolution, error) {
	resolution := continuityScopeResolution{
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"},
		VaultPath:     agentVaultPath(ctx),
		BindingStatus: continuitybinding.StatusMissing,
	}
	if scopeFlag != "" {
		resolution.Scope = parseScope(scopeFlag)
	}
	explicitVault := false
	if f := cmd.Root().PersistentFlags().Lookup("vault"); f != nil && f.Changed {
		explicitVault = true
	}
	if explicitVault || scopeFlag != "" {
		// 显式参数路径：binding 不参与解析，状态留空表示不适用。
		resolution.BindingStatus = ""
		return resolution, nil
	}
	resolved, err := agentSvc.ContinuityResolveBinding(cmd.Context(), app.ContinuityResolveRequest{RepoPath: "."})
	if err != nil {
		return resolution, err
	}
	resolution.BindingStatus = resolved.Status
	if resolved.Status == continuitybinding.StatusReady {
		resolution.Scope = resolved.Binding.Scope()
		resolution.VaultPath = resolved.VaultPath
		resolution.BindingDigest = resolved.Binding.RepoRootDigest
	}
	return resolution, nil
}

// addContinueBindingSubcommands 注册 additive experimental 子命令：
// `pinax continue bind` 与 `pinax continue status`。
//
// 所有 mutation 都由 app service 写入 user-level registry；CLI 只做参数
// 校验和输出 projection。默认输出 bounded：只暴露 digest 与 basename，
// 不泄漏用户 home 下的完整绝对路径。
func addContinueBindingSubcommands(parent *cobra.Command, ctx commandBuildContext) {
	var bindRepo, bindVault, bindScope string
	bindCmd := &cobra.Command{
		Use:   "bind",
		Short: "Experimental: bind the current Git worktree to a registered vault and bounded scope",
		Long: `Bind the current Git repository/worktree to one registered vault and a bounded
scope (project:<slug> or workspace:<id>) so that 'pinax continue' can auto-resolve.

The binding registry is a user-level, owner-only file written by the Pinax service.
All flags are required: no cross-vault search, no implicit project creation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if bindVault == "" {
				return renderCommandError(cmd, ctx.outputMode(), "continue.bind", "argument_required",
					"--vault <registered-vault-ref> is required", "pinax vault list --json")
			}
			if bindScope == "" {
				return renderCommandError(cmd, ctx.outputMode(), "continue.bind", "argument_required",
					"--scope <kind:id> is required", "pinax project list --vault <vault> --json")
			}
			scope := parseScope(bindScope)
			result, err := agentSvc.ContinuityBind(cmd.Context(), app.ContinuityBindingRequest{
				RepoPath:  bindRepo,
				VaultRef:  bindVault,
				ScopeKind: string(scope.Kind),
				ScopeID:   scope.ID,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.bind", err)
			}
			proj := domain.NewProjection("continue.bind", "Continuity binding saved.")
			proj.Facts["binding_status"] = result.BindingStatus
			proj.Facts["binding_id_digest"] = result.BindingDigest
			proj.Facts["repo_root"] = result.RepoRootBasename
			proj.Facts["vault_ref"] = result.VaultRef
			proj.Facts["scope"] = result.Scope
			proj.Facts["experimental"] = "true"
			proj.Data = result
			proj.Actions = []domain.Action{
				{Name: "continue", Command: "pinax continue"},
				// 默认输出 bounded：action 不携带绝对路径，只给 copyable 相对命令。
				{Name: "status", Command: "pinax continue status --repo ."},
			}
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	bindCmd.Flags().StringVar(&bindRepo, "repo", ".", "Git repository/worktree path to bind")
	bindCmd.Flags().StringVar(&bindVault, "vault", "", "Registered vault ref (alias or path)")
	bindCmd.Flags().StringVar(&bindScope, "scope", "", "Bounded scope as kind:id (e.g. project:pinax)")
	parent.AddCommand(bindCmd)

	var statusRepo string
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Experimental: show read-only continuity binding status and readiness diagnostics",
		Long: `Show repository detection, binding status (missing/disabled/invalid/ambiguous/ready),
vault availability, scope validity, and registry schema. This command is read-only:
it never creates run receipts, handoffs, proposals, or vault mutations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := agentSvc.ContinuityBindingStatus(cmd.Context(), app.ContinuityBindingStatusRequest{RepoPath: statusRepo})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.status", err)
			}
			proj := domain.NewProjection("continue.status", "Continuity binding status.")
			proj.Facts["binding_status"] = string(result.BindingStatus)
			proj.Facts["ready"] = fmt.Sprintf("%t", result.Ready)
			proj.Facts["is_repository"] = fmt.Sprintf("%t", result.IsRepository)
			proj.Facts["registry"] = result.Registry
			proj.Facts["vault_resolved"] = fmt.Sprintf("%t", result.VaultResolved)
			proj.Facts["scope_valid"] = fmt.Sprintf("%t", result.ScopeValid)
			if result.BindingDigest != "" {
				proj.Facts["binding_id_digest"] = result.BindingDigest
			}
			if result.RepoBasename != "" {
				proj.Facts["repo_root"] = result.RepoBasename
			}
			if result.VaultRef != "" {
				proj.Facts["vault_ref"] = result.VaultRef
			}
			if result.Scope != "" {
				proj.Facts["scope"] = result.Scope
			}
			proj.Facts["experimental"] = "true"
			proj.Data = result
			switch result.BindingStatus {
			case "ready":
				proj.Actions = []domain.Action{{Name: "continue", Command: "pinax continue"}}
			case "missing":
				proj.Actions = []domain.Action{{Name: "bind", Command: "pinax continue bind --repo . --vault <vault-ref> --scope <kind:id>"}}
			case "ambiguous", "invalid":
				proj.Actions = []domain.Action{{Name: "rebind", Command: "pinax continue bind --repo . --vault <vault-ref> --scope <kind:id>"}}
			case "not_a_repository":
				proj.Actions = []domain.Action{{Name: "inspect", Command: "pinax continue status --repo <git-worktree> --json"}}
			}
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	statusCmd.Flags().StringVar(&statusRepo, "repo", ".", "Git repository/worktree path to inspect")
	parent.AddCommand(statusCmd)
}
