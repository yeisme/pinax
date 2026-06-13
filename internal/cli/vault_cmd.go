package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/dashboard"
	"github.com/yeisme/pinax/internal/domain"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	"github.com/yeisme/pinax/internal/vaultregistry"
)

func addVaultCommands(root *cobra.Command, ctx commandBuildContext) {
	statsAliasCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show local Markdown vault statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VaultStats(cmd.Context(), app.VaultStatsRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	statsAliasCmd.Hidden = true
	root.AddCommand(statsAliasCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local Markdown vault health",
		RunE: func(cmd *cobra.Command, args []string) error {
			duration, parseErr := parseDurationDays(*ctx.staleAfter)
			if parseErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "vault.doctor", "invalid_stale_after", parseErr.Error(), "Use a value such as 90d or 2160h")
			}
			projection, err := ctx.svc.VaultDoctor(cmd.Context(), app.VaultDoctorRequest{VaultPath: *ctx.vaultPath, StaleAfter: duration})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	doctorCmd.Hidden = true
	doctorCmd.Flags().StringVar(ctx.staleAfter, "stale-after", "90d", "Stale note threshold, such as 90d or 2160h")
	root.AddCommand(doctorCmd)

	dashboardCmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the read-only local Markdown vault dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dashboard.ListenAndServe(cmd.Context(), ctx.svc, *ctx.vaultPath, *ctx.dashboardPort, func(format string, args ...any) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
			})
		},
	}
	dashboardCmd.Hidden = true
	dashboardCmd.Flags().IntVar(ctx.dashboardPort, "port", 0, "dashboard localhost port; 0 assigns automatically")
	root.AddCommand(dashboardCmd)

	vaultCmd := &cobra.Command{Use: "vault", Short: "Manage vault-level commands"}
	vaultCmd.AddCommand(&cobra.Command{Use: "stats", Short: "Show local Markdown vault statistics", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.VaultStats(cmd.Context(), app.VaultStatsRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	vaultCmd.AddCommand(&cobra.Command{Use: "validate", Short: "Validate the local Pinax vault", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ValidateVault(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	vaultRegisterName := ""
	vaultRegisterDefault := false
	vaultRegisterCmd := &cobra.Command{Use: "register <path>", Short: "Register a named local vault for selection and completion", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(vaultRegisterName)
		if name == "" {
			return renderCommandError(cmd, ctx.outputMode(), "vault.register", "vault_name_required", "vault register requires --name", "pinax vault register <path> --name <alias>")
		}
		projection, err := ctx.svc.ValidateVault(cmd.Context(), app.VaultRequest{VaultPath: args[0]})
		if err != nil {
			projection.Command = "vault.register"
			return ctx.renderProjection(cmd, projection, err)
		}
		if err := vaultregistry.RegisterLocal(vaultregistry.DefaultPaths(), name, args[0], vaultRegisterDefault); err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.register", "vault_register_failed", err.Error(), "Use a simple alias such as work or personal")
		}
		registry, _ := vaultregistry.LoadRegistry(vaultregistry.DefaultPaths())
		item := registry.Locals[name]
		out := domain.NewProjection("vault.register", "Vault registered.")
		out.Facts["name"] = name
		out.Facts["path"] = item.Path
		out.Facts["default"] = fmt.Sprint(registry.Default == name)
		out.Data = map[string]any{"name": name, "path": item.Path, "default": registry.Default == name}
		return ctx.renderProjection(cmd, out, nil)
	}}
	vaultRegisterCmd.Flags().StringVar(&vaultRegisterName, "name", "", "Vault alias name")
	vaultRegisterCmd.Flags().BoolVar(&vaultRegisterDefault, "default", false, "Select this vault as the default")
	vaultCmd.AddCommand(vaultRegisterCmd)
	vaultCmd.AddCommand(&cobra.Command{Use: "use <alias>", Short: "Select the default registered vault", Args: cobra.ExactArgs(1), ValidArgsFunction: localVaultAliasCompletion, RunE: func(cmd *cobra.Command, args []string) error {
		if err := vaultregistry.UseDefault(vaultregistry.DefaultPaths(), args[0]); err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.use", "vault_alias_unknown", err.Error(), "Run pinax vault list to inspect registered vaults")
		}
		projection := domain.NewProjection("vault.use", "Default vault selected.")
		projection.Facts["default"] = args[0]
		projection.Data = map[string]string{"default": args[0]}
		return ctx.renderProjection(cmd, projection, nil)
	}})
	vaultCmd.AddCommand(&cobra.Command{Use: "list", Short: "List registered and cached remote vault selectors", RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := vaultregistry.LoadRegistry(vaultregistry.DefaultPaths())
		if err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.list", "vault_registry_read_failed", err.Error(), "Check the user vault registry file")
		}
		cache, _ := vaultregistry.LoadCache(vaultregistry.DefaultPaths())
		projection := domain.NewProjection("vault.list", "Vault selectors listed.")
		projection.Facts["default"] = registry.Default
		projection.Facts["local_vaults"] = fmt.Sprint(len(registry.Locals))
		remoteCount := 0
		for _, entry := range cache.Profiles {
			remoteCount += len(entry.Vaults)
		}
		projection.Facts["remote_vaults"] = fmt.Sprint(remoteCount)
		projection.Data = vaultregistry.MarshalRegistryForData(registry, cache)
		return ctx.renderProjection(cmd, projection, nil)
	}})
	remoteCmd := &cobra.Command{Use: "remote", Short: "Manage cached remote vault discovery"}
	remoteProfile := ""
	remoteCmd.AddCommand(&cobra.Command{Use: "list", Short: "List cached remote vault selectors", RunE: func(cmd *cobra.Command, args []string) error {
		cache, err := vaultregistry.LoadCache(vaultregistry.DefaultPaths())
		if err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.remote.list", "vault_cache_read_failed", err.Error(), "Run pinax vault remote refresh --profile <profile>")
		}
		profiles := cache.Profiles
		if strings.TrimSpace(remoteProfile) != "" {
			entry, ok := cache.Profiles[remoteProfile]
			if !ok {
				return renderCommandError(cmd, ctx.outputMode(), "vault.remote.list", "vault_remote_cache_missing", "No cached remote vaults for profile "+remoteProfile, "Run pinax vault remote refresh --profile "+remoteProfile)
			}
			profiles = map[string]vaultregistry.RemoteEntry{remoteProfile: entry}
		}
		count := 0
		for _, entry := range profiles {
			count += len(entry.Vaults)
		}
		projection := domain.NewProjection("vault.remote.list", "Cached remote vault selectors listed.")
		projection.Facts["profiles"] = fmt.Sprint(len(profiles))
		projection.Facts["remote_vaults"] = fmt.Sprint(count)
		projection.Data = map[string]any{"profiles": profiles}
		return ctx.renderProjection(cmd, projection, nil)
	}})
	remoteRefreshCmd := &cobra.Command{Use: "refresh", Short: "Refresh cached remote vault selectors from a profile endpoint", RunE: func(cmd *cobra.Command, args []string) error {
		profileName := strings.TrimSpace(remoteProfile)
		if profileName == "" {
			return renderCommandError(cmd, ctx.outputMode(), "vault.remote.refresh", "profile_required", "vault remote refresh requires --profile", "pinax vault remote refresh --profile <profile>")
		}
		profiles, err := pinaxprofile.Load()
		if err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.remote.refresh", "profile_load_failed", err.Error(), "Run pinax profile list")
		}
		profile, ok := profiles.Profiles[profileName]
		if !ok {
			return renderCommandError(cmd, ctx.outputMode(), "vault.remote.refresh", "profile_not_found", "Profile not found: "+profileName, "Run pinax profile add "+profileName+" --endpoint <url>")
		}
		secret, err := pinaxprofile.ResolveSecretRef(profile.SecretRef)
		if err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.remote.refresh", "profile_secret_failed", err.Error(), "Check the profile secret reference")
		}
		entry, err := vaultregistry.RefreshRemote(vaultregistry.DefaultPaths(), vaultregistry.RemoteRefreshRequest{Profile: profileName, Endpoint: profile.Endpoint, Workspace: profile.Workspace, Token: secret})
		if err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.remote.refresh", "remote_vault_refresh_failed", err.Error(), "Check the profile endpoint and retry")
		}
		projection := domain.NewProjection("vault.remote.refresh", "Remote vault cache refreshed.")
		projection.Facts["profile"] = profileName
		projection.Facts["remote_vaults"] = fmt.Sprint(len(entry.Vaults))
		projection.Data = vaultregistry.RedactedRemoteEntry(entry)
		return ctx.renderProjection(cmd, projection, nil)
	}}
	remoteListCmd := remoteCmd.Commands()[0]
	remoteListCmd.Flags().StringVar(&remoteProfile, "profile", "", "Profile name")
	remoteRefreshCmd.Flags().StringVar(&remoteProfile, "profile", "", "Profile name")
	remoteCmd.AddCommand(remoteRefreshCmd)
	vaultCmd.AddCommand(remoteCmd)
	vaultDoctorCmd := &cobra.Command{Use: "doctor", Short: "Check local Markdown vault health", RunE: func(cmd *cobra.Command, args []string) error {
		duration, parseErr := parseDurationDays(*ctx.staleAfter)
		if parseErr != nil {
			return renderCommandError(cmd, ctx.outputMode(), "vault.doctor", "invalid_stale_after", parseErr.Error(), "Use a value such as 90d or 2160h")
		}
		projection, err := ctx.svc.VaultDoctor(cmd.Context(), app.VaultDoctorRequest{VaultPath: *ctx.vaultPath, StaleAfter: duration})
		return ctx.renderProjection(cmd, projection, err)
	}}
	vaultDoctorCmd.Flags().StringVar(ctx.staleAfter, "stale-after", "90d", "Stale note threshold, such as 90d or 2160h")
	vaultCmd.AddCommand(vaultDoctorCmd)
	vaultDashboardCmd := &cobra.Command{Use: "dashboard", Short: "Start the read-only local Markdown vault dashboard", RunE: func(cmd *cobra.Command, args []string) error {
		return dashboard.ListenAndServe(cmd.Context(), ctx.svc, *ctx.vaultPath, *ctx.dashboardPort, func(format string, args ...any) {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
		})
	}}
	vaultDashboardCmd.Flags().IntVar(ctx.dashboardPort, "port", 0, "dashboard localhost port; 0 assigns automatically")
	vaultCmd.AddCommand(vaultDashboardCmd)
	root.AddCommand(vaultCmd)

	initCmd := &cobra.Command{
		Use:     "init [vault]",
		Short:   "Initialize a local Pinax Markdown vault",
		Long:    "Initialize a local Pinax Markdown vault. If no vault argument is provided, use the path from --vault; the default is the current directory.",
		Example: "pinax init\npinax init ./my-notes --title \"My Knowledge Base\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return renderCommandError(cmd, ctx.outputMode(), "vault.init", "too_many_arguments", "init accepts at most one vault path", "Run pinax init --help for usage")
			}
			targetVault := *ctx.vaultPath
			if len(args) == 1 {
				targetVault = args[0]
			}
			projection, err := ctx.svc.InitVault(cmd.Context(), app.InitVaultRequest{VaultPath: targetVault, Title: *ctx.title})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	initCmd.Flags().StringVar(ctx.title, "title", "", "Vault title")
	root.AddCommand(initCmd)

	validateAliasCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the local Pinax vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ValidateVault(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	validateAliasCmd.Hidden = true
	root.AddCommand(validateAliasCmd)
}

func localVaultAliasCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	registry, err := vaultregistry.LoadRegistry(vaultregistry.DefaultPaths())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items := make([]string, 0, len(registry.Locals))
	for alias, local := range registry.Locals {
		desc := "local vault " + local.Path
		if alias == registry.Default {
			desc += " default"
		}
		items = append(items, alias+"\t"+desc)
	}
	sort.Strings(items)
	return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}
