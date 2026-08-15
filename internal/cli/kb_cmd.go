package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/domain"
)

// addKBCommands keeps the released command names parseable for one migration
// window, but deliberately contains no vector, embedding, sidecar, or remote
// index implementation. Pinax now owns Markdown, SQLite/local text search,
// memory, and sync; an external RAG system owns ingestion and retrieval.
func addKBCommands(root *cobra.Command, ctx commandBuildContext) {
	decoupled := func(command string) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			err := &domain.CommandError{
				Code:    "kb_decoupled",
				Message: "Pinax no longer manages vector databases, embeddings, or RAG retrieval",
				Hint:    "Export Markdown and configure the external RAG pipeline instead",
			}
			projection := domain.NewErrorProjection(command, err)
			projection.Facts["vector_runtime"] = "removed"
			projection.Facts["rag_owner"] = "external"
			projection.Actions = []domain.Action{
				{Name: "export", Command: "pinax export markdown <output-dir> --vault <vault> --json"},
				{Name: "search", Command: "pinax search <query> --vault <vault> --json"},
			}
			return ctx.renderProjection(cmd, projection, err)
		}
	}

	kbCmd := &cobra.Command{
		Use:   "kb",
		Short: "Deprecated compatibility surface for the removed vector KB",
		Long:  "The Pinax vector KB and embedding runtime were removed. Use Markdown export and an external RAG pipeline.",
		RunE:  decoupled("kb"),
	}

	newCommand := func(use, short, command string) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, RunE: decoupled(command)}
	}

	importCmd := newCommand("import <source>", "Deprecated KB import compatibility command", "kb.import")
	importCmd.Flags().StringArray("include", nil, "Deprecated; use pinax import markdown")
	importCmd.Flags().Bool("dry-run", false, "Deprecated; use pinax import markdown")
	importCmd.Flags().Bool("yes", false, "Deprecated; use pinax import markdown")

	for _, item := range []struct{ use, short, command string }{
		{"rebuild", "Deprecated vector projection rebuild", "kb.rebuild"},
		{"refresh", "Deprecated vector projection refresh", "kb.refresh"},
		{"doctor", "Deprecated vector KB doctor", "kb.doctor"},
		{"search <query>", "Deprecated semantic KB search", "kb.search"},
		{"context <task>", "Deprecated semantic context retrieval", "kb.context"},
		{"evaluate", "Deprecated retrieval evaluation", "kb.evaluate"},
		{"activate", "Deprecated KB generation activation", "kb.activate"},
		{"rollback", "Deprecated KB generation rollback", "kb.rollback"},
	} {
		cmd := newCommand(item.use, item.short, item.command)
		for _, flag := range []string{"backend", "provider", "model", "generation", "suite", "run-id", "expected-sequence", "limit", "k", "legacy-v1-readonly", "allow-disk-high-water"} {
			addDeprecatedKBFlag(cmd, flag)
		}
		kbCmd.AddCommand(cmd)
	}

	providerCmd := newCommand("provider", "Deprecated embedding provider inspection", "kb.provider")
	providerListCmd := newCommand("list", "Deprecated provider list", "kb.provider.list")
	providerDoctorCmd := newCommand("doctor <provider>", "Deprecated provider doctor", "kb.provider.doctor")
	providerDoctorCmd.Flags().String("model", "", "Deprecated embedding model")
	providerCmd.AddCommand(providerListCmd, providerDoctorCmd)

	generationsCmd := newCommand("generations", "Deprecated KB generation maintenance", "kb.generations")
	pruneCmd := newCommand("prune", "Deprecated KB generation pruning", "kb.generations.prune")
	pruneCmd.Flags().Int("keep", 1, "Deprecated generation retention")
	pruneCmd.Flags().Bool("dry-run", true, "Deprecated dry-run mode")
	pruneCmd.Flags().Bool("yes", false, "Deprecated confirmation")
	generationsCmd.AddCommand(pruneCmd)
	kbCmd.AddCommand(importCmd, providerCmd, generationsCmd)
	root.AddCommand(kbCmd)
}

func addDeprecatedKBFlag(cmd *cobra.Command, name string) {
	switch name {
	case "expected-sequence":
		cmd.Flags().Uint64(name, 0, "Deprecated compare-and-swap sequence")
	case "limit", "k":
		cmd.Flags().Int(name, 0, "Deprecated retrieval limit")
	case "legacy-v1-readonly", "allow-disk-high-water":
		cmd.Flags().Bool(name, false, "Deprecated compatibility flag")
	default:
		cmd.Flags().String(name, "", "Deprecated compatibility flag")
	}
}
