// Package config loads the deployment root's optional config.env into
// the process environment, the way devops-deployment.sh sources
// STACK_CONFIG_FILE near the top of the script, and saves it back (Save)
// the way ask_stack_cloud() writes it, so later child installers inherit
// DEVOPS_K8S_STACKS / DEVOPS_CLOUD_PROVIDERS.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The keys recognised in config.env; every other line is ignored.
const (
	KeyK8sStacks      = "DEVOPS_K8S_STACKS"
	KeyCloudProviders = "DEVOPS_CLOUD_PROVIDERS"
)

// keys lists the managed keys so Load can mirror the file exactly: a
// key ends up in the environment iff the file assigns it.
var keys = []string{KeyK8sStacks, KeyCloudProviders}

// Load reads <root>/config.env and sets the recognised keys into the
// process environment (os.Setenv). Lines look like
//
//	export DEVOPS_K8S_STACKS="rancher gke"
//
// and tolerate surrounding whitespace, an optional `export` prefix and
// optional surrounding quotes (single or double); unquoted values work
// too. The environment mirrors the file: a key that the file does not
// assign (or a missing file) is unset. A missing file is not an error;
// any other read failure is returned.
func Load(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "config.env"))
	if err != nil {
		unsetAll()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	values := parse(string(data))
	for _, key := range keys {
		if value, ok := values[key]; ok {
			os.Setenv(key, value)
		} else {
			os.Unsetenv(key)
		}
	}
	return nil
}

// headerLine is the first line of a saved config.env. Unlike the bash
// original it honestly names the Go port as the writer.
const headerLine = "# Written by devops-deployment-go — edit or re-run the menu's 'Config' entry."

// Save writes <root>/config.env in the exact format ask_stack_cloud()
// uses in devops-deployment.sh (the "Saved: ..." summary line comes
// later, with the ask flow) and mirrors the values into the process
// environment, like the bash assignments before the write:
//
//	# Written by devops-deployment-go — edit or re-run the menu's 'Config' entry.
//	export DEVOPS_K8S_STACKS="<stacks>"
//	export DEVOPS_CLOUD_PROVIDERS="<clouds>"
func Save(root, stacks, clouds string) error {
	contents := fmt.Sprintf("%s\nexport %s=\"%s\"\nexport %s=\"%s\"\n",
		headerLine, KeyK8sStacks, stacks, KeyCloudProviders, clouds)
	if err := os.WriteFile(filepath.Join(root, "config.env"), []byte(contents), 0o644); err != nil {
		return err
	}
	os.Setenv(KeyK8sStacks, stacks)
	os.Setenv(KeyCloudProviders, clouds)
	return nil
}

// parse extracts the managed keys from config.env contents. Each
// non-empty, non-comment line must look like [export] KEY=VALUE.
func parse(contents string) map[string]string {
	values := make(map[string]string)
	for _, raw := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if rest, ok := strings.CutPrefix(key, "export"); ok {
			key = strings.TrimSpace(rest)
		}
		if key == KeyK8sStacks || key == KeyCloudProviders {
			values[key] = unquote(strings.TrimSpace(value))
		}
	}
	return values
}

// unquote strips one pair of matching surrounding quotes, leaving
// whitespace inside the quotes intact.
func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// unsetAll removes the managed keys from the process environment.
func unsetAll() {
	for _, key := range keys {
		os.Unsetenv(key)
	}
}
