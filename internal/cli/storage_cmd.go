package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addStorageCommands(root *cobra.Command, ctx commandBuildContext) {
	storageCmd := &cobra.Command{Use: "storage", Short: "Configure the vault storage backend"}
	storageSetCmd := &cobra.Command{Use: "set", Short: "Configure storage backend"}
	storageSetLocalPrimaryCmd := &cobra.Command{Use: "local", Short: "Configure a local storage backend", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SetLocalStorage(cmd.Context(), app.StorageRequest{VaultPath: *ctx.vaultPath, Root: *ctx.storageRoot})
		return ctx.renderProjection(cmd, projection, err)
	}}
	storageSetLocalPrimaryCmd.Flags().StringVar(ctx.storageRoot, "root", "", "Local storage root directory")
	storageSetS3PrimaryCmd := &cobra.Command{Use: "s3", Short: "Configure an S3 storage backend", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SetS3Storage(cmd.Context(), app.StorageRequest{VaultPath: *ctx.vaultPath, Bucket: *ctx.s3Bucket, Region: *ctx.s3Region, Prefix: *ctx.s3Prefix, Endpoint: *ctx.s3Endpoint, Profile: *ctx.s3Profile})
		return ctx.renderProjection(cmd, projection, err)
	}}
	storageSetS3PrimaryCmd.Flags().StringVar(ctx.s3Bucket, "bucket", "", "S3 bucket name")
	storageSetS3PrimaryCmd.Flags().StringVar(ctx.s3Region, "region", "", "S3 region")
	storageSetS3PrimaryCmd.Flags().StringVar(ctx.s3Prefix, "prefix", "", "S3 object key prefix")
	storageSetS3PrimaryCmd.Flags().StringVar(ctx.s3Endpoint, "endpoint", "", "S3-compatible endpoint URL")
	storageSetS3PrimaryCmd.Flags().StringVar(ctx.s3Profile, "profile", "", "S3 credential profile name; do not save the secret")
	storageSetCmd.AddCommand(storageSetLocalPrimaryCmd, storageSetS3PrimaryCmd)
	storageCmd.AddCommand(storageSetCmd)
	storageCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show storage backend status",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.StorageStatus(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	storageCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Diagnose storage backend configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.StorageDoctor(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	// DriveBridge attach 组是加法命令：零拷贝挂接已有 local/S3、显式网盘
	// 工作副本 opt-in 与换机 hydrate；不替代 storage set local|s3。
	var attachSpace string
	var attachRemote string
	attachDrivebridgeCmd := &cobra.Command{
		Use:   "attach-drivebridge",
		Short: "Attach DriveBridge to the existing storage location (zero copy)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AttachDrivebridge(cmd.Context(), app.DrivebridgeAttachRequest{VaultPath: *ctx.vaultPath, Space: attachSpace, Remote: attachRemote})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	attachDrivebridgeCmd.Flags().StringVar(&attachSpace, "space", "", "DriveBridge space identifier")
	attachDrivebridgeCmd.Flags().StringVar(&attachRemote, "remote", "", "Existing rclone S3 remote for kind=s3 adopt (default: derived from the bucket)")
	_ = attachDrivebridgeCmd.MarkFlagRequired("space")
	storageCmd.AddCommand(attachDrivebridgeCmd)
	storageCmd.AddCommand(&cobra.Command{
		Use:   "detach-drivebridge",
		Short: "Remove the DriveBridge attach record; storage profile and notes stay untouched",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.DetachDrivebridge(cmd.Context(), app.DrivebridgeDetachRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	var bindProvider string
	var bindSpace string
	bindWorkingCopyCmd := &cobra.Command{
		Use:   "bind-working-copy",
		Short: "Opt in to a plaintext OneDrive/Google Drive working copy",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.BindWorkingCopy(cmd.Context(), app.DrivebridgeBindWorkingCopyRequest{VaultPath: *ctx.vaultPath, Provider: bindProvider, Space: bindSpace})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	bindWorkingCopyCmd.Flags().StringVar(&bindProvider, "provider", "", "Provider kind: onedrive or gdrive")
	bindWorkingCopyCmd.Flags().StringVar(&bindSpace, "space", "", "DriveBridge space identifier")
	_ = bindWorkingCopyCmd.MarkFlagRequired("provider")
	_ = bindWorkingCopyCmd.MarkFlagRequired("space")
	_ = bindWorkingCopyCmd.RegisterFlagCompletionFunc("provider", staticCompletion("provider", "onedrive", "gdrive"))
	storageCmd.AddCommand(bindWorkingCopyCmd)
	var hydrateSpace string
	hydrateCmd := &cobra.Command{
		Use:   "hydrate",
		Short: "Rebuild the working copy from the attached DriveBridge space on this device",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.StorageHydrate(cmd.Context(), app.DrivebridgeHydrateRequest{VaultPath: *ctx.vaultPath, Space: hydrateSpace})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	hydrateCmd.Flags().StringVar(&hydrateSpace, "space", "", "DriveBridge space identifier (default: attached space)")
	storageCmd.AddCommand(hydrateCmd)
	root.AddCommand(storageCmd)

}
