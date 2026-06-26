package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

var defaultProjectWorkspaceDirs = []string{
	"charter",
	"inbox",
	"sources",
	"runs",
	"outputs",
	"retros",
	"tool-candidates",
}

type ProjectWorkspaceRequest struct {
	VaultPath  string
	Project    string
	Subproject string
	Title      string
	Template   string
	DryRun     bool
}

type ProjectLearningRequest struct {
	VaultPath      string
	Project        string
	Subproject     string
	Title          string
	ProjectName    string
	NotesPrefix    string
	Preset         string
	DryRun         bool
	NoStarterItems bool
}

var learningBoardColumns = []string{"inbox", "planned", "learning", "practice", "review", "retrospective", "done"}

func (s *Service) ProjectSubprojectCreate(_ context.Context, req ProjectWorkspaceRequest) (domain.Projection, error) {
	root, project, subproject, err := validateProjectWorkspaceRequest(req)
	if err != nil {
		return errorProjection("project.subproject.create", err), err
	}
	workspace, err := buildProjectWorkspace(root, project, subproject, req.Title, req.Template)
	if err != nil {
		return errorProjection("project.subproject.create", err), err
	}
	if req.DryRun {
		projection := projectWorkspaceProjection(root, "project.subproject.create", "Project subproject workspace creation planned.", workspace)
		projection.Facts["dry_run"] = "true"
		projection.Facts["writes"] = "false"
		projection.Data = map[string]any{"workspace": workspace, "dry_run": true, "operations": projectWorkspaceCreateOperations(workspace), "vault_root": root, "workspace_full_path": workspaceFullPath(root, workspace.WorkspacePath)}
		return projection, nil
	}
	for _, dir := range workspace.Directories {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir.Path)), 0o755); err != nil {
			return errorProjection("project.subproject.create", err), err
		}
	}
	workspace.Directories = workspaceDirectoryStatuses(root, workspace.WorkspacePath)
	if err := saveProjectWorkspace(root, workspace); err != nil {
		return errorProjection("project.subproject.create", err), err
	}
	_ = appendEvent(root, "project.subproject.create", "success", map[string]string{"project": project.Slug, "subproject": subproject, "workspace_path": workspace.WorkspacePath})
	projection := projectWorkspaceProjection(root, "project.subproject.create", "Project subproject workspace created.", workspace)
	projection.Facts["dry_run"] = "false"
	projection.Facts["writes"] = "true"
	projection.Evidence = []string{projectWorkspaceRegistryRel(project.Slug, subproject), filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Actions = []domain.Action{{Name: "board_show", Command: fmt.Sprintf("pinax project board show %s --subproject %s --vault %s", shellQuote(project.Slug), shellQuote(subproject), shellQuote(root))}}
	return projection, nil
}

