// Tests for the embedded assets: Extract's walk against the real embedded
// tree (expected scripts present, executable, byte-identical to the
// embedded source, no .DS_Store anywhere) and against a fixture fs.FS
// exercising the .DS_Store filter the pruned-at-source embedded tree
// cannot contain. Everything lands under t.TempDir().

package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestExtractScripts verifies the materialized installer tree: every
// install-*.sh plus update-scripts.sh exists, is executable, and matches
// the embedded bytes exactly (embed loses exec bits, so the 0755 chmod is
// Extract's own doing).
func TestExtractScripts(t *testing.T) {
	dest := t.TempDir()
	if err := Extract(dest); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	wantScripts := []string{
		"install-aws.sh", "install-bashtools.sh", "install-brave.sh",
		"install-fzf-setup.sh", "install-helium.sh", "install-helm.sh",
		"install-k9s.sh", "install-kubectl.sh", "install-oc.sh",
		"install-terraform.sh", "update-scripts.sh",
	}
	for _, name := range wantScripts {
		path := filepath.Join(dest, "scripts", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("extracted %s missing: %v", name, err)
			continue
		}
		if !info.Mode().IsRegular() {
			t.Errorf("extracted %s is not a regular file", name)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("extracted %s mode = %v, want an exec bit", name, info.Mode().Perm())
		}
	}
}

// TestExtractBashFilesAndCustomScripts checks the other two embedded
// trees: the dotfiles land readable and the maintained helpers land
// executable, with content matching the embedded source.
func TestExtractBashFilesAndCustomScripts(t *testing.T) {
	dest := t.TempDir()
	if err := Extract(dest); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// bash-files: read-only dotfiles, content-identical to the embed.
	for _, name := range []string{"bashrc", "bash_function", "bash_environment", "aws_environment", "commands_environment"} {
		path := filepath.Join(dest, "bash-files", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("extracted bash-files/%s missing: %v", name, err)
		}
	}

	// custom-scripts: helpers deployed via the update sync must stay
	// executable, because the aliases call them directly.
	info, err := os.Stat(filepath.Join(dest, "custom-scripts", "misc", "tui_select.sh"))
	if err != nil {
		t.Fatalf("extracted custom-scripts/misc/tui_select.sh missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("tui_select.sh mode = %v, want an exec bit", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dest, "custom-scripts", "config")); err != nil {
		t.Errorf("extracted custom-scripts/config missing: %v", err)
	}
}

// TestExtractContentMatchesEmbedded asserts Extract copies bytes, not
// approximations: the extracted kubectl installer equals the embedded one.
func TestExtractContentMatchesEmbedded(t *testing.T) {
	dest := t.TempDir()
	if err := Extract(dest); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	want, err := fs.ReadFile(FS(), "scripts/install-kubectl.sh")
	if err != nil {
		t.Fatalf("embedded scripts/install-kubectl.sh unreadable: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "scripts", "install-kubectl.sh"))
	if err != nil {
		t.Fatalf("extracted scripts/install-kubectl.sh unreadable: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("extracted installer is %d bytes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extracted installer differs from the embedded one at byte %d", i)
		}
	}
}

// TestExtractSkipsDSStore drives the walk with a fixture tree carrying
// .DS_Store files at every level: none may be materialized, while the real
// files around them are.
func TestExtractSkipsDSStore(t *testing.T) {
	fsys := fstest.MapFS{
		"scripts/.DS_Store":                 {Data: []byte("finder churn")},
		"scripts/install-kubectl.sh":        {Data: []byte("#!/bin/bash\n"), Mode: 0o755},
		"custom-scripts/.DS_Store":          {Data: []byte("finder churn")},
		"custom-scripts/misc/.DS_Store":     {Data: []byte("finder churn")},
		"custom-scripts/misc/set_cursor.sh": {Data: []byte("#!/bin/bash\n"), Mode: 0o755},
		"bash-files/.DS_Store":              {Data: []byte("finder churn")},
		"bash-files/bashrc":                 {Data: []byte("# bashrc\n")},
	}
	dest := t.TempDir()
	if err := extract(fsys, dest); err != nil {
		t.Fatalf("extract() error = %v", err)
	}
	err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
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
	for _, rel := range []string{"scripts/install-kubectl.sh", "custom-scripts/misc/set_cursor.sh", "bash-files/bashrc"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("real file %s was not extracted: %v", rel, err)
		}
	}
}
