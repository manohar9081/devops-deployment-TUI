// Package assets embeds every runtime asset the port needs at build time:
// the bash installers (scripts/), the dotfiles (bash-files/) and the
// maintained helper scripts (custom-scripts/), all under GO/assets/. A
// freshly built binary therefore carries the whole deployment toolkit —
// `go build` is all it takes to ship a new version — and the repo folders
// are dev-time sources only.
//
// Extract materializes the embedded tree onto disk (the menu does this at
// startup, the update action per run), because the installers stay bash
// and child processes need real files. Embedding loses the Unix exec
// bits, so Extract sets them again for every *.sh file.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:scripts all:bash-files all:custom-scripts
var embedded embed.FS

// dsStore is macOS Finder's metadata file name. It carries no payload and
// appears/vanishes with mere folder browsing, so it is filtered out during
// Extract and in the update sync — otherwise every Finder visit of the
// source tree would show up as churn in `devops update`.
const dsStore = ".DS_Store"

// FS returns the embedded asset tree: scripts/, bash-files/ and
// custom-scripts/ at its root, exactly as they live under GO/assets/.
func FS() fs.FS { return embedded }

// Extract walks the embedded asset tree into dest, creating directories
// with 0755 and writing every file with its embedded permission bits —
// overridden to 0755 for every *.sh file (embed loses exec bits, so they
// are set unconditionally; the same for any file that does report an exec
// bit) — followed by an explicit chmod so the umask cannot narrow the
// bits. Existing files at the same paths are overwritten file-by-file;
// nothing outside the embedded tree is touched or deleted. Files named
// .DS_Store are skipped.
func Extract(dest string) (err error) {
	return extract(embedded, dest)
}

// extract is Extract against an injected fs.FS, so tests can drive the
// walk against a fixture tree (the embedded one cannot contain the
// .DS_Store filter case, since it is pruned at the source).
func extract(fsys fs.FS, dest string) (err error) {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == dsStore {
			return nil
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			if path == "." {
				return nil // dest itself; created by the first file write
			}
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Embed reports a uniform 0444; scripts (and anything that does
		// claim an exec bit) must land executable, so 0755 unconditionally.
		perm := info.Mode().Perm()
		if strings.HasSuffix(d.Name(), ".sh") || perm&0o111 != 0 {
			perm = 0o755
		}
		// Overwrite means remove-then-write: a previous extraction may
		// have left this very file read-only (the embedded 0444 bits),
		// and writing over such a file in place would fail for a non-root
		// owner. The directories are ours (0755), so the unlink works.
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(target, data, perm); err != nil {
			return err
		}
		// WriteFile's perm applies only when it creates the file and is
		// subject to the umask; the explicit chmod keeps the bits exact.
		return os.Chmod(target, perm)
	})
}
