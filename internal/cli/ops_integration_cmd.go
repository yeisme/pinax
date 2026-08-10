package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addBriefingCommands(root *cobra.Command, ctx commandBuildContext) {
	briefingCmd := &cobra.Command{Use: "briefing", Short: "Manage daily hot-notes briefing"}
	briefingRecipeCmd := &cobra.Command{Use: "recipe", Short: "Manage briefing recipes"}
	briefingRecipeInitCmd := &cobra.Command{Use: "init", Short: "Create the default briefing recipe", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.BriefingRecipeInit(cmd.Context(), app.BriefingRecipeRequest{VaultPath: *ctx.vaultPath, Topic: *ctx.briefingTopic, Limit: *ctx.briefingLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	briefingRecipeInitCmd.Flags().StringVar(ctx.briefingTopic, "topic", "", "briefing topic")
	briefingRecipeInitCmd.Flags().IntVar(ctx.briefingLimit, "limit", 0, "Maximum number of candidates")
	briefingRecipeCmd.AddCommand(briefingRecipeInitCmd)
	briefingRecipeCmd.AddCommand(&cobra.Command{Use: "show", Short: "Show the briefing recipe", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.BriefingRecipeShow(cmd.Context(), app.BriefingRecipeRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	briefingRecipeSetCmd := &cobra.Command{Use: "set", Short: "Update the briefing recipe", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.BriefingRecipeSet(cmd.Context(), app.BriefingRecipeRequest{VaultPath: *ctx.vaultPath, Topic: *ctx.briefingTopic, Limit: *ctx.briefingLimit, Source: *ctx.briefingSource})
		return ctx.renderProjection(cmd, projection, err)
	}}
	briefingRecipeSetCmd.Flags().StringVar(ctx.briefingTopic, "topic", "", "briefing topic")
	briefingRecipeSetCmd.Flags().IntVar(ctx.briefingLimit, "limit", 0, "Maximum number of candidates")
	briefingRecipeSetCmd.Flags().StringVar(ctx.briefingSource, "source", "", "New research source id")
	briefingRecipeCmd.AddCommand(briefingRecipeSetCmd)

	briefingDeliverCmd := &cobra.Command{Use: "deliver", Short: "Deliver a briefing"}
	feishuCmd := &cobra.Command{Use: "feishu", Short: "Deliver a briefing through a Feishu webhook", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.DeliverFeishu(cmd.Context(), app.FeishuDeliveryRequest{VaultPath: *ctx.vaultPath, WebhookURL: *ctx.feishuWebhook, SecretRef: *ctx.feishuSecretRef, Title: *ctx.feishuTitle, Text: *ctx.feishuText, DryRun: *ctx.deliveryDryRun, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	feishuCmd.Flags().StringVar(ctx.feishuWebhook, "webhook", "", "Feishu webhook URL")
	feishuCmd.Flags().StringVar(ctx.feishuSecretRef, "secret-ref", "", "Webhook secret reference; do not output the raw value")
	feishuCmd.Flags().StringVar(ctx.feishuTitle, "title", "", "Delivery title")
	feishuCmd.Flags().StringVar(ctx.feishuText, "text", "", "Delivery text")
	feishuCmd.Flags().BoolVar(ctx.deliveryDryRun, "dry-run", false, "Only generate a receipt preview; do not send the HTTP POST")
	feishuCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sending the Feishu webhook")
	briefingDeliverCmd.AddCommand(feishuCmd)
	briefingCmd.AddCommand(briefingDeliverCmd)
	briefingCmd.AddCommand(briefingRecipeCmd)

	briefingRunCmd := &cobra.Command{Use: "run", Short: "Run the daily hot-notes briefing", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.BriefingRun(cmd.Context(), app.BriefingRunRequest{VaultPath: *ctx.vaultPath, DryRun: *ctx.briefingDryRun, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	briefingRunCmd.Flags().BoolVar(ctx.briefingDryRun, "dry-run", false, "Only output candidates; do not write the vault or deliver")
	briefingRunCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm writing briefing candidate notes")
	briefingCmd.AddCommand(briefingRunCmd)
	root.AddCommand(briefingCmd)
}

func addCapsaCommands(root *cobra.Command, ctx commandBuildContext) {
	capsaCmd := &cobra.Command{Use: "capsa", Short: "Manage Capsa encrypted sync state"}
	capsaLoginCmd := &cobra.Command{
		Use:     "login",
		Short:   "Configure Capsa backend state",
		Example: "pinax capsa login --endpoint https://capsa.example.test --workspace ws_123 --device laptop --secret-ref op://pinax/capsa-token --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CapsaLogin(cmd.Context(), app.CloudLoginRequest{VaultPath: *ctx.vaultPath, Endpoint: *ctx.cloudEndpoint, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice, SecretRef: *ctx.cloudSecretRef, EncryptionSecretRef: *ctx.cloudEncryptionSecretRef})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	capsaLoginCmd.Flags().StringVar(ctx.cloudEndpoint, "endpoint", "", "Capsa backend URL")
	capsaLoginCmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "", "Capsa workspace id")
	capsaLoginCmd.Flags().StringVar(ctx.cloudDevice, "device", "", "Local device id")
	capsaLoginCmd.Flags().StringVar(ctx.cloudSecretRef, "secret-ref", "", "Capsa auth token reference; do not save the raw token")
	capsaLoginCmd.Flags().StringVar(ctx.cloudEncryptionSecretRef, "encryption-secret-ref", "", "Shared encryption secret reference; defaults to --secret-ref for old configs")
	capsaCmd.AddCommand(capsaLoginCmd)

	capsaBackendCmd := &cobra.Command{Use: "backend", Short: "Configure Capsa sync transport backend"}
	capsaBackendSetCmd := &cobra.Command{Use: "set", Short: "Set Capsa sync transport backend"}
	capsaBackendSetS3Cmd := &cobra.Command{
		Use:     "s3",
		Short:   "Configure S3-compatible direct Capsa backend",
		Example: "pinax capsa backend set s3 --bucket notes --region us-east-1 --prefix pinax-sync/ --profile work --workspace personal --device laptop --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CapsaBackendSetS3(cmd.Context(), app.CloudBackendSetRequest{VaultPath: *ctx.vaultPath, Kind: "s3", Bucket: *ctx.s3Bucket, Region: *ctx.s3Region, Prefix: *ctx.s3Prefix, Endpoint: *ctx.s3Endpoint, Profile: *ctx.s3Profile, AddressingStyle: *ctx.s3AddressingStyle, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice, SecretRef: *ctx.cloudSecretRef, EncryptionSecretRef: *ctx.cloudEncryptionSecretRef})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.s3Bucket, "bucket", "", "S3 bucket name")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.s3Region, "region", "", "S3 region")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.s3Prefix, "prefix", "", "S3 object key prefix")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.s3Endpoint, "endpoint", "", "S3-compatible endpoint URL")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.s3Profile, "profile", "", "S3 credential profile name; do not save the secret")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.s3AddressingStyle, "addressing-style", "auto", "S3 addressing style: auto, path, or virtual-hosted")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "", "Capsa workspace id")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.cloudDevice, "device", "", "Local device id")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.cloudSecretRef, "secret-ref", "", "Secret manager reference; do not save the raw secret")
	capsaBackendSetS3Cmd.Flags().StringVar(ctx.cloudEncryptionSecretRef, "encryption-secret-ref", "", "Dedicated sync encryption secret reference; avoids weak-key warning")
	capsaBackendSetRcloneCmd := &cobra.Command{
		Use:     "rclone",
		Short:   "Configure rclone direct Capsa backend",
		Example: "pinax capsa backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CapsaBackendSetRclone(cmd.Context(), app.CloudBackendSetRequest{VaultPath: *ctx.vaultPath, Kind: "rclone", Remote: *ctx.backendRemote, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	capsaBackendSetRcloneCmd.Flags().StringVar(ctx.backendRemote, "remote", "", "Rclone remote and path, for example onedrive:PinaxSync")
	capsaBackendSetRcloneCmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "", "Capsa workspace id")
	capsaBackendSetRcloneCmd.Flags().StringVar(ctx.cloudDevice, "device", "", "Local device id")
	capsaBackendSetCmd.AddCommand(capsaBackendSetRcloneCmd)
	capsaBackendSetCmd.AddCommand(capsaBackendSetS3Cmd)
	capsaBackendCmd.AddCommand(capsaBackendSetCmd)
	capsaCmd.AddCommand(capsaBackendCmd)
	capsaCmd.AddCommand(&cobra.Command{Use: "status", Short: "Show Capsa state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CapsaStatus(cmd.Context(), app.CloudRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	capsaCmd.AddCommand(&cobra.Command{Use: "logout", Short: "Log out the local Capsa device session", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CapsaLogout(cmd.Context(), app.CloudRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	capsaCmd.AddCommand(&cobra.Command{Use: "doctor", Short: "Diagnose Capsa state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CapsaDoctor(cmd.Context(), app.CloudRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	root.AddCommand(capsaCmd)
}

func addPlanningCommands(root *cobra.Command, ctx commandBuildContext) {
	planCmd := &cobra.Command{Use: "plan", Short: "Manage personal planning workflows"}
	planDailyCmd := &cobra.Command{Use: "daily", Short: "Generate a daily plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanDaily(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, TaskReview: *ctx.planTaskReview, DryRun: *ctx.planDryRun, Yes: *ctx.yes, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planDailyCmd.Flags().BoolVar(ctx.planTaskReview, "task-review", false, "Update the daily task review managed block")
	planDailyCmd.Flags().BoolVar(ctx.planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planDailyCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save a plan snapshot")
	planDailyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planDailyCmd)
	planWeeklyCmd := &cobra.Command{Use: "weekly", Short: "Generate a weekly plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanWeekly(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, DryRun: *ctx.planDryRun, Yes: *ctx.yes, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planWeeklyCmd.Flags().BoolVar(ctx.planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planWeeklyCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save a plan snapshot")
	planWeeklyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planWeeklyCmd)
	planMonthlyCmd := &cobra.Command{Use: "monthly", Short: "Generate a monthly plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanMonthly(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, DryRun: *ctx.planDryRun, Yes: *ctx.yes, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planMonthlyCmd.Flags().BoolVar(ctx.planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planMonthlyCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save a plan snapshot")
	planMonthlyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planMonthlyCmd)
	planActionsCmd := &cobra.Command{Use: "actions", Short: "Generate planning action drafts", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanActions(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, FromPeriod: *ctx.planFromPeriod, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planActionsCmd.Flags().StringVar(ctx.planFromPeriod, "from", "daily", "Source planning period: daily or weekly")
	planActionsCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save action drafts")
	planCmd.AddCommand(planActionsCmd)
	planSnapshotCmd := &cobra.Command{Use: "snapshot", Short: "Generate a plan snapshot", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanSnapshot(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planCmd.AddCommand(planSnapshotCmd)
	root.AddCommand(planCmd)
}

func addBackendCommands(root *cobra.Command, ctx commandBuildContext) {
	backendCmd := &cobra.Command{Use: "backend", Short: "Manage vault backend providers", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.list", "argument_unexpected", "backend does not accept positional arguments", "pinax backend list --vault <vault>")
		}
		projection, err := ctx.svc.ListBackends(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendListRun := func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.list", "argument_unexpected", "backend list does not accept positional arguments", "pinax backend list --vault <vault>")
		}
		projection, err := ctx.svc.ListBackends(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}
	backendCmd.AddCommand(&cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List all vault backends", RunE: backendListRun})
	backendAddCmd := &cobra.Command{Use: "add <kind> <name>", Short: "Add a backend profile", Example: "pinax backend add s3 work-s3 --bucket notes --region us-east-1 --vault ./my-notes\npinax backend add rclone work-drive --remote workdrive:pinax --vault ./my-notes", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.add", "argument_required", "backend add requires a backend kind and name", "pinax backend add <kind> <name> --vault <vault>")
		}
		projection, err := ctx.svc.AddBackend(cmd.Context(), app.BackendAddRequest{VaultPath: *ctx.vaultPath, Name: args[1], Kind: args[0], Root: *ctx.backendRoot, Bucket: *ctx.s3Bucket, Region: *ctx.s3Region, Prefix: *ctx.s3Prefix, Endpoint: *ctx.s3Endpoint, Profile: *ctx.s3Profile, Remote: *ctx.backendRemote})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendAddCmd.Flags().StringVar(ctx.backendRoot, "root", "", "Local backend root directory")
	backendAddCmd.Flags().StringVar(ctx.s3Bucket, "bucket", "", "S3 bucket name")
	backendAddCmd.Flags().StringVar(ctx.s3Region, "region", "", "S3 region")
	backendAddCmd.Flags().StringVar(ctx.s3Prefix, "prefix", "", "S3 object key prefix")
	backendAddCmd.Flags().StringVar(ctx.s3Endpoint, "endpoint", "", "S3-compatible endpoint URL")
	backendAddCmd.Flags().StringVar(ctx.s3Profile, "profile", "", "S3 credential profile name")
	backendAddCmd.Flags().StringVar(ctx.backendRemote, "remote", "", "rclone remote path")
	backendAddCmd.ValidArgsFunction = backendKindCompletion
	backendCmd.AddCommand(backendAddCmd)
	backendCmd.AddCommand(backendUnaryCommand(ctx, "show <name>", []string{"status"}, "Show backend status", "backend.show", "backend show requires a backend name", "pinax backend show <name> --vault <vault>", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.BackendShow(cmd.Context(), app.BackendRequest{VaultPath: *ctx.vaultPath, Name: name})
	}))
	backendCmd.AddCommand(backendUnaryCommand(ctx, "doctor <name>", nil, "Diagnose backend configuration", "backend.doctor", "backend doctor requires a backend name", "pinax backend doctor <name> --vault <vault>", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.BackendDoctor(cmd.Context(), app.BackendRequest{VaultPath: *ctx.vaultPath, Name: name})
	}))
	backendCmd.AddCommand(backendUnaryCommand(ctx, "capabilities <name>", nil, "Show backend capabilities", "backend.capabilities", "backend capabilities requires a backend name", "pinax backend capabilities <name> --vault <vault>", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.BackendCapabilities(cmd.Context(), app.BackendRequest{VaultPath: *ctx.vaultPath, Name: name})
	}))
	backendCmd.AddCommand(backendUnaryCommand(ctx, "remove <name>", nil, "Remove a backend profile", "backend.remove", "backend remove requires a backend name", "pinax backend remove <name> --vault <vault>", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.RemoveBackend(cmd.Context(), app.BackendRequest{VaultPath: *ctx.vaultPath, Name: name})
	}))
	backendObjectCmd := &cobra.Command{Use: "object", Short: "Browse backend objects"}
	backendObjectListCmd := &cobra.Command{Use: "list <name> [prefix]", Short: "List backend objects", ValidArgsFunction: backendObjectCompletion(func() string { return *ctx.vaultPath }, false, true), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.object.list", "argument_required", "backend object list requires a backend name", "pinax backend object list <name> [prefix] --vault <vault>")
		}
		prefix := ""
		if len(args) == 2 {
			prefix = args[1]
		}
		projection, err := ctx.svc.BackendObjectList(cmd.Context(), app.BackendObjectListRequest{VaultPath: *ctx.vaultPath, Name: args[0], Prefix: prefix})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendObjectCmd.AddCommand(backendObjectListCmd)
	backendObjectStatCmd := &cobra.Command{Use: "stat <name> <key>", Short: "Show backend object status", ValidArgsFunction: backendObjectCompletion(func() string { return *ctx.vaultPath }, false, false), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.object.stat", "argument_required", "backend object stat requires a backend name and key", "pinax backend object stat <name> <key> --vault <vault>")
		}
		projection, err := ctx.svc.BackendObjectStat(cmd.Context(), app.BackendObjectStatRequest{VaultPath: *ctx.vaultPath, Name: args[0], Key: args[1]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendObjectCmd.AddCommand(backendObjectStatCmd)
	backendCmd.AddCommand(backendObjectCmd)
	backendNotesCmd := &cobra.Command{Use: "notes", Short: "Inspect note-like backend objects"}
	backendNotesSummaryCmd := &cobra.Command{Use: "summary [name] [prefix]", Short: "Summarize backend note objects", ValidArgsFunction: backendObjectCompletion(func() string { return *ctx.vaultPath }, true, true), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.notes.summary", "argument_unexpected", "backend notes summary accepts at most a backend name and prefix", "pinax backend notes summary [name] [prefix] --vault <vault>")
		}
		name := ""
		prefix := ""
		if len(args) >= 1 {
			name = args[0]
		}
		if len(args) == 2 {
			prefix = args[1]
		}
		projection, err := ctx.svc.BackendNotesSummary(cmd.Context(), app.BackendNotesRequest{VaultPath: *ctx.vaultPath, Name: name, Prefix: prefix})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendNotesCmd.AddCommand(backendNotesSummaryCmd)
	backendNotesListCmd := &cobra.Command{Use: "list [name] [prefix]", Short: "List backend note objects", ValidArgsFunction: backendObjectCompletion(func() string { return *ctx.vaultPath }, true, true), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.notes.list", "argument_unexpected", "backend notes list accepts at most a backend name and prefix", "pinax backend notes list [name] [prefix] --vault <vault>")
		}
		name := ""
		prefix := ""
		if len(args) >= 1 {
			name = args[0]
		}
		if len(args) == 2 {
			prefix = args[1]
		}
		projection, err := ctx.svc.BackendNotesList(cmd.Context(), app.BackendNotesRequest{VaultPath: *ctx.vaultPath, Name: name, Prefix: prefix})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendNotesCmd.AddCommand(backendNotesListCmd)
	backendNotesStatCmd := &cobra.Command{Use: "stat <name> <path>", Short: "Show backend note object status", ValidArgsFunction: backendObjectCompletion(func() string { return *ctx.vaultPath }, true, false), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.notes.stat", "argument_required", "backend notes stat requires a backend name and path", "pinax backend notes stat <name> <path> --vault <vault>")
		}
		projection, err := ctx.svc.BackendNotesStat(cmd.Context(), app.BackendNotesRequest{VaultPath: *ctx.vaultPath, Name: args[0], Path: args[1]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	backendNotesCmd.AddCommand(backendNotesStatCmd)
	backendCmd.AddCommand(backendNotesCmd)
	root.AddCommand(backendCmd)
}

func backendUnaryCommand(ctx commandBuildContext, use string, aliases []string, short, command, msg, hint string, run func(*cobra.Command, string) (domain.Projection, error)) *cobra.Command {
	return &cobra.Command{Use: use, Aliases: aliases, Short: short, ValidArgsFunction: backendNameCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), command, "argument_required", msg, hint)
		}
		projection, err := run(cmd, args[0])
		return ctx.renderProjection(cmd, projection, err)
	}}
}
