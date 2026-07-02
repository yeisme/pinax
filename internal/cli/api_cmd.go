package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/output"
)

func addAPICommands(root *cobra.Command, ctx commandBuildContext) {
	var readonly bool
	var allowWrite bool
	var tokenFile string
	var noAuth bool
	var exposeGroups string
	var hideGroups string
	newSchemaCmd := func(example string) *cobra.Command {
		var schemaFormat string
		schemaCmd := &cobra.Command{Use: "schema", Short: "Export the local API schema", Example: example}
		schemaExportCmd := &cobra.Command{Use: "export", Short: "Export the local API schema", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.APISchemaExport(cmd.Context(), app.APIRequest{VaultPath: *ctx.vaultPath, Format: schemaFormat})
			return ctx.renderProjection(cmd, projection, err)
		}}
		schemaExportCmd.Flags().StringVar(&schemaFormat, "format", "openapi", "Schema format: openapi")
		_ = schemaExportCmd.RegisterFlagCompletionFunc("format", staticCompletion("format", "openapi"))
		schemaCmd.AddCommand(schemaExportCmd)
		return schemaCmd
	}

	schemaAliasCmd := newSchemaCmd("pinax schema export --format openapi --vault ./my-notes --json")
	schemaAliasCmd.Hidden = true
	root.AddCommand(schemaAliasCmd)

	apiCmd := &cobra.Command{Use: "api", Short: "Manage the local REST/RPC projection adapter"}
	routesCmd := &cobra.Command{Use: "routes", Short: "List local API capabilities", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.APIRoutes(cmd.Context(), app.APIRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	statusCmd := &cobra.Command{Use: "status", Short: "Show workbench status projection", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.WorkbenchStatus(cmd.Context(), app.APIRequest{VaultPath: *ctx.vaultPath, WriteMode: "local_cli"})
		return ctx.renderProjection(cmd, projection, err)
	}}
	serveCmd := &cobra.Command{Use: "serve", Short: "Start the local API server", RunE: func(cmd *cobra.Command, args []string) error {
		if readonly && allowWrite {
			return renderCommandError(cmd, ctx.outputMode(), "api.serve", "write_mode_conflict", "api serve cannot use --readonly and --allow-write together", "Keep only one write-mode flag")
		}
		mode := ctx.outputMode()
		if mode == output.ModeJSON || mode == output.ModeAgent {
			return renderCommandError(cmd, mode, "api.serve", "unsupported_output_mode", "api serve is long-running and does not support this output mode yet", "Use --events, or remove machine-output flags and read the URL from stderr")
		}

		authMode := api.AuthModeTemp
		if tokenFile != "" {
			authMode = api.AuthModeTokenFile
		}
		if noAuth {
			authMode = api.AuthModeNone
		}
		if noAuth && tokenFile != "" {
			return renderCommandError(cmd, ctx.outputMode(), "api.serve", "auth_mode_conflict", "api serve cannot use --no-auth and --token-file together", "Keep only one authentication-mode flag")
		}

		var exposeList, hideList []string
		if exposeGroups != "" {
			exposeList = strings.Split(exposeGroups, ",")
		}
		if hideGroups != "" {
			hideList = strings.Split(hideGroups, ",")
		}

		options := api.ServerOptions{
			AllowWrite:   allowWrite,
			AuthMode:     authMode,
			TokenFile:    tokenFile,
			ExposeGroups: exposeList,
			HideGroups:   hideList,
		}
		if mode == output.ModeEvents {
			return serveAPIEvents(cmd, ctx, options)
		}
		options.Logger = newAPIServeLogger(cmd)
		defer func() { _ = options.Logger.Sync() }()
		return api.ListenAndServe(cmd.Context(), ctx.svc, *ctx.vaultPath, *ctx.dashboardPort, nil, options)
	}}
	serveCmd.Flags().BoolVar(&readonly, "readonly", false, "Start in read-only mode (default)")
	serveCmd.Flags().BoolVar(&allowWrite, "allow-write", false, "Allow remote writes to controlled mutation routes such as folder")
	serveCmd.Flags().IntVar(ctx.dashboardPort, "port", 0, "localhost port; 0 assigns automatically")
	serveCmd.Flags().StringVar(&tokenFile, "token-file", "", "Load a long-lived token from a file")
	serveCmd.Flags().BoolVar(&noAuth, "no-auth", false, "No-auth mode (forces loopback)")
	serveCmd.Flags().StringVar(&exposeGroups, "expose", "", "Expose only the specified route groups (comma-separated)")
	serveCmd.Flags().StringVar(&hideGroups, "hide", "", "Hide the specified route groups (comma-separated)")
	apiCmd.AddCommand(routesCmd, statusCmd, newSchemaCmd("pinax api schema export --format openapi --vault ./my-notes --json"), serveCmd)
	root.AddCommand(apiCmd)
	addTokenCommands(root, ctx)
}

func newAPIServeLogger(cmd *cobra.Command) *zap.Logger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:          "time",
		LevelKey:         "level",
		NameKey:          "logger",
		CallerKey:        "caller",
		FunctionKey:      zapcore.OmitKey,
		MessageKey:       "msg",
		StacktraceKey:    "stacktrace",
		LineEnding:       zapcore.DefaultLineEnding,
		EncodeLevel:      zapcore.CapitalColorLevelEncoder,
		EncodeTime:       zapcore.TimeEncoderOfLayout(time.RFC3339),
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
		ConsoleSeparator: "  ",
	}
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), zapcore.AddSync(cmd.ErrOrStderr()), zapcore.InfoLevel)
	return zap.New(core)
}

func serveAPIEvents(cmd *cobra.Command, ctx commandBuildContext, options api.ServerOptions) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	seq := 1
	writeEvent := func(event map[string]any) {
		event["seq"] = seq
		seq++
		_ = enc.Encode(event)
	}
	writeEvent(map[string]any{"type": "start", "command": "api.serve"})
	readyEmitted := false
	err := api.ListenAndServe(cmd.Context(), ctx.svc, *ctx.vaultPath, *ctx.dashboardPort, func(format string, args ...any) {
		message := fmt.Sprintf(format, args...)
		if !readyEmitted {
			readyEmitted = true
			writeEvent(map[string]any{"type": "ready", "command": "api.serve", "url": extractLocalAPIURL(message)})
		} else if strings.HasPrefix(message, "Temp token:") {
			writeEvent(map[string]any{"type": "auth", "command": "api.serve", "credential": "temp_token", "secret": "redacted"})
		} else {
			writeEvent(map[string]any{"type": "log", "command": "api.serve", "message": message})
		}
	}, options)
	if err != nil {
		writeEvent(map[string]any{"type": "error", "command": "api.serve", "error": map[string]any{"code": "api_serve_failed", "message": err.Error()}})
		return err
	}
	writeEvent(map[string]any{"type": "shutdown", "command": "api.serve"})
	return nil
}

func extractLocalAPIURL(message string) string {
	idx := strings.Index(message, "http://")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(message[idx:])
}
