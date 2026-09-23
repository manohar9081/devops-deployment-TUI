// Tests for main's root resolution, asset materialization and copy-ensure
// helpers, all against t.TempDir() fixtures — never the real $HOME,
// ~/.local/bin, ~/.config or the real $TMPDIR. resolveRootFrom is the
// candidate chain (env → exe dir → exe-dir parent → pin → cwd → cwd
// parent → default), probed with the GO/assets-based marker: a candidate
// is a root when <dir>/GO/assets/scripts/install-kubectl.sh exists, so
// the repo root matches and the GO module dir itself does not;
// readRootPin/writeRootPin are the pin file's read and write halves,
// redirected away from the real ~/.config through
// DEVOPS_DEPLOYMENT_ROOT_PIN; materializeAssets/exportChildEnv are the
// embedded-assets runtime (redirected through TMPDIR); ensureCopy is the
// install-or-refresh primitive behind the `update` and `install`
// subcommands, with installTarget/ensureInstalledCopy pointing it at the
// conventional ~/.local/bin/devops; and classifyCommandStartup /
// applyStartupEnsure / ensureCommandOnStartup are the interactive path's
// startup keep-alive for that same command — silent when current, a
// one-line install or refresh otherwise, warnings only.

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile drops a file (and its parent directories) under root and
// returns its path.
func writeFile(t *testing.T, root, rel, body string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
	// WriteFile's perm is subject to the umask; set it exactly so the
	// mode assertions test the code, not the environment.
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	return path
}

// newCheckout builds a fixture deployment root: a directory carrying the
// GO/assets/scripts/install-kubectl.sh marker file the root chain looks
// for UNDER the candidate (a root is the repo root — config.env, custom/,
// devops-deployment.sh — not the GO module directory one level down,
// which has no GO/ of its own and cannot match).
func newCheckout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, filepath.Join("GO", "assets", "scripts", "install-kubectl.sh"), "#!/bin/bash\n", 0o755)
	return root
}

// TestHasRootMarker pins the marker's semantics directly: the repo root
// matches, the GO module directory inside it does not, and a plain
// directory does not either.
func TestHasRootMarker(t *testing.T) {
	repo := newCheckout(t)
	if !hasRootMarker(repo) {
		t.Errorf("hasRootMarker(%q) = false, want true for the repo root", repo)
	}
	if hasRootMarker(filepath.Join(repo, "GO")) {
		t.Errorf("hasRootMarker(%q) = true, want false — the GO module dir itself must not match", filepath.Join(repo, "GO"))
	}
	if hasRootMarker(t.TempDir()) {
		t.Errorf("hasRootMarker(plain dir) = true, want false")
	}
}

