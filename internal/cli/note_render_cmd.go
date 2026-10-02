package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/output"
)

func extendNotePreview(cmd *cobra.Command, ctx commandBuildContext) {
	legacy := cmd.RunE
	var view, format, display, resolved string
	cmd.Flags().StringVar(&view, "view", "", "Explicit rendered enables the experimental bounded tree contract")
	cmd.Flags().StringVar(&format, "format", "tree", "Rendered output: tree, json, or html")
	cmd.Flags().StringVar(&display, "display", "card", "Rendered depth: card, context, or explicit body")
	cmd.Flags().StringVar(&resolved, "resolved-revision", "", "Previously resolved revision for a floating reference")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("view") && !cmd.Flags().Changed("format") && !cmd.Flags().Changed("display") && !cmd.Flags().Changed("resolved-revision") {
			return legacy(cmd, args)
		}
		if view != "rendered" || len(args) != 1 || (format != "tree" && format != "json" && format != "html") {
			return renderCommandError(cmd, ctx.outputMode(), "note.preview.rendered", "render_arguments_invalid", "Use one note reference, --view rendered and --format tree|json|html", "pinax note preview <object-id> --view rendered")
		}
		if format != "tree" && ctx.outputMode() != output.ModeSummary && (format != "json" || ctx.outputMode() != output.ModeJSON) {
			return renderCommandError(cmd, ctx.outputMode(), "note.preview.rendered", "output_mode_conflict", "Use a machine output mode or an explicit format, not both", "")
		}
		p, err := ctx.svc.RenderNote(cmd.Context(), app.RenderNoteRequest{VaultPath: agentVaultPath(ctx), NoteRef: args[0], Display: display, ResolvedRevision: resolved})
		if err != nil {
			return ctx.renderProjection(cmd, p, err)
		}
		if format == "html" {
			return output.RenderRenderedHTML(cmd.OutOrStdout(), p)
		}
		if format == "json" {
			*ctx.jsonMode = true
		}
		return ctx.renderProjection(cmd, p, nil)
	}
}

func addNoteRefCommands(note *cobra.Command, ctx commandBuildContext) {
	var floating, copyRef bool
	var pick int
	ref := &cobra.Command{Use: "ref <note>", Short: "Read a stable note reference (experimental; no vault writes)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := ctx.svc.NoteReference(cmd.Context(), app.NoteRefRequest{VaultPath: agentVaultPath(ctx), NoteRef: args[0], Floating: floating, Pick: pick})
		if err != nil {
			return ctx.renderProjection(cmd, p, err)
		}
		if copyRef && p.Facts["reference_status"] == "resolved" {
			links := p.Facts["markdown_link"] + "\n" + p.Facts["wikilink"] + "\n"
			if copyErr := copyNoteReference(cmd, links); copyErr != nil {
				p.Facts["clipboard_status"] = "unavailable"
				p.Summary = "Clipboard unavailable; manually copy the two printed references."
			} else {
				p.Facts["clipboard_status"] = "copied"
			}
		}
		return ctx.renderProjection(cmd, p, nil)
	}}
	ref.Flags().BoolVar(&floating, "floating", false, "Emit an unpinned reference; retain resolved_revision for later drift checks")
	ref.Flags().BoolVar(&copyRef, "copy", false, "Copy both Markdown and wikilink formats; print them if unavailable")
	ref.Flags().IntVar(&pick, "pick", 0, "Select a numbered candidate from a bounded ambiguity list")
	var resolved string
	resolve := &cobra.Command{Use: "resolve <pinax-uri>", Short: "Resolve an exact version or detect caller-pinned floating drift", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := ctx.svc.NoteReference(cmd.Context(), app.NoteRefRequest{VaultPath: agentVaultPath(ctx), NoteRef: args[0], ResolvedRevision: resolved})
		return ctx.renderProjection(cmd, p, err)
	}}
	resolve.Flags().StringVar(&resolved, "resolved-revision", "", "Original resolved_revision; never silently advance a floating reference")
	ref.AddCommand(resolve)
	note.AddCommand(ref)
}

func copyNoteReference(cmd *cobra.Command, value string) error {
	commands := [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}}
	if runtime.GOOS == "darwin" {
		commands = [][]string{{"pbcopy"}}
	}
	if runtime.GOOS == "windows" {
		commands = [][]string{{"powershell", "-NoProfile", "-Command", "$input | Set-Clipboard"}}
	}
	for _, args := range commands {
		binary, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}
		c := exec.CommandContext(cmd.Context(), binary, args[1:]...)
		c.Stdin = strings.NewReader(value)
		if err := c.Run(); err != nil {
			return fmt.Errorf("clipboard unavailable")
		}
		return nil
	}
	return fmt.Errorf("clipboard unavailable")
}
