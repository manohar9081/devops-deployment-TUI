// Command devops-deployment-go is a Go port of the repo's bash entrypoint
// (devops-deployment.sh). Increment 1: deployment root resolution and OS
// detection. Increment 2: package-manager detection, package installation
// and the prerequisite checks. Increment 3: config.env loading and the
// stack/cloud notes shown after the prerequisite checks. Increment 4: the
// LOCAL_BIN PATH block and the tmp directory. Increment 5a: the terminal
// helpers in internal/term. Increment 6b: the stack/cloud questions
// (ask.AskStackCloud) asked before the menu when no config.env exists.
// Increment 7a: the menu entries and the action runner (internal/menu).
// Increment 8b: the menu phase wired into main — the pkg manager
// re-detection, the UseTUI gate dispatching to TuiLoop/FallbackMenu and
// the final Done line — completing the port.
//
// Increment 9: the stack/cloud questions accept an empty Enter as the
// defaults instead of cancelling; the deployment root resolves "from
// anywhere" (env override → beside the binary → cwd → the bash default),
// announced with the new Deployment root startup line; and the update
// action additionally syncs the maintained GO/custom-scripts folder into
// ~/.local/bin/scripts via internal/customsync.
//
// Bug-fix increment: the whole program shares one keystroke Reader
// (term.NewReaderStdin), constructed here and handed down sequentially to
// ask.AskStackCloud and menu.TuiLoop. Each interactive phase keeps reading
// the same instance, so an escape window left open when a phase ends (a
// lone ESC in a picker) parks its follow-up byte on the Reader the next
// phase reads, instead of stranding it on an abandoned Reader whose helper
// goroutine swallows the next keystroke — matching bash's single input
// stream.
//
// Bug-fix increment: the deployment-root probe chain also checks the
// parent directories of the executable's directory and of the cwd. The
// built binary lives at <repo>/GO/devops-deployment-go, so neither the
// exe-dir nor the cwd probe saw the repo-root scripts/ and resolution
// fell through to $HOME/devops-deployment, where install actions fail
// with "install script for 'kubectl' not found".
//
// Increment 10: the binary becomes the single entrypoint. With no
// argument it behaves exactly as before; `update` re-runs the menu's
// update action non-interactively (no config load, no prerequisite
// checks, no final Done line), `help`/`-h`/`--help` print a usage
// block, and any other argument is rejected with it.
//
// Increment 11: `devops` on the PATH becomes a portable copy instead of
// a symlink (install-bashtools.sh copies it there and pins the checkout).
// The root chain gains the pinned root — the absolute path stored in
// ~/.config/devops-deployment/path, written whenever the root is found
// beside the binary or the cwd — so the copy resolves its checkout from
// anywhere; and `update` afterwards refreshes that installed copy from
// <root>/GO/devops-deployment-go, atomically and best-effort.
//
// Increment 12: the binary becomes self-contained. Every runtime asset —
// scripts/install-*.sh, bash-files/, custom-scripts/ — is embedded at
// build time from GO/assets/ (package assets), so `go build` captures a
// new version of everything and the repo folders are dev-time sources
// only. The menu path materializes the embedded tree into a fresh
// per-run temp dir and dispatches the installers from there, exporting
// SCRIPT_DIR / SCRIPT_TMPDIR / ROOT / BASHFILES_DIR / DEVOPS_SELF for
// the child installers; the root chain's marker file becomes
// GO/assets/scripts/install-kubectl.sh under the candidate directory (a
// root is the repo root, not the GO module dir); and the update action
// is pure Go — it syncs the embedded custom-scripts into
// ~/.local/bin/scripts and refreshes the installed `devops` copy, no
// bash updater in between.
//
// Bug-fix increment (root semantics, refresh source, asset staleness):
// the root marker is validated UNDER each candidate — the GO module
// directory itself can no longer match, so the root stays the repo root
// (config.env, custom/, devops-deployment.sh) and a live mis-pin to GO
// cannot recur; `update` refreshes the installed copy from the checkout
// binary's real location <root>/GO/devops-deployment-go; asset
// materialization drops the shared $TMPDIR/devops-assets with its .ok
// reuse marker — whose stale trees survived binary upgrades — for a
// fresh os.MkdirTemp dir per run, removed when the interactive path
// returns; and the standalone installers self-locate their defaults from
// assets/scripts (SCRIPT_DIR = the script's own dir, SCRIPT_TMPDIR = the
// repo tmp, install-bashtools.sh ROOT = the repo root) instead of
// assuming the bash-era $HOME/devops-deployment layout.
//
// Increment 13: the deleted-command gap closes. The self-refresh logic
// becomes ensureCopy — a missing installed file is now installed, not an
// error — and `update` ensures the conventional command
// ~/.local/bin/devops instead of the running file, so running the freshly
// built binary's `update` re-creates a deleted `devops` command; the new
// `install` subcommand deploys whatever binary was run to that same
// target.
//
// Increment 14: the interactive path joins the ensure. At every launch
// with no argument, right before the menu, main best-effort ensures the
// installed `devops` command (~/.local/bin/devops) against the running
// binary: silent when it IS the running binary or matches it
// byte-for-byte (the common case), a one-line refresh when it differs, a
// one-line install when it is missing — a deleted command is re-created
// by the next interactive launch, the same gap `update` closes. Warnings
// only: the ensure never aborts the interactive session.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"devops-deployment-go/assets"
	"devops-deployment-go/internal/ask"
	"devops-deployment-go/internal/config"
	"devops-deployment-go/internal/menu"
	"devops-deployment-go/internal/pkgmgr"
	"devops-deployment-go/internal/term"
)

