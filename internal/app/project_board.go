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
	noteindex "github.com/yeisme/pinax/internal/index"
)

type ProjectBoardRequest struct {
	VaultPath   string
	Project     string
	Subproject  string
	View        string
	NoteDisplay string
	Columns     []string
	GroupBy     string
	Sort        string
	Display     string
	Save        bool
	Format      string
	Compact     bool
}

type ProjectItemRequest struct {
	VaultPath  string
	Project    string
	Subproject string
	Action     string
	Title      string
	ItemID     string
	Column     string
	Body       string
	Labels     []string
	Milestone  string
	Priority   string
	DueAt      string
	BlockedBy  []string
	Yes        bool
}

type TaskAdoptRequest struct {
	VaultPath string
	ItemID    string
	Yes       bool
}

var defaultBoardColumns = []domain.BoardColumn{
	{ID: "inbox", Name: "Inbox", Order: 10},
	{ID: "next", Name: "Next", Order: 20},
	{ID: "doing", Name: "Doing", Order: 30},
	{ID: "blocked", Name: "Blocked", Order: 40},
	{ID: "review", Name: "Review", Order: 50},
	{ID: "done", Name: "Done", Order: 60},
}

func (s *Service) ProjectBoardShow(_ context.Context, req ProjectBoardRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.board.show", err), err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return errorProjection("project.board.show", err), err
	}
	var workspace *domain.ProjectWorkspace
	if strings.TrimSpace(req.Subproject) != "" {
		subproject, pathErr := validateSubprojectSlug(req.Subproject)
		if pathErr != nil {
			return domain.NewErrorProjection("project.board.show", pathErr), pathErr
		}
		loaded, loadErr := loadProjectWorkspace(root, project.Slug, subproject)
		if loadErr != nil {
			return errorProjection("project.board.show", loadErr), loadErr
		}
		loaded.Directories = workspaceDirectoryStatuses(root, loaded.WorkspacePath)
		workspace = &loaded
	}
	boardColumns, err := loadProjectBoardColumns(root, project.Slug, workspaceSubproject(workspace))
	if err != nil {
		return errorProjection("project.board.show", err), err
	}
	var savedView *domain.ProjectBoardView
	if strings.TrimSpace(req.View) != "" {
		view, viewErr := loadProjectBoardView(root, project.Slug, workspaceSubproject(workspace), req.View)
		if viewErr != nil {
			return errorProjection("project.board.show", viewErr), viewErr
		}
		savedView = &view
		if len(view.Columns) > 0 {
			var columnErr *domain.CommandError
			boardColumns, columnErr = configuredBoardColumns(view.Columns)
			if columnErr != nil {
				return domain.NewErrorProjection("project.board.show", columnErr), columnErr
			}
		}
		if strings.TrimSpace(req.NoteDisplay) == "" && strings.TrimSpace(view.Display) != "" {
			req.NoteDisplay = view.Display
		}
	}
	display, displayErr := parseNoteDisplayKind(req.NoteDisplay)
	if displayErr != nil {
		return domain.NewErrorProjection("project.board.show", displayErr), displayErr
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("project.board.show", err), err
	}
	engine, indexStatus := boardIndexState(root, notes)
	board := buildProjectBoard(root, project, workspace, boardColumns, ordinaryNotes(notes), display, engine, indexStatus, savedView != nil)
	projection := domain.NewProjection("project.board.show", boardHumanSummary(board))
	projection.Facts["project"] = board.ProjectSlug
	if board.Subproject != "" {
		projection.Facts["subproject"] = board.Subproject
		projection.Facts["workspace_path"] = board.WorkspacePath
	}
	if savedView != nil {
		projection.Facts["view"] = savedView.View
	}
	projection.Facts["columns"] = fmt.Sprint(len(board.Columns))
	projection.Facts["items"] = fmt.Sprint(len(board.Items))
	projection.Facts["next"] = fmt.Sprint(board.Facts.Next)
	projection.Facts["doing"] = fmt.Sprint(board.Facts.Doing)
	projection.Facts["blocked"] = fmt.Sprint(board.Facts.Blocked)
	projection.Facts["review"] = fmt.Sprint(board.Facts.Review)
	projection.Facts["done"] = fmt.Sprint(board.Facts.Done)
	for _, column := range board.Columns {
		projection.Facts["column."+column.ID] = fmt.Sprint(board.Facts.ColumnCounts[column.ID])
	}
	projection.Facts["warnings"] = fmt.Sprint(len(board.Warnings))
	projection.Facts["engine"] = board.Facts.Engine
	projection.Facts["index_status"] = board.Facts.IndexStatus
	projection.Facts["note_display"] = string(display)
	projection.Facts["compact"] = fmt.Sprint(req.Compact)
	agentContexts := projectBoardAgentContexts(board)
	projection.Data = map[string]any{"board": board, "agent_contexts": agentContexts}
	if workspace != nil {
		projection.Data = map[string]any{"board": board, "workspace": *workspace, "agent_contexts": agentContexts}
	}
	if savedView != nil {
		data := projection.Data.(map[string]any)
		data["view"] = *savedView
		projection.Data = data
	}
	if indexStatus == "missing" || indexStatus == "stale" {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}
	} else {
		action := fmt.Sprintf("pinax project board plan %s --vault %s --save", project.Slug, shellQuote(root))
		if board.Subproject != "" {
			action = fmt.Sprintf("pinax project board plan %s --subproject %s --vault %s --save", project.Slug, shellQuote(board.Subproject), shellQuote(root))
		}
		projection.Actions = []domain.Action{{Name: "board_plan", Command: action}}
	}
	return projection, nil
}

func projectBoardAgentContexts(board domain.ProjectBoard) []domain.AgentContext {
	contexts := make([]domain.AgentContext, 0, len(board.Items))
	for _, item := range board.Items {
		if item.AgentContext != nil {
			contexts = append(contexts, *item.AgentContext)
		}
	}
	return contexts
}

