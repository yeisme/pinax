package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
			suffix := map[string]string{"bash": ".bash", "zsh": "", "fish": ".fish", "powershell": ".ps1"}
			ext, ok := suffix[shell]
			if !ok {
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
			name := product + ext
			if shell == "zsh" {
				name = "_" + product
			}
			report.Target = filepath.Join(dir, name)
			marker := "# managed-by: " + product + "-completion"
			if info, err := os.Lstat(report.Target); err == nil {
				report.Installed = true
				if !info.Mode().IsRegular() || info.Size() > 4<<20 {
					report.ErrorCode = "unsafe_completion_target"
					return render(cmd, report)
				}
				body, err := os.ReadFile(report.Target)
				if err != nil {
					report.ErrorCode = "completion_unreadable"
					return render(cmd, report)
				}
				report.Owned = strings.Contains(string(body), marker)
			} else if !os.IsNotExist(err) {
				report.ErrorCode = "completion_unreadable"
				return render(cmd, report)
			}
			if shell == "zsh" {
				report.Activation = "fpath=('" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "' $fpath); autoload -Uz compinit; compinit"
				if paths, ok := os.LookupEnv("FPATH"); ok {
					report.Discoverability = "not_on_fpath"
					for _, path := range filepath.SplitList(paths) {
						if filepath.Clean(path) == filepath.Clean(dir) {
							report.Discoverability = "available"
						}
					}
				}
			}
			yes, _ := cmd.Flags().GetBool("yes")
			dry, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")
			if action == "doctor" || !yes || dry {
				return render(cmd, report)
			}
			if report.Installed && !report.Owned && (!force || action == "uninstall") {
				report.ErrorCode = "foreign_completion_file"
				return render(cmd, report)
			}
			if action == "uninstall" {
				if report.Installed {
					if err := os.Remove(report.Target); err != nil {
						report.ErrorCode = "completion_write_failed"
						return render(cmd, report)
					}
					report.Installed = false
					report.Changed = true
				}
				return render(cmd, report)
			}
			var b bytes.Buffer
			switch shell {
			case "zsh":
				err = cmd.Root().GenZshCompletion(&b)
			case "bash":
				err = cmd.Root().GenBashCompletionV2(&b, true)
			case "fish":
				err = cmd.Root().GenFishCompletion(&b, true)
			case "powershell":
				err = cmd.Root().GenPowerShellCompletionWithDesc(&b)
			}
			if err != nil {
				report.ErrorCode = "completion_generation_failed"
				return render(cmd, report)
			}
			content := marker + "\n" + b.String()
			if shell == "zsh" {
				first, rest, _ := strings.Cut(b.String(), "\n")
				content = first + "\n" + marker + "\n" + rest
			}
			if err = os.MkdirAll(dir, 0700); err != nil {
				report.ErrorCode = "completion_write_failed"
				return render(cmd, report)
			}
			if force && report.Installed {
				old, err := os.ReadFile(report.Target)
				if err != nil || os.WriteFile(report.Target+".bak-"+time.Now().UTC().Format("20060102T150405.000000000"), old, 0600) != nil {
					report.ErrorCode = "completion_backup_failed"
					return render(cmd, report)
				}
			}
			tmp, err := os.CreateTemp(dir, ".completion-")
			if err != nil {
				report.ErrorCode = "completion_write_failed"
				return render(cmd, report)
			}
			defer func() { _ = os.Remove(tmp.Name()) }()
			_, err = tmp.WriteString(content)
			closeErr := tmp.Close()
			if err != nil || closeErr != nil {
				report.ErrorCode = "completion_write_failed"
				return render(cmd, report)
			}
			if err = os.Rename(tmp.Name(), report.Target); err != nil {
				report.ErrorCode = "completion_write_failed"
				return render(cmd, report)
			}
			report.Installed = true
			report.Owned = true
			report.Changed = true
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
