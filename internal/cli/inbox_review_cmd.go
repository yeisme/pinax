package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addInboxReviewCommand(parent *cobra.Command, ctx commandBuildContext) {
	var limit int
	var after string
	cmd := &cobra.Command{
		Use: "review", Short: "Preview a small read-only batch of inbox and unclassified index notes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.InboxReview(cmd.Context(), app.InboxReviewRequest{
				VaultPath: *ctx.vaultPath, Limit: limit, After: after,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "Batch size, from 1 to 20")
	cmd.Flags().StringVar(&after, "after", "", "Continue after a note reference from the current pending list; omit after changes")
	parent.AddCommand(cmd)
}
