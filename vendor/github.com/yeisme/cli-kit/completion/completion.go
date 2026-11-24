// Package completion provides the mechanical half of shell-completion
// installation: file ownership, atomic replace, backup, and post-mutation facts.
//
// Boundary: this package never generates a completion script and never decides
// what a CLI's candidates are. Owners pass the already-generated script body and
// keep their own per-shell help, candidate query, sorting and authorization.
// The shared part is only the file lifecycle that was duplicated 13 times across
// owner repositories.
package completion

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DigestPrefix marks the line that binds a generated script to its payload.
// The spelling matches the earliest owner implementation so adopting this
// package does not invalidate already-installed files.
const DigestPrefix = "# completion-body-sha256: "

// Options describes one completion install/doctor/uninstall request.
type Options struct {
	// Program is the binary name, e.g. "pinax". It forms the default file name
	// and the ownership marker.
	Program string
	// Shell is one of bash, zsh, fish, powershell, or a custom shell name.
	Shell string
	// Directory is the completion directory the owner resolved.
	Directory string
	// Script is the fully generated script body for this shell. Required for
	// ActionInstall. For zsh it may omit the leading `#compdef` prolog; Render
	// adds it.
	Script string
	// Force allows replacing a file that this tool does not own.
	Force bool
	// DirectoryMode is used when the completion directory must be created.
	// Zero means 0o755. Owners that keep completions private pass 0o700.
	DirectoryMode os.FileMode
}

// Action is the requested operation.
type Action int

const (
	ActionDoctor Action = iota
	ActionInstall
	ActionUninstall
)

// Report is the machine-readable outcome. Owners project it into their own
// envelope; this package does not own output formatting.
type Report struct {
	Shell           string `json:"shell"`
	TargetPath      string `json:"target_path"`
	Installed       bool   `json:"installed"`
	Owned           bool   `json:"owned"`
	Discoverability string `json:"discoverability"`
	Activation      string `json:"activation_command,omitempty"`
	Backup          string `json:"backup_path,omitempty"`
	Changed         bool   `json:"changed"`
}

// ErrForeignFile is returned when the target exists but is not owned by this
// tool and Force is false.
var ErrForeignFile = errors.New("completion target is not owned by this program")

// ErrBackupFailed is returned when a backup could not be created, written or
// closed. The target is left untouched.
var ErrBackupFailed = errors.New("completion backup failed; target left unchanged")

// Marker returns the ownership marker line for a program.
func Marker(program string) string {
	return "# managed-by: " + program + "-completion"
}

// TargetPath resolves the completion file name for a shell.
func TargetPath(directory, program, shell string) string {
	switch shell {
	case "zsh":
		return filepath.Join(directory, "_"+program)
	case "fish":
		return filepath.Join(directory, program+".fish")
	case "powershell":
		return filepath.Join(directory, program+".ps1")
	default:
		return filepath.Join(directory, program+".bash")
	}
}

// Payload returns the script body as it will be written after the digest line,
// including the zsh `#compdef` prolog that zsh autoload requires.
func Payload(program, shell, script string) string {
	if shell != "zsh" {
		return script
	}
	prolog := "#compdef " + program
	if strings.HasPrefix(script, prolog) {
		return script
	}
	if strings.TrimSpace(script) == "" {
		return prolog + "\n"
	}
	return prolog + "\n" + script
}

// Render produces the owned file body: optional #compdef prolog, marker line,
// digest line, then the exact payload. The digest binds the payload so an edited
// file is no longer considered owned.
func Render(program, shell, script string) string {
	prolog, payload := splitProlog(program, shell, Payload(program, shell, script))
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%s# managed-by: %s-completion\n%s%s\n%s", prolog, program, DigestPrefix, hex.EncodeToString(sum[:]), payload)
}

// splitProlog separates the zsh `#compdef` line from the rest of the payload.
// zsh only registers a completion function when `#compdef` is the FIRST line, so
// the marker and digest cannot precede it.
func splitProlog(program, shell, payload string) (string, string) {
	if shell != "zsh" {
		return "", payload
	}
	first, rest, ok := strings.Cut(payload, "\n")
	if !ok || !strings.HasPrefix(first, "#compdef ") {
		return "", payload
	}
	return first + "\n", rest
}

// IsOwned reports whether body was produced by this program for this shell.
//
// Layout, after an optional zsh `#compdef` first line:
//
//	# managed-by: <program>-completion
//	# completion-body-sha256: <hex>
//	<payload>
//
// Ownership is structural, never a substring test: the marker must start the
// file and the digest must match the bytes that follow it. A file without a
// digest is NOT owned here — see Run, which accepts a digest-less file only when
// its payload is byte-identical to the script it is about to write.
func IsOwned(program, shell, body string) bool {
	rest, ok := payloadOf(program, shell, body)
	if !ok {
		return false
	}
	// An empty payload is legitimate: a zsh file may consist only of its
	// #compdef prolog, and the digest of the empty string still binds it.
	digestLine, payload, ok := strings.Cut(rest, "\n")
	if !ok {
		return false
	}
	sum := sha256.Sum256([]byte(payload))
	return digestLine == DigestPrefix+hex.EncodeToString(sum[:])
}

