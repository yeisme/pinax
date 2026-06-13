package cli

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addJournalCommands(root *cobra.Command, ctx commandBuildContext) {
	dailyRequest := func() app.DailyRequest {
		return app.DailyRequest{VaultPath: *ctx.vaultPath, Editor: *ctx.noteEditor, Body: *ctx.noteBody, Date: *ctx.journalDate, Prev: *ctx.journalPrev, Next: *ctx.journalNext, Template: *ctx.noteTemplate}
	}
	addJournalFlags := func(c *cobra.Command, period string, includeEditor bool, includeBody bool) {
		c.Flags().StringVar(ctx.journalDate, "date", "", "Target date, format YYYY-MM-DD")
		_ = c.RegisterFlagCompletionFunc("date", journalDateCompletion(period, func() string { return *ctx.vaultPath }))
		c.Flags().BoolVar(ctx.journalPrev, "prev", false, "Read the previous day")
		c.Flags().BoolVar(ctx.journalNext, "next", false, "Read the next day")
		c.Flags().StringVar(ctx.noteTemplate, "template", "", "Journal template name")
		_ = c.RegisterFlagCompletionFunc("template", templateNameCompletion(func() string { return *ctx.vaultPath }, "journal_template", true, true))
		if includeEditor {
			c.Flags().StringVar(ctx.noteEditor, "editor", "", "Editor command; defaults to EDITOR")
		}
		if includeBody {
			c.Flags().StringVar(ctx.noteBody, "body", "", "Append body")
		}
	}

	journalSpecs := []struct {
		name   string
		short  string
		open   func(context.Context, app.DailyRequest) (domain.Projection, error)
		show   func(context.Context, app.DailyRequest) (domain.Projection, error)
		append func(context.Context, app.DailyRequest) (domain.Projection, error)
	}{
		{name: "daily", short: "Manage daily note workflows", open: ctx.svc.DailyOpen, show: ctx.svc.DailyShow, append: ctx.svc.DailyAppend},
		{name: "weekly", short: "Manage weekly note workflows", open: ctx.svc.WeeklyOpen, show: ctx.svc.WeeklyShow, append: ctx.svc.WeeklyAppend},
		{name: "monthly", short: "Manage monthly note workflows", open: ctx.svc.MonthlyOpen, show: ctx.svc.MonthlyShow, append: ctx.svc.MonthlyAppend},
	}

	newJournalPeriodCmd := func(journal struct {
		name   string
		short  string
		open   func(context.Context, app.DailyRequest) (domain.Projection, error)
		show   func(context.Context, app.DailyRequest) (domain.Projection, error)
		append func(context.Context, app.DailyRequest) (domain.Projection, error)
	}, hidden bool) *cobra.Command {
		journalCmd := &cobra.Command{Use: journal.name, Short: journal.short}
		journalCmd.Hidden = hidden
		openCmd := &cobra.Command{Use: "open", Short: "Create or open " + journal.name + " note", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := journal.open(cmd.Context(), dailyRequest())
			return ctx.renderProjection(cmd, projection, err)
		}}
		showCmd := &cobra.Command{Use: "show", Short: "Read " + journal.name + " note", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := journal.show(cmd.Context(), dailyRequest())
			return ctx.renderProjection(cmd, projection, err)
		}}
		appendCmd := &cobra.Command{Use: "append", Short: "Append content to " + journal.name + " note", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := journal.append(cmd.Context(), dailyRequest())
			return ctx.renderProjection(cmd, projection, err)
		}}
		addJournalFlags(openCmd, journal.name, true, false)
		addJournalFlags(showCmd, journal.name, false, false)
		addJournalFlags(appendCmd, journal.name, false, true)
		journalCmd.AddCommand(openCmd, showCmd, appendCmd)
		return journalCmd
	}

	for _, journal := range journalSpecs {
		root.AddCommand(newJournalPeriodCmd(journal, true))
	}

	journalRootCmd := &cobra.Command{Use: "journal", Short: "Manage daily/weekly/monthly journals"}
	for _, journal := range journalSpecs {
		journalRootCmd.AddCommand(newJournalPeriodCmd(journal, false))
	}
	root.AddCommand(journalRootCmd)
}