// exeDir returns the directory of the running executable, with symlinks
// resolved (os.Executable + filepath.EvalSymlinks + filepath.Dir), or ""
// when the executable's path cannot be determined.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return ""
	}
	return filepath.Dir(resolved)
}

// exeSelf returns the running executable's path verbatim — os.Executable
// only, deliberately NOT symlink-resolved — for the DEVOPS_SELF
// environment handed to the child installers: install-bashtools.sh copies
// that exact file onto the PATH as `devops`, and it must be the real copy
// that is running, not whatever a symlink on the way points at. Empty
// when the path cannot be determined (the env var is then not exported).
func exeSelf() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// cwd returns the process's working directory, or "" when it cannot be
// determined (the chain then skips the cwd probes).
func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// rootEvidence identifies which candidate of the root chain produced the
// deployment root. resolveRoot persists only the marker-validated
// filesystem candidates — the exe-dir and cwd ones — to the pin file.
type rootEvidence int

const (
	evidenceEnv          rootEvidence = iota // DEVOPS_DEPLOYMENT_ROOT, verbatim
	evidenceExeDir                           // marker beside the executable
	evidenceExeDirParent                     // marker one level above it
	evidencePin                              // the root-pin file's value
	evidenceCwd                              // marker in the working directory
	evidenceCwdParent                        // marker one level above it
	evidenceDefault                          // $HOME/devops-deployment fallback
)

// EnvRootPin overrides where the root-pin file lives (tests point it at
// a fixture); the file itself holds one absolute path — the deployment
// root an installed `devops` copy resolves when nothing beside the
// binary or the cwd matches.
const EnvRootPin = "DEVOPS_DEPLOYMENT_ROOT_PIN"

// rootPinPath returns the root-pin file's location:
// $DEVOPS_DEPLOYMENT_ROOT_PIN when set, else
// $HOME/.config/devops-deployment/path. Both come from the environment
// so tests can redirect them.
func rootPinPath() string {
	if p := os.Getenv(EnvRootPin); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "devops-deployment", "path")
}

// readRootPin returns the pinned root: the pin file's content, trimmed,
// and only when it is an absolute path. Anything else — a missing file,
// a read error, empty or relative content — reads as no pin (""); the
// candidate then just falls through the chain like any other
// non-matching one, so a stale pin never breaks resolution.
func readRootPin() string {
	data, err := os.ReadFile(rootPinPath())
	if err != nil {
		return ""
	}
	pin := strings.TrimSpace(string(data))
	if !filepath.IsAbs(pin) {
		return ""
	}
	return pin
}

// writeRootPin best-effort persists root to the pin file: the write is
// skipped when the file already holds the same root, and every error —
// an unwritable directory, a failed write — is ignored silently. The
// pin is a convenience, never a promise: a stale or missing one only
// costs the fallback probes below it.
func writeRootPin(root string) {
	path := rootPinPath()
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) == root {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(root+"\n"), 0o644)
}

// rootMarker is the file that marks a directory as a deployment root:
// the kubectl installer inside the GO module's assets tree, resolved
// UNDER the candidate directory. Since the assets are embedded from
// GO/assets/ (increment 12), the marker lives at
// <root>/GO/assets/scripts/install-kubectl.sh — so a root is the REPO
// ROOT (the checkout holding config.env, custom/ and
// devops-deployment.sh), not the GO module directory itself: the GO
// directory has no GO/ of its own and cannot match, which keeps an
// exeDir/cwd landing on the module dir from being mistaken for the root.
const rootMarker = "GO/assets/scripts/install-kubectl.sh"

