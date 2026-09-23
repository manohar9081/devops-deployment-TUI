// Tests for the fallback numeric menu in tui.go. TuiLoop itself is not
// tested: it is inherently interactive, needing a real terminal held in
// raw mode on fds 0 and 1 — the one part of the port that, like the bash
// tui_loop, can only be exercised by running it. FallbackMenu reads plain
// lines from stdin, so these tests swap os.Stdin/os.Stdout and point
// scriptDir at stub scripts that write marker files, the same technique
// runner_test.go uses for RunAction. Nothing touches the real $HOME or the
// real scripts directory.

package menu

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFallbackMenuRunsSelectedEntry(t *testing.T) {
	scriptDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker")
	writeScript(t, scriptDir, "install-kubectl.sh",
		"#!/usr/bin/env bash\necho ran-kubectl > "+marker+"\n")

	var out string
	swapStdin(t, "1\n", func() {
		out = captureStdout(t, func() {
			FallbackMenu("linux", "apt", t.TempDir(), scriptDir)
		})
	})
	assertMarker(t, marker)
	for _, want := range []string{
		"Select an option:",
		"  1 - Install Kubectl",
		" 14 - Update installed scripts",
		">>> Installing kubectl...",
		"✓ kubectl done",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestFallbackMenuInvalidOptionExits(t *testing.T) {
	scriptDir := t.TempDir() // nothing installable: a fall-through would show

	var out string
	exitTaken := false
	exitCode := -1
	origExit := osExit
	osExit = func(code int) {
		exitTaken = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	swapStdin(t, "99\n", func() {
		out = captureStdout(t, func() {
			FallbackMenu("linux", "apt", t.TempDir(), scriptDir)
		})
	})
	if !exitTaken {
		t.Fatal("option 99 did not take the exit path")
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if want := "Invalid option: '99'."; !strings.Contains(out, want) {
		t.Errorf("output missing %q, got:\n%s", want, out)
	}
	// The exit must cut the flow short: with osExit stubbed, a missing
	// return would run entry 1's action right after the message.
	if strings.Contains(out, ">>> Installing") {
		t.Errorf("invalid option ran an action anyway, got:\n%s", out)
	}
}
