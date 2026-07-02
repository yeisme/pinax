package cli

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addDraftCommands(root *cobra.Command, ctx commandBuildContext) {
	draftCmd := &cobra.Command{
		Use:   "draft",
		Short: "Manage and review draft inbox workflows",
	}

	// 1. draft create <title>
	draftCreateCmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a draft note",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "draft.create", "argument_required", "draft create requires a title", "pinax draft create <title> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(*ctx.templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "draft.create", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			stdinBody := ""
			if *ctx.noteUseStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return renderCommandError(cmd, ctx.outputMode(), "draft.create", "stdin_read_failed", err.Error(), "Check stdin input and retry")
				}
				stdinBody = string(b)
			}
			projection, err := ctx.svc.DraftCreate(cmd.Context(), app.CreateNoteRequest{
				VaultPath:  *ctx.vaultPath,
				Title:      args[0],
				Project:    *ctx.noteProject,
				Folder:     *ctx.noteFolder,
				Kind:       *ctx.noteKind,
				Tags:       splitCSV(*ctx.noteTags),
				Template:   *ctx.noteTemplate,
				Vars:       vars,
				Body:       *ctx.noteBody,
				SourcePath: *ctx.noteFrom,
				StdinBody:  stdinBody,
				Dir:        *ctx.noteDir,
				Slug:       *ctx.noteSlug,
				DryRun:     *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	draftCreateCmd.Flags().StringVar(ctx.noteBody, "body", "", "Note body")
	draftCreateCmd.Flags().StringVar(ctx.noteTags, "tags", "", "Comma-separated tags")
	draftCreateCmd.Flags().StringVar(ctx.noteSlug, "slug", "", "Filename slug")
	draftCreateCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Relative folder path")
	draftCreateCmd.Flags().StringVar(ctx.noteKind, "kind", "", "kind")
	draftCreateCmd.Flags().StringVar(ctx.noteTemplate, "template", "", "Creation template")
	draftCreateCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the draft creation without modifying files")
	draftCreateCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm remote draft creation when using --api-url")
	_ = draftCreateCmd.RegisterFlagCompletionFunc("folder", folderPathCompletion(func() string { return *ctx.vaultPath }))
	_ = draftCreateCmd.RegisterFlagCompletionFunc("kind", staticCompletion("kind", "fleeting", "reference", "project", "daily", "draft"))
	_ = draftCreateCmd.RegisterFlagCompletionFunc("template", templateNameCompletion(func() string { return *ctx.vaultPath }, "note_template", true, true))
	draftCmd.AddCommand(draftCreateCmd)

	// 2. draft list
	draftListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all draft notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.DraftList(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	draftCmd.AddCommand(draftListCmd)

	// 3. draft show <note>
	draftShowCmd := &cobra.Command{
		Use:               "show <note>",
		Short:             "Show the specified draft note content",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "draft.show", "argument_required", "draft show requires a note reference", "pinax draft show <note> --vault <vault>")
			}
			projection, err := ctx.svc.DraftShow(cmd.Context(), app.ShowNoteRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				View:      *ctx.noteView,
				Display:   *ctx.noteDisplay,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	draftShowCmd.Flags().StringVar(ctx.noteView, "view", "source", "View mode: source or rendered")
	draftShowCmd.Flags().StringVar(ctx.noteDisplay, "display", "", "Display style: card, detail, context, or body")
	_ = draftShowCmd.RegisterFlagCompletionFunc("view", staticCompletion("view", "source", "rendered"))
	_ = draftShowCmd.RegisterFlagCompletionFunc("display", staticCompletion("display", "card", "detail", "context", "body"))
	draftCmd.AddCommand(draftShowCmd)

	// 4. draft promote <note> --status active --folder folder --kind kind
	var promoteStatus string
	draftPromoteCmd := &cobra.Command{
		Use:               "promote <note>",
		Short:             "Promote a draft note to active, archived, or discarded",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "draft.promote", "argument_required", "draft promote requires a note reference", "pinax draft promote <note> --vault <vault>")
			}
			projection, err := ctx.svc.DraftPromote(cmd.Context(), app.DraftPromoteRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				Status:    promoteStatus,
				Folder:    *ctx.noteFolder,
				Kind:      *ctx.noteKind,
				Yes:       *ctx.yes,
				DryRun:    *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	draftPromoteCmd.Flags().StringVar(&promoteStatus, "status", "active", "Target status: active, archived, or discarded")
	draftPromoteCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Target relative folder")
	draftPromoteCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Target kind")
	draftPromoteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm the status transition")
	draftPromoteCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the status transition without modifying files")
	_ = draftPromoteCmd.RegisterFlagCompletionFunc("status", staticCompletion("status", "active", "archived", "discarded"))
	_ = draftPromoteCmd.RegisterFlagCompletionFunc("folder", folderPathCompletion(func() string { return *ctx.vaultPath }))
	_ = draftPromoteCmd.RegisterFlagCompletionFunc("kind", staticCompletion("kind", "fleeting", "reference", "project", "daily", "draft"))
	draftCmd.AddCommand(draftPromoteCmd)

	// 5. draft archive <note>
	draftArchiveCmd := &cobra.Command{
		Use:               "archive <note>",
		Short:             "Archive a draft",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "draft.archive", "argument_required", "draft archive requires a note reference", "pinax draft archive <note> --vault <vault>")
			}
			projection, err := ctx.svc.DraftArchive(cmd.Context(), app.NoteMutationRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				Yes:       *ctx.yes,
				DryRun:    *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	draftArchiveCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm archive")
	draftArchiveCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the archive plan only")
	draftCmd.AddCommand(draftArchiveCmd)

	// 6. draft discard <note>
	draftDiscardCmd := &cobra.Command{
		Use:               "discard <note>",
		Short:             "Discard a draft",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "draft.discard", "argument_required", "draft discard requires a note reference", "pinax draft discard <note> --vault <vault>")
			}
			projection, err := ctx.svc.DraftDiscard(cmd.Context(), app.NoteMutationRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				Yes:       *ctx.yes,
				DryRun:    *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	draftDiscardCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm discard")
	draftDiscardCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the discard plan only")
	draftCmd.AddCommand(draftDiscardCmd)

	// 7. draft index preview|create|refresh
	var indexTemplate string
	draftIndexCmd := &cobra.Command{
		Use:   "index",
		Short: "Draft review index page workflow",
	}

	wrapDraftIndexProjection := func(proj domain.Projection) domain.Projection {
		if proj.Facts == nil {
			proj.Facts = map[string]string{}
		}
		proj.Facts["workflow"] = "draft"
		proj.Facts["page"] = "drafts"
		proj.Facts["template"] = indexTemplate
		if proj.Facts["writes"] == "" {
			if proj.Command == "index.page.preview" {
				proj.Facts["writes"] = "false"
			} else {
				proj.Facts["writes"] = "true"
			}
		}
		return proj
	}

	draftIndexPreviewCmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview the draft review index page",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PreviewIndexPage(cmd.Context(), app.IndexPageRequest{
				VaultPath: *ctx.vaultPath,
				Name:      "drafts",
				Template:  indexTemplate,
			})
			return ctx.renderProjection(cmd, wrapDraftIndexProjection(projection), err)
		},
	}
	draftIndexCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create the draft review index page",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CreateIndexPage(cmd.Context(), app.IndexPageRequest{
				VaultPath: *ctx.vaultPath,
				Name:      "drafts",
				Template:  indexTemplate,
			})
			return ctx.renderProjection(cmd, wrapDraftIndexProjection(projection), err)
		},
	}
	draftIndexRefreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the draft review index page managed block",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.RefreshIndexPage(cmd.Context(), app.IndexPageRequest{
				VaultPath: *ctx.vaultPath,
				Name:      "drafts",
				Template:  indexTemplate,
			})
			return ctx.renderProjection(cmd, wrapDraftIndexProjection(projection), err)
		},
	}

	for _, c := range []*cobra.Command{draftIndexPreviewCmd, draftIndexCreateCmd, draftIndexRefreshCmd} {
		c.Flags().StringVar(&indexTemplate, "template", "index.drafts", "Custom review index page template")
		_ = c.RegisterFlagCompletionFunc("template", templateNameCompletion(func() string { return *ctx.vaultPath }, "index_template", true, true))
		draftIndexCmd.AddCommand(c)
	}
	draftCmd.AddCommand(draftIndexCmd)

	root.AddCommand(draftCmd)
}