func (s *Service) ProjectLearningInit(ctx context.Context, req ProjectLearningRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.learning.init", err), err
	}
	projectSlug, err := validateLearningProjectRequest(&req)
	if err != nil {
		return errorProjection("project.learning.init", err), err
	}
	if req.DryRun {
		projection := domain.NewProjection("project.learning.init", "Learning project initialization planned.")
		projection.Facts["project"] = projectSlug
		projection.Facts["subproject"] = req.Subproject
		projection.Facts["preset"] = req.Preset
		projection.Facts["title"] = req.Title
		projection.Facts["writes"] = "false"
		projection.Facts["dry_run"] = "true"
		projection.Data = map[string]any{"operations": learningInitOperations(req)}
		return projection, nil
	}
	if _, err := s.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: projectSlug, Name: req.ProjectName, NotesPrefix: req.NotesPrefix}); err != nil {
		return errorProjection("project.learning.init", err), err
	}
	workspaceProjection, err := s.ProjectSubprojectCreate(ctx, ProjectWorkspaceRequest{VaultPath: root, Project: projectSlug, Subproject: req.Subproject, Title: req.Title, Template: "long-term-learning"})
	if err != nil {
		return errorProjection("project.learning.init", err), err
	}
	if _, err := s.ProjectBoardConfigure(ctx, ProjectBoardRequest{VaultPath: root, Project: projectSlug, Subproject: req.Subproject, Columns: learningBoardColumns}); err != nil {
		return errorProjection("project.learning.init", err), err
	}
	workspace := workspaceProjection.Data.(map[string]any)["workspace"].(domain.ProjectWorkspace)
	notesCreated, notePaths, err := s.createLearningStarterNotes(ctx, root, projectSlug, req, workspace)
	if err != nil {
		return errorProjection("project.learning.init", err), err
	}
	itemsCreated := 0
	itemPaths := []string{}
	if !req.NoStarterItems {
		itemsCreated, itemPaths, err = s.createLearningStarterItems(ctx, root, projectSlug, req)
		if err != nil {
			return errorProjection("project.learning.init", err), err
		}
	}
	projection := domain.NewProjection("project.learning.init", "Learning project initialized.")
	projection.Facts["project"] = projectSlug
	projection.Facts["subproject"] = req.Subproject
	projection.Facts["preset"] = req.Preset
	projection.Facts["title"] = req.Title
	projection.Facts["workspace_path"] = workspace.WorkspacePath
	projection.Facts["columns"] = fmt.Sprint(len(learningBoardColumns))
	projection.Facts["notes.created"] = fmt.Sprint(notesCreated)
	projection.Facts["items.created"] = fmt.Sprint(itemsCreated)
	projection.Facts["writes"] = "true"
	projection.Facts["dry_run"] = "false"
	projection.Evidence = append([]string{projectWorkspaceRegistryRel(projectSlug, req.Subproject), projectBoardConfigRel(projectSlug, req.Subproject)}, notePaths...)
	projection.Evidence = append(projection.Evidence, itemPaths...)
	projection.Data = map[string]any{"learning_project": map[string]any{"project": projectSlug, "subproject": req.Subproject, "preset": req.Preset, "workspace": workspace, "columns": learningBoardColumns, "starter_notes": notePaths, "starter_items": itemPaths}}
	projection.Actions = []domain.Action{{Name: "board_show", Command: fmt.Sprintf("pinax project board show %s --subproject %s --vault %s", shellQuote(projectSlug), shellQuote(req.Subproject), shellQuote(root))}}
	_ = appendEvent(root, "project.learning.init", "success", map[string]string{"project": projectSlug, "subproject": req.Subproject, "preset": req.Preset})
	return projection, nil
}

func validateLearningProjectRequest(req *ProjectLearningRequest) (string, error) {
	projectSlug := strings.TrimSpace(req.Project)
	if err := validateProjectSlug(projectSlug); err != nil {
		return "", err
	}
	subproject, commandErr := validateSubprojectSlug(req.Subproject)
	if commandErr != nil {
		return "", commandErr
	}
	req.Subproject = subproject
	req.Preset = strings.TrimSpace(req.Preset)
	if req.Preset == "" {
		req.Preset = "learning"
	}
	switch req.Preset {
	case "learning", "stock-learning":
	default:
		return "", &domain.CommandError{Code: "invalid_learning_preset", Message: "Unknown learning project preset", Hint: "Use --preset learning or --preset stock-learning"}
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		req.Title = subproject
	}
	req.ProjectName = strings.TrimSpace(req.ProjectName)
	if req.ProjectName == "" {
		req.ProjectName = req.Title
	}
	req.NotesPrefix = filepath.ToSlash(strings.TrimSpace(req.NotesPrefix))
	if req.NotesPrefix == "" {
		req.NotesPrefix = filepath.ToSlash(filepath.Join("notes", projectSlug))
	}
	if err := validateProjectPrefix(req.NotesPrefix); err != nil {
		return "", err
	}
	return projectSlug, nil
}

