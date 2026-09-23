// Package pkgmgr ports the package-manager helpers of
// devops-deployment.sh: detect_pkg_manager() and install_pkg().
package pkgmgr

import (
	"fmt"
	"os"
	"os/exec"
)

// probe pairs the binary looked up on PATH with the manager name the
// bash script assigns it (e.g. apt-get is reported as "apt").
type probe struct {
	bin     string
	manager string
}

// probesPerOS mirrors the priority order of detect_pkg_manager()'s case
// statement, keyed by the OS labels of detect_os().
var probesPerOS = map[string][]probe{
	"macos": {
		{bin: "brew", manager: "brew"},
		{bin: "port", manager: "port"},
	},
	"linux": {
		{bin: "apt-get", manager: "apt"},
		{bin: "dnf", manager: "dnf"},
		{bin: "yum", manager: "yum"},
		{bin: "pacman", manager: "pacman"},
		{bin: "zypper", manager: "zypper"},
		{bin: "brew", manager: "brew"},
	},
	"windows": {
		{bin: "choco", manager: "choco"},
		{bin: "scoop", manager: "scoop"},
		{bin: "winget", manager: "winget"},
	},
}

// DetectPkgManager returns the first available package manager for goos
// ("macos", "linux" or "windows"), or "" when none of them is found —
// the same priority order as detect_pkg_manager() in
// devops-deployment.sh.
func DetectPkgManager(goos string) string {
	for _, p := range probesPerOS[goos] {
		if _, err := exec.LookPath(p.bin); err == nil {
			return p.manager
		}
	}
	return ""
}

// InstallPkg runs the install command for manager exactly as
// install_pkg() in devops-deployment.sh does. The child process
// inherits our stdout/stderr, so the manager's own output shows up
// unfiltered.
func InstallPkg(manager, brewPkg, aptPkg, dnfPkg, chocoPkg string) error {
	switch manager {
	case "brew":
		return run("brew", "install", brewPkg)
	case "port": // MacPorts uses same name usually
		return run("port", "install", brewPkg)
	case "apt":
		if err := run("apt-get", "update", "-qq"); err != nil {
			return err
		}
		return run("apt-get", "install", "-y", aptPkg)
	case "dnf":
		return run("dnf", "install", "-y", dnfPkg)
	case "yum":
		return run("yum", "install", "-y", dnfPkg)
	case "pacman":
		return run("pacman", "-Sy", "--noconfirm", dnfPkg)
	case "zypper":
		return run("zypper", "--non-interactive", "install", dnfPkg)
	case "choco":
		return run("choco", "install", chocoPkg, "-y")
	case "scoop":
		return run("scoop", "install", chocoPkg)
	case "winget":
		return run("winget", "install", "--accept-source-agreements", "--accept-package-agreements", chocoPkg)
	default:
		return fmt.Errorf("unknown package manager: %s", manager)
	}
}

// run executes name with args and lets the child write to our
// stdout/stderr (the bash case just invokes the command in place).
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