// hasRootMarker reports whether dir carries the deployment-root marker.
func hasRootMarker(dir string) bool {
	return hasRegularFile(filepath.Join(dir, rootMarker))
}

// resolveRoot determines the deployment root the way the bash script
// pins SCRIPT_DIR, extended so the binary works "from anywhere". The
// first match wins:
//
//  1. the DEVOPS_DEPLOYMENT_ROOT environment variable, verbatim;
//  2. the directory of the executable, when it looks like the checkout —
//     it must carry the root marker (GO/assets/scripts/install-kubectl.sh
//     under it);
//  3. the parent of the executable's directory, same marker file — the
//     normal case: the binary lives at <repo-root>/GO/devops-deployment-go
//     and the repo root one level up matches;
//  4. the pinned root from the pin file (rootPinPath), same marker
//     file — how the `devops` copy installed on the PATH finds the
//     checkout it was installed from;
//  5. the current working directory, same marker file;
//  6. the parent of the current working directory, same marker file;
//  7. the bash-compatible default $HOME/devops-deployment.
//
// The root — the repo root — carries config.env and tmp/; the
// installers themselves are dispatched from the materialized embedded
// copy, so a root without a GO/ module only limits those two, never the
// install actions.
//
// Side effect: when a marker-validated candidate wins (2, 3, 5 or 6 —
// not the env override, the pin itself or the default), the root is
// best-effort persisted to the pin file (writeRootPin), so the copy
// installed on the PATH keeps resolving this checkout however it is
// invoked.
func resolveRoot() string {
	root, evidence := resolveRootFrom(
		os.Getenv("DEVOPS_DEPLOYMENT_ROOT"),
		readRootPin(),
		exeDir(),
		cwd(),
		os.Getenv("HOME"),
	)
	switch evidence {
	case evidenceExeDir, evidenceExeDirParent, evidenceCwd, evidenceCwdParent:
		writeRootPin(root)
	}
	return root
}

// resolveRootFrom is resolveRoot's candidate chain with every input
// injected, so tests can drive it against fixtures. It probes envRoot,
// the marker file under exeDir and under its parent, pinValue, cwd and
// its parent, in that order, and falls back to $home/devops-deployment.
// The returned evidence says which candidate won. Every candidate except
// envRoot and the default must carry the marker file
// (GO/assets/scripts/install-kubectl.sh — see rootMarker); empty
// candidates are skipped, and pinValue only counts when it is absolute —
// the guarantee readRootPin makes for the real pin file.
func resolveRootFrom(envRoot, pinValue, exeDir, cwd, home string) (string, rootEvidence) {
	if envRoot != "" {
		return envRoot, evidenceEnv
	}
	if exeDir != "" {
		if hasRootMarker(exeDir) {
			return exeDir, evidenceExeDir
		}
		// The binary usually sits inside the checkout at
		// <repo-root>/GO/devops-deployment-go, so the exe-dir probe above
		// fails and one level up — the repo root — carries the marker
		// file. filepath.Join and filepath.Dir both clean, so the probe
		// path and the returned parent are canonical.
		if hasRootMarker(filepath.Join(exeDir, "..")) {
			return filepath.Dir(exeDir), evidenceExeDirParent
		}
	}
	if pinValue != "" && filepath.IsAbs(pinValue) && hasRootMarker(pinValue) {
		return pinValue, evidencePin
	}
	if cwd != "" {
		if hasRootMarker(cwd) {
			return cwd, evidenceCwd
		}
		if hasRootMarker(filepath.Join(cwd, "..")) {
			return filepath.Dir(cwd), evidenceCwdParent
		}
	}
	return filepath.Join(home, "devops-deployment"), evidenceDefault
}

// detectOS maps runtime.GOOS to the friendly OS labels used by
// devops-deployment.sh's detect_os().
func detectOS() string {
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return "unknown"
	}
}

// requiredTool mirrors one REQUIRED_TOOLS line of
// devops-deployment.sh: the command to look up plus the package names
// for the brew / apt / dnf / choco manager families (dnfPkg is also
// used for yum / pacman / zypper).
type requiredTool struct {
	cmd      string
	brewPkg  string
	aptPkg   string
	dnfPkg   string
	chocoPkg string
}

