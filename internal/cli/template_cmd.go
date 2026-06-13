package cli

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addTemplateCommands(root *cobra.Command, ctx commandBuildContext) {
	var templatePack string
	var templateUseCase string
	var templateIntent string
	templateCmd := &cobra.Command{Use: "template", Short: "Manage Markdown templates"}
	templateCreateCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a Markdown template",
		Example: `pinax template create "Video Study" --vault ./my-notes
pinax template create meeting --from ./meeting.md --vault ./my-notes
pinax template create daily-review --body "# {{date}}" --vault ./my-notes
pinax template create weekly --engine go-template --body "# {{ .Title }}" --vault ./my-notes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.create", "argument_required", "template create requires a template name", "pinax template create <name> --vault <vault>")
			}
			body := *ctx.templateBody
			if *ctx.templateUseStdin {
				b, readErr := io.ReadAll(cmd.InOrStdin())
				if readErr != nil {
					return renderCommandError(cmd, ctx.outputMode(), "template.create", "stdin_read_failed", readErr.Error(), "Check stdin input and retry")
				}
				body = string(b)
			}
			projection, err := ctx.svc.CreateTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0], SourcePath: *ctx.templateSourcePath, Body: body, UseStdin: *ctx.templateUseStdin, Overwrite: *ctx.templateOverwrite, Engine: *ctx.templateEngine})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	templateCreateCmd.Flags().StringVar(ctx.templateSourcePath, "from", "", "Create the template from a Markdown file")
	templateCreateCmd.Flags().StringVar(ctx.templateBody, "body", "", "Create the template body from a command argument")
	templateCreateCmd.Flags().BoolVar(ctx.templateUseStdin, "stdin", false, "Read the template body from stdin")
	templateCreateCmd.Flags().BoolVar(ctx.templateOverwrite, "overwrite", false, "Overwrite an existing template")
	templateCreateCmd.Flags().StringVar(ctx.templateEngine, "engine", "", "Template engine: simple or go-template")
	_ = templateCreateCmd.RegisterFlagCompletionFunc("engine", staticCompletion("engine", "simple", "go-template"))
	templateCmd.AddCommand(templateCreateCmd)
	templateCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize built-in templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.InitTemplates(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	templateListCmd := &cobra.Command{Use: "list", Short: "List templates", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ListTemplateCatalog(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Pack: templatePack, UseCase: templateUseCase})
		return ctx.renderProjection(cmd, projection, err)
	}}
	templateListCmd.Flags().StringVar(&templatePack, "pack", "", "Filter by template pack, such as starter")
	templateListCmd.Flags().StringVar(&templateUseCase, "use-case", "", "Filter templates by use case")
	templateCmd.AddCommand(templateListCmd)
	templateRecommendCmd := &cobra.Command{Use: "recommend", Short: "Recommend local templates by intent", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecommendTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Intent: templateIntent})
		return ctx.renderProjection(cmd, projection, err)
	}}
	templateRecommendCmd.Flags().StringVar(&templateIntent, "intent", "", "Local recommendation intent, such as meeting")
	templateCmd.AddCommand(templateRecommendCmd)
	templateCmd.AddCommand(&cobra.Command{
		Use:     "show <name>",
		Short:   "Read a template",
		Example: "pinax template show mermaid --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.show", "argument_required", "template show requires a template name", "pinax template show <name> --vault <vault>")
			}
			projection, err := ctx.svc.ShowTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	templateInspectCmd := &cobra.Command{
		Use:     "inspect <name>",
		Short:   "Check template metadata and engine",
		Example: "pinax template inspect meeting --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.inspect", "argument_required", "template inspect requires a template name", "pinax template inspect <name> --vault <vault>")
			}
			projection, err := ctx.svc.InspectTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0], Runs: *ctx.templateRuns})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	templateInspectCmd.ValidArgsFunction = templateNameCompletion(func() string { return *ctx.vaultPath }, "", true, true)
	templateInspectCmd.Flags().BoolVar(ctx.templateRuns, "runs", false, "List template render runs")
	templateCmd.AddCommand(templateInspectCmd)
	templateValidateCmd := &cobra.Command{
		Use:     "validate <name>",
		Short:   "Validate a template",
		Example: "pinax template validate meeting --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.validate", "argument_required", "template validate requires a template name", "pinax template validate <name> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(*ctx.templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "template.validate", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			projection, err := ctx.svc.ValidateTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0], Title: *ctx.title, Project: *ctx.noteProject, Tags: splitCSV(*ctx.noteTags), Vars: vars})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	templateValidateCmd.ValidArgsFunction = templateNameCompletion(func() string { return *ctx.vaultPath }, "", true, true)
	templateValidateCmd.Flags().StringArrayVar(ctx.templateVars, "var", nil, "Template variable in key=value format; repeatable")
	_ = templateValidateCmd.RegisterFlagCompletionFunc("var", templateVarCompletion(func() string { return *ctx.vaultPath }))
	templateValidateCmd.Flags().StringVar(ctx.title, "title", "", "Template title")
	templateValidateCmd.Flags().StringVar(ctx.noteProject, "project", "", "Project slug")
	templateValidateCmd.Flags().StringVar(ctx.noteTags, "tags", "", "Comma-separated tags")
	templateCmd.AddCommand(templateValidateCmd)
	templateDeleteCmd := &cobra.Command{
		Use:     "delete <name>",
		Short:   "Delete a custom template",
		Example: "pinax template delete meeting --vault ./my-notes --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.delete", "argument_required", "template delete requires a template name", "pinax template delete <name> --vault <vault> --yes")
			}
			projection, err := ctx.svc.DeleteTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0], Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	templateDeleteCmd.ValidArgsFunction = templateNameCompletion(func() string { return *ctx.vaultPath }, "", false, true)
	templateDeleteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm template deletion")
	templateCmd.AddCommand(templateDeleteCmd)
	templateRenderCmd := &cobra.Command{
		Use:     "render <name>",
		Short:   "Render a template",
		Example: "pinax template render mermaid --title \"Architecture\" --project research --tags pinax,sync --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.render", "argument_required", "template render requires a template name", "pinax template render <name> --title <title> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(*ctx.templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "template.render", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			projection, err := ctx.svc.RenderTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0], Title: *ctx.title, Project: *ctx.noteProject, Tags: splitCSV(*ctx.noteTags), Vars: vars, SaveRun: *ctx.templateSaveRun, Run: *ctx.templateRun})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	templateRenderCmd.ValidArgsFunction = templateNameCompletion(func() string { return *ctx.vaultPath }, "", true, true)
	templateRenderCmd.Flags().StringVar(ctx.title, "title", "", "Template title")
	templateRenderCmd.Flags().StringVar(ctx.noteProject, "project", "", "Project slug")
	templateRenderCmd.Flags().StringVar(ctx.noteTags, "tags", "", "Comma-separated tags")
	templateRenderCmd.Flags().StringArrayVar(ctx.templateVars, "var", nil, "Template variable in key=value format; repeatable")
	_ = templateRenderCmd.RegisterFlagCompletionFunc("var", templateVarCompletion(func() string { return *ctx.vaultPath }))
	templateRenderCmd.Flags().StringVar(ctx.templateSaveRun, "save-run", "", "Save a render run alias")
	templateRenderCmd.Flags().StringVar(ctx.templateRun, "run", "", "Reuse parameters from a historical render run")
	_ = templateRenderCmd.RegisterFlagCompletionFunc("run", templateRenderRunCompletion(func() string { return *ctx.vaultPath }))
	templateCmd.AddCommand(templateRenderCmd)

	templatePreviewCmd := &cobra.Command{
		Use:     "preview <name>",
		Short:   "Preview template render output",
		Example: "pinax template preview meeting --title \"Client Meeting\" --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "template.preview", "argument_required", "template preview requires a template name", "pinax template preview <name> --title <title> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(*ctx.templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "template.preview", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			projection, err := ctx.svc.PreviewTemplate(cmd.Context(), app.TemplateRequest{VaultPath: *ctx.vaultPath, Name: args[0], Title: *ctx.title, Project: *ctx.noteProject, Tags: splitCSV(*ctx.noteTags), Vars: vars})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	templatePreviewCmd.ValidArgsFunction = templateNameCompletion(func() string { return *ctx.vaultPath }, "", true, true)
	templatePreviewCmd.Flags().StringVar(ctx.title, "title", "", "Template title")
	templatePreviewCmd.Flags().StringVar(ctx.noteProject, "project", "", "Project slug")
	templatePreviewCmd.Flags().StringVar(ctx.noteTags, "tags", "", "Comma-separated tags")
	templatePreviewCmd.Flags().StringArrayVar(ctx.templateVars, "var", nil, "Template variable in key=value format; repeatable")
	_ = templatePreviewCmd.RegisterFlagCompletionFunc("var", templateVarCompletion(func() string { return *ctx.vaultPath }))
	templateCmd.AddCommand(templatePreviewCmd)
	templateRunsCmd := &cobra.Command{Use: "runs", Short: "Maintain template render runs"}
	templateRunsPruneCmd := &cobra.Command{Use: "prune <template>", Short: "Prune template render runs", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "template.runs.prune", "argument_required", "template runs prune requires a template name", "pinax template runs prune <template> --keep 20 --dry-run --vault <vault>")
		}
		projection, err := ctx.svc.PruneTemplateRuns(cmd.Context(), app.RenderRunRequest{VaultPath: *ctx.vaultPath, Template: args[0], Keep: *ctx.renderKeep, DryRun: *ctx.renderDryRun, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	templateRunsPruneCmd.Flags().IntVar(ctx.renderKeep, "keep", 20, "Keep the most recent n runs")
	templateRunsPruneCmd.Flags().BoolVar(ctx.renderDryRun, "dry-run", true, "Preview the deletion plan only")
	templateRunsPruneCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm deleting old runs")
	templateRunsRepairCmd := &cobra.Command{Use: "repair", Short: "Rebuild the render run index", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RepairTemplateRuns(cmd.Context(), app.RenderRunRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	templateRunsCmd.AddCommand(templateRunsPruneCmd, templateRunsRepairCmd)
	templateCmd.AddCommand(templateRunsCmd)
	root.AddCommand(templateCmd)

}