// TestResolveRootFromOrder walks the candidate chain in priority order:
// each row asserts both the winning root and which evidence produced it,
// so a reordering of the probes cannot pass unnoticed.
func TestResolveRootFromOrder(t *testing.T) {
	envRoot := t.TempDir()                 // the env override needs no marker
	exeRoot := newCheckout(t)              // wins through the exe-dir probes
	exeSub := filepath.Join(exeRoot, "GO") // the binary sits inside the checkout
	pinRoot := newCheckout(t)              // wins through the pin probe
	cwdRoot := newCheckout(t)              // wins through the cwd probes
	cwdSub := filepath.Join(cwdRoot, "bin")
	plain := t.TempDir()                            // exists, carries no marker
	missing := filepath.Join(t.TempDir(), "absent") // does not exist at all
	home := t.TempDir()
	for _, dir := range []string{exeSub, cwdSub} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	defaultRoot := filepath.Join(home, "devops-deployment")

	tests := []struct {
		name              string
		env, pin, exe, wd string
		want              string
		wantEvidence      rootEvidence
	}{
		{
			name: "env beats every marker candidate",
			env:  envRoot, pin: pinRoot, exe: exeRoot, wd: cwdRoot,
			want: envRoot, wantEvidence: evidenceEnv,
		},
		{
			name: "marker under the executable's dir beats the pin",
			pin:  pinRoot, exe: exeRoot, wd: cwdRoot,
			want: exeRoot, wantEvidence: evidenceExeDir,
		},
		{
			name: "binary inside the checkout: the repo root one level up",
			pin:  pinRoot, exe: exeSub, wd: cwdRoot,
			want: exeRoot, wantEvidence: evidenceExeDirParent,
		},
		{
			name: "pin wins only when nothing beside the binary matches",
			pin:  pinRoot, exe: plain, wd: plain,
			want: pinRoot, wantEvidence: evidencePin,
		},
		{
			name: "pin beats the cwd probes",
			pin:  pinRoot, exe: plain, wd: cwdRoot,
			want: pinRoot, wantEvidence: evidencePin,
		},
		{
			name: "empty exe and cwd candidates are skipped",
			pin:  pinRoot, exe: "", wd: "",
			want: pinRoot, wantEvidence: evidencePin,
		},
		{
			name: "marker in the working directory",
			pin:  missing, exe: plain, wd: cwdRoot,
			want: cwdRoot, wantEvidence: evidenceCwd,
		},
		{
			name: "marker one level above the working directory",
			pin:  plain, exe: plain, wd: cwdSub,
			want: cwdRoot, wantEvidence: evidenceCwdParent,
		},
		{
			name: "stale pin falls through to the next candidate",
			pin:  plain, exe: plain, wd: cwdRoot,
			want: cwdRoot, wantEvidence: evidenceCwd,
		},
		{
			name: "empty pin is skipped",
			pin:  "", exe: plain, wd: cwdRoot,
			want: cwdRoot, wantEvidence: evidenceCwd,
		},
		{
			name: "relative pin is skipped",
			pin:  filepath.Join("some", "relative", "root"), exe: plain, wd: cwdRoot,
			want: cwdRoot, wantEvidence: evidenceCwd,
		},
		{
			name: "missing pin target falls through",
			pin:  missing, exe: plain, wd: cwdRoot,
			want: cwdRoot, wantEvidence: evidenceCwd,
		},
		{
			name: "nothing matches: the bash default",
			pin:  plain, exe: plain, wd: plain,
			want: defaultRoot, wantEvidence: evidenceDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, evidence := resolveRootFrom(tt.env, tt.pin, tt.exe, tt.wd, home)
			if root != tt.want || evidence != tt.wantEvidence {
				t.Errorf("resolveRootFrom(%q, %q, %q, %q) = %q, %v; want %q, %v",
					tt.env, tt.pin, tt.exe, tt.wd, root, evidence, tt.want, tt.wantEvidence)
			}
		})
	}
}

// TestReadRootPin covers the pin file's read half: absolute content is
// trimmed and returned, everything else reads as no pin.
func TestReadRootPin(t *testing.T) {
	t.Run("missing file reads as no pin", func(t *testing.T) {
		t.Setenv(EnvRootPin, filepath.Join(t.TempDir(), "path"))
		if got := readRootPin(); got != "" {
			t.Errorf("readRootPin() = %q, want no pin for a missing file", got)
		}
	})

	t.Run("absolute content is trimmed", func(t *testing.T) {
		t.Setenv(EnvRootPin, writeFile(t, t.TempDir(), "path", "  /somewhere/root\n\n", 0o644))
		if got := readRootPin(); got != "/somewhere/root" {
			t.Errorf("readRootPin() = %q, want %q", got, "/somewhere/root")
		}
	})

	t.Run("relative content reads as no pin", func(t *testing.T) {
		t.Setenv(EnvRootPin, writeFile(t, t.TempDir(), "path", "somewhere/root\n", 0o644))
		if got := readRootPin(); got != "" {
			t.Errorf("readRootPin() = %q, want no pin for a relative path", got)
		}
	})

	t.Run("empty content reads as no pin", func(t *testing.T) {
		t.Setenv(EnvRootPin, writeFile(t, t.TempDir(), "path", "  \n", 0o644))
		if got := readRootPin(); got != "" {
			t.Errorf("readRootPin() = %q, want no pin for empty content", got)
		}
	})
}