// requiredTools lists the tools the install scripts themselves need;
// all four package names match the command name here.
var requiredTools = []requiredTool{
	{cmd: "jq", brewPkg: "jq", aptPkg: "jq", dnfPkg: "jq", chocoPkg: "jq"},
	{cmd: "unzip", brewPkg: "unzip", aptPkg: "unzip", dnfPkg: "unzip", chocoPkg: "unzip"},
	{cmd: "wget", brewPkg: "wget", aptPkg: "wget", dnfPkg: "wget", chocoPkg: "wget"},
}

// errNoPkgManager is returned by installIfMissing when the tool is
// missing and no supported package manager could be detected.
var errNoPkgManager = errors.New("no supported package manager detected")

// installIfMissing ports install_if_missing() from
// devops-deployment.sh. The ERROR lines for the no-manager case are
// printed here, like bash does; main turns the returned error into the
// process exit code.
func installIfMissing(tool requiredTool) error {
	if _, err := exec.LookPath(tool.cmd); err == nil {
		fmt.Printf("✓ %s is installed.\n", tool.cmd)
		return nil
	}

	fmt.Printf("⚠ %s is not installed.\n", tool.cmd)

	manager := pkgmgr.DetectPkgManager(detectOS())
	if manager == "" {
		fmt.Printf("ERROR: %s is required but not found, and no supported package manager was detected.\n", tool.cmd)
		fmt.Printf("       Please install %s manually, then re-run this script.\n", tool.cmd)
		return errNoPkgManager
	}

	fmt.Printf("   Installing %s via %s...\n", tool.cmd, manager)
	return pkgmgr.InstallPkg(manager, tool.brewPkg, tool.aptPkg, tool.dnfPkg, tool.chocoPkg)
}

// exitCodeFor mirrors the bash script's set -e behaviour: exit with the
// failed install command's status, or 1 when there is nothing finer
// (e.g. the no-package-manager case).
func exitCodeFor(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}
	return 1
}

// k8sStackNotes, cloudProviderNotes and printStackNotes now live in
// internal/ask (notes.go) so the ask flow can re-print them after a
// fresh config.env is saved.

// errNoShellConfig is returned by ensureLocalBinOnPath when neither
// ~/.bashrc nor ~/.zshrc exists to append the PATH line to.
var errNoShellConfig = errors.New("no compatible shell config file found")

