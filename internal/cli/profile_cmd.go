package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/profile"
)

func addProfileCommands(root *cobra.Command, ctx commandBuildContext) {
	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage backend connection profile aliases",
	}

	profileAddCmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add a backend connection profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			endpoint, _ := cmd.Flags().GetString("endpoint")
			workspace, _ := cmd.Flags().GetString("workspace")
			device, _ := cmd.Flags().GetString("device")
			secretRef, _ := cmd.Flags().GetString("secret-ref")
			defaultScope, _ := cmd.Flags().GetString("default-scope")

			if endpoint == "" {
				return renderCommandError(cmd, ctx.outputMode(), "profile.add", "missing_endpoint", "--endpoint is required", "")
			}

			cfg, err := profile.Load()
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "profile.add", "load_error", err.Error(), "")
			}
			cfg.Profiles[name] = profile.Profile{
				Endpoint:     endpoint,
				Workspace:    workspace,
				Device:       device,
				SecretRef:    secretRef,
				DefaultScope: defaultScope,
			}
			if err := profile.Save(cfg); err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "profile.add", "save_error", err.Error(), "")
			}
			projection := domain.NewProjection("profile.add", "Added profile: "+name)
			projection.Facts["profile"] = name
			projection.Facts["endpoint"] = endpoint
			projection.Facts["workspace"] = workspace
			if device != "" {
				projection.Facts["device"] = device
			}
			if defaultScope != "" {
				projection.Facts["default_scope"] = defaultScope
			}
			projection.Data = map[string]any{"profile": map[string]any{"name": name, "endpoint": endpoint, "workspace": workspace, "device": device, "default_scope": defaultScope}}
			return ctx.renderProjection(cmd, projection, nil)
		},
	}
	profileAddCmd.Flags().String("endpoint", "", "Backend storage address")
	profileAddCmd.Flags().String("workspace", "default", "workspace id")
	profileAddCmd.Flags().String("device", "", "device id")
	profileAddCmd.Flags().String("secret-ref", "", "Encryption secret reference (env://VAR, keychain://service/account, plain:text)")
	profileAddCmd.Flags().String("default-scope", "", "Default permission scope")

	profileListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all backend connection profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := profile.Load()
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "profile.list", "load_error", err.Error(), "")
			}
			profiles := make([]map[string]any, 0, len(cfg.Profiles))
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				p := cfg.Profiles[name]
				profiles = append(profiles, map[string]any{
					"name":          name,
					"endpoint":      p.Endpoint,
					"workspace":     p.Workspace,
					"device":        p.Device,
					"default_scope": p.DefaultScope,
					"default":       cfg.Defaults.Profile == name,
				})
			}
			projection := domain.NewProjection("profile.list", "Profiles listed.")
			if len(profiles) == 0 {
				projection.Summary = "No profiles."
			}
			projection.Facts["profiles"] = fmt.Sprint(len(profiles))
			if cfg.Defaults.Profile != "" {
				projection.Facts["default_profile"] = cfg.Defaults.Profile
			}
			projection.Data = map[string]any{"profiles": profiles}
			return ctx.renderProjection(cmd, projection, nil)
		},
	}

	profileShowCmd := &cobra.Command{
		Use:               "show [name]",
		Short:             "Show one profile in detail",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: profileNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := profile.Load()
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "profile.show", "load_error", err.Error(), "")
			}
			p, ok := cfg.Profiles[args[0]]
			if !ok {
				return renderCommandError(cmd, ctx.outputMode(), "profile.show", "not_found", "profile not found: "+args[0], "")
			}
			projection := domain.NewProjection("profile.show", "Profile shown.")
			projection.Facts["profile"] = args[0]
			projection.Facts["endpoint"] = p.Endpoint
			projection.Facts["workspace"] = p.Workspace
			if p.Device != "" {
				projection.Facts["device"] = p.Device
			}
			if p.SecretRef != "" {
				// Show type but not value
				if strings.HasPrefix(p.SecretRef, "env://") {
					projection.Facts["secret_ref"] = p.SecretRef
				} else if strings.HasPrefix(p.SecretRef, "keychain://") {
					projection.Facts["secret_ref"] = p.SecretRef
				} else {
					projection.Facts["secret_ref"] = "configured"
				}
			}
			if p.DefaultScope != "" {
				projection.Facts["default_scope"] = p.DefaultScope
			}
			projection.Data = map[string]any{"profile": map[string]any{"name": args[0], "endpoint": p.Endpoint, "workspace": p.Workspace, "device": p.Device, "default_scope": p.DefaultScope}}
			return ctx.renderProjection(cmd, projection, nil)
		},
	}

	profileRemoveCmd := &cobra.Command{
		Use:               "remove [name]",
		Short:             "Remove a backend connection profile",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: profileNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := profile.Load()
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "profile.remove", "load_error", err.Error(), "")
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return renderCommandError(cmd, ctx.outputMode(), "profile.remove", "not_found", "profile not found: "+args[0], "")
			}
			delete(cfg.Profiles, args[0])
			if err := profile.Save(cfg); err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "profile.remove", "save_error", err.Error(), "")
			}
			projection := domain.NewProjection("profile.remove", "Deleted profile: "+args[0])
			projection.Facts["profile"] = args[0]
			projection.Data = map[string]any{"profile": args[0]}
			return ctx.renderProjection(cmd, projection, nil)
		},
	}

	profileCmd.AddCommand(profileAddCmd, profileListCmd, profileShowCmd, profileRemoveCmd)
	root.AddCommand(profileCmd)
}