// TestWriteRootPin covers the pin file's write half: the root lands with
// a trailing newline under a freshly created directory, and an unchanged
// pin is left byte-for-byte alone.
func TestWriteRootPin(t *testing.T) {
	t.Run("writes the root and creates the directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config", "devops-deployment", "path")
		t.Setenv(EnvRootPin, path)
		writeRootPin("/somewhere/root")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("pin file not written: %v", err)
		}
		if string(data) != "/somewhere/root\n" {
			t.Errorf("pin content = %q, want %q", data, "/somewhere/root\n")
		}
	})

	t.Run("an unchanged pin is left alone", func(t *testing.T) {
		// Seeded without the trailing newline: a rewrite would add one, so
		// its byte-for-byte survival proves the unchanged-skip.
		path := writeFile(t, t.TempDir(), "path", "/somewhere/root", 0o600)
		t.Setenv(EnvRootPin, path)
		writeRootPin("/somewhere/root")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "/somewhere/root" {
			t.Errorf("pin content = %q, want the pre-existing %q untouched", data, "/somewhere/root")
		}
	})
}

// TestEnsureCopy covers the installed-copy ensure: a differing source
// replaces the installed file atomically with the exec bits set, a
// MISSING installed file is installed the same way (the case that turns a
// deleted `devops` into an install instead of an error), an identical
// source is reported as current, and the two skip cases — the installed
// file IS the source, or there is no usable source — leave everything
// untouched.
func TestEnsureCopy(t *testing.T) {
	t.Run("refreshes an older installed copy", func(t *testing.T) {
		dir := t.TempDir()
		installed := writeFile(t, dir, "devops", "# old\n", 0o755)
		source := writeFile(t, dir, "source", "# new\n", 0o755)

		if err := ensureCopy(installed, source); err != nil {
			t.Fatalf("ensureCopy() error = %v", err)
		}
		data, err := os.ReadFile(installed)
		if err != nil || string(data) != "# new\n" {
			t.Errorf("installed content = %q (err %v), want %q", data, err, "# new\n")
		}
		info, err := os.Stat(installed)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("installed mode = %v, want 0755", info.Mode().Perm())
		}
		// The rename must not leave its temp file behind.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("directory holds %v, want only devops and source", names)
		}
	})

	t.Run("installs a missing target", func(t *testing.T) {
		dir := t.TempDir()
		installed := filepath.Join(dir, "devops") // absent: the install case
		source := writeFile(t, dir, "source", "# fresh\n", 0o755)

		if err := ensureCopy(installed, source); err != nil {
			t.Fatalf("ensureCopy() error = %v, want a missing target installed", err)
		}
		data, err := os.ReadFile(installed)
		if err != nil || string(data) != "# fresh\n" {
			t.Errorf("installed content = %q (err %v), want the source's %q", data, err, "# fresh\n")
		}
		info, err := os.Stat(installed)
		if err != nil {
			t.Fatalf("installed file missing after ensureCopy: %v", err)
		}
		if !info.Mode().IsRegular() {
			t.Errorf("installed mode = %v, want a regular file", info.Mode())
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("installed mode = %v, want 0755", info.Mode().Perm())
		}
		// No temp file may survive the install.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("directory holds %v, want only devops and source", names)
		}
	})

	t.Run("an identical copy is already current", func(t *testing.T) {
		dir := t.TempDir()
		// The installed file's 0700 (vs the source's 0755) makes any
		// rewrite visible: a refresh would leave 0755 behind.
		installed := writeFile(t, dir, "devops", "# same\n", 0o700)
		source := writeFile(t, dir, "source", "# same\n", 0o755)

		if err := ensureCopy(installed, source); !errors.Is(err, errRefreshCurrent) {
			t.Errorf("ensureCopy() error = %v, want errRefreshCurrent", err)
		}
		info, err := os.Stat(installed)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("installed mode = %v, want the untouched 0700", info.Mode().Perm())
		}
	})

	t.Run("the installed file is the source", func(t *testing.T) {
		// Same path, needed or not: the skip happens before any I/O, so
		// even a path that does not exist is a no-op.
		path := filepath.Join(t.TempDir(), "devops")
		if err := ensureCopy(path, path); !errors.Is(err, errRefreshSkipped) {
			t.Errorf("ensureCopy() error = %v, want errRefreshSkipped", err)
		}
	})

	t.Run("a missing source is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		installed := writeFile(t, dir, "devops", "# old\n", 0o755)
		source := filepath.Join(dir, "absent")

		if err := ensureCopy(installed, source); !errors.Is(err, errRefreshSkipped) {
			t.Errorf("ensureCopy() error = %v, want errRefreshSkipped", err)
		}
		if data, err := os.ReadFile(installed); err != nil || string(data) != "# old\n" {
			t.Errorf("installed content = %q (err %v), want the untouched %q", data, err, "# old\n")
		}
	})

	t.Run("a non-regular source is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		installed := writeFile(t, dir, "devops", "# old\n", 0o755)
		source := t.TempDir() // a directory, not a file

		if err := ensureCopy(installed, source); !errors.Is(err, errRefreshSkipped) {
			t.Errorf("ensureCopy() error = %v, want errRefreshSkipped", err)
		}
		if data, err := os.ReadFile(installed); err != nil || string(data) != "# old\n" {
			t.Errorf("installed content = %q (err %v), want the untouched %q", data, err, "# old\n")
		}
	})

	t.Run("an existing but unreadable installed file still fails", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: mode 0000 is still readable")
		}
		dir := t.TempDir()
		installed := writeFile(t, dir, "devops", "# old\n", 0o000)
		source := writeFile(t, dir, "source", "# new\n", 0o755)

		err := ensureCopy(installed, source)
		if err == nil {
			t.Fatalf("ensureCopy() = nil, want an error for an unreadable installed file")
		}
		if errors.Is(err, errRefreshSkipped) || errors.Is(err, errRefreshCurrent) {
			t.Errorf("ensureCopy() error = %v, want a real failure, not a skip/current sentinel", err)
		}
	})
}

