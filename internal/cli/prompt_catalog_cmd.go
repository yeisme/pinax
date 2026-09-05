package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

// Federated prompt repository and catalog commands. These are additive under
// the existing `pinax prompt` group: local vault commands (search/show/
// resolve) keep their semantics, and external discovery goes through
// `prompt repository` and `prompt catalog`.

func addPromptRepositoryCommands(promptCmd *cobra.Command, ctx commandBuildContext) {
	var source, revision, channel, trust, locale, credentialRef string
	var syncAll bool

	repositoryCmd := &cobra.Command{
		Use:   "repository",
		Short: "Manage shared federated prompt repositories",
		Long:  "Manage user-level prompt repository profiles in the shared promptrepo store. Profiles are shared across CLIs for the same OS user; Pinax never copies repository credentials into the vault.",
		Example: "pinax prompt repository add team --source file:///path/to/catalog --trust verified --json\n" +
			"pinax prompt repository sync --all --json\n" +
			"pinax prompt repository doctor team --json",
	}

	addCmd := &cobra.Command{
		Use:   "add <id> --source <uri>",
		Short: "Register a shared prompt repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := app.PromptRepositoryRequest{ID: args[0], Source: source, Revision: revision, Channel: channel, Trust: trust, Locale: locale, CredentialRef: credentialRef}
			projection, err := ctx.svc.PromptRepositoryAdd(cmd.Context(), req)
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	addCmd.Flags().StringVar(&source, "source", "", "Repository source URI (file://, git://, https://, s3://)")
	addCmd.Flags().StringVar(&revision, "revision", "", "Pinned revision for git sources")
	addCmd.Flags().StringVar(&channel, "channel", "", "Release channel to track")
	addCmd.Flags().StringVar(&trust, "trust", "", "Trust level label for the repository")
	addCmd.Flags().StringVar(&locale, "locale", "", "Preferred locale for the repository")
	addCmd.Flags().StringVar(&credentialRef, "credential-ref", "", "Reference to a credential held by the secret store (never a value)")
	_ = addCmd.MarkFlagRequired("source")
	repositoryCmd.AddCommand(addCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List shared prompt repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositoryList(cmd.Context(), app.PromptRepositoryRequest{})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repositoryCmd.AddCommand(listCmd)

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a shared prompt repository profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositoryShow(cmd.Context(), app.PromptRepositoryRequest{ID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repositoryCmd.AddCommand(showCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor <id>",
		Short: "Report prompt repository health",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositoryDoctor(cmd.Context(), app.PromptRepositoryRequest{ID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repositoryCmd.AddCommand(doctorCmd)

	removeCmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a shared prompt repository profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositoryRemove(cmd.Context(), app.PromptRepositoryRequest{ID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repositoryCmd.AddCommand(removeCmd)

	enableCmd := &cobra.Command{
		Use:   "enable <id>",
		Short: "Enable a shared prompt repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositoryEnable(cmd.Context(), app.PromptRepositoryRequest{ID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repositoryCmd.AddCommand(enableCmd)

	disableCmd := &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a shared prompt repository without removing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositoryDisable(cmd.Context(), app.PromptRepositoryRequest{ID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repositoryCmd.AddCommand(disableCmd)

	syncCmd := &cobra.Command{
		Use:   "sync [<id>...] --all",
		Short: "Synchronize shared prompt repository snapshots",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptRepositorySync(cmd.Context(), app.PromptRepositoryRequest{Repositories: args, SyncAll: syncAll})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "Synchronize every registered repository")
	repositoryCmd.AddCommand(syncCmd)

	promptCmd.AddCommand(repositoryCmd)
}

func addPromptCatalogCommands(promptCmd *cobra.Command, ctx commandBuildContext) {
	var locale, role, selector, valuesFile string
	var tags []string
	var repositories, deny []string
	var confirm, fork bool

	catalogCmd := &cobra.Command{
		Use:   "catalog",
		Short: "Discover and verify prompts in federated repositories",
		Long:  "Search, resolve, inspect, validate, preview, and install prompts from federated repositories through the public promptrepo contracts. `pinax prompt search` stays local-vault-only; this group is the external catalog surface. Inspect, validate, and preview make zero provider calls and zero durable writes, never expose template bodies, and install only rights-permitted templates as local drafts.",
		Example: "pinax prompt catalog search \"meeting summary\" --json\n" +
			"pinax prompt catalog inspect promptrepo://official/audio/podcast@1.0.0?locale=en --json\n" +
			"pinax prompt catalog install promptrepo://official/audio/podcast@1.0.0 --yes --vault ./my-notes --json",
	}

	catalogRequest := func(ref string) app.PromptCatalogRequest {
		return app.PromptCatalogRequest{VaultPath: *ctx.vaultPath, Ref: ref, Locale: locale, Role: role, Selector: selector, ValuesFile: valuesFile, Tags: tags, RepositoryIDs: repositories, DenyIDs: deny, Confirm: confirm, Fork: fork}
	}

	searchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search the federated prompt catalog",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			req := catalogRequest("")
			req.Query = query
			projection, err := ctx.svc.PromptCatalogSearch(cmd.Context(), req)
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	searchCmd.Flags().StringSliceVar(&tags, "tag", nil, "Filter by tags (comma-separated)")
	searchCmd.Flags().StringSliceVar(&repositories, "repository", nil, "Restrict this command to these repositories (session scope)")
	searchCmd.Flags().StringSliceVar(&deny, "deny", nil, "Exclude these repositories for this command (deny wins)")
	catalogCmd.AddCommand(searchCmd)

	showCmd := &cobra.Command{
		Use:   "show <ref>",
		Short: "Show a federated catalog solution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCatalogShow(cmd.Context(), catalogRequest(args[0]))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	catalogCmd.AddCommand(showCmd)

	resolveCmd := &cobra.Command{
		Use:   "resolve <ref>",
		Short: "Resolve an exact federated catalog ref or template address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCatalogResolve(cmd.Context(), catalogRequest(args[0]))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	catalogCmd.AddCommand(resolveCmd)

	inspectCmd := &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Inspect template inputs and provenance without provider calls",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCatalogInspect(cmd.Context(), catalogRequest(args[0]))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	catalogCmd.AddCommand(inspectCmd)

	validateCmd := &cobra.Command{
		Use:   "validate <ref> --values <file>",
		Short: "Validate input values against the template contract",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCatalogValidate(cmd.Context(), catalogRequest(args[0]))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	catalogCmd.AddCommand(validateCmd)

	previewCmd := &cobra.Command{
		Use:   "preview <ref> --values <file>",
		Short: "Preview rendering in memory without provider calls",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCatalogPreview(cmd.Context(), catalogRequest(args[0]))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	catalogCmd.AddCommand(previewCmd)

	installCmd := &cobra.Command{
		Use:   "install <ref> --yes",
		Short: "Install a rights-permitted template as a local draft prompt asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCatalogInstall(cmd.Context(), catalogRequest(args[0]))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	catalogCmd.AddCommand(installCmd)

	for _, cmd := range []*cobra.Command{showCmd, resolveCmd, inspectCmd, validateCmd, previewCmd, installCmd} {
		cmd.Flags().StringSliceVar(&repositories, "repository", nil, "Restrict this command to these repositories (session scope)")
		cmd.Flags().StringSliceVar(&deny, "deny", nil, "Exclude these repositories for this command (deny wins)")
	}
	for _, cmd := range []*cobra.Command{showCmd, resolveCmd, inspectCmd, validateCmd, previewCmd, installCmd} {
		cmd.Flags().StringVar(&locale, "locale", "", "Locale for this command (default en)")
	}
	for _, cmd := range []*cobra.Command{inspectCmd, validateCmd, previewCmd, installCmd} {
		cmd.Flags().StringVar(&role, "role", "", "Template role (defaults to main)")
	}
	inspectCmd.Flags().StringVar(&selector, "selector", "", "Inspect-only selector (heading:, json-pointer:, yaml-pointer:)")
	for _, cmd := range []*cobra.Command{validateCmd, previewCmd} {
		cmd.Flags().StringVar(&valuesFile, "values", "", "JSON file of input values (values never appear in output)")
	}
	installCmd.Flags().BoolVar(&confirm, "yes", false, "Apply the install plan (explicit local durable write)")
	installCmd.Flags().BoolVar(&fork, "fork", false, "Resolve a local ID conflict by installing side-by-side")

	promptCmd.AddCommand(catalogCmd)
}