func (s *Service) ProjectBoardViewSave(_ context.Context, req ProjectBoardRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.board.view.save", err), err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return errorProjection("project.board.view.save", err), err
	}
	subproject := ""
	if strings.TrimSpace(req.Subproject) != "" {
		var pathErr *domain.CommandError
		subproject, pathErr = validateSubprojectSlug(req.Subproject)
		if pathErr != nil {
			return domain.NewErrorProjection("project.board.view.save", pathErr), pathErr
		}
		if _, err := loadProjectWorkspace(root, project.Slug, subproject); err != nil {
			return errorProjection("project.board.view.save", err), err
		}
	}
	viewName := strings.TrimSpace(req.View)
	if viewName == "" || !isSafeBoardSlug(viewName) {
		err := &domain.CommandError{Code: "invalid_board_view", Message: "Board view name must be a safe slug", Hint: "Use a name like active or weekly-review"}
		return domain.NewErrorProjection("project.board.view.save", err), err
	}
	if _, err := configuredBoardColumns(req.Columns); err != nil {
		return domain.NewErrorProjection("project.board.view.save", err), err
	}
	view := domain.ProjectBoardView{SchemaVersion: domain.ProjectBoardViewSchemaVersion, ProjectSlug: project.Slug, Subproject: subproject, View: viewName, Columns: req.Columns, GroupBy: strings.TrimSpace(req.GroupBy), Sort: strings.TrimSpace(req.Sort), Display: strings.TrimSpace(req.Display), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	rel := projectBoardViewRel(project.Slug, subproject, viewName)
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), view); err != nil {
		return errorProjection("project.board.view.save", err), err
	}
	_ = appendEvent(root, "project.board.view.save", "success", map[string]string{"project": project.Slug, "subproject": subproject, "view": viewName, "saved_path": rel})
	projection := domain.NewProjection("project.board.view.save", "Project board view saved.")
	projection.Facts["project"] = project.Slug
	projection.Facts["view"] = viewName
	projection.Facts["columns"] = fmt.Sprint(len(req.Columns))
	projection.Facts["writes"] = "true"
	projection.Facts["saved_path"] = rel
	if subproject != "" {
		projection.Facts["subproject"] = subproject
	}
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"view": view}
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax project board show %s --view %s --vault %s --json", shellQuote(project.Slug), shellQuote(viewName), shellQuote(root))}}
	return projection, nil
}

func (s *Service) ProjectBoardConfigure(_ context.Context, req ProjectBoardRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.board.configure", err), err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return errorProjection("project.board.configure", err), err
	}
	subproject := ""
	if strings.TrimSpace(req.Subproject) != "" {
		var pathErr *domain.CommandError
		subproject, pathErr = validateSubprojectSlug(req.Subproject)
		if pathErr != nil {
			return domain.NewErrorProjection("project.board.configure", pathErr), pathErr
		}
		if _, err := loadProjectWorkspace(root, project.Slug, subproject); err != nil {
			return errorProjection("project.board.configure", err), err
		}
	}
	columns, configErr := configuredBoardColumns(req.Columns)
	if configErr != nil {
		return domain.NewErrorProjection("project.board.configure", configErr), configErr
	}
	config := domain.ProjectBoardConfig{SchemaVersion: domain.ProjectBoardSchemaVersion, ProjectSlug: project.Slug, Subproject: subproject, Columns: columns, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	rel := projectBoardConfigRel(project.Slug, subproject)
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorProjection("project.board.configure", err), err
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return errorProjection("project.board.configure", err), err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return errorProjection("project.board.configure", err), err
	}
	_ = appendEvent(root, "project.board.configure", "success", map[string]string{"project": project.Slug, "subproject": subproject, "saved_path": rel})
	projection := domain.NewProjection("project.board.configure", "Project board configuration saved.")
	projection.Facts["project"] = project.Slug
	if subproject != "" {
		projection.Facts["subproject"] = subproject
	}
	projection.Facts["columns"] = fmt.Sprint(len(columns))
	projection.Facts["saved_path"] = rel
	projection.Facts["writes"] = "true"
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"config": config}
	return projection, nil
}

func (s *Service) ProjectBoardPlan(ctx context.Context, req ProjectBoardRequest) (domain.Projection, error) {
	boardProjection, err := s.ProjectBoardShow(ctx, req)
	if err != nil {
		boardProjection.Command = "project.board.plan"
		return boardProjection, err
	}
	boardProjection.Command = "project.board.plan"
	boardProjection.Summary = "Project board plan generated."
	boardProjection.Facts["writes"] = "false"
	if !req.Save {
		return boardProjection, nil
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.board.plan", err), err
	}
	data, _ := boardProjection.Data.(map[string]any)
	board, _ := data["board"].(domain.ProjectBoard)
	snapshotID := "project-board-" + time.Now().UTC().Format("20060102T150405Z")
	board.SourceSnapshotID = snapshotID
	board.Facts.SnapshotID = snapshotID
	rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "project-boards", snapshotID+".json"))
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorProjection("project.board.plan", err), err
	}
	payload, err := json.Marshal(board)
	if err != nil {
		return errorProjection("project.board.plan", err), err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return errorProjection("project.board.plan", err), err
	}
	_ = appendEvent(root, "project.board.plan", "success", map[string]string{"project": req.Project, "snapshot_id": snapshotID, "saved_path": rel})
	boardProjection.Facts["writes"] = "true"
	boardProjection.Facts["snapshot_id"] = snapshotID
	boardProjection.Facts["saved_path"] = rel
	boardProjection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	boardProjection.Data = map[string]any{"board": board}
	return boardProjection, nil
}

func (s *Service) ProjectBoardExport(ctx context.Context, req ProjectBoardRequest) (domain.Projection, error) {
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" {
		err := &domain.CommandError{Code: "unsupported_board_export_format", Message: "project board export currently only supports markdown", Hint: "Use --format markdown"}
		return domain.NewErrorProjection("project.board.export", err), err
	}
	projection, err := s.ProjectBoardShow(ctx, req)
	if err != nil {
		projection.Command = "project.board.export"
		return projection, err
	}
	projection.Command = "project.board.export"
	projection.Summary = "Project board exported."
	projection.Facts["format"] = format
	projection.Facts["writes"] = "false"
	data, _ := projection.Data.(map[string]any)
	board, _ := data["board"].(domain.ProjectBoard)
	body := renderProjectBoardMarkdown(board)
	projection.Data = map[string]any{"board": board, "body": body}
	return projection, nil
}