// hasRegularFile reports whether path exists as a regular file (bash's
// [[ -f ... ]], following symlinks).
func hasRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// ensureLocalBinOnPath ports the "Ensure $LOCAL_BIN is on PATH" block of
// devops-deployment.sh, which runs after the notes and before the menu.
// LOCAL_BIN is $HOME/.local/bin, built from the $HOME environment
// variable (not os.UserHomeDir) so tests can point it at a fixture home.
func ensureLocalBinOnPath() error {
	localBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")

	// [[ ":$PATH:" != *":$LOCAL_BIN:"* ]]
	if strings.Contains(":"+os.Getenv("PATH")+":", ":"+localBin+":") {
		fmt.Printf("%s is already in the PATH.\n", localBin)
		return nil
	}

	// Pick the shell config to append to: ~/.bashrc if it exists, else
	// ~/.zshrc if it exists.
	configFile := ""
	for _, name := range []string{".bashrc", ".zshrc"} {
		if candidate := filepath.Join(os.Getenv("HOME"), name); hasRegularFile(candidate) {
			configFile = candidate
			break
		}
	}
	if configFile == "" {
		fmt.Println("No compatible shell config file found.")
		return errNoShellConfig
	}

	fmt.Printf("%s is not in the PATH. Adding it...\n", localBin)
	// echo "export PATH=\"$LOCAL_BIN:\$PATH\"" >> "$CONFIG_FILE" — the
	// real path interpolated, the trailing $PATH literal.
	f, err := os.OpenFile(configFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(fmt.Sprintf("export PATH=\"%s:$PATH\"\n", localBin)); err != nil {
		return err
	}
	// bash re-sources the config file here; the Go port just prepends
	// LOCAL_BIN to the current process PATH so child installers inherit
	// it without re-executing the user's shell config.
	os.Setenv("PATH", localBin+":"+os.Getenv("PATH"))
	fmt.Printf("%s has been added to the PATH.\n", localBin)
	return nil
}

// materializeAssets extracts the embedded assets into a fresh directory —
// os.MkdirTemp("", "devops-assets-") — and returns its path. Every run
// materializes its own copy: there is no marker and no reuse, because the
// previous shared $TMPDIR/devops-assets (with its .ok marker) kept serving
// the previous build's installers after a binary upgrade — a fresh dir per
// run makes staleness impossible and concurrent invocations independent.
// The caller owns the directory: on the interactive path main removes it
// right before returning (child installers run synchronously inside the
// menu loop, so the tree is dead once TuiLoop/FallbackMenu return), while
// an os.Exit or a crashed session leaves it for the OS temp cleaner.
func materializeAssets() (string, error) {
	dir, err := os.MkdirTemp("", "devops-assets-")
	if err != nil {
		return "", err
	}
	if err := assets.Extract(dir); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// childEnvVars lists the environment handed to the child installers, so
// they work from the materialized embedded copy instead of assuming the
// bash-era $HOME/devops-deployment layout:
//
//   - SCRIPT_DIR: the materialized scripts/ dir (where install-<tool>.sh
//     live and where the menu dispatches to);
//   - SCRIPT_TMPDIR: <root>/tmp — the download scratch space, semantics
//     unchanged, only now always exported;
//   - ROOT: the resolved deployment root (config.env, tmp/);
//   - BASHFILES_DIR: the materialized bash-files/ dir (dotfile copies);
//   - DEVOPS_SELF: the running executable, verbatim, not
//     symlink-resolved — install-bashtools.sh copies it onto the PATH as
//     `devops` and, seeing it set, leaves the pin file to the Go side
//     (resolveRoot persists it on marker wins).
func exportChildEnv(root, assetsDir, tmpDir string) {
	values := map[string]string{
		"SCRIPT_DIR":    filepath.Join(assetsDir, "scripts"),
		"SCRIPT_TMPDIR": tmpDir,
		"ROOT":          root,
		"BASHFILES_DIR": filepath.Join(assetsDir, "bash-files"),
		"DEVOPS_SELF":   exeSelf(),
	}
	for key, value := range values {
		if value != "" {
			os.Setenv(key, value)
		}
	}
}

// usage prints the subcommand summary. help writes it to stdout and
// exits 0; a rejected argument gets it on stderr after a one-line
// error, exiting 1. The program name follows argv[0], so the same text
// reads right as `devops` (the copy installed on the PATH) and as the
// raw binary name.
func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [command]\n\n", filepath.Base(os.Args[0]))
	fmt.Fprintf(w, "  (no command)  interactive deployment menu (the default)\n")
	fmt.Fprintf(w, "  update        re-copy the helper scripts and refresh the installed devops command\n")
	fmt.Fprintf(w, "  install       copy this binary to ~/.local/bin/devops (sets up the command)\n")
	fmt.Fprintf(w, "  help          show this help\n")
}

// errRefreshSkipped and errRefreshCurrent are ensureCopy's non-failure
// outcomes: nothing to do (the installed file IS the source, or there is
// no usable source), and the installed copy already matches the source.
var (
	errRefreshSkipped = errors.New("self-refresh skipped")
	errRefreshCurrent = errors.New("installed copy already current")
)

// installTarget returns the conventional install target of the `devops`
// command: $HOME/.local/bin/devops, built from the $HOME environment
// variable (not os.UserHomeDir, like ensureLocalBinOnPath) so tests can
// point it at a fixture home.
func installTarget() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "bin", "devops")
}

// ensureInstalledCopy best-effort ensures the conventional `devops`
// command — installTarget, ~/.local/bin/devops — matches source: the
// checkout binary at <root>/GO/devops-deployment-go. Unlike the running
// file the old refresh targeted, the installed side need not exist: after
// `update` the canonical command is guaranteed present and current, whether
// that means installed (a deleted or never-installed `devops` — the
// user-reported gap), replaced, or left alone. When the running file IS
// the target, the rename-over-self below still works; when the running
// file is the source itself, ensureCopy skips. The target's directory is
// created first (0755), so a first-ever install on a fresh machine cannot
// fail for a missing ~/.local/bin. Every hard failure is the caller's
// warning, never the update's exit code.
func ensureInstalledCopy(source string) error {
	target := installTarget()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return ensureCopy(target, source)
}

