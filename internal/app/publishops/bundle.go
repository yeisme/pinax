package publishops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const PublishBundleSchemaVersion = "pinax.publish_bundle.v1"

type PublishBundleRequest struct {
	VaultRoot  string
	BundleRoot string
	Profile    domain.PublishProfile
	Plan       domain.PublishPlan
	Notes      map[string]domain.Note
}

type PublishBundleResult struct {
	BundleRoot    string `json:"bundle_root"`
	FilesWritten  int    `json:"files_written"`
	SelectedNotes int    `json:"selected_notes"`
	Assets        int    `json:"assets"`
}

type publishBundleManifest struct {
	SchemaVersion string               `json:"schema_version"`
	ProfileName   string               `json:"profile_name"`
	Target        domain.PublishTarget `json:"target"`
	Renderer      string               `json:"renderer,omitempty"`
	Selected      []domain.PublishItem `json:"selected,omitempty"`
	Skipped       []domain.PublishItem `json:"skipped,omitempty"`
}

func BuildPublishBundle(req PublishBundleRequest) (PublishBundleResult, error) {
	if strings.TrimSpace(req.BundleRoot) == "" {
		return PublishBundleResult{}, fmt.Errorf("publish bundle root required")
	}
	if err := os.MkdirAll(req.BundleRoot, 0o755); err != nil {
		return PublishBundleResult{}, err
	}
	result := PublishBundleResult{BundleRoot: req.BundleRoot}
	writeJSON := func(rel string, value any) error {
		body, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		result.FilesWritten++
		return writePublishBundleFile(req.BundleRoot, rel, append(body, '\n'))
	}
	selectedNotes := publishBundleSelectedNotes(req.Plan, req.Notes)
	result.SelectedNotes = len(selectedNotes)
	if err := writeJSON("manifest.json", publishBundleManifest{SchemaVersion: PublishBundleSchemaVersion, ProfileName: req.Profile.Name, Target: req.Profile.Target, Renderer: string(req.Profile.Renderer), Selected: req.Plan.Selected, Skipped: req.Plan.Skipped}); err != nil {
		return PublishBundleResult{}, err
	}
	if err := writeJSON("notes.json", map[string]any{"schema_version": "pinax.publish_bundle.notes.v1", "notes": publishBundleNotes(selectedNotes)}); err != nil {
		return PublishBundleResult{}, err
	}
	if err := writeJSON("graph.json", map[string]any{"schema_version": "pinax.publish_bundle.graph.v1", "links": req.Plan.LinkGraph}); err != nil {
		return PublishBundleResult{}, err
	}
	if err := writeJSON("taxonomies.json", map[string]any{"schema_version": "pinax.publish_bundle.taxonomies.v1", "tags": publishBundleTagCounts(selectedNotes), "types": publishBundleKindCounts(selectedNotes)}); err != nil {
		return PublishBundleResult{}, err
	}
	if err := writeJSON("search-index.json", map[string]any{"schema_version": "pinax.publish_bundle.search.v1", "entries": publishBundleSearchEntries(selectedNotes)}); err != nil {
		return PublishBundleResult{}, err
	}
	if err := writeJSON("sources.json", map[string]any{"schema_version": "pinax.publish_bundle.sources.v1", "sources": req.Plan.Sources}); err != nil {
		return PublishBundleResult{}, err
	}
	for _, item := range req.Plan.Selected {
		if item.Kind != "asset" {
			continue
		}
		if err := copyPublishBundleAsset(req.VaultRoot, req.BundleRoot, item.SourcePath); err != nil {
			return PublishBundleResult{}, err
		}
		result.FilesWritten++
		result.Assets++
	}
	return result, nil
}

func publishBundleSelectedNotes(plan domain.PublishPlan, notes map[string]domain.Note) []domain.Note {
	selected := make([]domain.Note, 0)
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		if note, ok := notes[item.SourcePath]; ok {
			selected = append(selected, note)
		}
	}
	return selected
}

func publishBundleNotes(notes []domain.Note) []map[string]any {
	out := make([]map[string]any, 0, len(notes))
	for _, note := range notes {
		out = append(out, map[string]any{"id": note.ID, "title": note.Title, "path": "notes/" + slugForPath(note.Path) + "/", "source_path": note.Path, "kind": note.Kind, "status": note.Status, "tags": note.Tags, "body": note.Body})
	}
	return out
}

func publishBundleTagCounts(notes []domain.Note) map[string]int {
	counts := map[string]int{}
	for _, note := range notes {
		for _, tag := range note.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				counts[tag]++
			}
		}
	}
	return counts
}

func publishBundleKindCounts(notes []domain.Note) map[string]int {
	counts := map[string]int{}
	for _, note := range notes {
		kind := strings.TrimSpace(note.Kind)
		if kind == "" {
			kind = "note"
		}
		counts[kind]++
	}
	return counts
}

func publishBundleSearchEntries(notes []domain.Note) []map[string]any {
	entries := make([]map[string]any, 0, len(notes))
	for _, note := range notes {
		entries = append(entries, map[string]any{"id": note.ID, "title": note.Title, "path": "notes/" + slugForPath(note.Path) + "/", "tags": note.Tags, "kind": note.Kind})
	}
	return entries
}

func copyPublishBundleAsset(vaultRoot, bundleRoot, rel string) error {
	rel, err := cleanPublishBundleRelPath(rel)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(filepath.Join(vaultRoot, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	return writePublishBundleFile(bundleRoot, rel, body)
}

func writePublishBundleFile(root, rel string, body []byte) error {
	rel, err := cleanPublishBundleRelPath(rel)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func cleanPublishBundleRelPath(raw string) (string, error) {
	slash := filepath.ToSlash(strings.TrimSpace(raw))
	if slash == "" || filepath.IsAbs(raw) || slash == "." || slash == ".." || strings.HasPrefix(slash, "../") || strings.Contains(slash, "/../") {
		return "", fmt.Errorf("publish bundle path is unsafe")
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(slash)), "/")
	for _, part := range parts {
		if part == ".pinax" {
			return "", fmt.Errorf("publish bundle path must not include .pinax")
		}
	}
	return strings.Join(parts, "/"), nil
}