// TestInstallTarget pins the conventional command target: $HOME/.local/bin/
// devops, built from the $HOME environment variable.
func TestInstallTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".local", "bin", "devops")
	if got := installTarget(); got != want {
		t.Errorf("installTarget() = %q, want %q", got, want)
	}
}

// TestEnsureInstalledCopy routes ensureCopy at installTarget: the source
// lands as $HOME/.local/bin/devops, and a missing ~/.local/bin is created
// rather than failing the install.
func TestEnsureInstalledCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := writeFile(t, t.TempDir(), "devops-deployment-go", "#!/bin/sh\n# source\n", 0o755)

	if err := ensureInstalledCopy(source); err != nil {
		t.Fatalf("ensureInstalledCopy() error = %v", err)
	}
	installed := filepath.Join(home, ".local", "bin", "devops")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("installTarget not written: %v", err)
	}
	if string(data) != "#!/bin/sh\n# source\n" {
		t.Errorf("installed content = %q, want the source's %q", data, "#!/bin/sh\n# source\n")
	}
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed mode = %v, want 0755", info.Mode().Perm())
	}

	// A second run over the fresh copy reports it as current.
	if err := ensureInstalledCopy(source); !errors.Is(err, errRefreshCurrent) {
		t.Errorf("second ensureInstalledCopy() error = %v, want errRefreshCurrent", err)
	}
}