// ensureCopy makes installed match source's bytes: a differing installed
// file is replaced, a MISSING installed file is installed — the new case
// that turns a deleted `devops` command into an install instead of an
// error — and matching bytes are reported as current. The new bytes land
// in a temp file beside installed, get the executable bits, and are
// renamed over installed — a rename is atomic and swaps the directory
// entry instead of clobbering a running binary in place (no ETXTBSY);
// rename-over-self, the running copy being the target, works the same
// way. Skips (errRefreshSkipped) when installed IS source or when source
// is missing or not a regular file; errRefreshCurrent when the bytes
// already match; a real error only when an existing installed file cannot
// be read (a directory, wrong permissions) or the replacement cannot be
// written — note the installed file's parent must exist, which
// ensureInstalledCopy and the `install` subcommand guarantee.
func ensureCopy(installed, source string) error {
	if installed == source {
		return errRefreshSkipped
	}
	if info, err := os.Stat(source); err != nil || !info.Mode().IsRegular() {
		return errRefreshSkipped
	}
	srcBytes, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	curBytes, err := os.ReadFile(installed)
	if err != nil {
		// A missing installed file is the install case, not a failure;
		// anything else — an existing but unreadable file, a directory —
		// still is.
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	} else if bytes.Equal(srcBytes, curBytes) {
		return errRefreshCurrent
	}
	tmp := installed + fmt.Sprintf(".tmp-%d", os.Getpid())
	if err := os.WriteFile(tmp, srcBytes, 0o755); err != nil {
		return err
	}
	// WriteFile's perm is subject to the umask; make the exec bit stick.
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, installed); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// startupAction is what the interactive startup ensure decided to do
// about the installed `devops` command.
type startupAction int

const (
	startupCurrent startupAction = iota // running as the target, or the bytes already match: a silent no-op
	startupRefresh                      // the target exists and differs: replace it
	startupInstall                      // the target is missing: install it
)

// classifyCommandStartup decides what the interactive startup ensure must
// do for the installed command at target given the running binary at
// source: startupCurrent — silently nothing — when target IS source (the
// launched copy is the installed command itself) or when both files exist
// and match byte-for-byte; startupRefresh when an existing target
// differs; startupInstall when the target is missing (a deleted or
// never-installed `devops`). A target or source that cannot be read is an
// error, not a guess — the caller warns once and moves on. The comparison
// reads both files whole; nothing is written here, so the common case
// costs two stat/read calls and zero output.
func classifyCommandStartup(target, source string) (startupAction, error) {
	if target == source {
		return startupCurrent, nil
	}
	if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
		return startupInstall, nil
	} else if err != nil {
		return 0, err
	}
	srcBytes, err := os.ReadFile(source)
	if err != nil {
		return 0, err
	}
	dstBytes, err := os.ReadFile(target)
	if err != nil {
		return 0, err
	}
	if bytes.Equal(srcBytes, dstBytes) {
		return startupCurrent, nil
	}
	return startupRefresh, nil
}

// applyStartupEnsure performs the action classifyCommandStartup chose:
// silent for startupCurrent; a refresh or install through ensureCopy —
// the same atomic temp-file-and-rename the subcommands use — announced
// with one line before and one after. Failures are warned on errW and
// swallowed: a broken ensure must never cost the interactive session its
// exit code. The target's directory is created only when something will
// actually be installed into it.
func applyStartupEnsure(out, errW io.Writer, target, source string, action startupAction) {
	switch action {
	case startupCurrent:
		// The common case: the command is the running binary itself, or an
		// identical copy of it. Zero output, zero writes.
	case startupRefresh:
		fmt.Fprintf(out, "devops: updating installed command at %s...\n", target)
		if err := ensureCopy(target, source); err != nil {
			fmt.Fprintf(errW, "devops: refresh failed: %v\n", err)
			return
		}
		fmt.Fprintf(out, "devops: refreshed from %s\n", source)
	case startupInstall:
		fmt.Fprintf(out, "devops: installing command to %s...\n", target)
		// A first-ever install on a fresh machine has no ~/.local/bin yet;
		// ensureCopy needs the target's parent to exist.
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fmt.Fprintf(errW, "devops: install failed: %v\n", err)
			return
		}
		if err := ensureCopy(target, source); err != nil {
			fmt.Fprintf(errW, "devops: install failed: %v\n", err)
			return
		}
		fmt.Fprintf(out, "devops: installed to %s\n", target)
	}
}

// ensureCommandOnStartup is the interactive path's best-effort keep-alive
// for the `devops` command: it classifies installTarget (~/.local/bin/
// devops, the $HOME-built target) against the running executable VERBATIM
// — not symlink-resolved, so launching through a symlink to the checkout
// binary counts as current — and applies the result. Nothing is printed
// when the command is already current, one line pair when it is installed
// or refreshed, one warning line when the check itself fails; it returns
// nothing and never exits, so the menu always comes up. Only the
// interactive flow calls this — the `update` and `install` subcommands
// keep their own ensure and messages.
func ensureCommandOnStartup(out, errW io.Writer) {
	source, err := os.Executable() // verbatim, NOT symlink-resolved
	if err != nil {
		fmt.Fprintf(errW, "devops: startup check skipped: %v\n", err)
		return
	}
	source = filepath.Clean(source)
	target := installTarget()
	action, err := classifyCommandStartup(target, source)
	if err != nil {
		fmt.Fprintf(errW, "devops: startup check failed: %v\n", err)
		return
	}
	applyStartupEnsure(out, errW, target, source, action)
}