func learningInitOperations(req ProjectLearningRequest) []domain.PlanOperation {
	workspacePath := filepath.ToSlash(filepath.Join("notes", "projects", req.Project, req.Subproject))
	ops := []domain.PlanOperation{
		{Kind: "ensure_project", Path: filepath.ToSlash(filepath.Join(".pinax", "projects.json")), Reason: "Create or reuse the learning project", Status: "planned"},
		{Kind: "ensure_project_workspace", Path: projectWorkspaceRegistryRel(req.Project, req.Subproject), Reason: "Create or reuse the learning workspace", Status: "planned"},
		{Kind: "write_project_board_config", Path: projectBoardConfigRel(req.Project, req.Subproject), Reason: "Configure learning board columns", Status: "planned"},
	}
	for _, rel := range learningStarterNotePaths(workspacePath) {
		ops = append(ops, domain.PlanOperation{Kind: "ensure_note", Path: rel, Reason: "Create starter learning note if missing", Status: "planned"})
	}
	if !req.NoStarterItems {
		for _, title := range learningStarterItemTitles(req.Preset) {
			rel := filepath.ToSlash(filepath.Join(workspacePath, "inbox", safeBoardItemSlug(title)+".md"))
			ops = append(ops, domain.PlanOperation{Kind: "ensure_project_item", Path: rel, Reason: "Create starter learning board item if missing", Status: "planned"})
		}
	}
	return ops
}

type learningStarterNote struct {
	Title string
	Slug  string
	Dir   string
	Kind  string
	Tags  []string
	Body  string
}

func (s *Service) createLearningStarterNotes(ctx context.Context, root, projectSlug string, req ProjectLearningRequest, workspace domain.ProjectWorkspace) (int, []string, error) {
	notes := learningStarterNotes(req, workspace.WorkspacePath)
	created := 0
	paths := make([]string, 0, len(notes))
	for _, note := range notes {
		rel := filepath.ToSlash(filepath.Join(note.Dir, note.Slug+".md"))
		paths = append(paths, rel)
		if fileExistsPath(root, rel) {
			continue
		}
		if _, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Project: projectSlug, Title: note.Title, Dir: note.Dir, Slug: note.Slug, Kind: note.Kind, Status: "active", Tags: note.Tags, Body: note.Body}); err != nil {
			return created, paths, err
		}
		created++
	}
	return created, paths, nil
}

type learningStarterItem struct {
	Title    string
	Column   string
	Labels   []string
	Priority string
	Body     string
}

func (s *Service) createLearningStarterItems(ctx context.Context, root, projectSlug string, req ProjectLearningRequest) (int, []string, error) {
	workspacePath := filepath.ToSlash(filepath.Join("notes", "projects", projectSlug, req.Subproject))
	items := learningStarterItems(req)
	created := 0
	paths := make([]string, 0, len(items))
	for _, item := range items {
		rel := filepath.ToSlash(filepath.Join(workspacePath, "inbox", safeBoardItemSlug(item.Title)+".md"))
		paths = append(paths, rel)
		if fileExistsPath(root, rel) {
			continue
		}
		if _, err := s.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: projectSlug, Subproject: req.Subproject, Title: item.Title, Column: item.Column, Labels: item.Labels, Priority: item.Priority, Body: item.Body}); err != nil {
			return created, paths, err
		}
		created++
	}
	return created, paths, nil
}

func learningStarterNotes(req ProjectLearningRequest, workspacePath string) []learningStarterNote {
	boundary := ""
	if req.Preset == "stock-learning" {
		boundary = "\n\n## 边界\n\n本项目仅用于学习、资料整理、历史案例复盘、模拟练习和风险原则记录；不构成投资建议、买卖建议、收益承诺或自动交易决策。"
	}
	return []learningStarterNote{
		{
			Title: "学习项目章程",
			Slug:  "learning-charter",
			Dir:   filepath.ToSlash(filepath.Join(workspacePath, "charter")),
			Kind:  "learning",
			Tags:  []string{"learning", "charter", req.Preset},
			Body:  fmt.Sprintf("## 目标\n\n- 主题：%s\n- 项目：%s\n- 先建立术语、资料来源、练习记录和复盘节奏。\n\n## 节奏\n\n- 每周补齐资料来源和复盘。\n- 把错误、疑问和风险原则单独沉淀。%s\n", req.Title, req.ProjectName, boundary),
		},
		{
			Title: "资料来源索引",
			Slug:  "source-index",
			Dir:   filepath.ToSlash(filepath.Join(workspacePath, "sources")),
			Kind:  "reference",
			Tags:  []string{"learning", "source", req.Preset},
			Body:  "## 资料来源\n\n| 来源 | 类型 | 可信度 | 关键主题 | 笔记 |\n| --- | --- | --- | --- | --- |\n|  |  |  |  |  |\n\n## 待核验\n\n- \n",
		},
		{
			Title: "每周复盘",
			Slug:  "weekly-review",
			Dir:   filepath.ToSlash(filepath.Join(workspacePath, "retros")),
			Kind:  "review",
			Tags:  []string{"learning", "weekly-review", req.Preset},
			Body:  "## 本周学习\n\n- \n\n## 已掌握\n\n- \n\n## 仍然模糊\n\n- \n\n## 错误与修正\n\n- \n\n## 下周动作\n\n- \n",
		},
	}
}

