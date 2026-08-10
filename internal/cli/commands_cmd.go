package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/domain"
)

type commandCatalogEntry struct {
	Name       string `json:"name"`
	Summary    string `json:"summary"`
	Group      string `json:"group"`
	Visibility string `json:"visibility"`
}

func addCommandsCommand(root *cobra.Command, ctx commandBuildContext) {
	root.AddCommand(&cobra.Command{
		Use:   "commands",
		Short: "List the complete command catalog",
		Long:  "List every supported top-level Pinax command, including advanced compatibility and integration surfaces hidden from the default help.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries := commandCatalog(root)
			coreCount := 0
			for _, entry := range entries {
				if entry.Visibility == rootHelpVisibilityCore {
					coreCount++
				}
			}
			projection := domain.NewProjection("commands.list", "Complete command catalog listed.")
			projection.Facts["commands.total"] = fmt.Sprint(len(entries))
			projection.Facts["commands.core"] = fmt.Sprint(coreCount)
			projection.Facts["commands.advanced"] = fmt.Sprint(len(entries) - coreCount)
			projection.Data = map[string]any{"commands": entries}
			return ctx.renderProjection(cmd, projection, nil)
		},
	})
}

func commandCatalog(root *cobra.Command) []commandCatalogEntry {
	entries := make([]commandCatalogEntry, 0, len(root.Commands()))
	for _, child := range root.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		visibility := child.Annotations[rootHelpVisibilityAnnotation]
		if visibility == "" {
			visibility = rootHelpVisibilityAdvanced
		}
		group := child.Annotations[rootHelpGroupAnnotation]
		if group == "" {
			group = "Other"
		}
		entries = append(entries, commandCatalogEntry{
			Name:       child.Name(),
			Summary:    child.Short,
			Group:      group,
			Visibility: visibility,
		})
	}
	// 目录顺序稳定：个人核心入口优先，其余高级入口按命令名排序，方便人和 Agent 比对版本差异。
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Visibility != entries[j].Visibility {
			return entries[i].Visibility == rootHelpVisibilityCore
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}
