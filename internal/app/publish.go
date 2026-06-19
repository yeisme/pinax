package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/publishops"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"gopkg.in/yaml.v3"
)

type PublishRequest struct {
	VaultPath  string
	Profile    string
	Target     string
	Renderer   string
	Title      string
	BaseURL    string
	Theme      string
	Out        string
	Repo       string
	Branch     string
	Endpoint   string
	Method     string
	SecretRef  string
	GistID     string
	Visibility string
	Host       string
	Port       int
	Once       bool
	Yes        bool
}

func (s *Service) PublishProfileInit(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.profile.init", err), err
	}
	name, commandErr := normalizePublishProfileName(req.Profile)
	if commandErr != nil {
		return domain.NewErrorProjection("publish.profile.init", commandErr), commandErr
	}
	target := domain.PublishTarget(strings.TrimSpace(req.Target))
	if target == "" {
		target = domain.PublishTargetGitHubPages
	}
	renderer := domain.PublishRenderer(strings.TrimSpace(req.Renderer))
	if renderer == "" {
		renderer = domain.PublishRendererHugo
	}
	profile := domain.NewDefaultPublishProfile(name, target, renderer)
	if strings.TrimSpace(req.Title) != "" {
		profile.Site.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		profile.Site.BaseURL = strings.TrimSpace(req.BaseURL)
	}
	if strings.TrimSpace(req.Theme) != "" {
		profile.Site.Theme.Value = strings.TrimSpace(req.Theme)
	}
	issues := publishops.ValidateProfile(profile)
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.profile.init", name, issues)
		return publishProfileProjection("publish.profile.init", profile, issues, "failed"), cmdErr
	}
	if err := writePublishProfile(root, profile); err != nil {
		return errorProjection("publish.profile.init", err), err
	}
	projection := publishProfileProjection("publish.profile.init", profile, nil, "success")
	projection.Summary = "Publish profile initialized."
	projection.Evidence = []string{publishProfileRelPath(name)}
	return projection, nil
}

func (s *Service) PublishProfileValidate(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.profile.validate")
	if err != nil {
		return errorProjection("publish.profile.validate", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.profile.validate", profile.Name, issues)
		return publishProfileProjection("publish.profile.validate", profile, issues, "failed"), cmdErr
	}
	projection := publishProfileProjection("publish.profile.validate", profile, nil, "success")
	projection.Summary = "Publish profile is valid."
	projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax publish plan --profile %s --target %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	return projection, nil
}

func (s *Service) PublishProfileShow(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.profile.show")
	if err != nil {
		return errorProjection("publish.profile.show", err), err
	}
	projection := publishProfileProjection("publish.profile.show", profile, issues, "success")
	projection.Summary = "Publish profile loaded."
	return projection, nil
}

func (s *Service) PublishProfileList(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.profile.list", err), err
	}
	profiles, err := listPublishProfiles(root)
	if err != nil {
		return errorProjection("publish.profile.list", err), err
	}
	projection := domain.NewProjection("publish.profile.list", "Publish profiles listed.")
	projection.Facts["profiles"] = fmt.Sprint(len(profiles))
	projection.Data = map[string]any{"profiles": profiles}
	if len(profiles) > 0 {
		projection.Actions = []domain.Action{{Name: "validate", Command: fmt.Sprintf("pinax publish profile validate %s --vault <vault> --json", shellQuote(profiles[0].Name))}}
	}
	return projection, nil
}

func (s *Service) PublishDoctor(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.doctor")
	if err != nil {
		return errorProjection("publish.doctor", err), err
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	outSafe := true
	if strings.TrimSpace(req.Out) != "" {
		_, outErr := cleanPublishOutPath(req.Out)
		outSafe = outErr == nil
		if outErr != nil {
			issues = append(issues, domain.PublishValidationIssue{Code: outErr.Code, Field: "out", Severity: "error", Message: outErr.Message})
		}
	}
	hugoAvailable := false
	if profile.Target == domain.PublishTargetGitHubPages && profile.Renderer == domain.PublishRendererHugo {
		_, hugoErr := exec.LookPath("hugo")
		hugoAvailable = hugoErr == nil
		if hugoErr != nil {
			issues = append(issues, domain.PublishValidationIssue{Code: "hugo_unavailable", Field: "hugo", Severity: "warning", Message: "Hugo executable was not found on PATH"})
		}
	}
	_, gitErr := exec.LookPath("git")
	projection := domain.NewProjection("publish.doctor", "发布环境检查完成。")
	if len(issues) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["profile_issues"] = fmt.Sprint(len(publishops.ValidateProfile(profile)))
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Facts["hugo_available"] = fmt.Sprint(hugoAvailable)
	projection.Facts["git_available"] = fmt.Sprint(gitErr == nil)
	projection.Facts["theme"] = profile.Site.Theme.Value
	projection.Facts["out_safe"] = fmt.Sprint(outSafe)
	projection.Data = map[string]any{"profile": profile.Name, "issues": issues}
	if len(issues) == 0 {
		projection.Actions = []domain.Action{{Name: "build", Command: fmt.Sprintf("pinax publish build --profile %s --target %s --out <out> --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	} else {
		projection.Actions = []domain.Action{{Name: "profile_validate", Command: fmt.Sprintf("pinax publish profile validate %s --vault <vault> --json", shellQuote(profile.Name))}}
	}
	return projection, nil
}

func readPublishProfileRequest(req PublishRequest, command string) (domain.PublishProfile, []domain.PublishValidationIssue, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return domain.PublishProfile{}, nil, err
	}
	name, commandErr := normalizePublishProfileName(req.Profile)
	if commandErr != nil {
		return domain.PublishProfile{}, nil, commandErr
	}
	profile, err := readPublishProfile(root, name)
	if err != nil {
		return domain.PublishProfile{}, nil, err
	}
	issues := publishops.ValidateProfile(profile)
	return profile, issues, nil
}

func normalizePublishProfileName(raw string) (string, *domain.CommandError) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", &domain.CommandError{Code: "publish_profile_required", Message: "publish profile name is required", Hint: "pinax publish profile init public --vault <vault> --json"}
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", &domain.CommandError{Code: "publish_profile_name_invalid", Message: "publish profile name must be one safe name", Hint: "Use a name like public or team-wiki"}
	}
	return name, nil
}

func publishProfileRelPath(name string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "publish", "profiles", name+".yaml"))
}