func (s *Service) ProjectItemAdd(_ context.Context, req ProjectItemRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.item.add", err), err
	}
	project, err := findProject(root, req.Project)
	if err != nil {
		return errorProjection("project.item.add", err), err
	}
	var workspace *domain.ProjectWorkspace
	if strings.TrimSpace(req.Subproject) != "" {
		subproject, pathErr := validateSubprojectSlug(req.Subproject)
		if pathErr != nil {
			return domain.NewErrorProjection("project.item.add", pathErr), pathErr
		}
		loaded, loadErr := loadProjectWorkspace(root, project.Slug, subproject)
		if loadErr != nil {
			return errorProjection("project.item.add", loadErr), loadErr
		}
		loaded.Directories = workspaceDirectoryStatuses(root, loaded.WorkspacePath)
		workspace = &loaded
	}
	column := strings.TrimSpace(req.Column)
	if column == "" {
		column = "next"
	}
	boardColumns, err := loadProjectBoardColumns(root, project.Slug, workspaceSubproject(workspace))
	if err != nil {
		return errorProjection("project.item.add", err), err
	}
	if !isKnownBoardColumnIn(column, boardColumns) {
		err := &domain.CommandError{Code: "invalid_board_column", Message: "Unknown board column", Hint: "Use a configured board column"}
		return domain.NewErrorProjection("project.item.add", err), err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "project item add requires a title", Hint: "pinax project item add <project> <title> --column next"}
		return domain.NewErrorProjection("project.item.add", err), err
	}
	slug := safeBoardItemSlug(title)
	dir := strings.Trim(strings.TrimPrefix(project.NotesPrefix, "notes/"), "/")
	if workspace != nil {
		dir = filepath.ToSlash(filepath.Join(workspace.WorkspacePath, "inbox"))
	} else if dir == "" {
		dir = project.Slug
	}
	rel := filepath.ToSlash(filepath.Join(dir, slug+".md"))
	path := filepath.Join(root, filepath.FromSlash(rel))
	if _, statErr := os.Stat(path); statErr == nil {
		rel = filepath.ToSlash(filepath.Join(dir, slug+"-"+time.Now().UTC().Format("150405")+".md"))
		path = filepath.Join(root, filepath.FromSlash(rel))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = "## Next Steps\n"
	}
	content := buildNoteContentWithStatus(title, rel, project.Slug, dir, "task", nil, statusForBoardColumn(column), now, body)
	fields := map[string]string{"board_column": column}
	if workspace != nil {
		fields["subproject"] = workspace.Subproject
		fields["workspace_path"] = workspace.WorkspacePath
	}
	if labels := cleanTags(req.Labels); len(labels) > 0 {
		fields["labels"] = formatTags(labels)
	}
	if strings.TrimSpace(req.Milestone) != "" {
		fields["milestone"] = strings.TrimSpace(req.Milestone)
	}
	if strings.TrimSpace(req.Priority) != "" {
		fields["priority"] = strings.TrimSpace(req.Priority)
	}
	if strings.TrimSpace(req.DueAt) != "" {
		fields["due_at"] = strings.TrimSpace(req.DueAt)
		fields["due"] = strings.TrimSpace(req.DueAt)
	}
	if blockedBy := cleanTags(req.BlockedBy); len(blockedBy) > 0 {
		fields["blocked_by"] = formatTags(blockedBy)
	}
	content, _ = patchFrontmatterFields(content, fields)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorProjection("project.item.add", err), err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return errorProjection("project.item.add", err), err
	}
	note := parseNote(rel, content)
	_ = refreshIndex(root)
	_ = appendEvent(root, "project.item.add", "success", map[string]string{"project": project.Slug, "subproject": req.Subproject, "path": rel, "column": column})
	projection := projectItemProjection("project.item.add", "Project item created.", note, column)
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	return projection, nil
}

func (s *Service) ProjectItemMove(ctx context.Context, req ProjectItemRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.item.move", err), err
	}
	column := strings.TrimSpace(req.Column)
	note, err := findProjectItemNote(root, req.ItemID)
	if err != nil {
		return errorProjection("project.item.move", err), err
	}
	boardColumns, err := loadProjectBoardColumns(root, note.Project, note.Subproject)
	if err != nil {
		return errorProjection("project.item.move", err), err
	}
	if !isKnownBoardColumnIn(column, boardColumns) {
		err := &domain.CommandError{Code: "invalid_board_column", Message: "Unknown board column", Hint: "Use a configured board column"}
		return domain.NewErrorProjection("project.item.move", err), err
	}
	if column == "done" {
		if !req.Yes {
			err := &domain.CommandError{Code: "approval_required", Message: "Moving a project item to done requires --yes", Hint: "Add --yes after confirming"}
			return domain.NewErrorProjection("project.item.move", err), err
		}
		if !hasVersionSnapshot(root) {
			err := &domain.CommandError{Code: "snapshot_required", Message: "Moving a project item to done requires an explicit version snapshot first", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before completing project item"))}
			projection := domain.NewErrorProjection("project.item.move", err)
			projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
			return projection, err
		}
	}
	if err := patchProjectItemNote(ctx, s, root, note, column, "project.item.move"); err != nil {
		return errorProjection("project.item.move", err), err
	}
	note.BoardColumn = column
	note.Status = statusForBoardColumn(column)
	return projectItemProjection("project.item.move", "Project item moved.", note, column), nil
}

func (s *Service) ProjectItemArchive(ctx context.Context, req ProjectItemRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Archiving a project item requires --yes", Hint: "Add --yes after confirming"}
		return domain.NewErrorProjection("project.item.archive", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.item.archive", err), err
	}
	if !hasVersionSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "Archiving a project item requires an explicit version snapshot first", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before archiving project item"))}
		projection := domain.NewErrorProjection("project.item.archive", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		return projection, err
	}
	note, err := findProjectItemNote(root, req.ItemID)
	if err != nil {
		return errorProjection("project.item.archive", err), err
	}
	if err := patchProjectItemNote(ctx, s, root, note, "done", "project.item.archive"); err != nil {
		return errorProjection("project.item.archive", err), err
	}
	note.BoardColumn = "done"
	note.Status = "done"
	return projectItemProjection("project.item.archive", "Project item archived.", note, "done"), nil
}

