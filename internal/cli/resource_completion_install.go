package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	completion "github.com/yeisme/cli-kit/completion"
)

type resourceCompletionReport struct {
	Shell           string `json:"shell"`
	Target          string `json:"target_path"`
	Installed       bool   `json:"installed"`
	Owned           bool   `json:"owned"`
	Discoverability string `json:"discoverability"`
	ShellLoaded     string `json:"shell_loaded"`
	Activation      string `json:"activation_command,omitempty"`
	Changed         bool   `json:"changed"`
	ErrorCode       string `json:"error_code,omitempty"`
}

func resourceCompletionCommands(render func(*cobra.Command, resourceCompletionReport) error) []*cobra.Command {
	commands := []*cobra.Command{}
	for _, action := range []string{"install", "doctor", "uninstall"} {
		action := action
		cmd := &cobra.Command{Use: action + " <shell>", Short: action + " shell completion without editing shell startup files"}
		cmd.ValidArgsFunction = func(_ *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			out := []string{}
			for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
				if strings.HasPrefix(sh, prefix) {
					out = append(out, sh)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			report := resourceCompletionReport{ShellLoaded: "unknown", Discoverability: "unknown"}
			if len(args) != 1 {
				report.ErrorCode = "missing_argument"
				return render(cmd, report)
			}
			shell := args[0]
			report.Shell = shell
			if _, ok := map[string]bool{"bash": true, "zsh": true, "fish": true, "powershell": true}[shell]; !ok {
				report.ErrorCode = "invalid_shell"
				return render(cmd, report)
			}
			base, err := os.UserConfigDir()
			if err != nil {
				report.ErrorCode = "config_unavailable"
				return render(cmd, report)
			}
			product := cmd.Root().Name()
			dir := filepath.Join(base, product, "completion", shell)
			report.Target = completion.TargetPath(dir, product, shell)
			var script bytes.Buffer
			switch shell {
			case "zsh":
				err = cmd.Root().GenZshCompletion(&script)
			case "bash":
				err = cmd.Root().GenBashCompletionV2(&script, true)
			case "fish":
				err = cmd.Root().GenFishCompletion(&script, true)
			case "powershell":
				err = cmd.Root().GenPowerShellCompletionWithDesc(&script)
			}
			if err != nil {
				report.ErrorCode = "completion_generation_failed"
				return render(cmd, report)
			}
			opts := completion.Options{
				Program:       product,
				Shell:         shell,
				Directory:     dir,
				Script:        script.String(),
				DirectoryMode: 0o700,
			}
			project := func(result completion.Report) {
				report.Shell = result.Shell
				report.Target = result.TargetPath
				report.Installed = result.Installed
				report.Owned = result.Owned
				report.Activation = result.Activation
				report.Changed = result.Changed
				if result.Discoverability != "" {
					report.Discoverability = result.Discoverability
				}
				if report.Discoverability == "not_installed" {
					report.Discoverability = "unknown"
				}
				if shell == "zsh" {
					if paths, ok := os.LookupEnv("FPATH"); ok {
						report.Discoverability = "not_on_fpath"
						for _, path := range filepath.SplitList(paths) {
							if filepath.Clean(path) == filepath.Clean(dir) {
								report.Discoverability = "available"
							}
						}
					}
				}
			}
			mapError := func(runErr error) string {
				if errors.Is(runErr, completion.ErrForeignFile) {
					return "foreign_completion_file"
				}
				if errors.Is(runErr, completion.ErrBackupFailed) {
					return "completion_backup_failed"
				}
				if strings.Contains(runErr.Error(), "unsafe completion target") {
					return "unsafe_completion_target"
				}
				if strings.Contains(runErr.Error(), "completion target unreadable") {
					return "completion_unreadable"
				}
				return "completion_write_failed"
			}
			preflight, err := completion.Run(completion.ActionDoctor, opts)
			project(preflight)
			if err != nil {
				report.ErrorCode = mapError(err)
				return render(cmd, report)
			}
			yes, _ := cmd.Flags().GetBool("yes")
			dry, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")
			if action == "doctor" || !yes || dry {
				return render(cmd, report)
			}
			opts.Force = force
			if action == "uninstall" {
				result, runErr := completion.Run(completion.ActionUninstall, opts)
				project(result)
				if runErr != nil {
					report.ErrorCode = mapError(runErr)
					return render(cmd, report)
				}
				return render(cmd, report)
			}
			result, runErr := completion.Run(completion.ActionInstall, opts)
			project(result)
			if runErr != nil {
				report.ErrorCode = mapError(runErr)
				return render(cmd, report)
			}
			return render(cmd, report)
		}
		if action != "doctor" {
			cmd.Flags().Bool("yes", false, "Confirm the local completion file change")
			cmd.Flags().Bool("dry-run", false, "Preview without changing files")
			cmd.Flags().Bool("force", false, "Back up a foreign file before replacing it")
		}
		commands = append(commands, cmd)
	}
	return commands
}

func resourceCompletionMessage(r resourceCompletionReport) string {
	if r.ErrorCode != "" {
		return "Completion operation failed: " + r.ErrorCode
	}
	return fmt.Sprintf("Completion file installed: %t; shell visibility: %s; current shell loaded: %s", r.Installed, r.Discoverability, r.ShellLoaded)
}

func resourceCompletionFacts(r resourceCompletionReport) map[string]any {
	return map[string]any{"shell": r.Shell, "installed": r.Installed, "owned": r.Owned, "discoverability": r.Discoverability, "shell_loaded": r.ShellLoaded, "target_path": r.Target, "changed": r.Changed, "activation_command": r.Activation}
}
func resourceCompletionStringFacts(r resourceCompletionReport) map[string]string {
	out := map[string]string{}
	for k, v := range resourceCompletionFacts(r) {
		out[k] = fmt.Sprint(v)
	}
	return out
}
func resourceCompletionNext(cmd *cobra.Command, r resourceCompletionReport) string {
	program := cmd.Root().Name()
	if r.ErrorCode != "" || r.Shell == "" {
		return program + " completion --help"
	}
	if cmd.Name() == "doctor" {
		if !r.Installed {
			return program + " completion install " + r.Shell + " --yes"
		}
		if r.Shell == "zsh" && r.Discoverability != "available" {
			return r.Activation
		}
		return program + " --help"
	}
	return program + " completion doctor " + r.Shell
}
