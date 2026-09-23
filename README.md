# DevOps Deployment TUI

DevOps Deployment TUI is a personal workstation bootstrap toolkit for
developers and DevOps engineers. It installs and configures commonly used
Kubernetes, cloud, infrastructure, and shell tools from one interactive
terminal menu.

The repository contains two front doors:

- **Bash** (`SHELL/`) is the original script-based installer.
- **Go** (`GO/`) is a self-contained TUI. It embeds the Bash installers,
  dotfiles, and helper scripts into one binary.

The toolkit can install or manage:

- Kubernetes clients and tools: `kubectl`, `k9s`, Helm, and OpenShift `oc`
- Terraform
- AWS CLI v2 and its SSM Session Manager plugin
- Optional Brave and Helium AppImages
- Bash dotfiles, aliases, command dispatchers, and helper scripts
- Optional `fzf` keybindings and terminal cursor styling

It is designed for Linux and macOS terminals. The download URLs and shell
integration are Unix-oriented; Windows users should run it inside a compatible
Linux environment such as WSL.

## Quick start

Clone the repository and choose either the Go or Bash entrypoint:

```bash
git clone https://github.com/manohar9081/devops-deployment-TUI.git
cd devops-deployment-TUI
```

### Recommended: build and run the Go TUI

Go 1.22 or newer is required. The Go module has no third-party dependencies.

```bash
cd GO
go build -o devops-deployment-go .
./devops-deployment-go
```

To install the built binary as `devops` under `~/.local/bin`:

```bash
./devops-deployment-go install
```

After `~/.local/bin` is on `PATH`, the application can be started with:

```bash
devops
```

Useful Go commands:

```bash
./devops-deployment-go help
./devops-deployment-go update
```

`update` refreshes the installed helper scripts from the binary's embedded
assets without opening the interactive menu.

### Bash entrypoint

The Bash entrypoint expects the repository to be available at
`$HOME/devops-deployment`, because its scripts use that location for the
deployment root:

```bash
# Run these commands from the directory containing the clone.
cp -R devops-deployment-TUI "$HOME/devops-deployment"
cd "$HOME/devops-deployment"
chmod +x devops-deployment.sh
./devops-deployment.sh
```

If the clone is already located at `$HOME/devops-deployment`, skip the copy
command and run the remaining commands from that directory.

On macOS, the script re-executes itself with Bash 4+ when available. Install
it first if necessary:

```bash
brew install bash
```

The first run asks which Kubernetes stacks and cloud providers are relevant
to you, then saves the choices in `config.env`. Select individual tools, use
**Install everything**, or use **Install core only** to skip browsers and the
optional `fzf` setup.

## What the installer changes

The Bash tools installer copies or creates the following user-level files:

| Location | Purpose |
| --- | --- |
| `~/.local/bin/scripts/` | AWS, Kubernetes, Terraform, and general helper scripts |
| `~/.local/bin/devops` | Installed Go command, when using the Go entrypoint |
| `~/.bashrc`, `~/.bash_function` | Shell startup and interactive functions |
| `~/.bash_environment`, `~/.aws_environment` | Aliases and cloud helpers |
| `~/.commands_environment` | `k`, `aw`, and `tf` dispatchers |
| `~/.aws/config` | Symlink to the toolkit's AWS SSO config template |
| `~/devops-deployment/config.env` | Selected Kubernetes stacks and cloud providers |

Existing dotfiles are backed up with a timestamp before being replaced.
Review the installer scripts before running them on a workstation with a
custom shell configuration.

## DIY guide

### 1. Check prerequisites

Have the following available:

- Bash 4 or newer
- `curl`, `wget`, `jq`, and `unzip`
- Internet access to download releases
- A supported package manager when a prerequisite is missing
  (`brew`, `apt`, `dnf`, `pacman`, `zypper`, `choco`, `scoop`, or `winget`)

The installer checks for prerequisites and can install several of them using
the package manager it detects. Cloud and cluster authentication are not
created automatically: you must provide your own credentials and kubeconfigs.

### 2. Choose your deployment style

Use the **Go TUI** when you want a portable, self-contained binary. Build it
again whenever you change files under `GO/assets/`; those files are embedded
at compile time.