// ownedWithPayload additionally accepts a pre-digest install whose body is
// exactly the payload this run would write. Without this an upgrade would treat
// every existing managed file as foreign and demand --force.
func ownedWithPayload(program, shell, body, script string) bool {
	if IsOwned(program, shell, body) {
		return true
	}
	// Compare against the body a pre-digest installer would have written: the
	// optional zsh prolog, the marker, then the payload with no digest line.
	prolog, payload := splitProlog(program, shell, Payload(program, shell, script))
	return prolog+Marker(program)+"\n"+payload == body
}

// payloadOf strips the optional prolog, the marker and the digest line.
func payloadOf(program, shell, body string) (string, bool) {
	rest := body
	if shell == "zsh" {
		if first, remainder, ok := strings.Cut(rest, "\n"); ok && strings.HasPrefix(first, "#compdef ") {
			rest = remainder
		}
	}
	rest, ok := strings.CutPrefix(rest, Marker(program)+"\n")
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(rest, DigestPrefix) {
		return "", false
	}
	return rest, true
}

// Run executes the requested action.
func Run(action Action, opts Options) (Report, error) {
	report := Report{Shell: opts.Shell, TargetPath: TargetPath(opts.Directory, opts.Program, opts.Shell)}
	if err := readState(&report, opts); err != nil {
		return report, err
	}
	report.Activation = activation(opts)

	switch action {
	case ActionDoctor:
		return report, nil

	case ActionUninstall:
		if !report.Installed {
			return report, nil
		}
		if !report.Owned {
			return report, ErrForeignFile
		}
		if err := os.Remove(report.TargetPath); err != nil {
			return report, err
		}
		return finish(report, opts, true)

	case ActionInstall:
		want := Render(opts.Program, opts.Shell, opts.Script)
		if report.Installed {
			body, err := os.ReadFile(report.TargetPath)
			if err != nil {
				return report, fmt.Errorf("completion target unreadable: %w", err)
			}
			if string(body) == want {
				return finish(report, opts, false)
			}
			if !report.Owned && !opts.Force {
				return report, ErrForeignFile
			}
			backupPath, err := backup(report.TargetPath)
			if err != nil {
				return report, err
			}
			report.Backup = backupPath
		}
		directoryMode := opts.DirectoryMode
		if directoryMode == 0 {
			directoryMode = 0o755
		}
		if err := os.MkdirAll(opts.Directory, directoryMode); err != nil {
			return report, err
		}
		if err := writeAtomic(report.TargetPath, want); err != nil {
			return report, err
		}
		return finish(report, opts, true)

	default:
		return report, fmt.Errorf("unknown completion action %d", action)
	}
}

// readState fills Installed/Owned from the current filesystem state. When the
// caller supplied a script, a digest-less legacy install with the exact payload
// still counts as owned.
func readState(report *Report, opts Options) error {
	report.Installed, report.Owned = false, false
	info, err := os.Lstat(report.TargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("completion target unreadable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return errors.New("unsafe completion target")
	}
	report.Installed = true
	body, err := os.ReadFile(report.TargetPath)
	if err != nil {
		return fmt.Errorf("completion target unreadable: %w", err)
	}
	if opts.Script != "" {
		report.Owned = ownedWithPayload(opts.Program, opts.Shell, string(body), opts.Script)
	} else {
		report.Owned = IsOwned(opts.Program, opts.Shell, string(body))
	}
	return nil
}

// finish recomputes facts from post-mutation state and records whether the run
// changed anything.
func finish(previous Report, opts Options, changed bool) (Report, error) {
	out := Report{
		Shell:      opts.Shell,
		TargetPath: previous.TargetPath,
		Backup:     previous.Backup,
		Activation: activation(opts),
	}
	if err := readState(&out, opts); err != nil {
		return out, err
	}
	out.Changed = changed
	if opts.Shell == "zsh" {
		out.Discoverability = discoverability(opts.Directory, out.Installed)
	}
	return out, nil
}

func activation(opts Options) string {
	if opts.Shell != "zsh" {
		return ""
	}
	quoted := strings.ReplaceAll(opts.Directory, "'", `'"'"'`)
	return fmt.Sprintf("fpath=('%s' $fpath); autoload -Uz compinit; compinit", quoted)
}

func discoverability(directory string, installed bool) string {
	paths, ok := os.LookupEnv("FPATH")
	if !ok {
		return "unknown"
	}
	if !installed {
		return "not_installed"
	}
	for _, entry := range filepath.SplitList(paths) {
		if entry == directory {
			return "available"
		}
	}
	return "not_on_fpath"
}

func backup(path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackupFailed, err)
	}
	defer src.Close()
	backupPath := path + ".bak"
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackupFailed, err)
	}
	if _, err := dst.ReadFrom(src); err != nil {
		dst.Close()
		return "", fmt.Errorf("%w: %v", ErrBackupFailed, err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackupFailed, err)
	}
	return backupPath, nil
}

func writeAtomic(path, body string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".completion-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
