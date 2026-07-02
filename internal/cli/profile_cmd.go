package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Added profile: %s\n", name)
			return nil
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
			if len(cfg.Profiles) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No profiles. ")
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-40s %-15s %-15s %s\n", "Name", "Endpoint", "Workspace", "Device", "Scope")
			for name, p := range cfg.Profiles {
				ep := p.Endpoint
				if len(ep) > 40 {
					ep = ep[:37] + "..."
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-40s %-15s %-15s %s\n",
					name, ep, p.Workspace, p.Device, p.DefaultScope)
			}
			if cfg.Defaults.Profile != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nDefault profile: %s\n", cfg.Defaults.Profile)
			}
			return nil
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "name:       %s\n", args[0])
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Endpoint:   %s\n", p.Endpoint)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Workspace:  %s\n", p.Workspace)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Device:     %s\n", p.Device)
			if p.SecretRef != "" {
				// Show type but not value
				if strings.HasPrefix(p.SecretRef, "env://") {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Secret:     %s\n", p.SecretRef)
				} else if strings.HasPrefix(p.SecretRef, "keychain://") {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Secret:     %s\n", p.SecretRef)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Secret:     [configured]\n")
				}
			}
			if p.DefaultScope != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Default scope: %s\n", p.DefaultScope)
			}
			return nil
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted profile: %s\n", args[0])
			return nil
		},
	}

	profileCmd.AddCommand(profileAddCmd, profileListCmd, profileShowCmd, profileRemoveCmd)
	root.AddCommand(profileCmd)
}
