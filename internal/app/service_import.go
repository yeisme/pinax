package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func planMarkdownImport(root, source string, req ImportMarkdownRequest) ([]domain.ImportPlan, error) {
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &domain.CommandError{Code: "import_source_missing", Message: "Import source does not exist", Hint: "Check the Markdown file or directory path"}
	}
	if err != nil {
		return nil, err
	}
	sources := []string{}
	if info.IsDir() {
		if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".md") {
				sources = append(sources, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	} else if strings.EqualFold(filepath.Ext(source), ".md") {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	plans := make([]domain.ImportPlan, 0, len(sources))
	used := map[string]bool{}
	for _, item := range sources {
		targetRel, err := importTargetRel(source, item, info.IsDir(), req)
		if err != nil {
			return nil, err
		}
		plan := domain.ImportPlan{SourcePath: item, TargetPath: targetRel, Status: "write"}
		if used[targetRel] || fileExistsPath(root, targetRel) {
			plan.Conflict = "exists"
			switch strings.TrimSpace(req.Conflict) {
			case "rename":
				plan.TargetPath, err = uniqueImportRel(root, targetRel, used)
				if err != nil {
					return nil, err
				}
				plan.Status = "rename"
			case "overwrite":
				plan.Status = "overwrite"
			case "skip", "":
				plan.Status = "skip"
			default:
				return nil, &domain.CommandError{Code: "invalid_import_conflict", Message: "Unknown import conflict policy", Hint: "Use --conflict skip, rename, or overwrite"}
			}
		}
		used[plan.TargetPath] = true
		plans = append(plans, plan)
	}
	return plans, nil
}

func importTargetRel(sourceRoot, sourceFile string, sourceIsDir bool, req ImportMarkdownRequest) (string, error) {
	name := filepath.Base(sourceFile)
	if sourceIsDir {
		rel, err := filepath.Rel(sourceRoot, sourceFile)
		if err != nil {
			return "", err
		}
		name = filepath.ToSlash(rel)
	}
	base := "notes"
	if strings.TrimSpace(req.Group) != "" {
		base = filepath.ToSlash(filepath.Join(base, strings.TrimSpace(req.Group)))
	}
	if strings.TrimSpace(req.Folder) != "" {
		folder, err := validateOptionalNoteFolder(req.Folder)
		if err != nil {
			return "", err
		}
		base = filepath.ToSlash(filepath.Join(base, folder))
	}
	return validateNoteDir(filepath.ToSlash(filepath.Join(base, name)))
}

func uniqueImportRel(root, targetRel string, used map[string]bool) (string, error) {
	dir := filepath.Dir(targetRel)
	base := filepath.Base(targetRel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	ext := filepath.Ext(base)
	for i := 2; i < 1000; i++ {
		candidate := filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext)))
		if !used[candidate] && !fileExistsPath(root, candidate) {
			return candidate, nil
		}
	}
	return "", &domain.CommandError{Code: "import_name_conflict", Message: "Too many import filename conflicts", Hint: "Choose another target group or filename, then retry"}
}

func fileExistsPath(root, rel string) bool {
	path, err := safeJoin(root, rel)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func countImportPlans(plans []domain.ImportPlan, status string) int {
	count := 0
	for _, plan := range plans {
		if plan.Status == status {
			count++
		}
	}
	return count
}
