package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addInboxCommands(root *cobra.Command, ctx commandBuildContext) {
	inboxCmd := &cobra.Command{
		Use:   "inbox",
		Short: "Manage inbox capture and triage workflows",
	}

	// 1. inbox capture <title>
	inboxCaptureCmd := &cobra.Command{
		Use:   "capture <title>",
		Short: "Quickly capture an inbox note",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "inbox.capture", "argument_required", "inbox capture requires a title", "pinax inbox capture <title> --vault <vault>")
			}
			projection, err := ctx.svc.InboxCapture(cmd.Context(), app.CreateNoteRequest{
				VaultPath: *ctx.vaultPath,
				Title:     args[0],
				Tags:      splitCSV(*ctx.noteTags),
				Body:      *ctx.noteBody,
				Slug:      *ctx.noteSlug,
				DryRun:    *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	inboxCaptureCmd.Flags().StringVar(ctx.noteBody, "body", "", "Note body")
	inboxCaptureCmd.Flags().StringVar(ctx.noteTags, "tags", "", "Comma-separated tags")
	inboxCaptureCmd.Flags().StringVar(ctx.noteSlug, "slug", "", "Filename slug")
	inboxCaptureCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the capture without modifying files")
	inboxCaptureCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm remote capture when using --api-url")
	inboxCmd.AddCommand(inboxCaptureCmd)

	// 2. inbox list
	inboxCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List inbox notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.InboxList(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	// 3. inbox triage <note>
	inboxTriageCmd := &cobra.Command{
		Use:               "triage <note>",
		Short:             "Triage an inbox note into a project and folder",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "inbox.triage", "argument_required", "inbox triage requires a note reference", "pinax inbox triage <note> --group <group> --vault <vault>")
			}
			projection, err := ctx.svc.InboxTriage(cmd.Context(), app.InboxTriageRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				Group:     *ctx.noteGroup,
				Folder:    *ctx.noteFolder,
				Kind:      *ctx.noteKind,
				Status:    *ctx.noteStatus,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	inboxTriageCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Target group or project slug")
	inboxTriageCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Target relative folder")
	inboxTriageCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Target kind")
	inboxTriageCmd.Flags().StringVar(ctx.noteStatus, "status", "", "Target status")
	_ = inboxTriageCmd.RegisterFlagCompletionFunc("group", projectSlugCompletion(func() string { return *ctx.vaultPath }))
	_ = inboxTriageCmd.RegisterFlagCompletionFunc("folder", folderPathCompletion(func() string { return *ctx.vaultPath }))
	_ = inboxTriageCmd.RegisterFlagCompletionFunc("kind", staticCompletion("kind", "fleeting", "reference", "project", "daily", "inbox"))
	_ = inboxTriageCmd.RegisterFlagCompletionFunc("status", staticCompletion("status", "active", "draft", "inbox", "archived", "discarded"))
	inboxCmd.AddCommand(inboxTriageCmd)

	// 4. inbox show <note>
	inboxShowCmd := &cobra.Command{
		Use:               "show <note>",
		Short:             "Show the specified inbox note content",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "inbox.show", "argument_required", "inbox show requires a note reference", "pinax inbox show <note> --vault <vault>")
			}
			projection, err := ctx.svc.InboxShow(cmd.Context(), app.ShowNoteRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				View:      *ctx.noteView,
				Display:   *ctx.noteDisplay,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	inboxShowCmd.Flags().StringVar(ctx.noteView, "view", "source", "View mode: source or rendered")
	inboxShowCmd.Flags().StringVar(ctx.noteDisplay, "display", "", "Display style: card, detail, context, or body")
	_ = inboxShowCmd.RegisterFlagCompletionFunc("view", staticCompletion("view", "source", "rendered"))
	_ = inboxShowCmd.RegisterFlagCompletionFunc("display", staticCompletion("display", "card", "detail", "context", "body"))
	inboxCmd.AddCommand(inboxShowCmd)

	// 5. inbox promote <note> --to <draft|active>
	var promoteTo string
	inboxPromoteCmd := &cobra.Command{
		Use:               "promote <note>",
		Short:             "Promote an inbox note to draft or active status",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "inbox.promote", "argument_required", "inbox promote requires a note reference", "pinax inbox promote <note> --vault <vault>")
			}
			projection, err := ctx.svc.InboxPromote(cmd.Context(), app.InboxPromoteRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				To:        promoteTo,
				Group:     *ctx.noteGroup,
				Folder:    *ctx.noteFolder,
				Kind:      *ctx.noteKind,
				Yes:       *ctx.yes,
				DryRun:    *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	inboxPromoteCmd.Flags().StringVar(&promoteTo, "to", "active", "Target status: draft or active")
	inboxPromoteCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Target group or project slug")
	inboxPromoteCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Target relative folder")
	inboxPromoteCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Target kind")
	inboxPromoteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm the status transition")
	inboxPromoteCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the transition plan without modifying files")
	_ = inboxPromoteCmd.RegisterFlagCompletionFunc("to", staticCompletion("status", "draft", "active"))
	_ = inboxPromoteCmd.RegisterFlagCompletionFunc("group", projectSlugCompletion(func() string { return *ctx.vaultPath }))
	_ = inboxPromoteCmd.RegisterFlagCompletionFunc("folder", folderPathCompletion(func() string { return *ctx.vaultPath }))
	_ = inboxPromoteCmd.RegisterFlagCompletionFunc("kind", staticCompletion("kind", "fleeting", "reference", "project", "daily", "inbox"))
	inboxCmd.AddCommand(inboxPromoteCmd)

	// 6. inbox discard <note>
	inboxDiscardCmd := &cobra.Command{
		Use:               "discard <note>",
		Short:             "Discard an inbox note",
		ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "inbox.discard", "argument_required", "inbox discard requires a note reference", "pinax inbox discard <note> --vault <vault>")
			}
			projection, err := ctx.svc.InboxDiscard(cmd.Context(), app.NoteMutationRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   args[0],
				Yes:       *ctx.yes,
				DryRun:    *ctx.noteDryRun,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	inboxDiscardCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm discard")
	inboxDiscardCmd.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the discard plan only")
	inboxCmd.AddCommand(inboxDiscardCmd)

	// 7. inbox index preview|create|refresh
	var indexTemplate string
	inboxIndexCmd := &cobra.Command{
		Use:   "index",
		Short: "Inbox review index page workflow",
	}

	wrapInboxIndexProjection := func(proj domain.Projection) domain.Projection {
		if proj.Facts == nil {
			proj.Facts = map[string]string{}
		}
		proj.Facts["workflow"] = "inbox"
		proj.Facts["page"] = "inbox"
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

	inboxIndexPreviewCmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview the inbox review index page",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PreviewIndexPage(cmd.Context(), app.IndexPageRequest{
				VaultPath: *ctx.vaultPath,
				Name:      "inbox",
				Template:  indexTemplate,
			})
			return ctx.renderProjection(cmd, wrapInboxIndexProjection(projection), err)
		},
	}
	inboxIndexCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create the inbox review index page",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CreateIndexPage(cmd.Context(), app.IndexPageRequest{
				VaultPath: *ctx.vaultPath,
				Name:      "inbox",
				Template:  indexTemplate,
			})
			return ctx.renderProjection(cmd, wrapInboxIndexProjection(projection), err)
		},
	}
	inboxIndexRefreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the inbox review index page managed block",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.RefreshIndexPage(cmd.Context(), app.IndexPageRequest{
				VaultPath: *ctx.vaultPath,
				Name:      "inbox",
				Template:  indexTemplate,
			})
			return ctx.renderProjection(cmd, wrapInboxIndexProjection(projection), err)
		},
	}

	for _, c := range []*cobra.Command{inboxIndexPreviewCmd, inboxIndexCreateCmd, inboxIndexRefreshCmd} {
		c.Flags().StringVar(&indexTemplate, "template", "index.inbox", "Custom inbox index page template")
		_ = c.RegisterFlagCompletionFunc("template", templateNameCompletion(func() string { return *ctx.vaultPath }, "index_template", true, true))
		inboxIndexCmd.AddCommand(c)
	}
	inboxCmd.AddCommand(inboxIndexCmd)

	root.AddCommand(inboxCmd)
}

func addDimensionRootCommands(root *cobra.Command, ctx commandBuildContext) {
	for _, dimension := range []string{"tag", "kind", "group"} {
		dim := dimension
		dimCmd := &cobra.Command{Use: dim, Short: "List " + dim + " organization views"}
		dimCmd.Hidden = true
		dimCmd.AddCommand(&cobra.Command{Use: "list", Short: "List " + dim + " counts", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ListDimension(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath}, dim)
			return ctx.renderProjection(cmd, projection, err)
		}})
		root.AddCommand(dimCmd)
	}
}