func learningStarterNotePaths(workspacePath string) []string {
	return []string{
		filepath.ToSlash(filepath.Join(workspacePath, "charter", "learning-charter.md")),
		filepath.ToSlash(filepath.Join(workspacePath, "sources", "source-index.md")),
		filepath.ToSlash(filepath.Join(workspacePath, "retros", "weekly-review.md")),
	}
}

func learningStarterItems(req ProjectLearningRequest) []learningStarterItem {
	if req.Preset == "stock-learning" {
		return []learningStarterItem{
			{Title: "建立交易术语表", Column: "planned", Labels: []string{"learning", "glossary"}, Priority: "high", Body: "## Definition of Done\n\n- 覆盖基础术语。\n- 每个术语写清来源和例子。\n"},
			{Title: "整理 K 线与成交量基础", Column: "learning", Labels: []string{"learning", "technical-analysis"}, Body: "## Definition of Done\n\n- 记录概念、典型误读和练习案例。\n"},
			{Title: "记录风险原则", Column: "review", Labels: []string{"risk", "rule"}, Priority: "high", Body: "## Definition of Done\n\n- 写下不做什么、何时停止、如何复盘。\n"},
			{Title: "完成第一周复盘", Column: "retrospective", Labels: []string{"review", "weekly"}, Body: "## Definition of Done\n\n- 汇总本周资料、疑问、错误和下周动作。\n"},
		}
	}
	return []learningStarterItem{
		{Title: "建立术语表", Column: "planned", Labels: []string{"learning", "glossary"}, Priority: "high", Body: "## Definition of Done\n\n- 收集核心术语和例子。\n"},
		{Title: "整理第一批资料来源", Column: "learning", Labels: []string{"learning", "source"}, Body: "## Definition of Done\n\n- 记录来源、可信度和对应笔记。\n"},
		{Title: "安排一次练习", Column: "practice", Labels: []string{"practice"}, Body: "## Definition of Done\n\n- 完成练习记录和结果复盘。\n"},
		{Title: "完成第一周复盘", Column: "retrospective", Labels: []string{"review", "weekly"}, Body: "## Definition of Done\n\n- 汇总本周学习、问题和下周动作。\n"},
	}
}

func learningStarterItemTitles(preset string) []string {
	items := learningStarterItems(ProjectLearningRequest{Preset: preset})
	titles := make([]string, 0, len(items))
	for _, item := range items {
		titles = append(titles, item.Title)
	}
	return titles
}

func projectWorkspaceCreateOperations(workspace domain.ProjectWorkspace) []domain.PlanOperation {
	ops := make([]domain.PlanOperation, 0, len(workspace.Directories)+1)
	ops = append(ops, domain.PlanOperation{Kind: "write_project_workspace", Path: projectWorkspaceRegistryRel(workspace.Project, workspace.Subproject), Reason: "Register project subproject workspace", Status: "planned"})
	for _, dir := range workspace.Directories {
		ops = append(ops, domain.PlanOperation{Kind: "mkdir", Path: dir.Path, Reason: "Create workspace directory", Status: "planned"})
	}
	return ops
}