func (s *Service) ProjectItemPlan(_ context.Context, req ProjectItemRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.item.plan", err), err
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		action = "archive"
	}
	if action != "archive" && action != "move" {
		err := &domain.CommandError{Code: "unsupported_project_item_action", Message: "project item plan only supports archive or move", Hint: "Use action=archive or action=move"}
		return domain.NewErrorProjection("project.item.plan", err), err
	}
	req.Action = action
	note, err := findProjectItemNote(root, req.ItemID)
	if err != nil {
		return errorProjection("project.item.plan", err), err
	}
	if action == "move" {
		boardColumns, loadErr := loadProjectBoardColumns(root, note.Project, note.Subproject)
		if loadErr != nil {
			return errorProjection("project.item.plan", loadErr), loadErr
		}
		if !isKnownBoardColumnIn(strings.TrimSpace(req.Column), boardColumns) {
			err := &domain.CommandError{Code: "invalid_board_column", Message: "Unknown board column", Hint: "Use a configured board column"}
			return domain.NewErrorProjection("project.item.plan", err), err
		}
	}
	requiresProtectedWrite := action == "archive" || (action == "move" && strings.TrimSpace(req.Column) == "done")
	if requiresProtectedWrite && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "High-risk project item changes require --yes", Hint: projectItemPlanApplyCommand(root, req, action)}
		projection := domain.NewErrorProjection("project.item.plan", err)
		projection.Actions = []domain.Action{{Name: action, Command: err.Hint}}
		projection.Data = map[string]any{"item": projectItemProjection("project.item.plan", "", note, boardColumnForItemPlan(note, req)).Data.(map[string]any)["item"]}
		return projection, err
	}
	if requiresProtectedWrite && !hasVersionSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "High-risk project item changes require an explicit version snapshot first", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before project item change"))}
		projection := domain.NewErrorProjection("project.item.plan", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		projection.Data = map[string]any{"item": projectItemProjection("project.item.plan", "", note, boardColumnForItemPlan(note, req)).Data.(map[string]any)["item"]}
		return projection, err
	}
	column := boardColumnForItemPlan(note, req)
	projection := projectItemProjection("project.item.plan", "Project item change plan generated.", note, column)
	projection.Facts["writes"] = "false"
	projection.Facts["action"] = action
	projection.Actions = []domain.Action{{Name: action, Command: projectItemPlanApplyCommand(root, req, action)}}
	return projection, nil
}

func (s *Service) ProjectItemShow(_ context.Context, req ProjectItemRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.item.show", err), err
	}
	note, err := findProjectItemNote(root, req.ItemID)
	if err != nil {
		return errorProjection("project.item.show", err), err
	}
	projection := projectItemProjection("project.item.show", "Project item read.", note, boardColumnForItemPlan(note, req))
	projection.Facts["writes"] = "false"
	projection.Evidence = []string{note.Path}
	projection.Actions = []domain.Action{{Name: "board_show", Command: fmt.Sprintf("pinax project board show %s --vault %s --json", shellQuote(note.Project), shellQuote(root))}}
	return projection, nil
}

func (s *Service) TaskAdopt(_ context.Context, req TaskAdoptRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("task.adopt", err), err
	}
	item, err := findBoardTaskItem(root, req.ItemID)
	if err != nil {
		return errorProjection("task.adopt", err), err
	}
	if item.SourceKind != domain.BoardItemSourceInlineTask {
		err := &domain.CommandError{Code: "task_adopt_unsupported", Message: "Only inferred checklist tasks can be adopted", Hint: "Run pinax project board show <project> --json to inspect task sources"}
		return domain.NewErrorProjection("task.adopt", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	adoption := domain.TaskAdoption{SchemaVersion: domain.TaskAdoptionSchemaVersion, TaskID: item.ItemID, Title: item.Title, Project: item.Project, Subproject: item.Subproject, SourcePath: item.Path, SourceLine: item.SourceLine, SourceStatus: "adopted", Column: item.Column, CreatedAt: now, UpdatedAt: now}
	projection := domain.NewProjection("task.adopt", "Task adoption plan generated.")
	projection.Facts["item_id"] = item.ItemID
	projection.Facts["project"] = item.Project
	projection.Facts["source_status"] = item.SourceStatus
	projection.Facts["source_kind"] = string(item.SourceKind)
	projection.Facts["writes"] = "false"
	projection.Facts["adopted"] = "0"
	projection.Data = map[string]any{"item": item, "adoption": adoption}
	projection.Actions = []domain.Action{{Name: "adopt", Command: fmt.Sprintf("pinax task adopt %s --vault %s --yes --json", shellQuote(item.ItemID), shellQuote(root))}}
	if !req.Yes {
		return projection, nil
	}
	ledgerRel := taskAdoptionLedgerRel(item.ItemID)
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(ledgerRel)), adoption); err != nil {
		return errorProjection("task.adopt", err), err
	}
	_ = appendEvent(root, "task.adopt", "success", map[string]string{"item_id": item.ItemID, "project": item.Project, "source_path": item.Path})
	projection.Summary = "Task adopted."
	projection.Facts["writes"] = "true"
	projection.Facts["adopted"] = "1"
	projection.Facts["ledger_path"] = ledgerRel
	projection.Evidence = []string{ledgerRel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	return projection, nil
}

func findBoardTaskItem(root, itemID string) (domain.BoardItem, error) {
	query := strings.TrimSpace(itemID)
	if query == "" {
		return domain.BoardItem{}, &domain.CommandError{Code: "argument_required", Message: "task adopt requires an item id", Hint: "Run pinax project board show <project> --json to inspect task ids"}
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return domain.BoardItem{}, err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return domain.BoardItem{}, err
	}
	for _, project := range registry.Projects {
		columns, _ := loadProjectBoardColumns(root, project.Slug, "")
		board := buildProjectBoard(root, project, nil, columns, ordinaryNotes(notes), domain.NoteDisplayCard, "scan", "unknown", false)
		for _, item := range board.Items {
			if item.ItemID == query {
				return item, nil
			}
		}
	}
	return domain.BoardItem{}, &domain.CommandError{Code: "task_not_found", Message: "Task item not found", Hint: "Run pinax project board show <project> --json to inspect task ids"}
}

func taskAdoptionLedgerRel(itemID string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "task-adoptions", itemID+".json"))
}

func projectItemPlanApplyCommand(root string, req ProjectItemRequest, action string) string {
	if action == "move" {
		return fmt.Sprintf("pinax project item move %s %s --vault %s --yes", shellQuote(req.ItemID), shellQuote(req.Column), shellQuote(root))
	}
	return fmt.Sprintf("pinax project item archive %s --vault %s --yes", shellQuote(req.ItemID), shellQuote(root))
}

