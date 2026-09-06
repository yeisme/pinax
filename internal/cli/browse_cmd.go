package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

// addBrowseCommand 注册只读合成目录导航。browse 永不写 vault 文件或索引，
// --lazy-index 只接受与 search 相同的取值并透传到 facts（off 时同样零写入）。
func addBrowseCommand(root *cobra.Command, ctx commandBuildContext) {
	var lazyIndex string
	browseCmd := &cobra.Command{
		Use:     "browse [path]",
		Short:   "Browse a synthesized read-only directory view",
		Long:    "Browse a synthesized read-only directory view of vault notes with trust and freshness badges. browse never writes index.md or .pinax/index.sqlite.",
		Example: "pinax browse notes/architecture --vault ./my-notes",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			projection, err := ctx.svc.Browse(cmd.Context(), app.BrowseRequest{VaultPath: *ctx.vaultPath, Path: path, LazyIndex: lazyIndex})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	browseCmd.Flags().StringVar(&lazyIndex, "lazy-index", "auto", "Accepted for symmetry with search; browse never writes the index regardless of this value")
	_ = browseCmd.RegisterFlagCompletionFunc("lazy-index", staticCompletion("lazy-index", "auto", "off", "sync"))
	root.AddCommand(browseCmd)
}