// runUpdate is the `update` subcommand: the menu's __update_scripts action
// (item 14 / the u key) without the interactive program around it. The
// root resolves the normal way — the ensure below needs it — and the
// shared update body, pure Go since the assets became embedded, syncs the
// embedded custom-scripts into ~/.local/bin/scripts. config.env is not
// loaded and the prerequisite checks are skipped. UpdateScripts' own
// output is the whole story — no startup lines, no final Done line.
// On top, once UpdateScripts returns, the conventional `devops` command
// (installTarget, ~/.local/bin/devops) is ensured from the checkout
// binary at <root>/GO/devops-deployment-go — the checkout binary's real
// location, inside the GO module the resolved repo root holds — so after
// `update` the command is guaranteed present and current: missing means
// installed, differing means replaced, matching means left alone,
// best-effort and printed, never reflected in the exit code, which stays
// the updater's, the same philosophy as the sync inside it.
func runUpdate() int {
	root := resolveRoot()
	updateErr := menu.UpdateScripts()

	// The ensure runs however the updater fared — the sync half above
	// does the same — and only ever prints or warns on stderr: updateErr
	// stays authoritative. The pre-stat only decides which success line
	// reads true; ensureCopy itself reports the missing-target install as
	// a plain nil.
	source := filepath.Join(root, "GO", "devops-deployment-go")
	target := installTarget()
	_, statErr := os.Stat(target)
	targetMissing := errors.Is(statErr, fs.ErrNotExist)
	switch err := ensureInstalledCopy(source); {
	case err == nil:
		if targetMissing {
			fmt.Printf("devops: installed to %s\n", target)
		} else {
			fmt.Printf("devops: refreshed from %s\n", source)
		}
	case errors.Is(err, errRefreshCurrent):
		fmt.Println("devops: already current.")
	case errors.Is(err, errRefreshSkipped):
		// Nothing to do: running from the checkout binary itself, or no
		// usable source beside the resolved root.
	default:
		fmt.Fprintf(os.Stderr, "devops: refresh failed: %v\n", err)
	}

	if updateErr != nil {
		return exitCodeFor(updateErr)
	}
	return 0
}

// runInstallCmd is the `install` subcommand: it deploys the binary that
// was run to the conventional command target (installTarget,
// ~/.local/bin/devops), creating the target's directory first. The root
// resolves the normal way — not because the install needs it, but for its
// side effect: running `install` from the checkout records the pin, so
// the copy left at the target keeps resolving that checkout. The
// convention: the canonical artifact <root>/GO/devops-deployment-go is
// what `update` refreshes from, while `install` deploys whatever binary
// you ran — honest, because a `go build` always carries the complete
// embedded assets.
//
// ensureCopy's two skip reasons are told apart BEFORE the call, so the
// message is accurate: errRefreshSkipped covers both installed==source
// and a missing source, and only the first can mean anything here —
// compare paths up front and report "already running as" for it; a skip
// from a missing source cannot happen for the binary that is running and
// stays a defensive failure.
func runInstallCmd() int {
	resolveRoot()
	source, err := os.Executable() // verbatim, NOT symlink-resolved
	if err != nil {
		fmt.Fprintf(os.Stderr, "devops: install failed: %v\n", err)
		return exitCodeFor(err)
	}
	target := installTarget()
	if target == filepath.Clean(source) {
		fmt.Printf("devops: skipped (already running as %s)\n", target)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "devops: install failed: %v\n", err)
		return exitCodeFor(err)
	}
	_, statErr := os.Stat(target)
	wasMissing := errors.Is(statErr, fs.ErrNotExist)
	switch err := ensureCopy(target, source); {
	case err == nil:
		if wasMissing {
			fmt.Printf("devops: installed to %s\n", target)
		} else {
			fmt.Printf("devops: refreshed from %s\n", source)
		}
	case errors.Is(err, errRefreshCurrent):
		fmt.Println("devops: already current.")
	default:
		fmt.Fprintf(os.Stderr, "devops: install failed: %v\n", err)
		return exitCodeFor(err)
	}
	return 0
}

