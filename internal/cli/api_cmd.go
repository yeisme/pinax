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
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
)

func addAPICommands(root *cobra.Command, ctx commandBuildContext) {
	var readonly bool
	var allowWrite bool
	var tokenStore string
	var legacyTokenFile string
	var noAuth bool
	var exposeGroups string
	var hideGroups string
	var sessionStdin bool
	var sessionRecoveryStdin bool
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
	manifestCmd := &cobra.Command{Use: "manifest", Short: "Show the authoritative transport manifest", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := TransportManifestProjection()
		return ctx.renderProjection(cmd, projection, err)
	}}
	routesCmd := &cobra.Command{Use: "routes", Short: "List local API capabilities", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.APIRoutes(cmd.Context(), app.APIRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	statusCmd := &cobra.Command{Use: "status", Short: "Show workbench status projection", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.WorkbenchStatus(cmd.Context(), app.APIRequest{VaultPath: *ctx.vaultPath, WriteMode: "local_cli"})
		return ctx.renderProjection(cmd, projection, err)
	}}
	serveCmd := &cobra.Command{Use: "serve", Short: "Start the local API server", RunE: func(cmd *cobra.Command, args []string) error {
		// Supervised-session mode speaks the JSON event protocol on stdout:
		// its flag rejections must stay machine events (stdout JSON event +
		// stderr human text), never the human status table on stdout.
		sessionFlagError := func(code, message, hint string) error {
			_ = api.WriteSessionError(cmd.OutOrStdout(), code, message)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s; %s\n", code, message, hint)
			return &domain.CommandError{Code: code, Message: message, Hint: hint}
		}
		if sessionRecoveryStdin && !sessionStdin {
			return renderCommandError(cmd, ctx.outputMode(), "api.serve", "session_mode_required", "Private recovery requires --session-stdin", "Add --session-stdin and use the private host protocol")
		}
		if readonly && allowWrite {
			if sessionStdin {
				return sessionFlagError("write_mode_conflict", "api serve cannot use --readonly and --allow-write together", "Keep only one write-mode flag")
			}
			return renderCommandError(cmd, ctx.outputMode(), "api.serve", "write_mode_conflict", "api serve cannot use --readonly and --allow-write together", "Keep only one write-mode flag")
		}
		if tokenStore != "" && legacyTokenFile != "" {
			if sessionStdin {
				return sessionFlagError("auth_mode_conflict", "api serve cannot use --token-store and --token-file together", "Keep --token-store and remove the deprecated --token-file alias")
			}
			return renderCommandError(cmd, ctx.outputMode(), "api.serve", "auth_mode_conflict", "api serve cannot use --token-store and --token-file together", "Keep --token-store and remove the deprecated --token-file alias")
		}
		if noAuth && (tokenStore != "" || legacyTokenFile != "") {
			if sessionStdin {
				return sessionFlagError("auth_mode_conflict", "api serve cannot combine --no-auth with a token store", "Keep only one authentication-mode flag")
			}
			return renderCommandError(cmd, ctx.outputMode(), "api.serve", "auth_mode_conflict", "api serve cannot combine --no-auth with a token store", "Keep only one authentication-mode flag")
		}
		if sessionStdin && (allowWrite || noAuth || tokenStore != "" || legacyTokenFile != "") {
			return sessionFlagError("auth_mode_conflict", "api serve --session-stdin is a readonly supervised reader session and cannot combine with write or legacy auth flags", "Keep --session-stdin alone")
		}
		mode := ctx.outputMode()
		if mode == output.ModeJSON || mode == output.ModeAgent {
			if sessionStdin {
				// Supervised mode speaks the frozen session event schema on
				// stdout; --events is also incompatible here, so the hint must
				// not suggest it.
				return sessionFlagError("unsupported_output_mode", "session-stdin speaks its own stdout protocol and does not support machine-output flags", "Drop the machine-output flag; consume stdout session events")
			}
			return renderCommandError(cmd, mode, "api.serve", "unsupported_output_mode", "api serve is long-running and does not support this output mode yet", "Use --events, or remove machine-output flags and read the URL from stderr")
		}

		if sessionStdin {
			// Supervised human reading session: the private stdin first frame
			// carries the bearer and grants; stdout speaks the redacted host
			// protocol. The legacy server path below stays untouched.
			if mode == output.ModeEvents {
				return sessionFlagError("unsupported_output_mode", "session-stdin speaks its own stdout protocol and does not support --events", "Drop --events; consume stdout JSON events")
			}
			frame, frameErr := api.ReadSessionBootstrapWithDeadline(cmd.Context(), cmd.InOrStdin())
			if frameErr != nil {
				// Diagnostics stay generic: bootstrap frames are private
				// input and their raw contents never reach logs. stdout
				// stays a PURE JSON event stream for line-decoding hosts —
				// the human-readable diagnostic goes to stderr only.
				_ = api.WriteSessionError(cmd.OutOrStdout(), "session_bootstrap_invalid", "The supervised session bootstrap frame was rejected")
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "session_bootstrap_invalid: the supervised session bootstrap frame was rejected; check the first stdin frame against pinax.local_session.bootstrap.v1alpha1\n")
				return &domain.CommandError{Code: "session_bootstrap_invalid", Message: "the supervised session bootstrap frame was rejected", Hint: "Check the first stdin frame against pinax.local_session.bootstrap.v1alpha1"}
			}
			return api.ServeSupervisedSession(cmd.Context(), api.ServeSupervisedSessionOptions{
				Service:       ctx.svc,
				Frame:         frame,
				Stdin:         cmd.InOrStdin(),
				Stdout:        cmd.OutOrStdout(),
				Stderr:        cmd.ErrOrStderr(),
				Manifest:      TransportManifestProjection,
				RecoveryStdin: sessionRecoveryStdin,
			})
		}

		authMode := api.AuthModeTemp
		selectedTokenStore := tokenStore
		if selectedTokenStore == "" {
			selectedTokenStore = legacyTokenFile
		}
		if selectedTokenStore != "" {
			authMode = api.AuthModeTokenStore
		}
		if noAuth {
			authMode = api.AuthModeNone
		}
		if legacyTokenFile != "" && mode != output.ModeEvents {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Deprecated: server --token-file names a hashed token registry; use: pinax api serve --token-store <hashed-token-store> [other flags]")
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
			TokenStore:   selectedTokenStore,
			ExposeGroups: exposeList,
			HideGroups:   hideList,
			Manifest:     TransportManifestProjection,
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
	serveCmd.Flags().StringVar(&tokenStore, "token-store", "", "Load a hashed long-lived token registry")
	serveCmd.Flags().StringVar(&legacyTokenFile, "token-file", "", "Deprecated alias for --token-store (hashed token registry)")
	serveCmd.Flags().BoolVar(&noAuth, "no-auth", false, "No-auth mode (forces loopback)")
	serveCmd.Flags().BoolVar(&sessionStdin, "session-stdin", false, "Supervised readonly human reading session: read a private bootstrap frame from stdin, serve /v1/reader on a random loopback port, stop on stdin EOF")
	serveCmd.Flags().BoolVar(&sessionRecoveryStdin, "session-recovery-stdin", false, "Require a private recovery challenge and one approved lookup grant before ready (requires --session-stdin)")
	serveCmd.Flags().StringVar(&exposeGroups, "expose", "", "Expose only the specified route groups (comma-separated)")
	serveCmd.Flags().StringVar(&hideGroups, "hide", "", "Hide the specified route groups (comma-separated)")
	apiCmd.AddCommand(manifestCmd, routesCmd, statusCmd, newSchemaCmd("pinax api schema export --format openapi --vault ./my-notes --json"), serveCmd)
	registerRemoteCommand(remoteCommandSpec{CommandPath: "api manifest", Method: "Pinax.Transport.Manifest"})
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