// TestClassifyCommandStartup covers the startup-ensure classifier: the
// running-as-the-target and byte-identical cases are silent no-ops, a
// differing target is a refresh, a missing target is an install, and an
// unreadable side is an error rather than a guess — the caller then warns
// once instead of overwriting something it could not judge.
func TestClassifyCommandStartup(t *testing.T) {
	t.Run("running as the target is current without touching anything", func(t *testing.T) {
		// Same path, needed or not: the skip happens before any I/O, so
		// even a path that does not exist reads as current.
		path := filepath.Join(t.TempDir(), "devops")
		action, err := classifyCommandStartup(path, path)
		if err != nil || action != startupCurrent {
			t.Errorf("classifyCommandStartup(same path) = %v, %v; want startupCurrent, nil", action, err)
		}
	})

	t.Run("matching bytes are current", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# same\n", 0o700)
		source := writeFile(t, dir, "source", "# same\n", 0o755)
		action, err := classifyCommandStartup(target, source)
		if err != nil || action != startupCurrent {
			t.Errorf("classifyCommandStartup() = %v, %v; want startupCurrent, nil", action, err)
		}
	})

	t.Run("a differing target is a refresh", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# old\n", 0o755)
		source := writeFile(t, dir, "source", "# new\n", 0o755)
		action, err := classifyCommandStartup(target, source)
		if err != nil || action != startupRefresh {
			t.Errorf("classifyCommandStartup() = %v, %v; want startupRefresh, nil", action, err)
		}
	})

	t.Run("a missing target is an install", func(t *testing.T) {
		dir := t.TempDir()
		source := writeFile(t, dir, "source", "# fresh\n", 0o755)
		action, err := classifyCommandStartup(filepath.Join(dir, "devops"), source)
		if err != nil || action != startupInstall {
			t.Errorf("classifyCommandStartup() = %v, %v; want startupInstall, nil", action, err)
		}
	})

	t.Run("an unreadable target is an error, not a guess", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: mode 0000 is still readable")
		}
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# old\n", 0o000)
		source := writeFile(t, dir, "source", "# new\n", 0o755)
		if action, err := classifyCommandStartup(target, source); err == nil {
			t.Errorf("classifyCommandStartup() = %v, nil; want an error for an unreadable target", action)
		}
	})

	t.Run("a missing source is an error", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# old\n", 0o755)
		if action, err := classifyCommandStartup(target, filepath.Join(dir, "absent")); err == nil {
			t.Errorf("classifyCommandStartup() = %v, nil; want an error for a missing source", action)
		}
	})

	t.Run("a directory source is an error", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# old\n", 0o755)
		if action, err := classifyCommandStartup(target, t.TempDir()); err == nil {
			t.Errorf("classifyCommandStartup() = %v, nil; want an error for a directory source", action)
		}
	})
}

