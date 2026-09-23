package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveLoadRoundTrip saves a stack/cloud pair, forgets it from the
// environment, loads it back and expects the same values — plus the
// exact file format ask_stack_cloud() writes.
func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	stacks, clouds := "rancher gke", "aws"

	if err := Save(root, stacks, clouds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	want := "# Written by devops-deployment-go — edit or re-run the menu's 'Config' entry.\n" +
		"export DEVOPS_K8S_STACKS=\"rancher gke\"\n" +
		"export DEVOPS_CLOUD_PROVIDERS=\"aws\"\n"
	data, err := os.ReadFile(filepath.Join(root, "config.env"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := string(data); got != want {
		t.Errorf("file contents:\n got: %q\nwant: %q", got, want)
	}

	// Forget the values Save put into the environment, then prove Load
	// restores them from the file alone.
	os.Unsetenv(KeyK8sStacks)
	os.Unsetenv(KeyCloudProviders)
	if err := Load(root); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for key, want := range map[string]string{KeyK8sStacks: stacks, KeyCloudProviders: clouds} {
		if got := os.Getenv(key); got != want {
			t.Errorf("after Load: %s = %q, want %q", key, got, want)
		}
	}
}
