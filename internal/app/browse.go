package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// BrowseRequest 是 pinax browse 的请求。Path 是 vault 相对目录（空 = vault 根）。
type BrowseRequest struct {
	VaultPath string
	Path      string
	// LazyIndex 接受与 search 相同的 --lazy-index 值；browse 无论如何都不写索引，
	// 该值只用于输出 facts 透传，保证 flag 语义显式。
	LazyIndex string
}

// BrowseSubfolder 是子目录行（含其下全部 note 计数）。
type BrowseSubfolder struct {
	Path      string `json:"path"`
	NoteCount int    `json:"note_count"`
}

// BrowseItem 是该层 note 行（按 updated 降序，带信任/新鲜度徽标数据）。
type BrowseItem struct {
	Title       string `json:"title"`
	Path        string `json:"path"`
	Kind        string `json:"kind,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Trust       string `json:"trust"`
	Fresh       string `json:"fresh"`
	Description string `json:"description,omitempty"`
}

// BrowseResult 是合成目录导航视图（只读，不写任何 vault 文件）。
type BrowseResult struct {
	Path       string            `json:"path"`
	Notes      int               `json:"notes"`
	Subfolders []BrowseSubfolder `json:"subfolders,omitempty"`
	Items      []BrowseItem      `json:"items,omitempty"`
}

// Browse 从 note 扫描投影合成目录视图。只读：绝不创建或修改 index.md，
// 绝不写 .pinax/index.sqlite（与 --lazy-index 取值无关）。
func (s *Service) Browse(_ context.Context, req BrowseRequest) (domain.Projection, error) {
	command := "browse"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	requested := normalizeBrowsePath(req.Path)
	dirNotes := map[string][]domain.Note{}
	dirs := map[string]bool{"": true}
	for _, note := range notes {
		dir := browseDirOf(note.Path)
		dirs[dir] = true
		dirNotes[dir] = append(dirNotes[dir], note)
	}
	if !dirs[requested] {
		available := browseAvailableDirs(dirs)
		err := &domain.CommandError{Code: "browse_path_not_found", Message: "browse path has no notes: " + requested, Hint: "Available folders: " + available}
		return domain.NewErrorProjection(command, err), err
	}

	result := BrowseResult{Path: requested}
	layer := dirNotes[requested]
	result.Notes = len(layer)
	result.Items = make([]BrowseItem, 0, len(layer))
	for _, note := range layer {
		result.Items = append(result.Items, BrowseItem{
			Title:       note.Title,
			Path:        note.Path,
			Kind:        note.Kind,
			UpdatedAt:   note.UpdatedAt,
			Trust:       domain.TrustTierOf(note.Trust),
			Fresh:       domain.FreshnessOf(note.Trust, s.currentTimeUTC()),
			Description: browseNoteDescription(note),
		})
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].UpdatedAt != result.Items[j].UpdatedAt {
			return result.Items[i].UpdatedAt > result.Items[j].UpdatedAt
		}
		return result.Items[i].Path < result.Items[j].Path
	})
	result.Subfolders = browseSubfolders(dirNotes, requested)

	projection := domain.NewProjection(command, "Directory view composed.")
	projection.Facts["path"] = requested
	projection.Facts["notes"] = fmt.Sprint(result.Notes)
	projection.Facts["subfolders"] = fmt.Sprint(len(result.Subfolders))
	projection.Facts["writes"] = "false"
	projection.Facts["index_writes"] = "false"
	if value := strings.TrimSpace(req.LazyIndex); value != "" {
		projection.Facts["lazy_index"] = value
	}
	projection.Data = result
	if len(result.Subfolders) > 0 {
		projection.Actions = append(projection.Actions, domain.Action{Name: "open_folder", Command: fmt.Sprintf("pinax browse %s --vault %s", shellQuote(result.Subfolders[0].Path), shellQuote(root))})
	}
	if len(result.Items) > 0 {
		projection.Actions = append(projection.Actions, domain.Action{Name: "show", Command: fmt.Sprintf("pinax search show %s --vault %s", shellQuote(result.Items[0].Path), shellQuote(root))})
	}
	return projection, nil
}

func normalizeBrowsePath(raw string) string {
	value := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	if value == "." {
		return ""
	}
	return value
}

func browseDirOf(notePath string) string {
	dir := filepath.ToSlash(filepath.Dir(filepath.ToSlash(notePath)))
	if dir == "." || dir == "/" {
		return ""
	}
	return dir
}

func browseAvailableDirs(dirs map[string]bool) string {
	top := make([]string, 0, len(dirs))
	for dir := range dirs {
		if dir != "" {
			top = append(top, dir)
		}
	}
	sort.Strings(top)
	if len(top) > 8 {
		top = top[:8]
	}
	if len(top) == 0 {
		return "(none)"
	}
	return strings.Join(top, ", ")
}

// browseSubfolders 计算直接子目录及其递归 note 计数。
func browseSubfolders(dirNotes map[string][]domain.Note, current string) []BrowseSubfolder {
	counts := map[string]int{}
	for dir, notes := range dirNotes {
		if dir == "" {
			continue
		}
		prefix := ""
		if current != "" {
			prefix = current + "/"
		}
		if !strings.HasPrefix(dir+"/", prefix) || dir+"/" == prefix {
			continue
		}
		rest := strings.TrimPrefix(dir, prefix)
		child := strings.SplitN(rest, "/", 2)[0]
		counts[prefix+child] += len(notes)
	}
	folders := make([]BrowseSubfolder, 0, len(counts))
	for path, count := range counts {
		folders = append(folders, BrowseSubfolder{Path: path, NoteCount: count})
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Path < folders[j].Path })
	return folders
}

// browseNoteDescription 取 note summary/description frontmatter（无则空）。
func browseNoteDescription(note domain.Note) string {
	for _, key := range []string{"description", "summary"} {
		if value := strings.TrimSpace(note.Frontmatter[key]); value != "" {
			return boundedSnippet(value)
		}
	}
	return ""
}