// TestApplyStartupEnsure covers the applier's every branch: the current
// action is fully silent and leaves the file — mode included — untouched;
// a refresh and an install announce themselves with one line before and
// after and land through ensureCopy (atomic replace, 0755, target
// directory created); and a failure warns on the error writer, one line,
// without touching the target or the caller's control flow.
func TestApplyStartupEnsure(t *testing.T) {
	t.Run("current is silent and untouched", func(t *testing.T) {
		dir := t.TempDir()
		// The 0700 target makes any rewrite visible: a refresh would leave
		// 0755 behind.
		target := writeFile(t, dir, "devops", "# same\n", 0o700)
		source := writeFile(t, dir, "source", "# same\n", 0o755)

		var out, errW bytes.Buffer
		applyStartupEnsure(&out, &errW, target, source, startupCurrent)
		if out.Len() != 0 || errW.Len() != 0 {
			t.Errorf("applyStartupEnsure(current) wrote %q / %q, want nothing", out.String(), errW.String())
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("target mode = %v, want the untouched 0700", info.Mode().Perm())
		}
	})

	t.Run("refresh replaces and announces", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# old\n", 0o755)
		source := writeFile(t, dir, "source", "# new\n", 0o755)

		var out, errW bytes.Buffer
		applyStartupEnsure(&out, &errW, target, source, startupRefresh)
		if errW.Len() != 0 {
			t.Errorf("stderr = %q, want empty", errW.String())
		}
		want := "devops: updating installed command at " + target + "...\ndevops: refreshed from " + source + "\n"
		if out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "# new\n" {
			t.Errorf("target content = %q (err %v), want %q", data, err, "# new\n")
		}
	})

	t.Run("install creates the directory and announces", func(t *testing.T) {
		dir := t.TempDir()
		source := writeFile(t, dir, "source", "# fresh\n", 0o755)
		target := filepath.Join(dir, "missing", ".local", "bin", "devops")

		var out, errW bytes.Buffer
		applyStartupEnsure(&out, &errW, target, source, startupInstall)
		if errW.Len() != 0 {
			t.Errorf("stderr = %q, want empty", errW.String())
		}
		want := "devops: installing command to " + target + "...\ndevops: installed to " + target + "\n"
		if out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "# fresh\n" {
			t.Errorf("target content = %q (err %v), want %q", data, err, "# fresh\n")
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("target mode = %v, want 0755", info.Mode().Perm())
		}
	})

	t.Run("a failed install warns once and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		source := writeFile(t, dir, "source", "# fresh\n", 0o755)
		blocker := writeFile(t, dir, "blocker", "# a file, not a directory\n", 0o644)
		target := filepath.Join(blocker, "devops") // MkdirAll cannot win

		var out, errW bytes.Buffer
		applyStartupEnsure(&out, &errW, target, source, startupInstall)
		if !strings.HasPrefix(errW.String(), "devops: install failed: ") {
			t.Errorf("stderr = %q, want a devops: install failed: warning", errW.String())
		}
		if lines := strings.Count(errW.String(), "\n"); lines != 1 {
			t.Errorf("stderr = %q (%d lines), want exactly one warning line", errW.String(), lines)
		}
		if _, err := os.Stat(target); err == nil {
			t.Errorf("target exists after the failed install, want nothing written")
		}
	})

	t.Run("a failed refresh warns", func(t *testing.T) {
		// The action is forced: in production a vanished source is already
		// an error in classifyCommandStartup, so this pins the applier's
		// own failure wording, not a reachable race.
		dir := t.TempDir()
		target := writeFile(t, dir, "devops", "# old\n", 0o755)
		source := filepath.Join(dir, "absent")

		var out, errW bytes.Buffer
		applyStartupEnsure(&out, &errW, target, source, startupRefresh)
		if !strings.HasPrefix(errW.String(), "devops: refresh failed: ") {
			t.Errorf("stderr = %q, want a devops: refresh failed: warning", errW.String())
		}
		if data, err := os.ReadFile(target); err != nil || string(data) != "# old\n" {
			t.Errorf("target content = %q (err %v), want the untouched %q", data, err, "# old\n")
		}
	})
}

// TestEnsureCommandOnStartup wires the real pieces — os.Executable (the
// test binary, verbatim) and installTarget under a fixture $HOME — and
// exercises two interactive launches: the first installs the missing
// command with the announced lines, the second finds the installed copy
// byte-identical to the running binary and stays completely silent, the
// zero-output contract of the common case.
func TestEnsureCommandOnStartup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out, errW bytes.Buffer

	ensureCommandOnStartup(&out, &errW)

	target := filepath.Join(home, ".local", "bin", "devops")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := "devops: installing command to " + target + "...\ndevops: installed to " + target + "\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if errW.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errW.String())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("installed command missing after the startup ensure: %v", err)
	}
	srcData, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, srcData) {
		t.Errorf("installed command differs from the running binary %q", self)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed mode = %v, want 0755", info.Mode().Perm())
	}

	// The second launch: the installed copy is the running binary's twin,
	// so the ensure prints nothing and touches nothing.
	out.Reset()
	errW.Reset()
	ensureCommandOnStartup(&out, &errW)
	if out.Len() != 0 || errW.Len() != 0 {
		t.Errorf("second ensureCommandOnStartup wrote %q / %q, want nothing", out.String(), errW.String())
	}
	after, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(after, srcData) {
		t.Errorf("installed command changed on the silent second launch (err %v)", err)
	}
}

