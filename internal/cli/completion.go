package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/index/query"
	pinaxplugin "github.com/yeisme/pinax/internal/plugin"
	"github.com/yeisme/pinax/internal/profile"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func projectSlugCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := projectCompletionItems(completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func projectThenSubprojectCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		root := completionVaultRoot(vaultPathValue())
		switch len(args) {
		case 0:
			items, err := projectCompletionItems(root)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
		case 1:
			items, err := projectSubprojectCompletionItems(root, args[0])
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}

func folderPathCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := folderCompletionItems(completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func profileNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := profile.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items := make([]string, 0, len(cfg.Profiles))
	for name, p := range cfg.Profiles {
		description := "profile"
		if strings.TrimSpace(p.Workspace) != "" {
			description += " workspace=" + p.Workspace
		}
		items = append(items, name+"\t"+description)
	}
	sort.Strings(items)
	return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func backendNameCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := backendCompletionItems(completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func backendKindCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(domain.ValidBackendKinds()))
	for _, kind := range domain.ValidBackendKinds() {
		values = append(values, string(kind))
	}
	return staticCompletion("backend kind", values...)(cmd, args, toComplete)
}

func promptAssetCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := promptAssetCompletionItems(completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func pluginIDCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := pluginCompletionItems(completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func pluginRunCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		root := completionVaultRoot(vaultPathValue())
		switch len(args) {
		case 0:
			items, err := pluginCompletionItems(root)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
		case 1:
			items, err := pluginCapabilityCompletionItems(root, args[0])
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}

func syncConflictCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := syncConflictCompletionItems(completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func projectCompletionItems(root string) ([]string, error) {
	var registry domain.ProjectRegistry
	if err := readJSONCompletionAsset(filepath.Join(root, ".pinax", "projects.json"), &registry); err != nil {
		return nil, err
	}
	items := make([]string, 0, len(registry.Projects))
	for _, project := range registry.Projects {
		slug := strings.TrimSpace(project.Slug)
		if slug == "" {
			continue
		}
		description := strings.TrimSpace(project.Name)
		if description == "" {
			description = "project"
		}
		items = append(items, slug+"\t"+description)
	}
	sort.Strings(items)
	return items, nil
}

func folderCompletionItems(root string) ([]string, error) {
	var registry domain.FolderRegistry
	if err := readJSONCompletionAsset(filepath.Join(root, ".pinax", "folders.json"), &registry); err != nil {
		return nil, err
	}
	items := make([]string, 0, len(registry.Folders))
	for _, folder := range registry.Folders {
		path := strings.TrimSpace(folder.Path)
		if path == "" {
			continue
		}
		description := string(folder.Purpose)
		if description == "" {
			description = "folder"
		}
		items = append(items, path+"\t"+description)
	}
	sort.Strings(items)
	return items, nil
}

func backendCompletionItems(root string) ([]string, error) {
	var registry domain.BackendRegistry
	if err := readJSONCompletionAsset(filepath.Join(root, ".pinax", "backends.json"), &registry); err != nil {
		return nil, err
	}
	items := make([]string, 0, len(registry.Backends))
	for _, backend := range registry.Backends {
		name := strings.TrimSpace(backend.Name)
		if name == "" {
			continue
		}
		description := string(backend.Kind)
		if description == "" {
			description = "backend"
		}
		items = append(items, name+"\t"+description)
	}
	sort.Strings(items)
	return items, nil
}

func promptAssetCompletionItems(root string) ([]string, error) {
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(indexPath))), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	q := query.Use(db)
	records, err := q.PromptAssetRecord.WithContext(context.Background()).Find()
	if err != nil {
		return nil, err
	}
	items := make([]string, 0, len(records))
	for _, record := range records {
		description := strings.TrimSpace(record.Title)
		if description == "" {
			description = "prompt"
		}
		items = append(items, record.PromptAssetID+"\t"+description)
	}
	sort.Strings(items)
	return items, nil
}

func pluginCompletionItems(root string) ([]string, error) {
	registry, err := loadPluginRegistry(root)
	if err != nil {
		return nil, err
	}
	items := make([]string, 0, len(registry.Plugins))
	for _, p := range registry.Plugins {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		description := strings.TrimSpace(p.Name)
		if description == "" {
			description = "plugin"
		}
		items = append(items, id+"\t"+description)
	}
	sort.Strings(items)
	return items, nil
}

func pluginCapabilityCompletionItems(root, pluginID string) ([]string, error) {
	registry, err := loadPluginRegistry(root)
	if err != nil {
		return nil, err
	}
	installed, ok := pinaxplugin.FindPlugin(registry, pluginID)
	if !ok {
		return []string{}, nil
	}
	capabilities := installed.Capabilities
	items := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		id := strings.TrimSpace(capability.ID)
		if id == "" {
			continue
		}
		description := strings.TrimSpace(capability.Kind)
		if description == "" {
			description = "capability"
		}
		items = append(items, id+"\t"+description)
	}
	sort.Strings(items)
	return items, nil
}

func syncConflictCompletionItems(root string) ([]string, error) {
	items := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".conflict.md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		items = append(items, filepath.ToSlash(rel)+"\tconflict")
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(items)
	return items, nil
}

func loadPluginRegistry(root string) (pinaxplugin.Registry, error) {
	var registry pinaxplugin.Registry
	err := readJSONCompletionAsset(filepath.Join(root, ".pinax", "plugins", "registry.json"), &registry)
	if err != nil {
		return pinaxplugin.Registry{}, err
	}
	return registry, nil
}

func readJSONCompletionAsset(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return json.Unmarshal(body, target)
}
