// RunAction: the Go port of run_menu_action() and run_install() from
// devops-deployment.sh. Unlike bash, the confirm prompt and the ✓/✗
// markers stay plain text — the colors.go palette paints the TUI chrome
// only, so this output reads the same under the TUI and the fallback.

package menu

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"devops-deployment-go/assets"
	"devops-deployment-go/internal/ask"
	"devops-deployment-go/internal/customsync"
	"devops-deployment-go/internal/term"
)

// errInstallScriptMissing is returned by runInstall when the tool's
// install-<tool>.sh is missing; the ERROR line itself is printed where
// bash prints it, inside run_install.
var errInstallScriptMissing = errors.New("install script not found")

// RunAction ports run_menu_action() from devops-deployment.sh: it runs the
// action behind one menu entry. root is the deployment root (passed through
// to ask.AskStackCloud for the __config entry); scriptDir is the directory
// holding install-<tool>.sh — the materialized embedded copy since the
// assets became built-in (the bash SCRIPT_DIR); rd is the caller's shared
// keystroke Reader, handed to __config's stack/cloud questions so their
// pickers read the same instance the menu reads. Callers that read no
// keystrokes — the line-based fallback menu, the tests — pass nil, and
// AskStackCloud creates a Reader only if a question turns out to be
// askable.
//
// Special actions:
//   - __all / __core install their tool lists in order, after a y/N
//     confirmation read from stdin;
//   - __config re-asks the stack/cloud questions — bash ends that case
//     with an explicit `return 0`, so the outcome is not propagated;
//   - __update_scripts re-runs the update (UpdateScripts — pure Go: sync
//     the embedded custom-scripts, no bash updater). Unlike bash, which
//     ends that case with an unconditional `return 0` too, the exit
//     status is returned here so a failed update can reach the caller.
//
// Any other action is a tool name: run_install() looks up
// <scriptDir>/install-<action>.sh and runs it. Single-tool runs go through
// the same ✓/✗ loop as the bulk runs, exactly like bash.
func RunAction(action, root, scriptDir string, rd *term.Reader) error {
	var tools []string

	switch action {
	case "__all":
		tools = []string{"kubectl", "k9s", "helm", "terraform", "aws", "brave", "helium", "bashtools", "fzf-setup"}
	case "__core":
		tools = []string{"kubectl", "k9s", "helm", "terraform", "aws", "bashtools"}
	case "__config":
		// bash: ask_stack_cloud; return 0 — errors are its own to report.
		ask.AskStackCloud(root, rd)
		return nil
	case "__update_scripts":
		return UpdateScripts()
	default:
		tools = []string{action}
	}

	// bash keys the confirmation on the action (`__all`/`__core`), not on
	// the tool count; those two happen to be the multi-tool entries.
	if action == "__all" || action == "__core" {
		fmt.Printf("\n? This will install %d tools. Continue? [y/N] ", len(tools))
		// bash `read -r confirm`: an empty line or a failed read (EOF)
		// both leave confirm empty, which fails the y/Y test → abort.
		line, _ := readLine(os.Stdin)
		if answer := strings.TrimSpace(line); answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	var firstErr error
	for _, tool := range tools {
		if err := runInstall(tool, scriptDir); err == nil {
			fmt.Printf("✓ %s done\n", tool)
		} else {
			fmt.Printf("✗ %s FAILED\n", tool)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// runInstall ports run_install() from devops-deployment.sh: run one tool's
// installer, or report it missing. TOOL_SCRIPTS maps every tool name to
// $SCRIPT_DIR/install-<tool>.sh, so constructing the path covers the whole
// mapping.
func runInstall(tool, scriptDir string) error {
	script := filepath.Join(scriptDir, "install-"+tool+".sh")
	// bash [[ -f "$script" ]]: exists as a regular file, symlinks included
	// (os.Stat follows them).
	if info, err := os.Stat(script); err != nil || !info.Mode().IsRegular() {
		fmt.Printf("ERROR: install script for '%s' not found (%s).\n", tool, script)
		return errInstallScriptMissing
	}
	fmt.Printf(">>> Installing %s...\n", tool)
	cmd := exec.Command("bash", script)
	// bash runs the installer with all three streams attached: installers
	// print progress and may prompt.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// UpdateScripts is the shared body of the __update_scripts action, the
// TUI's u/U binding and main's non-interactive `update` subcommand.
// Since the assets became embedded (built into the binary from
// GO/assets/), the update is pure Go: the embedded tree is extracted to a
// fresh temp directory and the maintained custom-scripts folder is synced
// from there into $HOME/.local/bin/scripts via customsync.Sync — the old
// bash update-scripts.sh half is gone (its only real behavior, re-copying
// the helpers into ~/.local/bin/scripts, is this sync). New files are
// added, differing files are overwritten, exec bits preserved, .DS_Store
// filtered out, and anything that exists only in the destination is never
// deleted; each new or changed item is listed indented, followed by
// `Custom scripts: <n> item(s) updated.` (or
// `Custom scripts: already up to date.`). The sync source line names the
// embedded tree and the build the binary came from (buildLabel). The temp
// extraction is removed when the function returns; a sync or extraction
// failure is the returned error.
func UpdateScripts() error {
	tmp, err := os.MkdirTemp("", "devops-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := assets.Extract(tmp); err != nil {
		return err
	}

	dst := filepath.Join(os.Getenv("HOME"), ".local", "bin", "scripts")
	fmt.Printf("\nSyncing custom scripts from embedded scripts (build %s) → %s\n", buildLabel(), dst)
	changed, report, err := customsync.Sync(filepath.Join(tmp, "custom-scripts"), dst)
	if err != nil {
		return err
	}
	for _, rel := range report {
		fmt.Printf("  %s\n", rel)
	}
	if changed == 0 {
		fmt.Println("Custom scripts: already up to date.")
	} else {
		fmt.Printf("Custom scripts: %d item(s) updated.\n", changed)
	}
	return nil
}

// buildLabel names the build the running binary came from: the vcs
// revision the Go toolchain records at build time (vcs.revision) when one
// exists — a build from a checkout — or "dev" otherwise (e.g. a plain
// `go build` in a non-VCS workspace like this repo). Best-effort by
// design: a missing label must never fail the update.
func buildLabel() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}
	return "dev"
}

// readLine reads one line from f (without the trailing newline), one byte
// at a time so nothing beyond the newline is consumed from the terminal —
// a later reader along the line still gets the remaining keystrokes. It
// mirrors bash `read -r`: end of input returns whatever was collected plus
// the read error, and the caller treats that as an empty answer. The
// caller's strings.TrimSpace stands in for the IFS whitespace trimming
// bash's read applies.
func readLine(f *os.File) (string, error) {
	var line []byte
	var buf [1]byte
	for {
		n, err := f.Read(buf[:])
		if n == 1 {
			if buf[0] == '\n' {
				return string(line), nil
			}
			line = append(line, buf[0])
			continue
		}
		if err != nil {
			return string(line), err
		}
	}
}
