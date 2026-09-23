// Tests for customsync: Sync's copy semantics against a fixture
// source/destination pair (never the real $HOME or ~/.local/bin), and
// ResolveSource's candidate order. Everything happens under t.TempDir().

package customsync

import (
	"os"
	"path/filepath"
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
	// exec-bit assertions test the code, not the environment.
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// TestSyncCountsNewAndModifiedWithoutDeleting is the smoke-C contract:
// new and modified files are counted and reported, a destination-only
// file survives, and the exec bit travels with the copy.
func TestSyncCountsNewAndModifiedWithoutDeleting(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Source: one existing dir (not new), one new dir, a modified file and
	// a new executable file.
	if err := os.Mkdir(filepath.Join(src, "aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, src, "aws/helper.sh", "v1\n", 0o755) // will differ
	writeFile(t, src, "k8s/k_restart.sh", "#!/bin/sh\n", 0o755)
	writeFile(t, src, "plain.txt", "a\n", 0o644)

	// Destination already has the aws/ dir, an older helper.sh with the
	// exec bit missing, and a file the source knows nothing about.
	if err := os.MkdirAll(filepath.Join(dst, "aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dst, "aws/helper.sh", "v0\n", 0o644)
	dstOnly := writeFile(t, dst, "local_only.sh", "mine\n", 0o755)

	changed, report, err := Sync(src, dst)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	// aws/ exists, so it is not counted; aws/helper.sh differs, k8s/ is
	// new, k8s/k_restart.sh is new, plain.txt is new.
	if changed != 4 {
		t.Errorf("changed = %d, want 4 (report %v)", changed, report)
	}
	want := []string{"aws/helper.sh", "k8s", "k8s/k_restart.sh", "plain.txt"}
	if len(report) != len(want) {
		t.Fatalf("report = %v, want %v", report, want)
	}
	for i := range want {
		if report[i] != want[i] {
			t.Errorf("report[%d] = %q, want %q (full: %v)", i, report[i], want[i], report)
		}
	}

	// The modified file was overwritten with the source's content and bits.
	info, err := os.Stat(filepath.Join(dst, "aws", "helper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "aws", "helper.sh")); err != nil || string(data) != "v1\n" {
		t.Errorf("aws/helper.sh content = %q (err %v), want %q", data, err, "v1\n")
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("aws/helper.sh mode = %v, want the source's exec bit", info.Mode().Perm())
	}
	// New files arrive with the source's bits, exec included.
	if info, err := os.Stat(filepath.Join(dst, "k8s", "k_restart.sh")); err != nil {
		t.Errorf("k8s/k_restart.sh not copied: %v", err)
	} else if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("k8s/k_restart.sh mode = %v, want an exec bit", info.Mode().Perm())
	}

	// Nothing is ever deleted: the destination-only file is still there,
	// with its own bits.
	if !exists(t, dstOnly) {
		t.Errorf("Sync deleted the destination-only file %s", dstOnly)
	}

	// A second pass is a no-op: content is compared, so nothing changes.
	again, reportAgain, err := Sync(src, dst)
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if again != 0 || len(reportAgain) != 0 {
		t.Errorf("second Sync() = %d %v, want 0 and no report", again, reportAgain)
	}
}

// TestSyncCreatesDestination covers the `mkdir -p "$DST"` half: the
// destination root itself is created when missing but not counted.
func TestSyncCreatesDestination(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "misc/port_check.sh", "#!/bin/sh\n", 0o755)
	dst := filepath.Join(t.TempDir(), "scripts") // does not exist yet

	changed, report, err := Sync(src, dst)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if changed != 2 || len(report) != 2 {
		t.Errorf("changed = %d, report = %v, want 2 items", changed, report)
	}
	if !exists(t, filepath.Join(dst, "misc", "port_check.sh")) {
		t.Error("nested file was not copied under a freshly created destination")
	}
}

// TestSyncSkipsDSStore covers the Finder-churn filter: .DS_Store files in
// the source are neither copied nor reported, while the real files around
// them sync normally. This is what keeps a Finder visit of a source tree
// from showing up as churn in the update report.
func TestSyncSkipsDSStore(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "scripts")
	writeFile(t, src, ".DS_Store", "finder churn", 0o644)
	writeFile(t, src, "misc/.DS_Store", "finder churn", 0o644)
	writeFile(t, src, "misc/set_cursor.sh", "#!/bin/sh\n", 0o755)

	changed, report, err := Sync(src, dst)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if changed != 2 || len(report) != 2 {
		t.Fatalf("changed = %d, report = %v, want only misc/ and misc/set_cursor.sh", changed, report)
	}
	if _, err := os.Stat(filepath.Join(dst, ".DS_Store")); err == nil {
		t.Error("root .DS_Store was copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "misc", ".DS_Store")); err == nil {
		t.Error("nested .DS_Store was copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "misc", "set_cursor.sh")); err != nil {
		t.Errorf("real file not copied: %v", err)
	}
}

func TestResolveSourceOrder(t *testing.T) {
	// Nothing exists: no source, and the caller must skip the sync.
	t.Run("none", func(t *testing.T) {
		t.Setenv(EnvSource, "")
		if got, ok := ResolveSource(t.TempDir(), t.TempDir()); ok {
			t.Errorf("ResolveSource() = %q, true; want false with no candidates", got)
		}
	})

	// Only <root>/GO/assets/custom-scripts exists.
	t.Run("root", func(t *testing.T) {
		t.Setenv(EnvSource, "")
		root := t.TempDir()
		want := filepath.Join(root, "GO", "assets", "custom-scripts")
		if err := os.MkdirAll(want, 0o755); err != nil {
			t.Fatal(err)
		}
		got, ok := ResolveSource(t.TempDir(), root)
		if !ok || got != want {
			t.Errorf("ResolveSource() = %q, %v; want %q, true", got, ok, want)
		}
	})

	// Both directory candidates exist: the one beside the binary wins.
	t.Run("exeDir", func(t *testing.T) {
		t.Setenv(EnvSource, "")
		exe := t.TempDir()
		root := t.TempDir()
		want := filepath.Join(exe, "custom-scripts")
		for _, dir := range []string{want, filepath.Join(root, "GO", "assets", "custom-scripts")} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		got, ok := ResolveSource(exe, root)
		if !ok || got != want {
			t.Errorf("ResolveSource() = %q, %v; want %q, true", got, ok, want)
		}
	})

	// The environment override wins over both, even when it is the only
	// candidate that is not a directory — the bool is then false.
	t.Run("env", func(t *testing.T) {
		exe := t.TempDir()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(exe, "custom-scripts"), 0o755); err != nil {
			t.Fatal(err)
		}
		env := t.TempDir()
		t.Setenv(EnvSource, env)
		got, ok := ResolveSource(exe, root)
		if !ok || got != env {
			t.Errorf("ResolveSource() = %q, %v; want %q, true", got, ok, env)
		}

		// A non-empty env candidate that does not exist is skipped, not
		// returned: with nothing else present there is no source at all.
		t.Setenv(EnvSource, filepath.Join(t.TempDir(), "missing"))
		if got, ok := ResolveSource(t.TempDir(), t.TempDir()); ok {
			t.Errorf("ResolveSource() = %q, true; want false when no candidate is an existing directory", got)
		}
	})
}