func boardColumnForItemPlan(note domain.Note, req ProjectItemRequest) string {
	if req.Action == "archive" {
		return "done"
	}
	if column := strings.TrimSpace(req.Column); column != "" {
		return column
	}
	column, _ := boardColumnForNote(note, defaultBoardColumns)
	return column
}

func hasVersionSnapshot(root string) bool {
	snapshots, err := loadVersionSnapshots(root, 1)
	return err == nil && len(snapshots) > 0
}

func validateProjectBoardAssets(root string) []domain.Issue {
	issues := make([]domain.Issue, 0)
	issues = append(issues, validateProjectBoardConfigAssets(root)...)
	issues = append(issues, validateProjectBoardSnapshotAssets(root)...)
	return issues
}

func validateProjectBoardConfigAssets(root string) []domain.Issue {
	dir := filepath.Join(root, ".pinax", "project-boards")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	issues := make([]domain.Issue, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".pinax", "project-boards", entry.Name()))
		payload, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		var config domain.ProjectBoardConfig
		if readErr != nil || json.Unmarshal(payload, &config) != nil || config.SchemaVersion != domain.ProjectBoardSchemaVersion || strings.TrimSpace(config.ProjectSlug) == "" || len(config.Columns) == 0 || !boardColumnsValid(config.Columns) {
			issues = append(issues, domain.Issue{Code: "invalid_project_board_config", Path: rel, Message: "Project board configuration asset is invalid"})
		}
	}
	return issues
}

func validateProjectBoardSnapshotAssets(root string) []domain.Issue {
	dir := filepath.Join(root, ".pinax", "planning", "project-boards")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	issues := make([]domain.Issue, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "project-boards", entry.Name()))
		payload, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		var board domain.ProjectBoard
		if readErr != nil || json.Unmarshal(payload, &board) != nil || board.SchemaVersion != domain.ProjectBoardSchemaVersion || strings.TrimSpace(board.ProjectSlug) == "" || len(board.Columns) == 0 {
			issues = append(issues, domain.Issue{Code: "invalid_project_board_snapshot", Path: rel, Message: "Project board planning snapshot asset is invalid"})
		}
	}
	return issues
}

func boardColumnsValid(columns []domain.BoardColumn) bool {
	seen := map[string]bool{}
	for _, column := range columns {
		if !isSafeBoardSlug(column.ID) || seen[column.ID] {
			return false
		}
		seen[column.ID] = true
	}
	return true
}

func latestProjectBoardSnapshot(root string) (domain.ProjectBoard, string, bool, error) {
	dir := filepath.Join(root, ".pinax", "planning", "project-boards")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.ProjectBoard{}, "", false, nil
		}
		return domain.ProjectBoard{}, "", false, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return domain.ProjectBoard{}, "", false, nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	name := names[0]
	rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "project-boards", name))
	payload, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return domain.ProjectBoard{}, "", false, err
	}
	var board domain.ProjectBoard
	if err := json.Unmarshal(payload, &board); err != nil {
		return domain.ProjectBoard{}, "", false, err
	}
	if board.Facts.SnapshotID == "" {
		board.Facts.SnapshotID = strings.TrimSuffix(name, ".json")
	}
	if board.SourceSnapshotID == "" {
		board.SourceSnapshotID = board.Facts.SnapshotID
	}
	return board, rel, true, nil
}

func mergeProjectBoardPlanningFacts(snapshot *domain.PlanningSnapshot, board domain.ProjectBoard, rel string) {
	if snapshot.Facts == nil {
		snapshot.Facts = map[string]string{}
	}
	snapshotID := strings.TrimSpace(board.Facts.SnapshotID)
	if snapshotID == "" {
		snapshotID = strings.TrimSpace(board.SourceSnapshotID)
	}
	snapshot.Facts["board_snapshot_id"] = snapshotID
	snapshot.Facts["board_project"] = board.ProjectSlug
	snapshot.Facts["board_items"] = fmt.Sprint(board.Facts.TotalItems)
	snapshot.Facts["board_next"] = fmt.Sprint(board.Facts.Next)
	snapshot.Facts["board_doing"] = fmt.Sprint(board.Facts.Doing)
	snapshot.Facts["board_blocked"] = fmt.Sprint(board.Facts.Blocked)
	snapshot.Facts["board_review"] = fmt.Sprint(board.Facts.Review)
	snapshot.Facts["board_done"] = fmt.Sprint(board.Facts.Done)
	snapshot.Facts["board_evidence"] = rel
}

func copyProjectBoardPlanningFacts(projection *domain.Projection, snapshot domain.PlanningSnapshot) {
	for _, key := range []string{"board_snapshot_id", "board_project", "board_items", "board_next", "board_doing", "board_blocked", "board_review", "board_done", "board_evidence"} {
		if value := snapshot.Facts[key]; value != "" {
			projection.Facts[key] = value
		}
	}
}