Use the **Bash entrypoint** when you want to edit and run the scripts directly
from a checkout. It is also the easiest path for experimenting with a new
installer.

### 3. Install the core tools

From the menu, the core workflow installs:

1. `kubectl`
2. `k9s`
3. Helm
4. Terraform
5. AWS CLI
6. Bash tools and aliases

Install OpenShift `oc` separately when you selected an OpenShift stack.
Brave, Helium, and `fzf`/cursor setup are optional.

### 4. Configure AWS SSO safely

Do not commit real credentials or account details. Copy the template and
replace its placeholders only in your local installed configuration:

```bash
$EDITOR ~/.local/bin/scripts/config
```

The installer links `~/.aws/config` to that file. Use
`SHELL/custom/config.example` as a reference for the expected AWS SSO
sections.

### 5. Reload the shell

Close and reopen the terminal, or explicitly reload your shell configuration:

```bash
source ~/.bashrc
```

Then verify the command paths and aliases:

```bash
command -v devops
command -v kubectl
type k
type aw
type tf
```

### 6. Use the helper scripts

Installed helpers are grouped by domain:

- `~/.local/bin/scripts/aws/`: AMI, ACM, CloudWatch, EC2, IAM, S3, and
  security-group workflows
- `~/.local/bin/scripts/k8s/`: Argo CD, Helm, events, logs, port forwarding,
  restarts, cleanup, secrets, and manifest checks
- `~/.local/bin/scripts/tf/`: Terraform state browsing
- `~/.local/bin/scripts/misc/`: Docker cleanup, formatting, Git cleanup,
  networking, SSH auditing, and interactive selection

Most scripts support `--help` or print usage when called without the required
arguments. Read a script before using a command that can delete resources or
change a cluster.

## Customizing the toolkit

### Add or change a helper script

For the Bash workflow, edit the appropriate file under `SHELL/custom/`.
For the Go workflow, edit the corresponding embedded source under
`GO/assets/custom-scripts/`.

After changing embedded assets:

```bash
cd GO
go build -o devops-deployment-go .
./devops-deployment-go update
```

The update operation adds new or changed helper scripts and does not delete
user-only files from `~/.local/bin/scripts/`.

### Add a new installer

1. Add `install-<tool>.sh` under `SHELL/scripts/`.
2. Add the same script under `GO/assets/scripts/`.
3. Add the action to `SHELL/devops-deployment.sh`.
4. Add the corresponding menu action in `GO/internal/menu/menu.go`.
5. Keep platform-specific behavior in the installer and preserve clear
   failure messages.

The Go menu intentionally dispatches to Bash installers. This keeps the
download and platform logic easy to inspect while providing a single binary
front end.

### Customize shell behavior

The source dotfiles live in `SHELL/bash-files/` and are mirrored into
`GO/assets/bash-files/`. Edit both sides when changing behavior for both
entrypoints. The full beginner-oriented explanation of aliases, files,
shortcuts, troubleshooting, and daily usage is in
[`SHELL/GUIDE.md`](SHELL/GUIDE.md).

## Go architecture

The Go application:

1. Detects the operating system and deployment root.
2. Embeds and extracts its assets into a temporary per-run directory.
3. Runs Bash installers with the extracted asset paths in the environment.
4. Syncs maintained helper scripts into `~/.local/bin/scripts/`.
5. Removes the temporary extraction directory on normal exit.

The root-level deployment location can be overridden with
`DEVOPS_DEPLOYMENT_ROOT`. The installed command can retain a checkout
association through the root pin described in [`GO/README.md`](GO/README.md).

The repository's detailed references are:

- [`SHELL/ReadMe.md`](SHELL/ReadMe.md): Bash feature and helper reference
- [`SHELL/GUIDE.md`](SHELL/GUIDE.md): beginner setup and troubleshooting guide
- [`GO/README.md`](GO/README.md): Go runtime, subcommands, asset embedding,
  root resolution, and TUI behavior

## Safety notes

This project downloads software, modifies shell startup files, creates an AWS
config symlink, and includes helpers that can affect cloud or Kubernetes
resources. Review scripts and confirm the active account, cluster, namespace,
and context before executing destructive actions. Keep local `config.env`,
AWS SSO values, kubeconfigs, and downloaded artifacts out of version control.
