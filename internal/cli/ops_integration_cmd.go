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

func addCloudCommands(root *cobra.Command, ctx commandBuildContext) {
	cloudCmd := &cobra.Command{Use: "cloud", Short: "Manage Pinax cloud sync state"}
	cloudLoginCmd := &cobra.Command{
		Use:     "login",
		Short:   "Configure Pinax cloud backend state",
		Example: "pinax cloud login --endpoint https://cloud.example.test --workspace ws_123 --device laptop --secret-ref op://pinax/cloud-token --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CloudLogin(cmd.Context(), app.CloudLoginRequest{VaultPath: *ctx.vaultPath, Endpoint: *ctx.cloudEndpoint, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice, SecretRef: *ctx.cloudSecretRef, EncryptionSecretRef: *ctx.cloudEncryptionSecretRef})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	cloudLoginCmd.Flags().StringVar(ctx.cloudEndpoint, "endpoint", "", "Pinax cloud backend URL")
	cloudLoginCmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "", "Pinax cloud workspace id")
	cloudLoginCmd.Flags().StringVar(ctx.cloudDevice, "device", "", "Local device id")
	cloudLoginCmd.Flags().StringVar(ctx.cloudSecretRef, "secret-ref", "", "Cloud auth token reference; do not save the raw token")
	cloudLoginCmd.Flags().StringVar(ctx.cloudEncryptionSecretRef, "encryption-secret-ref", "", "Shared encryption secret reference; defaults to --secret-ref for old configs")
	cloudCmd.AddCommand(cloudLoginCmd)
	cloudBackendCmd := &cobra.Command{Use: "backend", Short: "Configure Cloud Sync transport backend"}
	cloudBackendSetCmd := &cobra.Command{Use: "set", Short: "Set Cloud Sync transport backend"}
	cloudBackendSetS3Cmd := &cobra.Command{
		Use:     "s3",
		Short:   "Configure S3-compatible direct Cloud Sync backend",
		Example: "pinax cloud backend set s3 --bucket notes --region us-east-1 --prefix pinax-sync/ --profile work --workspace personal --device laptop --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CloudBackendSetS3(cmd.Context(), app.CloudBackendSetRequest{VaultPath: *ctx.vaultPath, Kind: "s3", Bucket: *ctx.s3Bucket, Region: *ctx.s3Region, Prefix: *ctx.s3Prefix, Endpoint: *ctx.s3Endpoint, Profile: *ctx.s3Profile, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice, SecretRef: *ctx.cloudSecretRef})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.s3Bucket, "bucket", "", "S3 bucket name")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.s3Region, "region", "", "S3 region")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.s3Prefix, "prefix", "", "S3 object key prefix")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.s3Endpoint, "endpoint", "", "S3-compatible endpoint URL")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.s3Profile, "profile", "", "S3 credential profile name; do not save the secret")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "", "Cloud workspace id")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.cloudDevice, "device", "", "Local device id")
	cloudBackendSetS3Cmd.Flags().StringVar(ctx.cloudSecretRef, "secret-ref", "", "Secret manager reference; do not save the raw secret")
	cloudBackendSetRcloneCmd := &cobra.Command{
		Use:     "rclone",
		Short:   "Configure rclone direct Cloud Sync backend",
		Example: "pinax cloud backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CloudBackendSetRclone(cmd.Context(), app.CloudBackendSetRequest{VaultPath: *ctx.vaultPath, Kind: "rclone", Remote: *ctx.backendRemote, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	cloudBackendSetRcloneCmd.Flags().StringVar(ctx.backendRemote, "remote", "", "Rclone remote and path, for example onedrive:PinaxSync")
	cloudBackendSetRcloneCmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "", "Cloud workspace id")
	cloudBackendSetRcloneCmd.Flags().StringVar(ctx.cloudDevice, "device", "", "Local device id")
	cloudBackendSetCmd.AddCommand(cloudBackendSetRcloneCmd)
	cloudBackendSetCmd.AddCommand(cloudBackendSetS3Cmd)
	cloudBackendCmd.AddCommand(cloudBackendSetCmd)
	cloudCmd.AddCommand(cloudBackendCmd)
	cloudCmd.AddCommand(&cobra.Command{Use: "status", Short: "Show Pinax cloud state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CloudStatus(cmd.Context(), app.CloudRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	cloudCmd.AddCommand(&cobra.Command{Use: "logout", Short: "Log out the local cloud device session", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CloudLogout(cmd.Context(), app.CloudRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	cloudCmd.AddCommand(&cobra.Command{Use: "doctor", Short: "Diagnose Pinax cloud state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CloudDoctor(cmd.Context(), app.CloudRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	root.AddCommand(cloudCmd)
}

func addPlanningCommands(root *cobra.Command, ctx commandBuildContext) {
	planCmd := &cobra.Command{Use: "plan", Short: "Manage personal planning workflows"}
	planDailyCmd := &cobra.Command{Use: "daily", Short: "Generate a daily plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanDaily(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, WithTaskBridge: *ctx.planWithTaskBridge, DryRun: *ctx.planDryRun, Yes: *ctx.yes, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planDailyCmd.Flags().BoolVar(ctx.planWithTaskBridge, "taskbridge", false, "Read task facts from TaskBridge")
	planDailyCmd.Flags().BoolVar(ctx.planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planDailyCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save a plan snapshot")
	planDailyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planDailyCmd)
	planWeeklyCmd := &cobra.Command{Use: "weekly", Short: "Generate a weekly plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanWeekly(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, WithTaskBridge: *ctx.planWithTaskBridge, DryRun: *ctx.planDryRun, Yes: *ctx.yes, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planWeeklyCmd.Flags().BoolVar(ctx.planWithTaskBridge, "taskbridge", false, "Read task facts from TaskBridge")
	planWeeklyCmd.Flags().BoolVar(ctx.planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planWeeklyCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save a plan snapshot")
	planWeeklyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planWeeklyCmd)
	planMonthlyCmd := &cobra.Command{Use: "monthly", Short: "Generate a monthly plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PlanMonthly(cmd.Context(), app.PlanningRequest{VaultPath: *ctx.vaultPath, WithTaskBridge: *ctx.planWithTaskBridge, DryRun: *ctx.planDryRun, Yes: *ctx.yes, Save: *ctx.planSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	planMonthlyCmd.Flags().BoolVar(ctx.planWithTaskBridge, "taskbridge", false, "Read task facts from TaskBridge")
	planMonthlyCmd.Flags().BoolVar(ctx.planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planMonthlyCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save a plan snapshot")
	planMonthlyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planMonthlyCmd)
	planActionsCmd := &cobra.Command{Use: "actions", Short: "Generate TaskBridge action drafts", RunE: func(cmd *cobra.Command, args []string) error {
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
	backendCmd.AddCommand(backendUnaryCommand(ctx, "diff <name>", nil, "Generate a backend dry-run sync plan", "backend.diff", "backend diff requires a backend name", "pinax backend diff <name> --vault <vault>", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.BackendDiff(cmd.Context(), app.BackendPlanRequest{VaultPath: *ctx.vaultPath, Name: name})
	}))
	backendPushCmd := backendPlanCommand(ctx, "push <name>", "Run backend push sync", "backend.push", "backend push requires a backend name", "pinax backend push <name> --vault <vault> --dry-run", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.BackendPush(cmd.Context(), app.BackendPlanRequest{VaultPath: *ctx.vaultPath, Name: name, DryRun: *ctx.backendDryRun, Yes: *ctx.yes})
	})
	backendCmd.AddCommand(backendPushCmd)
	backendPullCmd := backendPlanCommand(ctx, "pull <name>", "Run backend pull sync", "backend.pull", "backend pull requires a backend name", "pinax backend pull <name> --vault <vault> --dry-run", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.BackendPull(cmd.Context(), app.BackendPlanRequest{VaultPath: *ctx.vaultPath, Name: name, DryRun: *ctx.backendDryRun, Yes: *ctx.yes})
	})
	backendCmd.AddCommand(backendPullCmd)
	backendCmd.AddCommand(backendUnaryCommand(ctx, "remove <name>", nil, "Remove a backend profile", "backend.remove", "backend remove requires a backend name", "pinax backend remove <name> --vault <vault>", func(cmd *cobra.Command, name string) (domain.Projection, error) {
		return ctx.svc.RemoveBackend(cmd.Context(), app.BackendRequest{VaultPath: *ctx.vaultPath, Name: name})
	}))
	backendObjectCmd := &cobra.Command{Use: "object", Short: "Browse backend objects"}
	backendObjectCmd.AddCommand(&cobra.Command{Use: "list <name> [prefix]", Short: "List backend objects", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.object.list", "argument_required", "backend object list requires a backend name", "pinax backend object list <name> [prefix] --vault <vault>")
		}
		prefix := ""
		if len(args) == 2 {
			prefix = args[1]
		}
		projection, err := ctx.svc.BackendObjectList(cmd.Context(), app.BackendObjectListRequest{VaultPath: *ctx.vaultPath, Name: args[0], Prefix: prefix})
		return ctx.renderProjection(cmd, projection, err)
	}})
	backendObjectCmd.AddCommand(&cobra.Command{Use: "stat <name> <key>", Short: "Show backend object status", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "backend.object.stat", "argument_required", "backend object stat requires a backend name and key", "pinax backend object stat <name> <key> --vault <vault>")
		}
		projection, err := ctx.svc.BackendObjectStat(cmd.Context(), app.BackendObjectStatRequest{VaultPath: *ctx.vaultPath, Name: args[0], Key: args[1]})
		return ctx.renderProjection(cmd, projection, err)
	}})
	backendCmd.AddCommand(backendObjectCmd)
	root.AddCommand(backendCmd)
}

func backendUnaryCommand(ctx commandBuildContext, use string, aliases []string, short, command, msg, hint string, run func(*cobra.Command, string) (domain.Projection, error)) *cobra.Command {
	return &cobra.Command{Use: use, Aliases: aliases, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), command, "argument_required", msg, hint)
		}
		projection, err := run(cmd, args[0])
		return ctx.renderProjection(cmd, projection, err)
	}}
}

func backendPlanCommand(ctx commandBuildContext, use, short, command, msg, hint string, run func(*cobra.Command, string) (domain.Projection, error)) *cobra.Command {
	cmd := backendUnaryCommand(ctx, use, nil, short, command, msg, hint, run)
	cmd.Flags().BoolVar(ctx.backendDryRun, "dry-run", false, "Preview the plan only; do not write")
	cmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm writes")
	return cmd
}
