package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addKnowledgeCommands(root *cobra.Command, ctx commandBuildContext) {
	var outputPath string
	var allowlist []string
	var allowlistFile string
	var fromPackage string

	knowledgeCmd := &cobra.Command{
		Use:   "knowledge",
		Short: "Export allowlisted knowledge projections",
		Long:  "Export a provider-neutral, refs-only knowledge projection for Inferrum ingestion. The Pinax vault remains the source of truth; export never writes notes, Inferrum databases, or retrieval indexes.",
		Example: "pinax knowledge export-projection --output ./projection.json --vault ./my-notes --json\n" +
			"pinax knowledge export-projection --output ./projection.json --allowlist notes/public.md --vault ./my-notes --json\n" +
			"pinax knowledge export-projection --output ./projection.json --allowlist-file ./allowlist.txt --from-package ./prior.json --vault ./my-notes --json",
	}

	exportCmd := &cobra.Command{
		Use:   "export-projection",
		Short: "Export an allowlisted knowledge projection package",
		Long:  "Export only notes that match both an explicit path allowlist and an entry allow marker. The default empty allowlist produces an empty package with an explicit reason.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.ExportKnowledgeProjection(cmd.Context(), app.KnowledgeExportProjectionRequest{
				VaultPath:     *ctx.vaultPath,
				Output:        outputPath,
				Allowlist:     append([]string{}, allowlist...),
				AllowlistFile: allowlistFile,
				FromPackage:   fromPackage,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	exportCmd.Flags().StringVar(&outputPath, "output", "", "Projection package JSON path; must be outside the vault")
	exportCmd.Flags().StringArrayVar(&allowlist, "allowlist", nil, "Vault-relative path or directory prefix to allow; repeatable")
	exportCmd.Flags().StringVar(&allowlistFile, "allowlist-file", "", "Optional file of vault-relative allowlist paths, one per line")
	exportCmd.Flags().StringVar(&fromPackage, "from-package", "", "Prior projection package for incremental digest-diff export")
	knowledgeCmd.AddCommand(exportCmd)
	root.AddCommand(knowledgeCmd)
}