// The subcommand dispatch in front of the interactive default: `update`,
// `install`, the help spellings, and anything else rejected with the
// usage. With no argument main falls through to the interactive menu
// exactly as before — the dispatch runs ahead of the config load and the
// prerequisite checks, so the subcommands never pay for the interactive
// front end.
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update":
			os.Exit(runUpdate())
		case "install":
			os.Exit(runInstallCmd())
		case "-h", "--help", "help":
			usage(os.Stdout)
			return
		default:
			if arg := os.Args[1]; strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "unknown option: %s\n", arg)
			} else {
				fmt.Fprintf(os.Stderr, "unknown command: %s\n", arg)
			}
			usage(os.Stderr)
			os.Exit(1)
		}
	}

	root := resolveRoot()

	// Source config.env from the deployment root into the process
	// environment first, so the notes below — and later child
	// installers — see DEVOPS_K8S_STACKS / DEVOPS_CLOUD_PROVIDERS.
	if err := config.Load(root); err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring unreadable config.env: %v\n", err)
	}

	fmt.Printf("Detected OS: %s\n", detectOS())
	fmt.Printf("Deployment root: %s\n", root)

	fmt.Println()
	fmt.Println("Checking prerequisites...")

	for _, tool := range requiredTools {
		if err := installIfMissing(tool); err != nil {
			os.Exit(exitCodeFor(err))
		}
	}

	ask.PrintStackNotes()

	fmt.Println()

	// Ensure $LOCAL_BIN is on PATH (bash: after the notes, before the
	// menu), then create SCRIPT_TMPDIR.
	if err := ensureLocalBinOnPath(); err != nil {
		os.Exit(exitCodeFor(err))
	}

	tmpDir := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot create %s: %v\n", tmpDir, err)
		os.Exit(1)
	}

	// Materialize the embedded assets for the child bash scripts and
	// export the layout they expect (SCRIPT_DIR, SCRIPT_TMPDIR, ROOT,
	// BASHFILES_DIR, DEVOPS_SELF — see exportChildEnv). A failed
	// extraction means a broken binary or an unusable temp dir: there is
	// nothing the menu could still do, so exit rather than limp on.
	assetsDir, err := materializeAssets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot materialize the embedded assets: %v\n", err)
		os.Exit(1)
	}
	// The materialized tree is this run's own (a fresh MkdirTemp) and no
	// longer needed once the menu loop returns — child installers run
	// synchronously inside it — so remove it right before main returns.
	// The os.Exit paths behind the menu (an invalid fallback option, the
	// TUI's Ctrl-C) skip this defer: a session that ends that way leaves
	// one devops-assets-* dir for the OS temp cleaner.
	defer os.RemoveAll(assetsDir)
	exportChildEnv(root, assetsDir, tmpDir)

	// bash MAIN re-runs detect_pkg_manager after setup_colors/tui_geometry
	// for the menu's status line; an empty result renders as "pkg: none".
	pkgMgr := pkgmgr.DetectPkgManager(detectOS())

	// One keystroke Reader for the whole program, constructed once and
	// handed down sequentially — the stack/cloud questions below and the
	// menu loop both read this instance, mirroring bash's single input
	// stream: an escape window left open when a phase ends parks its
	// follow-up byte here, and the next phase's first read drains it.
	rd := term.NewReaderStdin()

	// The bash script asks the stack/cloud questions just before the menu
	// when no config.env exists yet, and follows them with a blank line.
	if !hasRegularFile(filepath.Join(root, "config.env")) {
		if _, err := ask.AskStackCloud(root, rd); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		fmt.Println()
	}

	// Best-effort keep the installed `devops` command current at every
	// interactive launch (after the PATH block, before the menu): a
	// command deleted after an install is re-created here, a stale one
	// refreshed, and an already-current one costs two stat/read calls and
	// zero output. Warnings only — never an exit; the menu must come up
	// however the ensure fares.
	ensureCommandOnStartup(os.Stdout, os.Stderr)

	// The full-screen TUI when the terminal can hold it, the plain numeric
	// prompt otherwise. scriptDir is the bash SCRIPT_DIR: the materialized
	// embedded scripts/ directory, where the install-<tool>.sh the actions
	// dispatch to live — the one wiring change the assets-embedding brings
	// to the menu itself.
	scriptDir := filepath.Join(assetsDir, "scripts")
	if menu.UseTUI() {
		menu.TuiLoop(detectOS(), pkgMgr, root, scriptDir, rd)
	} else {
		menu.FallbackMenu(detectOS(), pkgMgr, root, scriptDir)
	}

	// bash MAIN's final line; falling off main exits 0.
	fmt.Println("Done. Please reopen all terminals.")
}