func (s *Service) ProjectSubprojectList(_ context.Context, req ProjectWorkspaceRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.subproject.list", err), err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return errorProjection("project.subproject.list", err), err
	}
	workspaces, err := listProjectWorkspaces(root, project.Slug)
	if err != nil {
		return errorProjection("project.subproject.list", err), err
	}
	projection := domain.NewProjection("project.subproject.list", "Project subprojects listed.")
	projection.Facts["project"] = project.Slug
	projection.Facts["subprojects"] = fmt.Sprint(len(workspaces))
	for i, workspace := range workspaces {
		projection.Facts[fmt.Sprintf("subproject.%d", i+1)] = workspace.Subproject
		projection.Facts[fmt.Sprintf("workspace.%d", i+1)] = workspace.WorkspacePath
	}
	projection.Data = map[string]any{"project": project, "subprojects": workspaces}
	projection.Actions = []domain.Action{{Name: "create", Command: fmt.Sprintf("pinax project subproject create %s <slug> --vault %s --json", shellQuote(project.Slug), shellQuote(root))}}
	return projection, nil
}

func (s *Service) ProjectSubprojectShow(_ context.Context, req ProjectWorkspaceRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.subproject.show", err), err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return errorProjection("project.subproject.show", err), err
	}
	subproject, pathErr := validateSubprojectSlug(req.Subproject)
	if pathErr != nil {
		return domain.NewErrorProjection("project.subproject.show", pathErr), pathErr
	}
	workspace, err := loadProjectWorkspace(root, project.Slug, subproject)
	if err != nil {
		return errorProjection("project.subproject.show", err), err
	}
	workspace.Directories = workspaceDirectoryStatuses(root, workspace.WorkspacePath)
	projection := projectWorkspaceProjection(root, "project.subproject.show", "Project subproject workspace read.", workspace)
	projection.Actions = []domain.Action{{Name: "board_show", Command: fmt.Sprintf("pinax project board show %s --subproject %s --vault %s", shellQuote(project.Slug), shellQuote(subproject), shellQuote(root))}}
	return projection, nil
}

func validateProjectWorkspaceRequest(req ProjectWorkspaceRequest) (string, domain.Project, string, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return "", domain.Project{}, "", err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return "", domain.Project{}, "", err
	}
	subproject, commandErr := validateSubprojectSlug(req.Subproject)
	if commandErr != nil {
		return "", domain.Project{}, "", commandErr
	}
	return root, project, subproject, nil
}

func validateSubprojectSlug(slug string) (string, *domain.CommandError) {
	value := strings.TrimSpace(slug)
	if err := validateProjectSlug(value); err != nil {
		if commandErr, ok := err.(*domain.CommandError); ok {
			return "", commandErr
		}
		return "", &domain.CommandError{Code: "invalid_subproject_slug", Message: err.Error()}
	}
	switch value {
	case "temp", "dist", "node_modules", "vendor":
		return "", &domain.CommandError{Code: "reserved_subproject_slug", Message: "Subproject slug is reserved", Hint: "Choose a workspace slug that is not temp, dist, node_modules, or vendor"}
	}
	return value, nil
}

