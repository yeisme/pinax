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
	root.AddCommand(storageCmd)

}