// redirectAssetsTempDir points the materialization at this test's own
// temp dir: materializeAssets creates its fresh directory under
// os.TempDir(), which honors $TMPDIR, so the real $TMPDIR is never
// touched from the tests. Returns the base the fresh devops-assets-*
// dirs land in.
func redirectAssetsTempDir(t *testing.T) string {
	t.Helper()
	base := filepath.Join(t.TempDir(), "tmp")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", base)
	return base
}

// TestMaterializeAssets covers the embedded-assets runtime: every call
// extracts into a fresh temp dir of its own — no reuse across calls, so
// a binary upgrade can never serve a previous build's installers — and
// each tree is complete, executable, and free of .DS_Store.
func TestMaterializeAssets(t *testing.T) {
	base := redirectAssetsTempDir(t)

	dirs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		dir, err := materializeAssets()
		if err != nil {
			t.Fatalf("materializeAssets() error = %v", err)
		}
		if filepath.Dir(dir) != base {
			t.Errorf("materializeAssets() = %q, want a fresh dir under %q", dir, base)
		}
		dirs = append(dirs, dir)
	}
	if dirs[0] == dirs[1] {
		t.Errorf("two materializeAssets() calls returned the same dir %q, want a fresh dir per call", dirs[0])
	}

	for _, dir := range dirs {
		// The tree is complete and the installers are executable.
		for _, name := range []string{"install-bashtools.sh", "install-kubectl.sh", "update-scripts.sh"} {
			info, err := os.Stat(filepath.Join(dir, "scripts", name))
			if err != nil {
				t.Errorf("extracted scripts/%s missing: %v", name, err)
				continue
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Errorf("extracted scripts/%s mode = %v, want an exec bit", name, info.Mode().Perm())
			}
		}
		// No Finder metadata anywhere in the materialized tree.
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Name() == ".DS_Store" {
				t.Errorf(".DS_Store was materialized at %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestExportChildEnv asserts the environment handed to the child
// installers: the materialized scripts/ and bash-files/ dirs, the root and
// its tmp/, and the running executable itself (verbatim, not
// symlink-resolved — os.Executable in a test is the test binary).
func TestExportChildEnv(t *testing.T) {
	root := t.TempDir()
	assetsDir := t.TempDir()
	tmpDir := filepath.Join(root, "tmp")

	t.Setenv("SCRIPT_DIR", "")
	t.Setenv("SCRIPT_TMPDIR", "")
	t.Setenv("ROOT", "")
	t.Setenv("BASHFILES_DIR", "")
	t.Setenv("DEVOPS_SELF", "")

	exportChildEnv(root, assetsDir, tmpDir)

	want := map[string]string{
		"SCRIPT_DIR":    filepath.Join(assetsDir, "scripts"),
		"SCRIPT_TMPDIR": tmpDir,
		"ROOT":          root,
		"BASHFILES_DIR": filepath.Join(assetsDir, "bash-files"),
	}
	for key, value := range want {
		if got := os.Getenv(key); got != value {
			t.Errorf("exported %s = %q, want %q", key, got, value)
		}
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DEVOPS_SELF"); got != self {
		t.Errorf("exported DEVOPS_SELF = %q, want the verbatim executable %q", got, self)
	}
}