func buildProjectWorkspace(root string, project domain.Project, subproject, title, template string) (domain.ProjectWorkspace, error) {
	if strings.TrimSpace(title) == "" {
		title = subproject
	}
	if strings.TrimSpace(template) == "" {
		template = "scenario"
	}
	workspacePath := filepath.ToSlash(filepath.Join("notes", "projects", project.Slug, subproject))
	if err := validateProjectPrefix(workspacePath); err != nil {
		return domain.ProjectWorkspace{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if existing, err := loadProjectWorkspace(root, project.Slug, subproject); err == nil {
		now = existing.CreatedAt
	}
	return domain.ProjectWorkspace{SchemaVersion: domain.ProjectWorkspaceSchemaVersion, Project: project.Slug, Subproject: subproject, Title: title, Template: template, WorkspacePath: workspacePath, Directories: workspaceDirectoryStatuses(root, workspacePath), Status: "active", CreatedAt: now, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func workspaceDirectoryStatuses(root, workspacePath string) []domain.ProjectWorkspaceDirectory {
	dirs := make([]domain.ProjectWorkspaceDirectory, 0, len(defaultProjectWorkspaceDirs))
	for _, name := range defaultProjectWorkspaceDirs {
		rel := filepath.ToSlash(filepath.Join(workspacePath, name))
		status := "missing"
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && info.IsDir() {
			status = "ok"
		}
		dirs = append(dirs, domain.ProjectWorkspaceDirectory{Name: name, Path: rel, Status: status})
	}
	return dirs
}

func saveProjectWorkspace(root string, workspace domain.ProjectWorkspace) error {
	workspace.SchemaVersion = domain.ProjectWorkspaceSchemaVersion
	workspace.Directories = workspaceDirectoryStatuses(root, workspace.WorkspacePath)
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(projectWorkspaceRegistryRel(workspace.Project, workspace.Subproject))), workspace); err != nil {
		return err
	}
	return saveCurrentWorkspace(root, workspace)
}

func saveCurrentWorkspace(root string, workspace domain.ProjectWorkspace) error {
	current := domain.CurrentWorkspace{SchemaVersion: domain.CurrentWorkspaceSchemaVersion, Project: workspace.Project, Subproject: workspace.Subproject, WorkspacePath: workspace.WorkspacePath, UpdatedAt: workspace.UpdatedAt}
	return writeJSONAsset(filepath.Join(root, ".pinax", "workspaces", "current.json"), current)
}

func loadProjectWorkspace(root, project, subproject string) (domain.ProjectWorkspace, error) {
	var workspace domain.ProjectWorkspace
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(projectWorkspaceRegistryRel(project, subproject))))
	if errors.Is(err, os.ErrNotExist) {
		return workspace, &domain.CommandError{Code: "subproject_not_found", Message: "Project subproject not found", Hint: "Run pinax project subproject list <project> --vault <vault>"}
	}
	if err != nil {
		return workspace, err
	}
	if err := json.Unmarshal(b, &workspace); err != nil {
		return workspace, err
	}
	if workspace.SchemaVersion == "" {
		workspace.SchemaVersion = domain.ProjectWorkspaceSchemaVersion
	}
	return workspace, nil
}

func listProjectWorkspaces(root, project string) ([]domain.ProjectWorkspace, error) {
	dir := filepath.Join(root, ".pinax", "project-workspaces", project)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	workspaces := make([]domain.ProjectWorkspace, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		subproject := strings.TrimSuffix(entry.Name(), ".json")
		workspace, err := loadProjectWorkspace(root, project, subproject)
		if err != nil {
			return nil, err
		}
		workspace.Directories = workspaceDirectoryStatuses(root, workspace.WorkspacePath)
		workspaces = append(workspaces, workspace)
	}
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].Subproject < workspaces[j].Subproject })
	return workspaces, nil
}

func projectWorkspaceProjection(root, command, summary string, workspace domain.ProjectWorkspace) domain.Projection {
	projection := domain.NewProjection(command, summary)
	fullPath := workspaceFullPath(root, workspace.WorkspacePath)
	projection.Facts["project"] = workspace.Project
	projection.Facts["subproject"] = workspace.Subproject
	projection.Facts["title"] = workspace.Title
	projection.Facts["template"] = workspace.Template
	projection.Facts["vault_root"] = root
	projection.Facts["workspace_path"] = workspace.WorkspacePath
	projection.Facts["workspace.project"] = workspace.Project
	projection.Facts["workspace.subproject"] = workspace.Subproject
	projection.Facts["workspace.path"] = workspace.WorkspacePath
	projection.Facts["workspace.full_path"] = fullPath
	projection.Facts["directories"] = fmt.Sprint(len(workspace.Directories))
	projection.Facts["status"] = workspace.Status
	projection.Data = map[string]any{"workspace": workspace, "vault_root": root, "workspace_full_path": fullPath}
	return projection
}

func workspaceFullPath(root, workspacePath string) string {
	return filepath.Join(root, filepath.FromSlash(workspacePath))
}

func projectWorkspaceRegistryRel(project, subproject string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "project-workspaces", project, subproject+".json"))
}