func publishProfileAbsPath(root, name string) string {
	return filepath.Join(root, ".pinax", "publish", "profiles", name+".yaml")
}

func writePublishProfile(root string, profile domain.PublishProfile) error {
	path := publishProfileAbsPath(root, profile.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+profile.Name+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readPublishProfile(root, name string) (domain.PublishProfile, error) {
	body, err := os.ReadFile(publishProfileAbsPath(root, name))
	if errors.Is(err, os.ErrNotExist) {
		return domain.PublishProfile{}, &domain.CommandError{Code: "publish_profile_not_found", Message: "Publish profile not found", Hint: "Run pinax publish profile init " + shellQuote(name) + " --vault <vault> --json"}
	}
	if err != nil {
		return domain.PublishProfile{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	var profile domain.PublishProfile
	if err := decoder.Decode(&profile); err != nil {
		if strings.Contains(err.Error(), "field ") && strings.Contains(err.Error(), " not found in type ") {
			return domain.PublishProfile{}, &domain.CommandError{Code: "publish_profile_unknown_field", Message: "Publish profile contains an unknown field", Hint: "Recreate or repair the profile with pinax publish profile init"}
		}
		return domain.PublishProfile{}, &domain.CommandError{Code: "publish_profile_invalid", Message: err.Error(), Hint: "Recreate or repair the profile with pinax publish profile init"}
	}
	return profile, nil
}

func listPublishProfiles(root string) ([]domain.PublishProfile, error) {
	dir := filepath.Join(root, ".pinax", "publish", "profiles")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]domain.PublishProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		profile, err := readPublishProfile(root, name)
		if err != nil {
			var pathErr *fs.PathError
			if errors.As(err, &pathErr) {
				return nil, err
			}
			continue
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func publishValidationError(command, profile string, issues []domain.PublishValidationIssue) *domain.CommandError {
	return &domain.CommandError{Code: "publish_profile_invalid", Message: fmt.Sprintf("Publish profile %s has %d validation issue(s)", profile, len(issues)), Hint: "Fix the profile or recreate it with pinax publish profile init"}
}

func publishProfileProjection(command string, profile domain.PublishProfile, issues []domain.PublishValidationIssue, status string) domain.Projection {
	projection := domain.NewProjection(command, "Publish profile processed.")
	projection.Status = status
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Evidence = []string{publishProfileRelPath(profile.Name)}
	projection.Data = map[string]any{"profile": profile, "issues": issues}
	if len(issues) > 0 {
		for i, issue := range issues {
			projection.Facts[fmt.Sprintf("issue.%d.code", i+1)] = issue.Code
		}
		projection.Error = publishValidationError(command, profile.Name, issues)
	}
	return projection
}

func (s *Service) PublishPlan(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.plan")
	if err != nil {
		return errorProjection("publish.plan", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.plan", profile.Name, issues)
		return publishProfileProjection("publish.plan", profile, issues, "failed"), cmdErr
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.plan", err), err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("publish.plan", err), err
	}
	facts = ordinaryNoteFacts(facts)
	plan := domain.PublishPlan{ProfileName: profile.Name, Target: profile.Target, Renderer: profile.Renderer}
	selectedAssets := map[string]bool{}
	violatedAssets := map[string]bool{}
	linkGraphNotes := make([]domain.Note, 0, len(facts))
	for _, fact := range facts {
		eligibility := publishops.ClassifyNoteEligibility(profile, fact.note)
		if !eligibility.Selected {
			plan.Skipped = append(plan.Skipped, domain.PublishItem{ID: fact.note.ID, Kind: "note", Title: fact.note.Title, SourcePath: fact.note.Path, Reason: eligibility.Reason})
			continue
		}
		violations := publishops.ClassifyNoteViolations(fact.note)
		if len(violations) > 0 {
			plan.Violations = append(plan.Violations, violations...)
			continue
		}
		plan.Selected = append(plan.Selected, domain.PublishItem{ID: fact.note.ID, Kind: "note", Title: fact.note.Title, SourcePath: fact.note.Path, OutputPath: publishNoteOutputPath(profile.Target, fact.note)})
		plan.Sources = append(plan.Sources, publishSourceFromNote(fact.note))
		linkGraphNotes = append(linkGraphNotes, fact.note)
		if profile.Assets.IncludeLinkedAssets {
			for _, link := range pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: fact.note.ID, SourcePath: fact.note.Path, Body: fact.note.Body}) {
				item, violation := classifyPublishAsset(root, profile, link)
				if violation != nil {
					if !violatedAssets[violation.Path] {
						plan.Violations = append(plan.Violations, *violation)
						violatedAssets[violation.Path] = true
					}
					continue
				}
				if !selectedAssets[item.SourcePath] {
					plan.Selected = append(plan.Selected, item)
					selectedAssets[item.SourcePath] = true
				}
			}
		}
	}
	plan.LinkGraph = publishPlanLinkGraph(linkGraphNotes)
	projection := domain.NewProjection("publish.plan", "发布计划已生成。")
	if len(plan.Violations) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["source_count"] = fmt.Sprint(len(plan.Sources))
	projection.Facts["link_count"] = fmt.Sprint(len(plan.LinkGraph))
	projection.Facts["broken_link_count"] = fmt.Sprint(publishBrokenLinkCount(plan.LinkGraph))
	projection.Facts["selected_count"] = fmt.Sprint(publishItemCount(plan.Selected, "note"))
	projection.Facts["selected_asset_count"] = fmt.Sprint(publishItemCount(plan.Selected, "asset"))
	projection.Facts["skipped_count"] = fmt.Sprint(len(plan.Skipped))
	projection.Facts["asset_violation_count"] = fmt.Sprint(publishViolationClassCount(plan.Violations, domain.PublishViolationAssetNotAllowed))
	projection.Facts["blocking_count"] = fmt.Sprint(publishBlockingItemCount(plan.Violations))
	projection.Facts["manual_review_count"] = fmt.Sprint(len(plan.ManualReview))
	projection.Data = map[string]any{"plan": plan}
	if len(plan.Violations) == 0 {
		projection.Actions = []domain.Action{{Name: "build", Command: fmt.Sprintf("pinax publish build --profile %s --target %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	} else {
		projection.Actions = []domain.Action{{Name: "review", Command: fmt.Sprintf("pinax publish plan --profile %s --target %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	}
	return projection, nil
}

func (s *Service) PublishBuild(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.build")
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.build", profile.Name, issues)
		return publishProfileProjection("publish.build", profile, issues, "failed"), cmdErr
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	if profile.Target != domain.PublishTargetGitHubWiki && profile.Target != domain.PublishTargetGitHubGist && profile.Target != domain.PublishTargetHTTP && (profile.Target != domain.PublishTargetGitHubPages || profile.Renderer != domain.PublishRendererHugo) {
		return publishPlaceholder("publish.build", req)
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.build", cmdErr), cmdErr
	}
	planProjection, err := s.PublishPlan(ctx, PublishRequest{VaultPath: root, Profile: profile.Name, Target: string(profile.Target)})
	if err != nil {
		return planProjection, err
	}
	plan, err := publishPlanFromProjection(planProjection)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(plan.Violations) > 0 {
		err := &domain.CommandError{Code: "publish_plan_blocked", Message: "Publish build is blocked by plan violations", Hint: "Run pinax publish plan --profile " + shellQuote(profile.Name) + " --target " + shellQuote(string(profile.Target)) + " --vault <vault> --json"}
		projection := domain.NewErrorProjection("publish.build", err)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Data = map[string]any{"plan": plan}
		return projection, err
	}
	notes, err := publishNotesByPath(root)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	started := time.Now().UTC()
	if profile.Target == domain.PublishTargetGitHubPages && profile.Renderer == domain.PublishRendererHugo {
		return s.publishBuildHugoPages(ctx, root, outDir, profile, plan, notes, started)
	}
	if profile.Target == domain.PublishTargetGitHubGist || profile.Target == domain.PublishTargetHTTP {
		err = writeBundlePublishOutput(root, outDir, profile, plan, notes)
	} else {
		err = writeWikiPublishOutput(root, outDir, profile, plan, notes)
	}
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(scan.Findings) > 0 {
		err := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Review the redacted scan findings and rebuild"}
		projection := domain.NewErrorProjection("publish.build", err)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, err
	}
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	finished := time.Now().UTC()
	receiptRel, err := publishops.WritePublishReceipt(root, domain.PublishReceipt{RunID: publishRunID(finished), ProfileName: profile.Name, Target: profile.Target, Renderer: profile.Renderer, StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), DurationMS: finished.Sub(started).Milliseconds(), Counts: map[string]int{"selected": publishItemCount(plan.Selected, "note"), "assets": publishItemCount(plan.Selected, "asset"), "violations": len(plan.Violations)}, OutputHash: outputHash, RedactionSummary: map[string]string{"scan_findings": fmt.Sprint(len(scan.Findings))}, DeployStatus: "not_deployed"})
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	summary := "Wiki 发布产物已生成。"
	if profile.Target == domain.PublishTargetGitHubGist || profile.Target == domain.PublishTargetHTTP {
		summary = "Markdown 分享产物已生成。"
	}
	projection := domain.NewProjection("publish.build", summary)
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["selected_count"] = fmt.Sprint(publishItemCount(plan.Selected, "note"))
	projection.Facts["asset_count"] = fmt.Sprint(publishItemCount(plan.Selected, "asset"))
	projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
	projection.Facts["manifest_path"] = "pinax-publish-manifest.json"
	projection.Facts["receipt_path"] = receiptRel
	projection.Evidence = []string{"pinax-publish-manifest.json", receiptRel}
	projection.Data = map[string]any{"manifest_path": "pinax-publish-manifest.json", "receipt_path": receiptRel, "scan": scan}
	return projection, nil
}

func (s *Service) publishBuildHugoPages(ctx context.Context, root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note, started time.Time) (domain.Projection, error) {
	runID := publishRunID(started)
	stageRel := filepath.ToSlash(filepath.Join(".pinax", "publish", "staging", runID))
	stageDir := filepath.Join(root, filepath.FromSlash(stageRel))
	staging, err := publishops.BuildHugoStagingProject(publishops.HugoStagingRequest{VaultRoot: root, StageRoot: stageDir, Profile: profile, Plan: plan, Notes: notes})
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	stageScan, err := publishops.ScanPublishTree(stageDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(stageScan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish staging output contains blocked content", Hint: "Review the redacted scan findings and rebuild"}
		projection := domain.NewErrorProjection("publish.build", cmdErr)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["scan_findings"] = fmt.Sprint(len(stageScan.Findings))
		projection.Data = map[string]any{"scan": stageScan}
		return projection, cmdErr
	}
	hugoResult, err := publishops.HugoAdapter{Timeout: 2 * time.Minute}.Build(ctx, stageDir, outDir)
	if err != nil {
		projection := errorProjection("publish.build", err)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["renderer"] = string(profile.Renderer)
		projection.Data = map[string]any{"hugo": hugoResult}
		return projection, err
	}
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(scan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Review the redacted scan findings and rebuild"}
		projection := domain.NewErrorProjection("publish.build", cmdErr)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	}
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	finished := time.Now().UTC()
	receiptRel, err := publishops.WritePublishReceipt(root, domain.PublishReceipt{RunID: runID, ProfileName: profile.Name, Target: profile.Target, Renderer: profile.Renderer, StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), DurationMS: finished.Sub(started).Milliseconds(), Counts: map[string]int{"selected": publishItemCount(plan.Selected, "note"), "assets": publishItemCount(plan.Selected, "asset"), "violations": len(plan.Violations)}, OutputHash: outputHash, RedactionSummary: map[string]string{"scan_findings": fmt.Sprint(len(scan.Findings)), "staging_scan_findings": fmt.Sprint(len(stageScan.Findings))}, DeployStatus: "not_deployed"})
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	projection := domain.NewProjection("publish.build", "Pages 发布产物已生成。")
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["selected_count"] = fmt.Sprint(publishItemCount(plan.Selected, "note"))
	projection.Facts["asset_count"] = fmt.Sprint(publishItemCount(plan.Selected, "asset"))
	projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
	projection.Facts["staging_scan_findings"] = fmt.Sprint(len(stageScan.Findings))
	projection.Facts["theme"] = staging.Theme
	projection.Facts["staging_files"] = fmt.Sprint(staging.FilesWritten)
	projection.Facts["manifest_path"] = filepath.ToSlash(filepath.Join(stageRel, "data", "pinax", "manifest.json"))
	projection.Facts["receipt_path"] = receiptRel
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(stageRel, "data", "pinax", "manifest.json")), receiptRel}
	projection.Data = map[string]any{"manifest_path": filepath.ToSlash(filepath.Join(stageRel, "data", "pinax", "manifest.json")), "receipt_path": receiptRel, "scan": scan, "staging_scan": stageScan, "hugo": hugoResult}
	return projection, nil
}

func (s *Service) PublishThemeList(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	themes := publishops.BuiltinThemeInfos()
	projection := domain.NewProjection("publish.theme.list", "发布主题已列出。")
	projection.Facts["themes"] = fmt.Sprint(len(themes))
	for i, theme := range themes {
		prefix := fmt.Sprintf("theme.%d", i+1)
		projection.Facts[prefix+".name"] = theme.Name
		projection.Facts[prefix+".source"] = theme.Source
		projection.Facts[prefix+".contract"] = theme.ContractVersion
		projection.Facts[prefix+".required_layouts"] = fmt.Sprint(len(theme.RequiredLayouts))
	}
	projection.Data = map[string]any{"themes": themes}
	return projection, nil
}

func (s *Service) PublishThemeEject(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.theme.eject", cmdErr), cmdErr
	}
	files, err := publishops.WriteBuiltinTheme(strings.TrimSpace(req.Theme), outDir)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_theme_unavailable", Message: err.Error(), Hint: "Run pinax publish theme list --json to inspect available built-in themes"}
		return domain.NewErrorProjection("publish.theme.eject", cmdErr), cmdErr
	}
	infos := publishops.BuiltinThemeInfos()
	info := infos[0]
	projection := domain.NewProjection("publish.theme.eject", "发布主题已导出。")
	projection.Facts["theme"] = info.Name
	projection.Facts["source"] = info.Source
	projection.Facts["contract"] = info.ContractVersion
	projection.Facts["files"] = fmt.Sprint(len(files))
	projection.Evidence = files
	projection.Data = map[string]any{"theme": info, "files": files}
	return projection, nil
}

func (s *Service) PublishDeploy(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.deploy")
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	if strings.TrimSpace(req.Repo) != "" {
		profile.Deploy.Mode = domain.PublishDeployModeGit
		profile.Deploy.Repo = strings.TrimSpace(req.Repo)
	}
	if strings.TrimSpace(req.Branch) != "" {
		profile.Deploy.Branch = strings.TrimSpace(req.Branch)
	}
	if strings.TrimSpace(req.Endpoint) != "" {
		profile.Deploy.Mode = domain.PublishDeployModeHTTP
		profile.Deploy.Endpoint = strings.TrimSpace(req.Endpoint)
	}
	if strings.TrimSpace(req.Method) != "" {
		profile.Deploy.Method = strings.TrimSpace(req.Method)
	}
	if strings.TrimSpace(req.SecretRef) != "" {
		profile.Deploy.SecretRef = strings.TrimSpace(req.SecretRef)
	}
	if strings.TrimSpace(req.GistID) != "" {
		profile.Deploy.Mode = domain.PublishDeployModeGist
		profile.Deploy.GistID = strings.TrimSpace(req.GistID)
	}
	if strings.TrimSpace(req.Visibility) != "" {
		profile.Deploy.Visibility = strings.TrimSpace(req.Visibility)
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.deploy", profile.Name, issues)
		return publishProfileProjection("publish.deploy", profile, issues, "failed"), cmdErr
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	policy, err := publishops.ParseDeployPolicy(publishops.DeployPolicyRequest{VaultRoot: root, Profile: profile})
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_deploy_policy_invalid", Message: err.Error(), Hint: "Check deploy mode, repo, branch and strategy"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	if policy.Mode == domain.PublishDeployModeNone {
		projection := domain.NewProjection("publish.deploy", "发布部署已跳过。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		return projection, nil
	}
	if !req.Yes {
		cmdErr := &domain.CommandError{Code: "approval_required", Message: "Publish deploy requires --yes before writing to the delivery target", Hint: "Review the publish output and rerun with --yes"}
		projection := domain.NewErrorProjection("publish.deploy", cmdErr)
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["branch"] = policy.Branch
		return projection, cmdErr
	}
	if projection, err := validatePublishDeployInput(root, outDir, profile); err != nil {
		return projection, err
	}
	if policy.Mode == domain.PublishDeployModeGist {
		result, err := publishDeployGist(ctx, root, outDir, policy)
		if err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Check gh authentication and retry"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["files"] = fmt.Sprint(result.Files)
		projection.Facts["visibility"] = policy.Visibility
		if result.URL != "" {
			projection.Facts["url"] = result.URL
		}
		projection.Data = map[string]any{"files": result.Files, "url": result.URL}
		return projection, nil
	}
	if policy.Mode == domain.PublishDeployModeHTTP {
		result, err := publishDeployHTTP(ctx, root, outDir, policy)
		if err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Check the HTTP endpoint and retry"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["http_status"] = fmt.Sprint(result.StatusCode)
		projection.Facts["files"] = fmt.Sprint(result.Files)
		if result.URL != "" {
			projection.Facts["url"] = result.URL
		}
		projection.Data = map[string]any{"files": result.Files, "http_status": result.StatusCode, "url": result.URL}
		return projection, nil
	}
	if publishDeployRepoIsRemote(policy.Repo) {
		cmdErr := &domain.CommandError{Code: "publish_deploy_remote_unsupported", Message: "Remote publish deploy is not implemented yet", Hint: "Use a local deploy repository path for this step"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	result, err := publishDeployLocalGit(ctx, root, outDir, policy)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Inspect the deploy repository and retry"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
	projection.Facts["mode"] = string(policy.Mode)
	projection.Facts["target"] = string(policy.Target)
	projection.Facts["branch"] = policy.Branch
	projection.Facts["strategy"] = policy.Strategy
	projection.Facts["files"] = fmt.Sprint(result.Files)
	projection.Facts["committed"] = fmt.Sprint(result.Committed)
	projection.Data = map[string]any{"files": result.Files, "committed": result.Committed}
	return projection, nil
}

func (s *Service) PublishServe(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.serve", cmdErr), cmdErr
	}
	if scan, err := publishops.ScanPublishTree(outDir); err != nil {
		return errorProjection("publish.serve", err), err
	} else if len(scan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Rebuild publish output before preview"}
		projection := domain.NewErrorProjection("publish.serve", cmdErr)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if !publishServeHostAllowed(host) {
		cmdErr := &domain.CommandError{Code: "publish_serve_host_unsafe", Message: "publish serve host must be loopback", Hint: "Use --host 127.0.0.1"}
		return domain.NewErrorProjection("publish.serve", cmdErr), cmdErr
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(req.Port)))
	if err != nil {
		return errorProjection("publish.serve", err), err
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{Handler: http.FileServer(http.Dir(outDir))}
	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	addr := listener.Addr().(*net.TCPAddr)
	served := false
	if req.Once {
		url := "http://" + net.JoinHostPort(host, fmt.Sprint(addr.Port)) + "/"
		resp, err := http.Get(url)
		if err != nil {
			_ = server.Shutdown(ctx)
			return errorProjection("publish.serve", err), err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		served = resp.StatusCode >= 200 && resp.StatusCode < 500
		_ = server.Shutdown(ctx)
		if err := <-serveErr; err != nil {
			return errorProjection("publish.serve", err), err
		}
	} else {
		select {
		case err := <-serveErr:
			if err != nil {
				return errorProjection("publish.serve", err), err
			}
		case <-ctx.Done():
			_ = server.Shutdown(context.Background())
			return errorProjection("publish.serve", ctx.Err()), ctx.Err()
		}
	}
	projection := domain.NewProjection("publish.serve", "发布预览服务已启动。")
	projection.Facts["profile"] = strings.TrimSpace(req.Profile)
	projection.Facts["host"] = host
	projection.Facts["port"] = fmt.Sprint(addr.Port)
	projection.Facts["served"] = fmt.Sprint(served || !req.Once)
	projection.Facts["url"] = "http://" + net.JoinHostPort(host, fmt.Sprint(addr.Port)) + "/"
	return projection, nil
}

func publishServeHostAllowed(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validatePublishDeployInput(root, outDir string, profile domain.PublishProfile) (domain.Projection, error) {
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	if len(scan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Rebuild publish output before deploy"}
		projection := domain.NewErrorProjection("publish.deploy", cmdErr)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	}
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	receipt, err := latestPublishReceipt(root, profile)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_deploy_validation_failed", Message: err.Error(), Hint: "Run pinax publish build for this profile and output before deploy"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	if receipt.OutputHash != outputHash {
		cmdErr := &domain.CommandError{Code: "publish_deploy_validation_failed", Message: "Publish output hash does not match the latest receipt", Hint: "Rebuild publish output before deploy"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	return domain.Projection{}, nil
}

func latestPublishReceipt(root string, profile domain.PublishProfile) (domain.PublishReceipt, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "runs", "*", "receipt.json"))
	if err != nil {
		return domain.PublishReceipt{}, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return domain.PublishReceipt{}, err
		}
		var receipt domain.PublishReceipt
		if err := json.Unmarshal(body, &receipt); err != nil {
			return domain.PublishReceipt{}, err
		}
		if receipt.ProfileName == profile.Name && receipt.Target == profile.Target {
			return receipt, nil
		}
	}
	return domain.PublishReceipt{}, fmt.Errorf("matching publish receipt was not found")
}

type publishDeployResult struct {
	Files      int
	Committed  bool
	StatusCode int
	URL        string
}

func publishDeployGist(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	contentPath := filepath.Join(outDir, "pinax-gist.md")
	if _, err := os.Stat(contentPath); err != nil {
		return publishDeployResult{}, err
	}
	args := []string{"gist"}
	if policy.GistID == "" {
		args = append(args, "create", contentPath, "--desc", "Pinax publish")
		if policy.Visibility == "public" {
			args = append(args, "--public")
		}
	} else {
		args = append(args, "edit", policy.GistID, contentPath)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return publishDeployResult{}, fmt.Errorf("gh gist deploy failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return publishDeployResult{Files: 1, URL: publishRedactGitOutput(strings.TrimSpace(stdout.String()), vaultRoot)}, nil
}

func publishDeployHTTP(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	endpoint, err := url.Parse(policy.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return publishDeployResult{}, fmt.Errorf("publish http endpoint is invalid")
	}
	manifest, err := os.ReadFile(filepath.Join(outDir, "pinax-publish-manifest.json"))
	if err != nil {
		return publishDeployResult{}, err
	}
	content, err := os.ReadFile(filepath.Join(outDir, "pinax-gist.md"))
	if err != nil {
		return publishDeployResult{}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("manifest", string(manifest)); err != nil {
		return publishDeployResult{}, err
	}
	if err := writer.WriteField("content", string(content)); err != nil {
		return publishDeployResult{}, err
	}
	if err := writer.Close(); err != nil {
		return publishDeployResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, policy.Method, endpoint.String(), &body)
	if err != nil {
		return publishDeployResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token, ok := publishTokenFromSecretRef(policy.SecretRef); ok {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return publishDeployResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return publishDeployResult{}, fmt.Errorf("publish http endpoint returned status %d", resp.StatusCode)
	}
	return publishDeployResult{Files: 2, StatusCode: resp.StatusCode, URL: publishHTTPResponseURL(respBody, vaultRoot)}, nil
}

func publishTokenFromSecretRef(secretRef string) (string, bool) {
	secretRef = strings.TrimSpace(secretRef)
	if !strings.HasPrefix(secretRef, "env:") {
		return "", false
	}
	token := os.Getenv(strings.TrimPrefix(secretRef, "env:"))
	return token, token != ""
}

func publishHTTPResponseURL(body []byte, root string) string {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return publishRedactGitOutput(strings.TrimSpace(payload.URL), root)
}

func publishDeployLocalGit(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	repoDir := policy.Repo
	if !filepath.IsAbs(repoDir) {
		repoDir = filepath.Join(vaultRoot, filepath.FromSlash(repoDir))
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return publishDeployResult{}, err
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return publishDeployResult{}, err
		}
		if err := runPublishGit(ctx, repoDir, "init"); err != nil {
			return publishDeployResult{}, err
		}
	}
	if err := runPublishGit(ctx, repoDir, "checkout", "-B", policy.Branch); err != nil {
		return publishDeployResult{}, err
	}
	if err := runPublishGit(ctx, repoDir, "config", "user.email", "pinax@example.local"); err != nil {
		return publishDeployResult{}, err
	}
	if err := runPublishGit(ctx, repoDir, "config", "user.name", "Pinax"); err != nil {
		return publishDeployResult{}, err
	}
	if err := cleanPublishDeployWorktree(repoDir); err != nil {
		return publishDeployResult{}, err
	}
	files, err := copyPublishTree(outDir, repoDir)
	if err != nil {
		return publishDeployResult{}, err
	}
	if err := runPublishGit(ctx, repoDir, "add", "-A"); err != nil {
		return publishDeployResult{}, err
	}
	status, err := publishGitOutput(ctx, repoDir, "status", "--porcelain")
	if err != nil {
		return publishDeployResult{}, err
	}
	committed := strings.TrimSpace(status) != ""
	if committed {
		if err := runPublishGit(ctx, repoDir, "commit", "-m", "pinax publish deploy"); err != nil {
			return publishDeployResult{}, err
		}
	}
	return publishDeployResult{Files: files, Committed: committed}, nil
}

func cleanPublishDeployWorktree(repoDir string) error {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(repoDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyPublishTree(srcRoot, dstRoot string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := writePublishFile(filepath.Join(dstRoot, rel), body); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func runPublishGit(ctx context.Context, dir string, args ...string) error {
	_, err := publishGitOutput(ctx, dir, args...)
	return err
}

func publishGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func publishDeployRepoIsRemote(repo string) bool {
	repo = strings.TrimSpace(repo)
	return strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")
}

var publishCredentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
var publishAuthorizationPattern = regexp.MustCompile(`(?i)authorization:\s*bearer\s+[^\s]+`)
var publishTokenPattern = regexp.MustCompile(`(?i)token=[^\s]+`)
var publishAbsolutePathPattern = regexp.MustCompile(`(?i)(/home|/users)/[^\s'\"]+|[a-z]:\\[^\s'\"]+`)

func publishRedactGitOutput(value, root string) string {
	value = publishCredentialURLPattern.ReplaceAllString(value, "${1}[REDACTED_URL]@")
	value = publishAuthorizationPattern.ReplaceAllString(value, "[REDACTED]")
	value = publishTokenPattern.ReplaceAllString(value, "token=[REDACTED]")
	if strings.TrimSpace(root) != "" {
		value = strings.ReplaceAll(value, root, "[REDACTED_PATH]")
	}
	value = publishAbsolutePathPattern.ReplaceAllString(value, "[REDACTED_PATH]")
	return value
}

func cleanPublishOutPath(raw string) (string, *domain.CommandError) {
	out := strings.TrimSpace(raw)
	if out == "" {
		return "", &domain.CommandError{Code: "publish_out_required", Message: "publish build output directory is required", Hint: "Use --out ./dist/wiki"}
	}
	if hasPinaxPathSegment(out) {
		return "", &domain.CommandError{Code: "publish_out_unsafe", Message: "publish output directory must not be inside .pinax", Hint: "Use a dedicated publish output directory such as ./dist/wiki"}
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", &domain.CommandError{Code: "publish_out_invalid", Message: err.Error(), Hint: "Use a valid output directory"}
	}
	return abs, nil
}

func hasPinaxPathSegment(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if part == ".pinax" {
			return true
		}
	}
	return false
}

func publishPlanFromProjection(projection domain.Projection) (domain.PublishPlan, error) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return domain.PublishPlan{}, fmt.Errorf("publish plan data missing")
	}
	plan, ok := data["plan"].(domain.PublishPlan)
	if !ok {
		return domain.PublishPlan{}, fmt.Errorf("publish plan payload missing")
	}
	return plan, nil
}

func publishNotesByPath(root string) (map[string]domain.Note, error) {
	facts, err := scanNoteFacts(root)
	if err != nil {
		return nil, err
	}
	notes := map[string]domain.Note{}
	for _, fact := range ordinaryNoteFacts(facts) {
		notes[fact.note.Path] = fact.note
	}
	return notes, nil
}

func writeWikiPublishOutput(root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	pageByTitle := publishWikiPageIndex(plan, notes)
	for _, item := range plan.Selected {
		switch item.Kind {
		case "note":
			note, ok := notes[item.SourcePath]
			if !ok {
				continue
			}
			if err := writePublishFile(filepath.Join(outDir, publishWikiPageFilename(note)), []byte(publishWikiNoteBody(note, pageByTitle))); err != nil {
				return err
			}
		case "asset":
			if err := copyPublishAsset(root, outDir, item.SourcePath); err != nil {
				return err
			}
		}
	}
	if err := writePublishFile(filepath.Join(outDir, "Home.md"), []byte(publishWikiHome(plan))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "_Sidebar.md"), []byte(publishWikiSidebar(plan, notes))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "Tags.md"), []byte(publishWikiTagsIndex(plan, notes))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "Types.md"), []byte(publishWikiTypesIndex(plan, notes))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "Sources.md"), []byte(publishWikiSourcesIndex(plan, notes))); err != nil {
		return err
	}
	manifest := domain.PublishManifest{SchemaVersion: "pinax.publish_manifest.v1", ProfileName: profile.Name, Target: profile.Target, Renderer: string(profile.Renderer), Items: plan.Selected}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(outDir, "pinax-publish-manifest.json"), append(body, '\n'))
}

func writeBundlePublishOutput(root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	title := strings.TrimSpace(profile.Site.Title)
	if title == "" {
		title = profile.Name
	}
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, note := range publishSelectedNotes(plan, notes) {
		body := strings.TrimSpace(note.Body)
		body = strings.ReplaceAll(body, "../assets/", "assets/")
		body = strings.ReplaceAll(body, "../attachments/", "attachments/")
		if body == "" {
			body = "# " + note.Title
		}
		b.WriteString(body)
		b.WriteString("\n\n---\n\n")
	}
	if err := writePublishFile(filepath.Join(outDir, "pinax-gist.md"), []byte(b.String())); err != nil {
		return err
	}
	for _, item := range plan.Selected {
		if item.Kind == "asset" {
			if err := copyPublishAsset(root, outDir, item.SourcePath); err != nil {
				return err
			}
		}
	}
	manifest := domain.PublishManifest{SchemaVersion: "pinax.publish_manifest.v1", ProfileName: profile.Name, Target: profile.Target, Renderer: string(profile.Renderer), Items: plan.Selected}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(outDir, "pinax-publish-manifest.json"), append(body, '\n'))
}

func writePublishFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func copyPublishAsset(root, outDir, rel string) error {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(outDir, filepath.FromSlash(rel)), body)
}

func publishWikiPageFilename(note domain.Note) string {
	return publishSlug(note) + ".md"
}

func publishWikiNoteBody(note domain.Note, pageByTitle map[string]string) string {
	body := strings.TrimSpace(note.Body)
	body = rewriteWikiNoteLinks(body, pageByTitle)
	body = strings.ReplaceAll(body, "../assets/", "assets/")
	body = strings.ReplaceAll(body, "../attachments/", "attachments/")
	if body == "" {
		body = "# " + note.Title
	}
	return body + "\n"
}

var wikiNoteLinkPattern = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]+)?(?:\|([^\]]+))?\]\]`)

func rewriteWikiNoteLinks(body string, pageByTitle map[string]string) string {
	return wikiNoteLinkPattern.ReplaceAllStringFunc(body, func(raw string) string {
		match := wikiNoteLinkPattern.FindStringSubmatch(raw)
		if len(match) < 2 {
			return raw
		}
		target := strings.TrimSpace(match[1])
		display := target
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			display = strings.TrimSpace(match[2])
		}
		page, ok := pageByTitle[strings.ToLower(target)]
		if !ok {
			return display + " (unpublished)"
		}
		return "[[" + display + "|" + page + "]]"
	})
}

func publishWikiPageIndex(plan domain.PublishPlan, notes map[string]domain.Note) map[string]string {
	index := map[string]string{}
	for _, note := range publishSelectedNotes(plan, notes) {
		index[strings.ToLower(note.Title)] = strings.TrimSuffix(publishWikiPageFilename(note), ".md")
		if note.ID != "" {
			index[strings.ToLower(note.ID)] = strings.TrimSuffix(publishWikiPageFilename(note), ".md")
		}
	}
	return index
}

func publishWikiHome(plan domain.PublishPlan) string {
	var b strings.Builder
	b.WriteString("# Home\n\n")
	b.WriteString("## Entries\n\n")
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		b.WriteString("- [[")
		b.WriteString(strings.TrimSuffix(filepath.Base(item.OutputPath), filepath.Ext(item.OutputPath)))
		b.WriteString("|")
		b.WriteString(item.Title)
		b.WriteString("]]\n")
	}
	return b.String()
}

func publishWikiSidebar(plan domain.PublishPlan, notes map[string]domain.Note) string {
	var b strings.Builder
	b.WriteString("# Pages\n\n")
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		note, ok := notes[item.SourcePath]
		if !ok {
			continue
		}
		b.WriteString("- [[")
		b.WriteString(strings.TrimSuffix(publishWikiPageFilename(note), ".md"))
		b.WriteString("|")
		b.WriteString(note.Title)
		b.WriteString("]]\n")
	}
	return b.String()
}

func publishWikiTagsIndex(plan domain.PublishPlan, notes map[string]domain.Note) string {
	groups := map[string][]domain.Note{}
	for _, note := range publishSelectedNotes(plan, notes) {
		for _, tag := range note.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				groups[tag] = append(groups[tag], note)
			}
		}
	}
	return publishWikiGroupedIndex("Tags", groups)
}

func publishWikiTypesIndex(plan domain.PublishPlan, notes map[string]domain.Note) string {
	groups := map[string][]domain.Note{}
	for _, note := range publishSelectedNotes(plan, notes) {
		kind := strings.TrimSpace(note.Kind)
		if kind == "" {
			kind = "note"
		}
		groups[kind] = append(groups[kind], note)
	}
	return publishWikiGroupedIndex("Types", groups)
}

func publishWikiSourcesIndex(plan domain.PublishPlan, notes map[string]domain.Note) string {
	var b strings.Builder
	b.WriteString("# Sources\n\n")
	for _, note := range publishSelectedNotes(plan, notes) {
		if note.Kind != "source" {
			continue
		}
		b.WriteString("- [[")
		b.WriteString(strings.TrimSuffix(publishWikiPageFilename(note), ".md"))
		b.WriteString("|")
		b.WriteString(note.Title)
		b.WriteString("]]\n")
	}
	return b.String()
}

func publishWikiGroupedIndex(title string, groups map[string][]domain.Note) string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, key := range keys {
		b.WriteString("## ")
		b.WriteString(key)
		b.WriteString("\n\n")
		for _, note := range groups[key] {
			b.WriteString("- [[")
			b.WriteString(strings.TrimSuffix(publishWikiPageFilename(note), ".md"))
			b.WriteString("|")
			b.WriteString(note.Title)
			b.WriteString("]]\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func publishSelectedNotes(plan domain.PublishPlan, notes map[string]domain.Note) []domain.Note {
	selected := make([]domain.Note, 0)
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		note, ok := notes[item.SourcePath]
		if ok {
			selected = append(selected, note)
		}
	}
	return selected
}

func hashPublishTree(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		hash.Write(body)
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func publishRunID(t time.Time) string {
	return "run-" + t.UTC().Format("20060102T150405.000000000Z")
}

func publishPlaceholder(command string, req PublishRequest) (domain.Projection, error) {
	err := &domain.CommandError{Code: "publish_not_implemented", Message: "publish command is not implemented yet", Hint: "Continue the pinax-hugo-static-publish OpenSpec change"}
	projection := domain.NewErrorProjection(command, err)
	projection.Facts["profile"] = strings.TrimSpace(req.Profile)
	projection.Facts["target"] = strings.TrimSpace(req.Target)
	projection.Facts["renderer"] = strings.TrimSpace(req.Renderer)
	projection.Actions = []domain.Action{{Name: "implement", Command: "openspec status --change pinax-hugo-static-publish --json"}}
	return projection, err
}

func publishNoteOutputPath(target domain.PublishTarget, note domain.Note) string {
	slug := publishSlug(note)
	if target == domain.PublishTargetGitHubWiki {
		return slug + ".md"
	}
	if target == domain.PublishTargetGitHubGist || target == domain.PublishTargetHTTP {
		return "pinax-gist.md#" + slug
	}
	return filepath.ToSlash(filepath.Join("entries", slug, "index.html"))
}

func publishSourceFromNote(note domain.Note) domain.PublishSource {
	return domain.PublishSource{ID: note.ID, Title: note.Title, SourcePath: note.Path, Kind: note.Kind, Status: note.Status, Project: note.Project, Folder: note.Folder}
}

func publishPlanLinkGraph(notes []domain.Note) []domain.NoteLink {
	if len(notes) == 0 {
		return nil
	}
	outgoing, _ := BuildEnhancedLinkGraph(notes)
	links := make([]domain.NoteLink, 0)
	for _, note := range notes {
		links = append(links, outgoing[note.Path]...)
	}
	return links
}

func classifyPublishAsset(root string, profile domain.PublishProfile, link domain.AssetLink) (domain.PublishItem, *domain.PublishViolation) {
	assetPath := strings.TrimSpace(filepath.ToSlash(link.AssetPath))
	if !isSafePublishAssetPath(assetPath) {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset path is not safe for publishing")
	}
	if !publishAssetExtensionAllowed(profile.Assets.AllowedExtensions, filepath.Ext(assetPath)) {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset extension is not allowed for publishing")
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(assetPath)))
	if err != nil || info.IsDir() {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset file is missing")
	}
	if profile.Assets.MaxBytes > 0 && info.Size() > profile.Assets.MaxBytes {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset exceeds the publish size limit")
	}
	return domain.PublishItem{Kind: "asset", SourcePath: assetPath, OutputPath: publishAssetOutputPath(assetPath)}, nil
}

func isSafePublishAssetPath(assetPath string) bool {
	if assetPath == "" || filepath.IsAbs(assetPath) || strings.HasPrefix(assetPath, ".pinax/") || strings.HasPrefix(assetPath, "../") || strings.Contains(assetPath, "/../") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(assetPath)))
	return clean == assetPath && clean != "." && clean != ".."
}

func publishAssetExtensionAllowed(allowed []string, ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	for _, candidate := range allowed {
		if strings.ToLower(strings.TrimSpace(candidate)) == ext {
			return true
		}
	}
	return false
}

func publishAssetOutputPath(assetPath string) string {
	if strings.HasPrefix(assetPath, "assets/") {
		return assetPath
	}
	return filepath.ToSlash(filepath.Join("assets", filepath.Base(assetPath)))
}

func publishAssetViolation(path, message string) *domain.PublishViolation {
	return &domain.PublishViolation{Class: domain.PublishViolationAssetNotAllowed, Path: path, Severity: "blocking", Message: message}
}

func publishItemCount(items []domain.PublishItem, kind string) int {
	count := 0
	for _, item := range items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func publishViolationClassCount(violations []domain.PublishViolation, class domain.PublishViolationClass) int {
	count := 0
	for _, violation := range violations {
		if violation.Class == class {
			count++
		}
	}
	return count
}

func publishBrokenLinkCount(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if link.Broken || link.Status == string(domain.LinkStatusBroken) {
			count++
		}
	}
	return count
}

func publishBlockingItemCount(violations []domain.PublishViolation) int {
	seen := map[string]bool{}
	for _, violation := range violations {
		key := violation.Path
		if key == "" {
			key = string(violation.Class)
		}
		seen[key] = true
	}
	return len(seen)
}

func publishSlug(note domain.Note) string {
	base := strings.TrimSpace(note.Title)
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path))
	}
	slug := publishSlugString(base)
	if slug == "" && strings.TrimSpace(note.Title) != "" {
		slug = publishSlugString(strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)))
	}
	if slug == "" {
		return "note"
	}
	return slug
}

func publishSlugString(base string) string {
	base = strings.ToLower(base)
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
