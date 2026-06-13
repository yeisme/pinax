package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/templateengine"
)

type indexPageRender struct {
	Root          string
	Name          string
	Template      string
	Path          string
	Title         string
	Body          string
	ManagedBlocks []templateengine.ManagedBlock
	QueryCount    int
}

func (s *Service) PreviewIndexPage(ctx context.Context, req IndexPageRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.page.preview", err), err
	}
	rendered, err := s.renderIndexPage(ctx, root, req)
	if err != nil {
		return errorProjection("index.page.preview", err), err
	}
	projection := domain.NewProjection("index.page.preview", "Index page preview generated.")
	fillIndexPageFacts(&projection, rendered)
	projection.Facts["writes"] = "false"
	projection.Data = map[string]any{"path": rendered.Path, "body": rendered.Body, "managed_blocks": rendered.ManagedBlocks}
	return projection, nil
}

func (s *Service) CreateIndexPage(ctx context.Context, req IndexPageRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.page.create", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("index.page.create", err), err
	}
	rendered, err := s.renderIndexPage(ctx, root, req)
	if err != nil {
		return errorProjection("index.page.create", err), err
	}
	path, err := safeJoin(root, rendered.Path)
	if err != nil {
		return errorProjection("index.page.create", err), err
	}
	created := false
	if _, err := os.Stat(path); err == nil {
		created = false
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errorProjection("index.page.create", err), err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		content := buildNoteContentWithStatus(rendered.Title, rendered.Path, "", "index", "index", []string{"index"}, "system", now, rendered.Body)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return errorProjection("index.page.create", err), err
		}
		created = true
	} else {
		return errorProjection("index.page.create", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("index.page.create", err), err
	}
	_ = appendEvent(root, "index.page.create", "success", map[string]string{"path": rendered.Path, "template": rendered.Template, "created": fmt.Sprint(created)})
	projection := domain.NewProjection("index.page.create", "Index page created.")
	fillIndexPageFacts(&projection, rendered)
	projection.Facts["created"] = fmt.Sprint(created)
	projection.Facts["writes"] = fmt.Sprint(created)
	projection.Evidence = []string{rendered.Path, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) RefreshIndexPage(ctx context.Context, req IndexPageRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	rendered, err := s.renderIndexPage(ctx, root, req)
	if err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	path, err := safeJoin(root, rendered.Path)
	if err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	body := string(b)
	for _, block := range rendered.ManagedBlocks {
		replacement := rendered.Body[block.ContentStart:block.ContentEnd]
		body, err = templateengine.ReplaceManagedBlock(body, block.Name, replacement)
		if err != nil {
			return errorProjection("index.page.refresh", templateEngineCommandError(err)), err
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("index.page.refresh", err), err
	}
	_ = appendEvent(root, "index.page.refresh", "success", map[string]string{"path": rendered.Path, "template": rendered.Template})
	projection := domain.NewProjection("index.page.refresh", "Index page managed block refreshed.")
	fillIndexPageFacts(&projection, rendered)
	projection.Facts["writes"] = "true"
	projection.Evidence = []string{rendered.Path, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) renderIndexPage(ctx context.Context, root string, req IndexPageRequest) (indexPageRender, error) {
	name, err := cleanIndexPageName(req.Name)
	if err != nil {
		return indexPageRender{}, err
	}
	templateName := strings.TrimSpace(req.Template)
	if templateName == "" {
		templateName = "index." + name
	}
	body, err := loadTemplate(root, templateName)
	if err != nil {
		return indexPageRender{}, err
	}
	doc, err := templateengine.ParseDocument(templateName, body)
	if err != nil {
		return indexPageRender{}, templateEngineCommandError(err)
	}
	queries, err := s.executeTemplateQueries(ctx, root, doc.Metadata.Queries, true)
	if err != nil {
		return indexPageRender{}, err
	}
	now := time.Now().UTC()
	rendered, err := templateengine.New().Render(doc, templateengine.Context{
		Title:    indexPageTitle(name, doc),
		Date:     now.Format("2006-01-02"),
		DateTime: now.Format(time.RFC3339),
		Vars:     map[string]string{"name": name},
		Queries:  queries,
	})
	if err != nil {
		return indexPageRender{}, templateEngineCommandError(err)
	}
	blocks, err := templateengine.InspectManagedBlocks(rendered.Body)
	if err != nil {
		return indexPageRender{}, templateEngineCommandError(err)
	}
	rel := indexPagePathFromPattern(doc.Metadata.Output.PathPattern, name)
	if _, err := safeJoin(root, rel); err != nil {
		return indexPageRender{}, err
	}
	return indexPageRender{Root: root, Name: name, Template: templateName, Path: rel, Title: indexPageTitle(name, doc), Body: rendered.Body, ManagedBlocks: blocks, QueryCount: len(doc.Metadata.Queries)}, nil
}

func fillIndexPageFacts(projection *domain.Projection, rendered indexPageRender) {
	projection.Facts["path"] = rendered.Path
	projection.Facts["template"] = rendered.Template
	projection.Facts["name"] = rendered.Name
	projection.Facts["managed_blocks"] = fmt.Sprint(len(rendered.ManagedBlocks))
	projection.Facts["query_count"] = fmt.Sprint(rendered.QueryCount)
}

func cleanIndexPageName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "home"
	}
	if strings.HasPrefix(name, ".") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", &domain.CommandError{Code: "invalid_index_page_name", Message: "Index page name must be one safe name", Hint: "For example, home or project-board"}
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", &domain.CommandError{Code: "invalid_index_page_name", Message: "Index page name may only contain letters, numbers, -, _, and .", Hint: "For example, home or project-board"}
	}
	return name, nil
}

func indexPageTitle(name string, doc templateengine.TemplateDocument) string {
	if title := strings.TrimSpace(doc.Metadata.Title); title != "" {
		return title
	}
	if name == "home" {
		return "Home Index"
	}
	return name + " Index"
}

func indexPagePathFromPattern(pattern, name string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return filepath.ToSlash(filepath.Join("index", name+".md"))
	}
	for _, token := range []string{"{{ .Name }}", "{{.Name}}", "{{ name }}", "{{name}}"} {
		pattern = strings.ReplaceAll(pattern, token, name)
	}
	return filepath.ToSlash(pattern)
}
