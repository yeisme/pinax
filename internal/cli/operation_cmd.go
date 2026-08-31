package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addOperationCommands(root *cobra.Command, ctx commandBuildContext) {
	operationCmd := &cobra.Command{Use: "operation", Short: "Inspect and reconcile remote mutation operations"}
	showCmd := &cobra.Command{
		Use:   "show <operation-id>",
		Short: "Show one operation without replaying its mutation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.OperationShow(cmd.Context(), app.OperationRequest{
				VaultPath: *ctx.vaultPath, OperationID: args[0], Access: app.OperationAccess{OwnerLocal: true},
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	reconcileCmd := &cobra.Command{
		Use:   "reconcile <operation-id>",
		Short: "Reconcile one operation from owner evidence without replaying it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.OperationReconcile(cmd.Context(), app.OperationRequest{
				VaultPath: *ctx.vaultPath, OperationID: args[0], Access: app.OperationAccess{OwnerLocal: true},
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	operationCmd.AddCommand(showCmd, reconcileCmd)
	root.AddCommand(operationCmd)
	registerRemoteCommand(remoteCommandSpec{CommandPath: "operation show", Method: "Pinax.Operation.Get", ArgParams: []string{"operation_id"}})
	registerRemoteCommand(remoteCommandSpec{CommandPath: "operation reconcile", Method: "Pinax.Operation.Reconcile", ArgParams: []string{"operation_id"}})
}
