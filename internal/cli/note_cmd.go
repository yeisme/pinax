package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addNoteCommands(root *cobra.Command, ctx commandBuildContext) {
	noteCmd := &cobra.Command{Use: "note", Short: "Manage local Markdown notes"}
	noteCreateRun := func(commandName string) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "note.new", "argument_required", commandName+" requires a title", "pinax note "+commandName+" <title> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(*ctx.templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "note.new", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			stdinBody := ""
			if *ctx.noteUseStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return renderCommandError(cmd, ctx.outputMode(), "note.new", "stdin_read_failed", err.Error(), "Check stdin input and retry")
				}
				stdinBody = string(b)
			}
			project := *ctx.noteProject
			if project == "" {
				project = *ctx.noteGroup
			}
			projection, err := ctx.svc.CreateNote(cmd.Context(), app.CreateNoteRequest{VaultPath: *ctx.vaultPath, Title: args[0], Project: project, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Tags: splitCSV(*ctx.noteTags), Template: *ctx.noteTemplate, Vars: vars, Body: *ctx.noteBody, SourcePath: *ctx.noteFrom, StdinBody: stdinBody, Dir: *ctx.noteDir, Slug: *ctx.noteSlug, Status: *ctx.noteStatus, DryRun: *ctx.noteDryRun})
			if err != nil {
				return ctx.renderProjection(cmd, projection, err)
			}
			if *ctx.noteOpen && !*ctx.noteDryRun {
				if path := projection.Facts["path"]; path != "" {
					editProjection, editErr := ctx.svc.EditNote(cmd.Context(), app.NoteEditRequest{VaultPath: *ctx.vaultPath, NoteRef: path, Editor: *ctx.noteEditor})
					if editErr != nil {
						return ctx.renderProjection(cmd, editProjection, editErr)
					}
					projection.Facts["opened"] = "true"
					for _, key := range []string{"editor", "editor_executable", "editor_args"} {
						if value := editProjection.Facts[key]; value != "" {
							projection.Facts[key] = value
						}
					}
				}
			}
			return ctx.renderProjection(cmd, projection, nil)
		}
	}
	addCreateFlags := func(c *cobra.Command) {
		c.Flags().StringVar(ctx.noteProject, "project", "", "Project slug")
		c.Flags().StringVar(ctx.noteGroup, "group", "", "Group slug; equivalent to project when --project is unset")
		c.Flags().StringVar(ctx.noteFolder, "folder", "", "Relative folder under the project or notes")
		c.Flags().StringVar(ctx.noteKind, "kind", "", "Note kind, such as fleeting, reference, project, or daily")
		c.Flags().StringVar(ctx.noteTags, "tags", "", "Comma-separated tags")
		c.Flags().StringVar(ctx.noteTemplate, "template", "", "Template name")
		_ = c.RegisterFlagCompletionFunc("template", templateNameCompletion(func() string { return *ctx.vaultPath }, "note_template", true, true))
		c.Flags().StringArrayVar(ctx.templateVars, "var", nil, "Template variable in key=value format; repeatable")
		c.Flags().StringVar(ctx.noteBody, "body", "", "Note body")
		c.Flags().StringVar(ctx.noteFrom, "from", "", "Read the body from a Markdown file")
		c.Flags().BoolVar(ctx.noteUseStdin, "stdin", false, "Read the body from stdin")
		c.Flags().StringVar(ctx.noteDir, "dir", "", "Target directory under notes/")
		c.Flags().StringVar(ctx.noteSlug, "slug", "", "Filename slug")
		c.Flags().StringVar(ctx.noteStatus, "status", "", "frontmatter status")
		c.Flags().BoolVar(ctx.noteDryRun, "dry-run", false, "Preview the plan only; do not write files")
		c.Flags().BoolVar(ctx.noteOpen, "open", false, "Open in an editor after creation")
		c.Flags().StringVar(ctx.noteEditor, "editor", "", "Editor command; defaults to EDITOR")
	}
	noteNewCmd := &cobra.Command{Use: "new <title>", Short: "Create a local Markdown note", Example: "pinax note new \"Research Log\" --body body --tags pinax --vault ./my-notes", RunE: noteCreateRun("new")}
	addCreateFlags(noteNewCmd)
	noteAddCmd := &cobra.Command{Use: "add <title>", Short: "Add a local Markdown note", Example: "pinax note add \"Research Log\" --body body --tags pinax --vault ./my-notes", RunE: noteCreateRun("add")}
	addCreateFlags(noteAddCmd)
	noteCreateCmd := &cobra.Command{Use: "create <title>", Short: "Create a local Markdown note", Example: "pinax note create \"Meeting\" --stdin --vault ./my-notes", RunE: noteCreateRun("create")}
	addCreateFlags(noteCreateCmd)
	noteCmd.AddCommand(noteNewCmd, noteAddCmd, noteCreateCmd)

	var noteListPeriod string
	var noteListUpdatedAfter string
	noteListCmd := &cobra.Command{Use: "list", Short: "List local notes", RunE: func(cmd *cobra.Command, args []string) error {
		group := *ctx.noteGroup
		if group == "" {
			group = *ctx.noteListProject
		}
		projection, err := ctx.svc.ListNotesQuery(cmd.Context(), app.NoteListRequest{VaultPath: *ctx.vaultPath, Tags: splitCSV(*ctx.noteListTag), Project: *ctx.noteListProject, Group: group, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Status: *ctx.noteListStatus, CreatedAfter: *ctx.noteListCreatedAfter, UpdatedAfter: noteListUpdatedAfter, UpdatedBefore: *ctx.noteListUpdatedBefore, Period: noteListPeriod, Recent: *ctx.noteRecent, Limit: *ctx.noteLimit, Sort: *ctx.noteListSort, PathPrefix: *ctx.noteListPathPrefix, Properties: *ctx.noteListProperties, StrictProperties: *ctx.noteStrictProperties})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteListCmd.Flags().StringVar(ctx.noteListTag, "tag", "", "Filter by tag")
	noteListCmd.Flags().StringVar(ctx.noteListProject, "project", "", "Filter by project")
	noteListCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Filter by group")
	noteListCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Filter by folder")
	noteListCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Filter by kind")
	noteListCmd.Flags().StringVar(ctx.noteListStatus, "status", "", "Filter by status")
	noteListCmd.Flags().StringVar(ctx.noteListCreatedAfter, "created-after", "", "Filter by minimum creation date; format YYYY-MM-DD or RFC3339")
	noteListCmd.Flags().StringVar(&noteListUpdatedAfter, "updated-after", "", "Filter by minimum update date; format YYYY-MM-DD or RFC3339")
	noteListCmd.Flags().StringVar(ctx.noteListUpdatedBefore, "updated-before", "", "Filter by maximum update date; format YYYY-MM-DD or RFC3339")
	noteListCmd.Flags().StringVar(&noteListPeriod, "period", "", "Filter by recent period: 5h, daily, weekly, or monthly")
	noteListCmd.Flags().BoolVar(ctx.noteRecent, "recent", false, "Sort by recent updates")
	noteListCmd.Flags().IntVar(ctx.noteLimit, "limit", 0, "Limit the number of results")
	noteListCmd.Flags().StringVar(ctx.noteListSort, "sort", "", "Sort: updated, path, or title")
	noteListCmd.Flags().StringVar(ctx.noteListPathPrefix, "path-prefix", "", "Filter by path prefix")
	noteListCmd.Flags().StringArrayVar(ctx.noteListProperties, "property", nil, "Select properties; repeatable")
	noteListCmd.Flags().BoolVar(ctx.noteStrictProperties, "strict-properties", false, "Error on unknown properties")
	_ = noteListCmd.RegisterFlagCompletionFunc("status", staticCompletion("status", "active", "done", "inbox", "archived", "paused"))
	_ = noteListCmd.RegisterFlagCompletionFunc("sort", staticCompletion("sort", "updated", "path", "title"))
	_ = noteListCmd.RegisterFlagCompletionFunc("period", staticCompletion("period", "5h", "daily", "weekly", "monthly"))
	_ = noteListCmd.RegisterFlagCompletionFunc("limit", staticCompletion("limit", "10", "25", "50", "100"))
	noteCmd.AddCommand(noteListCmd)
	for _, dimension := range []struct{ name, dim string }{{"tags", "tag"}, {"folders", "folder"}, {"kinds", "kind"}, {"groups", "group"}} {
		dimension := dimension
		dimensionCmd := &cobra.Command{Use: dimension.name, Short: "List note " + dimension.name, RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ListDimension(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath}, dimension.dim)
			return ctx.renderProjection(cmd, projection, err)
		}}
		dimensionCmd.AddCommand(&cobra.Command{Use: "list", Short: "List note " + dimension.name, RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ListDimension(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath}, dimension.dim)
			return ctx.renderProjection(cmd, projection, err)
		}})
		if dimension.dim == "tag" {
			var renameDryRun bool
			var renameYes bool
			renameCmd := &cobra.Command{Use: "rename <old> <new>", Short: "Bulk rename a tag", RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) != 2 {
					return renderCommandError(cmd, ctx.outputMode(), "tag.rename", "argument_required", "tag rename requires old and new", "pinax note tags rename <old> <new> --vault <vault> --yes")
				}
				projection, err := ctx.svc.BulkTag(cmd.Context(), app.NoteTagBulkRequest{VaultPath: *ctx.vaultPath, Operation: "rename", OldTag: args[0], NewTag: args[1], DryRun: renameDryRun, Yes: renameYes})
				return ctx.renderProjection(cmd, projection, err)
			}}
			renameCmd.Flags().BoolVar(&renameDryRun, "dry-run", false, "Preview matching notes only; do not write files")
			renameCmd.Flags().BoolVar(&renameYes, "yes", false, "Confirm bulk tag writes")
			var deleteDryRun bool
			var deleteYes bool
			deleteCmd := &cobra.Command{Use: "delete <tag>", Short: "Bulk delete a tag", RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) != 1 {
					return renderCommandError(cmd, ctx.outputMode(), "tag.delete", "argument_required", "tag delete requires a tag", "pinax note tags delete <tag> --vault <vault> --yes")
				}
				projection, err := ctx.svc.BulkTag(cmd.Context(), app.NoteTagBulkRequest{VaultPath: *ctx.vaultPath, Operation: "delete", OldTag: args[0], DryRun: deleteDryRun, Yes: deleteYes})
				return ctx.renderProjection(cmd, projection, err)
			}}
			deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "Preview matching notes only; do not write files")
			deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "Confirm bulk tag writes")
			dimensionCmd.AddCommand(renameCmd, deleteCmd)
		}
		if dimension.dim == "folder" {
			var renameDryRun bool
			var renameYes bool
			renameCmd := &cobra.Command{Use: "rename <old> <new>", Short: "Bulk rename a folder and move note files", RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) != 2 {
					return renderCommandError(cmd, ctx.outputMode(), "folder.rename", "argument_required", "folder rename requires old and new", "pinax note folders rename <old> <new> --vault <vault> --yes")
				}
				projection, err := ctx.svc.BulkFolder(cmd.Context(), app.NoteFolderBulkRequest{VaultPath: *ctx.vaultPath, Operation: "rename", OldFolder: args[0], NewFolder: args[1], DryRun: renameDryRun, Yes: renameYes})
				return ctx.renderProjection(cmd, projection, err)
			}}
			renameCmd.Flags().BoolVar(&renameDryRun, "dry-run", false, "Preview matching notes and target paths only; do not write files")
			renameCmd.Flags().BoolVar(&renameYes, "yes", false, "Confirm bulk note moves and folder updates")
			dimensionCmd.AddCommand(renameCmd)
		}
		noteCmd.AddCommand(dimensionCmd)
	}

	var noteEmbedAttachments string
	var noteMaxEmbedDepth int
	var noteMaxEmbedBytes int
	var noteMaxPreviewBytes int
	noteShowRun := func(command, forcedView string) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), command, "argument_required", "requires a note path, title, or note_id", "pinax note show <note> --vault <vault>")
			}
			view := *ctx.noteView
			if forcedView != "" {
				view = forcedView
			}
			projection, err := ctx.svc.ShowNoteProjection(cmd.Context(), app.ShowNoteRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], View: view, Display: *ctx.noteDisplay, Snapshot: *ctx.noteSnapshot, Runs: *ctx.noteRuns, EmbedAttachments: noteEmbedAttachments, MaxEmbedDepth: noteMaxEmbedDepth, MaxEmbedBytes: noteMaxEmbedBytes, MaxPreviewBytes: noteMaxPreviewBytes})
			projection.Command = command
			return ctx.renderProjection(cmd, projection, err)
		}
	}
	addPreviewFlags := func(c *cobra.Command) {
		c.Flags().StringVar(&noteEmbedAttachments, "embed-attachments", "", "Inline attachment preview mode: markdown, text, or none")
		c.Flags().IntVar(&noteMaxEmbedDepth, "max-embed-depth", 0, "Maximum recursive inline attachment depth; 0 uses the default")
		c.Flags().IntVar(&noteMaxEmbedBytes, "max-embed-bytes", 0, "Maximum inline bytes per attachment; 0 uses the default")
		c.Flags().IntVar(&noteMaxPreviewBytes, "max-preview-bytes", 0, "Maximum total preview bytes; 0 uses the default")
		_ = c.RegisterFlagCompletionFunc("embed-attachments", staticCompletion("embed-attachments", "markdown", "text", "none"))
	}
	noteShowCmd := &cobra.Command{Use: "show <note>", Short: "Read a local note", Example: "pinax note show note_01 --vault ./my-notes\npinax note show \"Inbox Note\" --view rendered --vault ./my-notes", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: noteShowRun("note.show", "")}
	noteShowCmd.Flags().StringVar(ctx.noteView, "view", "", "View: source or rendered")
	noteShowCmd.Flags().StringVar(ctx.noteDisplay, "display", "", "Information display level: card, detail, context, or body")
	noteShowCmd.Flags().StringVar(ctx.noteSnapshot, "snapshot", "", "Read rendered snapshot: run id, alias, or latest")
	noteShowCmd.Flags().BoolVar(ctx.noteRuns, "runs", false, "List render runs for this note")
	addPreviewFlags(noteShowCmd)
	_ = noteShowCmd.RegisterFlagCompletionFunc("view", staticCompletion("view", "source", "rendered"))
	_ = noteShowCmd.RegisterFlagCompletionFunc("display", staticCompletion("display", "card", "detail", "context", "body"))
	_ = noteShowCmd.RegisterFlagCompletionFunc("snapshot", noteRenderRunCompletion(func() string { return *ctx.vaultPath }))
	noteReadCmd := &cobra.Command{Use: "read <note>", Short: "Read a local note", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: noteShowRun("note.show", "")}
	noteReadCmd.Flags().StringVar(ctx.noteView, "view", "", "View: source or rendered")
	noteReadCmd.Flags().StringVar(ctx.noteDisplay, "display", "", "Information display level: card, detail, context, or body")
	noteReadCmd.Flags().StringVar(ctx.noteSnapshot, "snapshot", "", "Read rendered snapshot: run id, alias, or latest")
	noteReadCmd.Flags().BoolVar(ctx.noteRuns, "runs", false, "List render runs for this note")
	addPreviewFlags(noteReadCmd)
	_ = noteReadCmd.RegisterFlagCompletionFunc("view", staticCompletion("view", "source", "rendered"))
	_ = noteReadCmd.RegisterFlagCompletionFunc("display", staticCompletion("display", "card", "detail", "context", "body"))
	_ = noteReadCmd.RegisterFlagCompletionFunc("snapshot", noteRenderRunCompletion(func() string { return *ctx.vaultPath }))
	notePreviewCmd := &cobra.Command{Use: "preview <note>", Short: "Read-only rendered note preview", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: noteShowRun("note.preview", "rendered")}
	addPreviewFlags(notePreviewCmd)
	noteCmd.AddCommand(noteShowCmd, noteReadCmd, notePreviewCmd)
	noteRefreshCmd := &cobra.Command{Use: "refresh <note>", Short: "Refresh note rendered managed blocks", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.refresh", "argument_required", "note refresh requires a note reference", "pinax note refresh <note> --rendered --vault <vault> --yes")
		}
		if !*ctx.noteRefreshRendered {
			return renderCommandError(cmd, ctx.outputMode(), "note.refresh", "rendered_required", "note refresh currently requires --rendered", "pinax note refresh <note> --rendered --vault <vault> --yes")
		}
		projection, err := ctx.svc.RefreshNoteRendered(cmd.Context(), app.NoteRefreshRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Rendered: *ctx.noteRefreshRendered, Yes: *ctx.yes, SaveRun: *ctx.templateSaveRun, Snapshot: *ctx.noteSnapshot})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteRefreshCmd.Flags().BoolVar(ctx.noteRefreshRendered, "rendered", false, "Refresh rendered managed blocks")
	noteRefreshCmd.Flags().StringVar(ctx.templateSaveRun, "save-run", "", "Save a render run alias")
	noteRefreshCmd.Flags().StringVar(ctx.noteSnapshot, "snapshot", "", "Use a historical rendered snapshot")
	_ = noteRefreshCmd.RegisterFlagCompletionFunc("snapshot", noteRenderRunCompletion(func() string { return *ctx.vaultPath }))
	noteRefreshCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm writing back to Markdown")
	noteCmd.AddCommand(noteRefreshCmd)
	noteLinksCmd := &cobra.Command{Use: "links <note>", Short: "List note outgoing links", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.links", "argument_required", "note links requires a note reference", "pinax note links <note> --vault <vault>")
		}
		projection, err := ctx.svc.NoteLinks(cmd.Context(), app.NoteLinkRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteCmd.AddCommand(noteLinksCmd)
	noteBacklinksCmd := &cobra.Command{Use: "backlinks <note>", Short: "List note backlinks", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.backlinks", "argument_required", "note backlinks requires a note reference", "pinax note backlinks <note> --vault <vault>")
		}
		projection, err := ctx.svc.NoteBacklinks(cmd.Context(), app.NoteLinkRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteCmd.AddCommand(noteBacklinksCmd)
	noteCmd.AddCommand(&cobra.Command{Use: "orphans", Short: "List notes with no incoming or outgoing links", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.NoteOrphans(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	var attachPlacement string
	var attachLinkStyle string
	var attachMode string
	var attachRename string
	var attachEmbed bool
	attachCmd := &cobra.Command{Use: "attach <note> <file>", Short: "Copy a file into the vault and append an attachment reference", Example: "pinax note attach \"Auth Plan\" ./diagram.png --placement note-folder --embed --vault ./my-notes --json", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "note.attach", "argument_required", "note attach requires a note and source file", "pinax note attach <note> <file> --vault <vault>")
		}
		projection, err := ctx.svc.AttachNoteFile(cmd.Context(), app.NoteAttachRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], SourcePath: args[1], Placement: attachPlacement, LinkStyle: attachLinkStyle, Embed: attachEmbed, Mode: attachMode, Rename: attachRename, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	attachCmd.Flags().StringVar(&attachPlacement, "placement", "per-note", "Attachment directory policy: per-note, vault-folder, or note-folder")
	attachCmd.Flags().StringVar(&attachLinkStyle, "link-style", "markdown", "Reference style to write: markdown, wiki, or auto")
	attachCmd.Flags().BoolVar(&attachEmbed, "embed", false, "Write an embed reference")
	attachCmd.Flags().StringVar(&attachMode, "mode", "copy", "Source file handling mode: copy, move, or register")
	attachCmd.Flags().StringVar(&attachRename, "rename", "", "New filename to use when writing into the vault")
	attachCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm moving the source file")
	_ = attachCmd.RegisterFlagCompletionFunc("placement", staticCompletion("placement", "per-note", "vault-folder", "note-folder"))
	_ = attachCmd.RegisterFlagCompletionFunc("link-style", staticCompletion("link-style", "markdown", "wiki", "auto"))
	_ = attachCmd.RegisterFlagCompletionFunc("mode", staticCompletion("mode", "copy", "move", "register"))
	noteCmd.AddCommand(attachCmd)
	var attachmentsPathStyle string
	var attachmentsIncludePaths bool
	attachmentsCmd := &cobra.Command{Use: "attachments <note>", Short: "List note attachment references", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.attachments", "argument_required", "note attachments requires a note reference", "pinax note attachments <note> --vault <vault>")
		}
		projection, err := ctx.svc.NoteAttachments(cmd.Context(), app.NoteLinkRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], PathStyle: attachmentsPathStyle, IncludePaths: attachmentsIncludePaths})
		return ctx.renderProjection(cmd, projection, err)
	}}
	attachmentsCmd.Flags().StringVar(&attachmentsPathStyle, "path-style", "", "Path display style: vault-relative, note-relative, absolute, markdown, or wiki")
	attachmentsCmd.Flags().BoolVar(&attachmentsIncludePaths, "include-paths", false, "Include display_path in the requested style")
	_ = attachmentsCmd.RegisterFlagCompletionFunc("path-style", staticCompletion("path-style", "vault-relative", "note-relative", "absolute", "markdown", "wiki"))
	noteCmd.AddCommand(attachmentsCmd)

	noteEditRun := func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.edit", "argument_required", "note edit requires a note reference", "pinax note edit <note> --vault <vault>")
		}
		projection, err := ctx.svc.EditNote(cmd.Context(), app.NoteEditRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Editor: *ctx.noteEditor})
		return ctx.renderProjection(cmd, projection, err)
	}
	noteEditCmd := &cobra.Command{Use: "edit <note>", Short: "Open a note in an editor", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: noteEditRun}
	noteEditCmd.Flags().StringVar(ctx.noteEditor, "editor", "", "Editor command; defaults to EDITOR")
	noteOpenCmd := &cobra.Command{Use: "open <note>", Short: "Open a note in an editor", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: noteEditRun}
	noteOpenCmd.Flags().StringVar(ctx.noteEditor, "editor", "", "Editor command; defaults to EDITOR")
	noteCmd.AddCommand(noteEditCmd, noteOpenCmd)

	noteRenameCmd := &cobra.Command{Use: "rename <note> <title>", Short: "Rename a note", ValidArgsFunction: firstNoteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "note.rename", "argument_required", "note rename requires a note and new title", "pinax note rename <note> <title> --vault <vault>")
		}
		projection, err := ctx.svc.RenameNote(cmd.Context(), app.NoteMutationRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Title: args[1]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteCmd.AddCommand(noteRenameCmd)
	noteMoveCmd := &cobra.Command{Use: "move <note> <dir>", Short: "Move a note to a directory under notes/", ValidArgsFunction: firstNoteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "note.move", "argument_required", "note move requires a note and folder", "pinax note move <note> <dir> --vault <vault>")
		}
		projection, err := ctx.svc.MoveNote(cmd.Context(), app.NoteMutationRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], TargetDir: args[1]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteCmd.AddCommand(noteMoveCmd)
	noteArchiveCmd := &cobra.Command{Use: "archive <note>", Short: "Archive a note", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.archive", "argument_required", "note archive requires a note", "pinax note archive <note> --vault <vault>")
		}
		projection, err := ctx.svc.ArchiveNote(cmd.Context(), app.NoteMutationRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteCmd.AddCommand(noteArchiveCmd)
	noteDeleteCmd := &cobra.Command{Use: "delete <note>", Short: "Delete or move to trash", ValidArgsFunction: noteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "note.delete", "argument_required", "note delete requires a note", "pinax note delete <note> --vault <vault> --yes")
		}
		yes := *ctx.yes
		if !yes && ctx.outputMode() == "summary" {
			confirmed, confirmErr := confirmNoteDelete(cmd, args[0], *ctx.noteHard)
			if confirmErr != nil {
				return renderCommandError(cmd, ctx.outputMode(), "note.delete", "confirmation_failed", "Failed to read confirmation input", "Rerun the command and type y, or add --yes")
			}
			if !confirmed {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Canceled")
				return nil
			}
			yes = confirmed
		}
		projection, err := ctx.svc.DeleteNote(cmd.Context(), app.NoteDeleteRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Yes: yes, Hard: *ctx.noteHard})
		return ctx.renderProjection(cmd, projection, err)
	}}
	noteDeleteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm delete or move to trash")
	noteDeleteCmd.Flags().BoolVar(ctx.noteHard, "hard", false, "Actually delete the file; requires --yes")
	noteCmd.AddCommand(noteDeleteCmd)

	notePropertyCmd := &cobra.Command{Use: "property", Short: "Manage note frontmatter properties"}
	notePropertySetCmd := &cobra.Command{Use: "set <note> <property> <value>", Short: "Set a note property", ValidArgsFunction: firstNoteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 3 {
			return renderCommandError(cmd, ctx.outputMode(), "note.property", "argument_required", "note property set requires note, property, and value", "pinax note property set <note> <property> <value> --vault <vault>")
		}
		projection, err := ctx.svc.PatchNoteProperty(cmd.Context(), app.NotePropertyRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Operation: "set", Key: args[1], Value: args[2]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	notePropertyCmd.AddCommand(notePropertySetCmd)
	notePropertyRemoveCmd := &cobra.Command{Use: "remove <note> <property>", Short: "Remove a note property", ValidArgsFunction: firstNoteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "note.property", "argument_required", "note property remove requires note and property", "pinax note property remove <note> <property> --vault <vault>")
		}
		projection, err := ctx.svc.PatchNoteProperty(cmd.Context(), app.NotePropertyRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Operation: "remove", Key: args[1]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	notePropertyCmd.AddCommand(notePropertyRemoveCmd)
	noteCmd.AddCommand(notePropertyCmd)

	noteTagCmd := &cobra.Command{Use: "tag", Short: "Manage note tags"}
	for _, op := range []string{"add", "remove", "set"} {
		operation := op
		noteTagOperationCmd := &cobra.Command{Use: operation + " <note> <tag>...", Short: "Update note tags", ValidArgsFunction: firstNoteRefCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return renderCommandError(cmd, ctx.outputMode(), "note.tag", "argument_required", "note tag requires a note and at least one tag", "pinax note tag "+operation+" <note> <tag> --vault <vault>")
			}
			projection, err := ctx.svc.TagNote(cmd.Context(), app.NoteTagRequest{VaultPath: *ctx.vaultPath, NoteRef: args[0], Operation: operation, Tags: args[1:]})
			return ctx.renderProjection(cmd, projection, err)
		}}
		noteTagCmd.AddCommand(noteTagOperationCmd)
	}
	noteCmd.AddCommand(noteTagCmd)
	root.AddCommand(noteCmd)

}

func confirmNoteDelete(cmd *cobra.Command, noteRef string, hard bool) (bool, error) {
	action := "move to trash"
	if hard {
		action = "permanently delete"
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Confirm %s %s? Type y to confirm: ", action, noteRef); err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr()); err != nil {
		return false, err
	}
	answer := strings.TrimSpace(scanner.Text())
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") || answer == "\u662f", nil
}
