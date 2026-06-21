package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addPluginCommands(root *cobra.Command, ctx commandBuildContext) {
	var scope string
	var yes bool
	var dryRun bool
	var capability string
	pluginCmd := &cobra.Command{
		Use:     "plugin",
		Short:   "Manage dynamic plugins",
		Long:    "Validate and manage dynamic Pinax plugins through audited CLI services.",
		Example: "pinax plugin validate ./plugins/project-dashboard --vault ./my-notes --json",
	}
	validateCmd := &cobra.Command{
		Use:   "validate <plugin-path>",
		Short: "Validate a plugin manifest without installing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginValidate(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, Path: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	pluginCmd.AddCommand(validateCmd)
	installCmd := &cobra.Command{
		Use:   "install <plugin-path>",
		Short: "Install a plugin manifest into the vault registry disabled",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginInstall(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, Path: args[0], Scope: scope})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	installCmd.Flags().StringVar(&scope, "scope", "vault", "Plugin install scope: vault")
	pluginCmd.AddCommand(installCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginList(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	pluginCmd.AddCommand(listCmd)

	inspectCmd := &cobra.Command{
		Use:   "inspect <plugin-id>",
		Short: "Inspect an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginInspect(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	pluginCmd.AddCommand(inspectCmd)

	enableCmd := &cobra.Command{
		Use:   "enable <plugin-id>",
		Short: "Enable an installed plugin after explicit approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginEnable(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0], Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	enableCmd.Flags().BoolVar(&yes, "yes", false, "Approve plugin state change")
	pluginCmd.AddCommand(enableCmd)

	disableCmd := &cobra.Command{
		Use:   "disable <plugin-id>",
		Short: "Disable an installed plugin after explicit approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginDisable(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0], Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	disableCmd.Flags().BoolVar(&yes, "yes", false, "Approve plugin state change")
	pluginCmd.AddCommand(disableCmd)

	permissionsCmd := &cobra.Command{Use: "permissions", Short: "Manage plugin permission grants"}
	permissionsListCmd := &cobra.Command{
		Use:   "list <plugin-id>",
		Short: "List permission grants for an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginPermissionsList(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	permissionsCmd.AddCommand(permissionsListCmd)
	permissionsGrantCmd := &cobra.Command{
		Use:   "grant <plugin-id> <permission>",
		Short: "Grant a permission to a plugin capability after approval",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginPermissionsGrant(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0], Permission: args[1], Capability: capability, Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	permissionsGrantCmd.Flags().StringVar(&capability, "capability", "", "Capability id the permission applies to")
	permissionsGrantCmd.Flags().BoolVar(&yes, "yes", false, "Approve permission grant")
	permissionsCmd.AddCommand(permissionsGrantCmd)
	permissionsRevokeCmd := &cobra.Command{
		Use:   "revoke <plugin-id> <permission>",
		Short: "Revoke a plugin permission grant after approval",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginPermissionsRevoke(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0], Permission: args[1], Capability: capability, Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	permissionsRevokeCmd.Flags().StringVar(&capability, "capability", "", "Capability id the permission applies to")
	permissionsRevokeCmd.Flags().BoolVar(&yes, "yes", false, "Approve permission revoke")
	permissionsCmd.AddCommand(permissionsRevokeCmd)
	pluginCmd.AddCommand(permissionsCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check plugin registry and runtime readiness",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginDoctor(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	pluginCmd.AddCommand(doctorCmd)

	uninstallCmd := &cobra.Command{
		Use:   "uninstall <plugin-id>",
		Short: "Remove an installed plugin after explicit approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginUninstall(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0], Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	uninstallCmd.Flags().BoolVar(&yes, "yes", false, "Approve plugin uninstall")
	pluginCmd.AddCommand(uninstallCmd)

	runCmd := &cobra.Command{
		Use:   "run <plugin-id> <capability-id>",
		Short: "Run an enabled plugin capability through the plugin runtime",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PluginRun(cmd.Context(), app.PluginRequest{VaultPath: *ctx.vaultPath, PluginID: args[0], Capability: args[1], DryRun: dryRun})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	runCmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview plugin output without applying writes")
	pluginCmd.AddCommand(runCmd)
	root.AddCommand(pluginCmd)
}