func buildProjectBoard(root string, project domain.Project, workspace *domain.ProjectWorkspace, columns []domain.BoardColumn, notes []domain.Note, display domain.NoteDisplayKind, engine, indexStatus string, strictColumns bool) domain.ProjectBoard {
	if len(columns) == 0 {
		columns = defaultBoardColumns
	}
	items := make([]domain.BoardItem, 0)
	warnings := make([]domain.ProjectBoardWarning, 0)
	counts := domain.ProjectBoardFacts{Engine: engine, IndexStatus: indexStatus, ColumnCounts: emptyBoardColumnCounts(columns)}
	adoptions := loadTaskAdoptionsForBoard(root, project.Slug)
	for _, note := range notes {
		if note.Project != project.Slug {
			continue
		}
		if workspace != nil && note.Subproject != workspace.Subproject {
			continue
		}
		column, warning := boardColumnForNote(note, columns)
		if warning != nil {
			if strictColumns {
				continue
			}
			warnings = append(warnings, *warning)
		}
		item := domain.BoardItem{
			ItemID:       boardItemID(note),
			Title:        note.Title,
			Column:       column,
			SourceKind:   domain.BoardItemSourceNote,
			SourceStatus: "managed",
			NoteID:       note.ID,
			Path:         note.Path,
			Project:      note.Project,
			Subproject:   note.Subproject,
			Labels:       note.Labels,
			Tags:         note.Tags,
			Status:       note.Status,
			Milestone:    strings.TrimSpace(note.Milestone),
			Priority:     strings.TrimSpace(note.Priority),
			Due:          strings.TrimSpace(note.Due),
			DueAt:        strings.TrimSpace(firstBoardNonEmpty(note.DueAt, note.Due)),
			BlockedBy:    note.BlockedBy,
			EvidenceRefs: []string{note.Path},
			Writable:     true,
		}
		if workspace != nil {
			item.WorkspacePath = workspace.WorkspacePath
		}
		displayNote := buildNoteDisplayWithColumns(note, display, domain.NoteExposureAgent, columns)
		item.Note = &displayNote
		ctx := boardItemAgentContext(item, root)
		item.AgentContext = &ctx
		items = append(items, item)
		counts.TotalItems++
		counts.WritableItems++
		incrementBoardCount(&counts, column)
		for _, checklistItem := range checklistBoardItems(note, workspace, columns, adoptions) {
			items = append(items, checklistItem)
			counts.TotalItems++
			if checklistItem.Writable {
				counts.WritableItems++
			}
			incrementBoardCount(&counts, checklistItem.Column)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Column != items[j].Column {
			return boardColumnOrder(items[i].Column, columns) < boardColumnOrder(items[j].Column, columns)
		}
		return items[i].Path < items[j].Path
	})
	board := domain.ProjectBoard{SchemaVersion: domain.ProjectBoardSchemaVersion, ProjectSlug: project.Slug, Title: project.Name, Columns: columns, Items: items, Facts: counts, Warnings: warnings, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	if workspace != nil {
		board.Subproject = workspace.Subproject
		board.WorkspacePath = workspace.WorkspacePath
		workspaceCopy := *workspace
		board.Workspace = &workspaceCopy
	}
	return board
}

func loadTaskAdoptionsForBoard(root, project string) map[string]domain.TaskAdoption {
	dir := filepath.Join(root, ".pinax", "task-adoptions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]domain.TaskAdoption{}
	}
	out := map[string]domain.TaskAdoption{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var adoption domain.TaskAdoption
		payload, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil || json.Unmarshal(payload, &adoption) != nil || adoption.SchemaVersion != domain.TaskAdoptionSchemaVersion || adoption.Project != project {
			continue
		}
		out[adoption.TaskID] = adoption
	}
	return out
}

func checklistBoardItems(note domain.Note, workspace *domain.ProjectWorkspace, columns []domain.BoardColumn, adoptions map[string]domain.TaskAdoption) []domain.BoardItem {
	items := []domain.BoardItem{}
	lines := strings.Split(note.Body, "\n")
	for i, line := range lines {
		title, ok := parseUncheckedChecklistTitle(line)
		if !ok {
			continue
		}
		itemID := checklistTaskID(note.Path, i+1, title)
		adoption, adopted := adoptions[itemID]
		column := "inbox"
		status := "inferred"
		writable := false
		if adopted {
			column = adoption.Column
			status = "adopted"
			writable = true
		}
		if !isKnownBoardColumnIn(column, columns) {
			column = "inbox"
		}
		item := domain.BoardItem{ItemID: itemID, Title: title, Column: column, SourceKind: domain.BoardItemSourceInlineTask, SourceStatus: status, NoteID: note.ID, Path: note.Path, SourceLine: i + 1, Project: note.Project, Subproject: note.Subproject, Status: statusForBoardColumn(column), EvidenceRefs: []string{fmt.Sprintf("%s:%d", note.Path, i+1)}, Writable: writable}
		if workspace != nil {
			item.WorkspacePath = workspace.WorkspacePath
		}
		items = append(items, item)
	}
	return items
}

func parseUncheckedChecklistTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "- [ ] ") && !strings.HasPrefix(trimmed, "* [ ] ") {
		return "", false
	}
	title := strings.TrimSpace(trimmed[6:])
	return title, title != ""
}

func checklistTaskID(path string, line int, title string) string {
	return "task_" + strings.TrimPrefix(stableNoteID(fmt.Sprintf("%s:%d:%s", filepath.ToSlash(path), line, title)), "note_")
}

func firstBoardNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func configuredBoardColumns(values []string) ([]domain.BoardColumn, *domain.CommandError) {
	if len(values) == 0 {
		return defaultBoardColumns, nil
	}
	columns := make([]domain.BoardColumn, 0, len(values))
	seen := map[string]bool{}
	for i, raw := range values {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !isSafeBoardSlug(id) {
			return nil, &domain.CommandError{Code: "invalid_board_column", Message: "Board columns must use safe slugs", Hint: "For example, inbox,next,doing,blocked,review,done"}
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		columns = append(columns, domain.BoardColumn{ID: id, Name: boardColumnName(id), Order: (i + 1) * 10})
	}
	if len(columns) == 0 {
		return nil, &domain.CommandError{Code: "invalid_board_columns", Message: "At least one board column is required", Hint: "For example, --columns inbox,next,doing,blocked,review,done"}
	}
	return columns, nil
}

func findProjectItemNote(root, itemID string) (domain.Note, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return domain.Note{}, err
	}
	query := strings.TrimSpace(itemID)
	for _, note := range notes {
		if boardItemID(note) == query || note.ID == query || note.Path == query {
			if note.Kind != "task" || note.Project == "" {
				return domain.Note{}, &domain.CommandError{Code: "project_item_unmanaged", Message: "This object is not a Pinax-managed project item", Hint: "Use pinax project item add to create a managed item"}
			}
			return note, nil
		}
	}
	if strings.HasPrefix(query, "task_") {
		return domain.Note{}, &domain.CommandError{Code: "task_unmanaged", Message: "Checklist task is not a Pinax-managed task yet", Hint: "Run pinax task adopt <item_id> --plan before moving or archiving it"}
	}
	return domain.Note{}, &domain.CommandError{Code: "project_item_not_found", Message: "Project item not found", Hint: "Run pinax project board show <project> --json to view item_id"}
}

