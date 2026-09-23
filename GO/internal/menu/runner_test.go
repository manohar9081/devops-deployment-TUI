// Tests for RunAction against a stub scriptDir: fake install-<tool>.sh
// scripts that write marker files, so the bash dispatch and the ✓/✗ loop
// are verified without ever touching the real scripts or $HOME (the
// __update_scripts action is pure Go since the assets became embedded and
// is tested against a fixture HOME below). The __config action is
// intentionally not exercised here:
// AskStackCloud goes interactive when fd 0 is a terminal (e.g. `go test`
// run from an attached shell), and its logic is covered in package ask.

package menu

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn while os.Stdout is redirected into a pipe and
// returns everything written — both by the Go code and by child bash
// processes, which inherit the swapped os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	func() {
		defer func() {
			os.Stdout = orig
			w.Close()
		}()
		fn()
	}()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	r.Close()
	return buf.String()
}

// swapStdin points os.Stdin at a temp file holding content for the
// duration of fn, so confirm-prompt reads are answered deterministically.
func swapStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = f
	defer func() {
		os.Stdin = orig
		f.Close()
	}()
	fn()
}

// writeScript drops a bash script into dir and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertMarker fails the test unless the stub script's marker file exists.
func assertMarker(t *testing.T, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("stub script did not write its marker: %v", err)
	}
}

func TestRunActionRunsToolScript(t *testing.T) {
	scriptDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker")
	writeScript(t, scriptDir, "install-kubectl.sh",
		"#!/usr/bin/env bash\necho ran-kubectl > "+marker+"\n")

	out := captureStdout(t, func() {
		// nil Reader: only __config touches it, and these tests never
		// select that action.
		if err := RunAction("kubectl", t.TempDir(), scriptDir, nil); err != nil {
			t.Errorf("RunAction(kubectl) = %v, want nil", err)
		}
	})
	assertMarker(t, marker)
	for _, want := range []string{">>> Installing kubectl...", "✓ kubectl done"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestRunActionMissingScript(t *testing.T) {
	scriptDir := t.TempDir() // no install scripts at all

	var out string
	swapStdin(t, "y\n", func() { // single-tool runs never prompt; harmless
		out = captureStdout(t, func() {
			if err := RunAction("nosuchtool", t.TempDir(), scriptDir, nil); err == nil {
				t.Error("RunAction(nosuchtool) = nil, want an error")
			}
		})
	})
	for _, want := range []string{
		"ERROR: install script for 'nosuchtool' not found",
		"✗ nosuchtool FAILED",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestRunActionBulkAbortsOnN(t *testing.T) {
	scriptDir := t.TempDir()

	var out string
	swapStdin(t, "n\n", func() {
		out = captureStdout(t, func() {
			if err := RunAction("__core", t.TempDir(), scriptDir, nil); err != nil {
				t.Errorf("RunAction(__core) after 'n' = %v, want nil", err)
			}
		})
	})
	for _, want := range []string{
		"This will install 6 tools. Continue? [y/N]",
		"Aborted.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, ">>> Installing") {
		t.Errorf("abort must not install anything, got:\n%s", out)
	}
}

func TestRunActionBulkAcceptsYAndCollectsFailures(t *testing.T) {
	scriptDir := t.TempDir() // nothing installed: every tool must fail

	var out string
	var gotErr error
	swapStdin(t, "Y\n", func() {
		out = captureStdout(t, func() {
			gotErr = RunAction("__core", t.TempDir(), scriptDir, nil)
		})
	})
	if gotErr == nil {
		t.Error("RunAction(__core) with no scripts = nil, want an error")
	}
	for _, want := range []string{"✗ kubectl FAILED", "✗ bashtools FAILED"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "✓"); n != 0 {
		t.Errorf("got %d ✓ lines with no scripts, want 0:\n%s", n, out)
	}
}

// TestRunActionUpdateScripts covers the __update_scripts action since the
// assets became embedded: the action syncs the embedded custom-scripts
// tree into $HOME/.local/bin/scripts (fixture HOME) with no bash updater
// and no injectable source. The report names the embedded tree and the
// build label, the deployed helper lands executable, and the action's
// error stays nil.
func TestRunActionUpdateScripts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out := captureStdout(t, func() {
		if err := RunAction("__update_scripts", t.TempDir(), t.TempDir(), nil); err != nil {
			t.Errorf("RunAction(__update_scripts) = %v, want nil", err)
		}
	})
	deployed := filepath.Join(home, ".local", "bin", "scripts", "misc", "tui_select.sh")
	if info, err := os.Stat(deployed); err != nil {
		t.Errorf("embedded custom script not deployed to %s: %v", deployed, err)
	} else if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("deployed %s mode = %v, want an exec bit", deployed, info.Mode().Perm())
	}
	for _, want := range []string{
		"Syncing custom scripts from embedded scripts (build ",
		"Custom scripts: ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

// TestUpdateScriptsIdempotentAndBuildLabel covers UpdateScripts directly:
// a second sync into the same fixture HOME finds everything current, and
// the build label is one of the two honest values ("dev" here — the test
// binary carries no vcs.revision).
func TestUpdateScriptsIdempotentAndBuildLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out := captureStdout(t, func() {
		if err := UpdateScripts(); err != nil {
			t.Fatalf("first UpdateScripts() error = %v", err)
		}
	})
	if !strings.Contains(out, "build dev") {
		t.Errorf("first run output missing a \"build dev\" label, got:\n%s", out)
	}
	if !strings.Contains(out, "Custom scripts: ") {
		t.Errorf("first run output missing the change summary, got:\n%s", out)
	}

	again := captureStdout(t, func() {
		if err := UpdateScripts(); err != nil {
			t.Fatalf("second UpdateScripts() error = %v", err)
		}
	})
	if !strings.Contains(again, "Custom scripts: already up to date.") {
		t.Errorf("second run = %q, want the already-up-to-date line", again)
	}
	if strings.Contains(again, "  misc/tui_select.sh") {
		t.Errorf("second run re-reported changes:\n%s", again)
	}
}
