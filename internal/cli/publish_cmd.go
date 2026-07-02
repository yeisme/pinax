package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
)

func renderPublishCommand(cmd *cobra.Command, ctx commandBuildContext, command string, run func(app.PublishEventSink) (domain.Projection, error)) error {
	mode := ctx.outputMode()
	seq := 1
	if mode == output.ModeEvents {
		if err := writePublishStreamEvent(cmd.OutOrStdout(), command, &seq, app.PublishEvent{Type: "start", Status: "running"}); err != nil {
			return err
		}
	}
	projection, err := run(publishLiveSink(cmd.ErrOrStderr(), cmd.OutOrStdout(), mode, command, &seq))
	if mode == output.ModeEvents {
		endType := "end"
		status := projection.Status
		if status == "" {
			status = "success"
		}
		if err != nil || status == "failed" {
			endType = "error"
			status = "failed"
		}
		if writeErr := writePublishStreamEvent(cmd.OutOrStdout(), command, &seq, app.PublishEvent{Type: endType, Status: status, Facts: map[string]string{"summary": projection.Summary}}); writeErr != nil {
			return writeErr
		}
		return err
	}
	return ctx.renderProjection(cmd, projection, err)
}

func publishLiveSink(stderr io.Writer, stdout io.Writer, mode output.Mode, command string, seq *int) app.PublishEventSink {
	switch mode {
	case output.ModeSummary:
		return func(event app.PublishEvent) {
			_, _ = fmt.Fprintf(stderr, "%s %s%s\n", event.Type, publishEventStatus(event.Status), publishEventFactsSuffix(event.Facts))
		}
	case output.ModeEvents:
		return func(event app.PublishEvent) {
			_ = writePublishStreamEvent(stdout, command, seq, event)
		}
	default:
		return nil
	}
}