func patchProjectItemNote(_ context.Context, _ *Service, root string, note domain.Note, column, eventType string) error {
	path := filepath.Join(root, filepath.FromSlash(note.Path))
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated, _ := patchFrontmatterFields(string(content), map[string]string{"board_column": column, "status": statusForBoardColumn(column), "updated_at": now})
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	_ = refreshIndex(root)
	_ = appendEvent(root, eventType, "success", map[string]string{"path": note.Path, "column": column})
	return nil
}

func projectItemProjection(command, summary string, note domain.Note, column string) domain.Projection {
	projection := domain.NewProjection(command, summary)
	item := domain.BoardItem{ItemID: boardItemID(note), Title: note.Title, Column: column, SourceKind: domain.BoardItemSourceNote, NoteID: note.ID, Path: note.Path, Project: note.Project, Subproject: note.Subproject, Tags: note.Tags, Labels: note.Labels, Status: statusForBoardColumn(column), Milestone: note.Milestone, Priority: note.Priority, Due: note.Due, DueAt: firstBoardNonEmpty(note.DueAt, note.Due), BlockedBy: note.BlockedBy, EvidenceRefs: []string{note.Path}, Writable: true}
	if note.Subproject != "" {
		item.WorkspacePath = note.Frontmatter["workspace_path"]
	}
	projection.Facts["item_id"] = item.ItemID
	projection.Facts["project"] = note.Project
	if note.Subproject != "" {
		projection.Facts["subproject"] = note.Subproject
		projection.Facts["workspace_path"] = item.WorkspacePath
	}
	projection.Facts["path"] = note.Path
	projection.Facts["column"] = column
	projection.Facts["status"] = item.Status
	if len(note.Labels) > 0 {
		projection.Facts["labels"] = strings.Join(note.Labels, ",")
	}
	if note.Milestone != "" {
		projection.Facts["milestone"] = note.Milestone
	}
	if note.Priority != "" {
		projection.Facts["priority"] = note.Priority
	}
	if item.DueAt != "" {
		projection.Facts["due_at"] = item.DueAt
	}
	if len(note.BlockedBy) > 0 {
		projection.Facts["blocked_by"] = strings.Join(note.BlockedBy, ",")
	}
	projection.Facts["writes"] = "true"
	projection.Evidence = []string{note.Path, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"item": item}
	return projection
}

func safeBoardItemSlug(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r > 127 {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "item"
	}
	return slug
}

func statusForBoardColumn(column string) string {
	switch column {
	case "inbox":
		return "inbox"
	case "blocked":
		return "blocked"
	case "review":
		return "review"
	case "done":
		return "done"
	default:
		return "active"
	}
}

func projectBoardConfigRel(project, subproject string) string {
	if strings.TrimSpace(subproject) == "" {
		return filepath.ToSlash(filepath.Join(".pinax", "project-boards", project+".json"))
	}
	return filepath.ToSlash(filepath.Join(".pinax", "project-boards", project, subproject+".json"))
}

func projectBoardViewRel(project, subproject, view string) string {
	if strings.TrimSpace(subproject) == "" {
		return filepath.ToSlash(filepath.Join(".pinax", "project-board-views", project, view+".json"))
	}
	return filepath.ToSlash(filepath.Join(".pinax", "project-board-views", project, subproject, view+".json"))
}

func loadProjectBoardView(root, project, subproject, viewName string) (domain.ProjectBoardView, error) {
	viewName = strings.TrimSpace(viewName)
	if viewName == "" || !isSafeBoardSlug(viewName) {
		return domain.ProjectBoardView{}, &domain.CommandError{Code: "invalid_board_view", Message: "Board view name must be a safe slug", Hint: "Use a name like active or weekly-review"}
	}
	rel := projectBoardViewRel(project, subproject, viewName)
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return domain.ProjectBoardView{}, &domain.CommandError{Code: "project_board_view_not_found", Message: "Project board view not found", Hint: "Run pinax project board view save <project> <view> --vault <vault> --json"}
	}
	if err != nil {
		return domain.ProjectBoardView{}, err
	}
	var view domain.ProjectBoardView
	if err := json.Unmarshal(payload, &view); err != nil {
		return domain.ProjectBoardView{}, err
	}
	if view.SchemaVersion == "" {
		view.SchemaVersion = domain.ProjectBoardViewSchemaVersion
	}
	return view, nil
}

func boardColumnName(id string) string {
	for _, column := range defaultBoardColumns {
		if column.ID == id {
			return column.Name
		}
	}
	return id
}

