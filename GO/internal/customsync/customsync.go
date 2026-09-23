// Package customsync keeps the Go port's own copy of the repo's custom
// helper scripts deployable. Sync mirrors a custom-scripts directory into
// ~/.local/bin/scripts, the directory the aliases in the Bash-tools
// dotfiles source from — since the assets became embedded, the menu's
// update action feeds it the custom-scripts tree extracted from the
// binary's built-in copy (package assets).
//
// It is the Go counterpart of the bash-era update-scripts.sh's copy half,
// with two deliberate differences: the Go sync compares contents
// byte-for-byte instead of shelling out to `diff -rq`, and the source
// needs no checkout on disk at all — the embedded tree extracted to a
// temp dir does, so a machine without the repo can still deploy helpers.
// Like the bash script, it only ever adds or overwrites — anything that
// exists solely in the destination is left alone.
package customsync

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// EnvSource is the environment variable that overrides the source lookup.
// When set and non-empty it wins over both directory candidates, whether
// or not it points at anything.
const EnvSource = "DEVOPS_CUSTOM_SCRIPTS"

// dsStore is macOS Finder's metadata file name. It is skipped in the sync
// so Finder churn in a source tree — a .DS_Store appearing because someone
// browsed it, or vanishing with a cleanup — never shows up as an update
// and never lands in the deployed scripts. (The assets.Extract walk
// applies the same filter to the embedded tree.)
const dsStore = ".DS_Store"

// ResolveSource returns the first custom-scripts directory that exists,
// tried in order:
//
//  1. $DEVOPS_CUSTOM_SCRIPTS, when set and non-empty;
//  2. <exeDir>/custom-scripts — beside the running binary, so a checkout
//     copied or symlinked anywhere still deploys its helpers;
//  3. <root>/GO/assets/custom-scripts — the maintained tree inside the
//     GO module under the root (the root is the repo root), when the
//     binary runs from somewhere else (kept for direct use; the menu's
//     update action no longer needs a disk source).
//
// An empty exeDir or root skips that candidate. The bool is false when
// none of the candidates is an existing directory; the caller then skips
// the sync rather than guessing.
func ResolveSource(exeDir, root string) (string, bool) {
	candidates := make([]string, 0, 3)
	if env := os.Getenv(EnvSource); env != "" {
		candidates = append(candidates, env)
	}
	if exeDir != "" {
		candidates = append(candidates, filepath.Join(exeDir, "custom-scripts"))
	}
	if root != "" {
		candidates = append(candidates, filepath.Join(root, "GO", "assets", "custom-scripts"))
	}
	for _, dir := range candidates {
		if isDir(dir) {
			return dir, true
		}
	}
	return "", false
}

// isDir reports whether path exists and is a directory (bash's [[ -d ]]).
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Sync copies src into dst the way `cp -R "$SRC"/. "$DST"/` does in
// update-scripts.sh: it walks src recursively, creating the directories
// and writing the files that dst is missing, overwriting a file whose
// bytes differ, and preserving the source's permission bits — so an
// executable helper stays executable. It never deletes: an entry that
// exists only in dst (a hand-added script — the "stale files" the bash
// script deliberately ignores) is left untouched.
//
// It returns how many items were created or overwritten and their paths
// relative to src, in walk order (lexical, parents before children); the
// caller prints them indented, like the bash script prints its change
// list. dst is created when missing — that itself is not counted, and
// neither is an identical file. The first error aborts the walk and is
// returned with whatever the walk reported so far.
func Sync(src, dst string) (changed int, report []string, err error) {
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			// The source root: only make sure dst exists, mirroring the
			// bash script's `mkdir -p "$DST"`.
			return os.MkdirAll(dst, 0o755)
		}
		if !d.IsDir() && d.Name() == dsStore {
			return nil // Finder metadata: never copied, never reported
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			if isDir(target) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			changed++
			report = append(report, rel)
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// os.Stat, not d.Info: WalkDir does not descend into symlinks, so
		// a link is copied as a regular file holding its target's content,
		// with the target's permission bits.
		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		switch existing, err := os.ReadFile(target); {
		case err == nil && bytes.Equal(existing, data):
			// Identical bytes: leave dst — content and bits — as it is.
			return nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			// An unreadable destination (or a directory sitting where a
			// file belongs) is reported instead of clobbered.
			return err
		}

		perm := info.Mode().Perm()
		if err := os.WriteFile(target, data, perm); err != nil {
			return err
		}
		// WriteFile applies perm only when it creates the file, and umask
		// narrows it even then; the explicit Chmod keeps the source's bits
		// — the exec bit above all — on both new and overwritten files.
		if err := os.Chmod(target, perm); err != nil {
			return err
		}
		changed++
		report = append(report, rel)
		return nil
	})
	return changed, report, err
}
