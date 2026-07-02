package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	pinaxconfig "github.com/yeisme/pinax/internal/config"
	"github.com/yeisme/pinax/internal/domain"
)

func addConfigCommands(root *cobra.Command, ctx commandBuildContext) {
	var scope string
	configCmd := &cobra.Command{Use: "config", Short: "View and modify Pinax configuration"}

	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Show user-level and project-level config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := currentConfigPaths(ctx)
			projection := domain.NewProjection("config.path", "Resolved Pinax configuration paths.")
			projection.Facts["user_config"] = paths.User
			projection.Facts["project_config"] = paths.Project
			projection.Data = paths
			return ctx.renderProjection(cmd, projection, nil)
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Read the merged effective config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(args[0])
			value, ok := pinaxconfig.Value(ctx.configResult.Config, key)
			if !ok {
				return renderCommandError(cmd, ctx.outputMode(), "config.get", "config_key_unknown", "Unknown config key "+key, "Run pinax config doctor to inspect valid config keys")
			}
			projection := domain.NewProjection("config.get", fmt.Sprintf("%s = %s", key, value))
			projection.Facts["key"] = key
			projection.Facts["value"] = value
			projection.Data = map[string]any{"key": key, "value": value}
			if setting, ok := configSetting(*ctx.configResult, key); ok {
				projection.Facts["source"] = setting.Source
				projection.Facts["writable"] = fmt.Sprint(setting.Writable)
				projection.Facts["write_scope"] = setting.WriteScope
				if len(setting.WritableScopes) > 0 {
					projection.Facts["write_scopes"] = strings.Join(setting.WritableScopes, ",")
				}
				projection.Data = map[string]any{"key": key, "value": value, "setting": setting}
				if setting.NextAction != "" {
					projection.Actions = []domain.Action{{Name: "set", Command: setting.NextAction}}
				}
			}
			return ctx.renderProjection(cmd, projection, nil)
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check config sources and overrides",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := currentConfigPaths(ctx)
			projection := domain.NewProjection("config.doctor", "Configuration check passed.")
			projection.Facts["user_config"] = paths.User
			projection.Facts["project_config"] = paths.Project
			projection.Facts["output.color"] = ctx.configResult.Config.Output.Color
			projection.Facts["output.theme"] = ctx.configResult.Config.Output.Theme
			projection.Facts["output.width"] = fmt.Sprint(ctx.configResult.Config.Output.Width)
			diagnostics := map[string]string{
				"local_api_status":      configuredStatus(ctx.configResult.Config.Remote.APIURL),
				"remote_api_source":     configSourceForKey(*ctx.configResult, "remote.api_url"),
				"write_mode":            "local_config_write_requires_scope",
				"redaction_status":      "enabled",
				"profile_status":        "not_inspected",
				"token_status":          "not_inspected",
				"secret_ref_boundary":   "no_plaintext_secret_values",
				"body_exposure_default": "none",
			}
			for key, value := range diagnostics {
				projection.Facts[key] = value
			}
			projection.Data = map[string]any{"config": ctx.configResult.Config, "sources": ctx.configResult.Sources, "paths": paths, "settings": ctx.configResult.Settings, "diagnostics": diagnostics}
			return ctx.renderProjection(cmd, projection, nil)
		},
	})

	setCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Write user-level or project-level config",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := scopedConfigPath(ctx, scope)
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "config.set", err.Code, err.Message, err.Hint)
			}
			if err := pinaxconfig.SetValue(path, args[0], args[1]); err != nil {
				return renderConfigError(cmd, ctx.outputMode(), err)
			}
			projection := domain.NewProjection("config.set", "Configuration written.")
			projection.Facts["key"] = args[0]
			projection.Facts["scope"] = scope
			projection.Facts["path"] = path
			return ctx.renderProjection(cmd, projection, nil)
		},
	}
	setCmd.Flags().StringVar(&scope, "scope", "", "Write scope: user or project")
	configCmd.AddCommand(setCmd)

	unsetCmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Delete a user-level or project-level config item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := scopedConfigPath(ctx, scope)
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "config.unset", err.Code, err.Message, err.Hint)
			}
			if err := pinaxconfig.UnsetValue(path, args[0]); err != nil {
				return renderConfigError(cmd, ctx.outputMode(), err)
			}
			projection := domain.NewProjection("config.unset", "Configuration item deleted.")
			projection.Facts["key"] = args[0]
			projection.Facts["scope"] = scope
			projection.Facts["path"] = path
			return ctx.renderProjection(cmd, projection, nil)
		},
	}
	unsetCmd.Flags().StringVar(&scope, "scope", "", "Write scope: user or project")
	configCmd.AddCommand(unsetCmd)

	root.AddCommand(configCmd)
}

func configSetting(result pinaxconfig.LoadResult, key string) (pinaxconfig.SettingProjection, bool) {
	for _, setting := range result.Settings {
		if setting.Key == key {
			return setting, true
		}
	}
	return pinaxconfig.SettingProjection{}, false
}

func configSourceForKey(result pinaxconfig.LoadResult, key string) string {
	if setting, ok := configSetting(result, key); ok {
		return setting.Source
	}
	return "unknown"
}

func configuredStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not_configured"
	}
	return "configured"
}

func currentConfigPaths(ctx commandBuildContext) pinaxconfig.Paths {
	return pinaxconfig.ResolvePaths(pinaxconfig.PathOptions{VaultPath: *ctx.vaultPath, XDGConfigHome: os.Getenv("XDG_CONFIG_HOME")})
}

func scopedConfigPath(ctx commandBuildContext, scope string) (string, *domain.CommandError) {
	paths := currentConfigPaths(ctx)
	switch scope {
	case "user":
		return paths.User, nil
	case "project":
		return paths.Project, nil
	default:
		return "", &domain.CommandError{Code: "config_scope_required", Message: "config set/unset requires explicit --scope user or --scope project", Hint: "pinax config set output.theme mono --scope user"}
	}
}