func isSafeBoardSlug(value string) bool {
	if value == "" || strings.Contains(value, "..") || strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func renderProjectBoardMarkdown(board domain.ProjectBoard) string {
	var b strings.Builder
	b.WriteString("# " + board.Title + "\n\n")
	byColumn := map[string][]domain.BoardItem{}
	for _, item := range board.Items {
		byColumn[item.Column] = append(byColumn[item.Column], item)
	}
	for _, column := range board.Columns {
		b.WriteString("## " + column.ID + "\n\n")
		items := byColumn[column.ID]
		if len(items) == 0 {
			b.WriteString("_None_\n\n")
			continue
		}
		for _, item := range items {
			b.WriteString("- " + item.Title)
			if item.Path != "" {
				b.WriteString(" (" + item.Path + ")")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func buildNoteDisplay(note domain.Note, display domain.NoteDisplayKind, exposure domain.NoteExposure) domain.NoteDisplay {
	return buildNoteDisplayWithColumns(note, display, exposure, defaultBoardColumns)
}

func buildNoteDisplayWithColumns(note domain.Note, display domain.NoteDisplayKind, exposure domain.NoteExposure, columns []domain.BoardColumn) domain.NoteDisplay {
	if display == "" {
		display = domain.NoteDisplayCard
	}
	if exposure == "" {
		exposure = domain.NoteExposureAgent
	}
	out := domain.NoteDisplay{NoteID: note.ID, Title: note.Title, Path: note.Path, Display: display, Exposure: exposure, Project: note.Project, Subproject: note.Subproject, WorkspacePath: note.Frontmatter["workspace_path"], BoardColumn: strings.TrimSpace(note.BoardColumn), Kind: note.Kind, Status: note.Status, Tags: note.Tags, Labels: note.Labels, Milestone: note.Milestone, Priority: note.Priority, DueAt: firstBoardNonEmpty(note.DueAt, note.Due), BlockedBy: note.BlockedBy, UpdatedAt: note.UpdatedAt, Excerpt: noteExcerpt(note.Body)}
	if out.BoardColumn == "" {
		out.BoardColumn, _ = boardColumnForNote(note, columns)
	}
	if display == domain.NoteDisplayBody {
		out.Exposure = domain.NoteExposureLocalBody
		out.Body = note.Body
	} else if note.Body != "" {
		out.RedactionWarnings = []string{"body_omitted"}
	}
	ctx := noteAgentContext(note, display, "note", nil, nil)
	out.AgentContext = &ctx
	return out
}

func parseNoteDisplayKind(value string) (domain.NoteDisplayKind, *domain.CommandError) {
	switch strings.TrimSpace(value) {
	case "", string(domain.NoteDisplayCard):
		return domain.NoteDisplayCard, nil
	case string(domain.NoteDisplayDetail):
		return domain.NoteDisplayDetail, nil
	case string(domain.NoteDisplayContext):
		return domain.NoteDisplayContext, nil
	case string(domain.NoteDisplayBody):
		return domain.NoteDisplayBody, nil
	default:
		return "", &domain.CommandError{Code: "invalid_note_display", Message: "note display only supports card, detail, context, or body", Hint: "Use --note-display card or --display card"}
	}
}

func boardColumnForNote(note domain.Note, columns []domain.BoardColumn) (string, *domain.ProjectBoardWarning) {
	if column := strings.TrimSpace(note.BoardColumn); column != "" {
		if isKnownBoardColumnIn(column, columns) {
			return column, nil
		}
		fallback := boardColumnFromStatus(note.Status, columns)
		return fallback, &domain.ProjectBoardWarning{Code: "unknown_board_column", Message: "Unknown board column; placed in the default column by status", Path: note.Path}
	}
	return boardColumnFromStatus(note.Status, columns), nil
}

func boardColumnFromStatus(status string, columns []domain.BoardColumn) string {
	preferred := "next"
	switch strings.TrimSpace(status) {
	case "inbox", "":
		preferred = "inbox"
	case "doing":
		preferred = "doing"
	case "blocked":
		preferred = "blocked"
	case "review":
		preferred = "review"
	case "done", "archived":
		preferred = "done"
	}
	if isKnownBoardColumnIn(preferred, columns) {
		return preferred
	}
	if preferred == "next" {
		for _, fallback := range []string{"planned", "learning", "practice"} {
			if isKnownBoardColumnIn(fallback, columns) {
				return fallback
			}
		}
	}
	if len(columns) > 0 {
		return columns[0].ID
	}
	return preferred
}

func isKnownBoardColumnIn(column string, columns []domain.BoardColumn) bool {
	column = strings.TrimSpace(column)
	if column == "" {
		return false
	}
	for _, item := range columns {
		if item.ID == column {
			return true
		}
	}
	return false
}

func emptyBoardColumnCounts(columns []domain.BoardColumn) map[string]int {
	counts := make(map[string]int, len(columns))
	for _, column := range columns {
		counts[column.ID] = 0
	}
	return counts
}

func incrementBoardCount(facts *domain.ProjectBoardFacts, column string) {
	if facts.ColumnCounts == nil {
		facts.ColumnCounts = map[string]int{}
	}
	facts.ColumnCounts[column]++
	switch column {
	case "inbox":
		facts.Inbox++
	case "next":
		facts.Next++
	case "doing":
		facts.Doing++
	case "blocked":
		facts.Blocked++
	case "review":
		facts.Review++
	case "done":
		facts.Done++
	}
}

func boardColumnOrder(column string, columns []domain.BoardColumn) int {
	for _, item := range columns {
		if item.ID == column {
			return item.Order
		}
	}
	return 999
}

func workspaceSubproject(workspace *domain.ProjectWorkspace) string {
	if workspace == nil {
		return ""
	}
	return workspace.Subproject
}

func loadProjectBoardColumns(root, project, subproject string) ([]domain.BoardColumn, error) {
	config, ok, err := loadProjectBoardConfig(root, project, subproject)
	if err != nil {
		return nil, err
	}
	if ok && len(config.Columns) > 0 {
		return config.Columns, nil
	}
	if strings.TrimSpace(subproject) != "" {
		config, ok, err = loadProjectBoardConfig(root, project, "")
		if err != nil {
			return nil, err
		}
		if ok && len(config.Columns) > 0 {
			return config.Columns, nil
		}
	}
	return defaultBoardColumns, nil
}

func loadProjectBoardConfig(root, project, subproject string) (domain.ProjectBoardConfig, bool, error) {
	rel := projectBoardConfigRel(project, subproject)
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return domain.ProjectBoardConfig{}, false, nil
	}
	if err != nil {
		return domain.ProjectBoardConfig{}, false, err
	}
	var config domain.ProjectBoardConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		return domain.ProjectBoardConfig{}, false, err
	}
	if config.SchemaVersion != domain.ProjectBoardSchemaVersion || strings.TrimSpace(config.ProjectSlug) == "" || !boardColumnsValid(config.Columns) {
		return domain.ProjectBoardConfig{}, false, &domain.CommandError{Code: "invalid_project_board_config", Message: "Project board configuration asset is invalid", Hint: "Run pinax vault validate --vault <vault> --json"}
	}
	return config, true, nil
}

func boardItemID(note domain.Note) string {
	if note.ID != "" {
		return "item_" + strings.TrimPrefix(note.ID, "note_")
	}
	return "item_" + strings.TrimPrefix(stableNoteID(note.Path), "note_")
}

func boardIndexState(root string, notes []domain.Note) (string, string) {
	status, err := noteindex.Inspect(root, notes)
	if err != nil || status.Status == "" {
		return "scan", "missing"
	}
	if status.Status != "fresh" {
		return "scan", status.Status
	}
	return "index", "fresh"
}

func noteExcerpt(body string) string {
	clean := strings.Join(strings.Fields(body), " ")
	if len(clean) <= 96 {
		return clean
	}
	return clean[:96]
}

func boardHumanSummary(board domain.ProjectBoard) string {
	name := board.ProjectSlug
	if strings.TrimSpace(board.Title) != "" {
		name = board.Title
	}
	return fmt.Sprintf("Project %s board: next %d, doing %d, blocked %d, review %d, done %d.", name, board.Facts.Next, board.Facts.Doing, board.Facts.Blocked, board.Facts.Review, board.Facts.Done)
}