func writePublishStreamEvent(w io.Writer, command string, seq *int, event app.PublishEvent) error {
	status := publishEventStatus(event.Status)
	payload := map[string]any{
		"spec_version": "1.0",
		"mode":         "events",
		"command":      command,
		"type":         event.Type,
		"seq":          *seq,
		"status":       status,
	}
	(*seq)++
	for key, value := range event.Facts {
		if strings.TrimSpace(value) != "" {
			payload[key] = value
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

func publishEventStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return "running"
	}
	return strings.TrimSpace(status)
}

func publishEventFactsSuffix(facts map[string]string) string {
	if len(facts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(facts[key])
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func addPublishCommands(root *cobra.Command, ctx commandBuildContext) {
	var profileName string
	var planTarget string
	var buildTarget string
	var buildOut string
	var doctorTarget string
	var doctorOut string
	var deployTarget string
	var deployOut string
	var deployRepo string
	var deployBranch string
	var deployEndpoint string
	var deployMethod string
	var deploySecretRef string
	var deployGistID string
	var deployVisibility string
	var deployProject string
	var deployYes bool
	var previewOut string
	var devOut string
	var devHost string
	var devPort int
	var devOnce bool
	var devWatch bool
	var serveOut string
	var serveHost string
	var servePort int
	var serveOnce bool
	var themeOut string
	var profileTarget string
	var renderer string
	var title string
	var baseURL string
	var theme string

	publishCmd := &cobra.Command{
		Use:   "publish",
		Short: "Build safe static publishing surfaces",
		Long:  "Build safe static publishing surfaces from a local Pinax vault. GitHub Pages and Wiki outputs are publish targets, not the vault source of truth.",
		Example: "pinax publish profile init public --target github-pages --renderer pinax-web --vault ./my-notes --json\n" +
			"pinax publish profile validate public --vault ./my-notes --json",
	}
	publishPlanCmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview what a publish operation would include",
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderPublishCommand(cmd, ctx, "publish.plan", func(sink app.PublishEventSink) (domain.Projection, error) {
				return ctx.svc.PublishPlan(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Target: planTarget, LiveEvents: sink})
			})
		},
	}
	publishPlanCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	publishPlanCmd.Flags().StringVar(&planTarget, "target", "", "Override publish target: local, github-pages, vercel, cloudflare-pages, github-wiki, github-gist, or http")
	publishCmd.AddCommand(publishPlanCmd)

	publishBuildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build a static publish output",
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderPublishCommand(cmd, ctx, "publish.build", func(sink app.PublishEventSink) (domain.Projection, error) {
				return ctx.svc.PublishBuild(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Target: buildTarget, Out: buildOut, LiveEvents: sink})
			})
		},
	}
	publishBuildCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	publishBuildCmd.Flags().StringVar(&buildTarget, "target", "", "Override publish target: local, github-pages, vercel, cloudflare-pages, github-wiki, github-gist, or http")
	publishBuildCmd.Flags().StringVar(&buildOut, "out", "", "Output directory for generated publish files")
	publishCmd.AddCommand(publishBuildCmd)

	publishDoctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check publish prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishDoctor(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Target: doctorTarget, Out: doctorOut})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	publishDoctorCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	publishDoctorCmd.Flags().StringVar(&doctorTarget, "target", "", "Override publish target: local, github-pages, vercel, cloudflare-pages, github-wiki, github-gist, or http")
	publishDoctorCmd.Flags().StringVar(&doctorOut, "out", "", "Output directory to validate")
	publishCmd.AddCommand(publishDoctorCmd)

	themeCmd := &cobra.Command{
		Use:   "theme",
		Short: "Manage built-in publish themes",
	}
	themeListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available publish themes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishThemeList(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	themeCmd.AddCommand(themeListCmd)
	themeEjectCmd := &cobra.Command{
		Use:   "eject <name>",
		Short: "Copy a built-in publish theme to a reviewable directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishThemeEject(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Theme: args[0], Out: themeOut})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	themeEjectCmd.Flags().StringVar(&themeOut, "out", "", "Output directory for ejected theme files")
	themeCmd.AddCommand(themeEjectCmd)
	publishCmd.AddCommand(themeCmd)

	publishDeployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a publish output to a controlled delivery target",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishDeploy(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Target: deployTarget, Out: deployOut, Repo: deployRepo, Branch: deployBranch, Endpoint: deployEndpoint, Method: deployMethod, SecretRef: deploySecretRef, GistID: deployGistID, Visibility: deployVisibility, Project: deployProject, Yes: deployYes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	publishDeployCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	publishDeployCmd.Flags().StringVar(&deployTarget, "target", "", "Override publish target: local, github-pages, vercel, cloudflare-pages, github-wiki, github-gist, or http")
	publishDeployCmd.Flags().StringVar(&deployOut, "out", "", "Publish output directory to deploy")
	publishDeployCmd.Flags().StringVar(&deployRepo, "repo", "", "Git repository path or URL for deploy")
	publishDeployCmd.Flags().StringVar(&deployBranch, "branch", "", "Git branch to deploy")
	publishDeployCmd.Flags().StringVar(&deployEndpoint, "endpoint", "", "HTTP endpoint for deploy")
	publishDeployCmd.Flags().StringVar(&deployMethod, "method", "", "HTTP method for deploy: POST or PUT")
	publishDeployCmd.Flags().StringVar(&deploySecretRef, "secret-ref", "", "Secret reference for deploy auth, such as env:PINAX_SHARE_TOKEN")
	publishDeployCmd.Flags().StringVar(&deployGistID, "gist-id", "", "Existing GitHub Gist ID to update")
	publishDeployCmd.Flags().StringVar(&deployVisibility, "visibility", "", "Gist visibility: secret or public")
	publishDeployCmd.Flags().StringVar(&deployProject, "project", "", "Project name for Vercel or Cloudflare Pages deploy")
	publishDeployCmd.Flags().BoolVar(&deployYes, "yes", false, "Approve deploy writes")
	publishCmd.AddCommand(publishDeployCmd)

	previewCmd := &cobra.Command{
		Use:   "preview",
		Short: "Approve local publish previews before deploy",
	}
	previewApproveCmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve the current publish output for deploy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderPublishCommand(cmd, ctx, "publish.preview.approve", func(sink app.PublishEventSink) (domain.Projection, error) {
				return ctx.svc.PublishPreviewApprove(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Out: previewOut, LiveEvents: sink})
			})
		},
	}
	previewApproveCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	previewApproveCmd.Flags().StringVar(&previewOut, "out", "", "Publish output directory to approve")
	previewCmd.AddCommand(previewApproveCmd)
	publishCmd.AddCommand(previewCmd)

	publishDevCmd := &cobra.Command{
		Use:   "dev",
		Short: "Build and serve a local publish preview",
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderPublishCommand(cmd, ctx, "publish.dev", func(sink app.PublishEventSink) (domain.Projection, error) {
				return ctx.svc.PublishDev(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Out: devOut, Host: devHost, Port: devPort, Once: devOnce, Watch: devWatch, LiveEvents: sink})
			})
		},
	}
	publishDevCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	publishDevCmd.Flags().StringVar(&devOut, "out", "", "Publish output directory to build and serve")
	publishDevCmd.Flags().StringVar(&devHost, "host", "127.0.0.1", "Loopback host for preview")
	publishDevCmd.Flags().IntVar(&devPort, "port", 4173, "Port for preview, or 0 for an available port")
	publishDevCmd.Flags().BoolVar(&devOnce, "once", false, "Serve one smoke request and exit")
	publishDevCmd.Flags().BoolVar(&devWatch, "watch", false, "Watch vault Markdown, publish profiles, and renderer source for rebuilds")
	publishCmd.AddCommand(publishDevCmd)

	publishServeCmd := &cobra.Command{
		Use:   "serve",
		Short: "Preview a publish output on loopback HTTP",
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderPublishCommand(cmd, ctx, "publish.serve", func(sink app.PublishEventSink) (domain.Projection, error) {
				return ctx.svc.PublishServe(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: profileName, Out: serveOut, Host: serveHost, Port: servePort, Once: serveOnce, LiveEvents: sink})
			})
		},
	}
	publishServeCmd.Flags().StringVar(&profileName, "profile", "", "Publish profile name")
	publishServeCmd.Flags().StringVar(&serveOut, "out", "", "Publish output directory to preview")
	publishServeCmd.Flags().StringVar(&serveHost, "host", "127.0.0.1", "Loopback host for preview")
	publishServeCmd.Flags().IntVar(&servePort, "port", 4173, "Port for preview, or 0 for an available port")
	publishServeCmd.Flags().BoolVar(&serveOnce, "once", false, "Serve one smoke request and exit")
	publishCmd.AddCommand(publishServeCmd)

	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage static publish profiles",
	}

	profileInitCmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Create or update a publish profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderPublishCommand(cmd, ctx, "publish.profile.init", func(sink app.PublishEventSink) (domain.Projection, error) {
				return ctx.svc.PublishProfileInit(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: args[0], Target: profileTarget, Renderer: renderer, Title: title, BaseURL: baseURL, Theme: theme, LiveEvents: sink})
			})
		},
	}
	profileInitCmd.Flags().StringVar(&profileTarget, "target", "github-pages", "Publish target: local, github-pages, vercel, cloudflare-pages, github-wiki, github-gist, or http")
	profileInitCmd.Flags().StringVar(&renderer, "renderer", "pinax-web", "Renderer: pinax-web, hugo, or none")
	profileInitCmd.Flags().StringVar(&title, "title", "", "Site title for the publish profile")
	profileInitCmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL for GitHub Pages output")
	profileInitCmd.Flags().StringVar(&theme, "theme", "builtin:pinax-encyclopedia", "Theme source, such as builtin:pinax-encyclopedia or local:<path>")
	profileCmd.AddCommand(profileInitCmd)

	profileValidateCmd := &cobra.Command{
		Use:   "validate <name>",
		Short: "Validate a publish profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishProfileValidate(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	profileCmd.AddCommand(profileValidateCmd)

	profileShowCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a publish profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishProfileShow(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath, Profile: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	profileCmd.AddCommand(profileShowCmd)

	profileListCmd := &cobra.Command{
		Use:   "list",
		Short: "List publish profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PublishProfileList(cmd.Context(), app.PublishRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	profileCmd.AddCommand(profileListCmd)

	publishCmd.AddCommand(profileCmd)
	root.AddCommand(publishCmd)
}
