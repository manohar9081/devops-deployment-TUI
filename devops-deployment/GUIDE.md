# 📖 Complete Beginner's Guide

> **Goal of this guide:** Even if you have never used a terminal before, by the
> end of this page you will understand what each file does, how to install
> everything, and how to use the shortcuts on a daily basis.
>
> Read it top to bottom the first time. After that, jump to any section using
> the table of contents below.

---

## Table of Contents

1. [What is this folder?](#1-what-is-this-folder)
2. [Big-picture map of the files](#2-big-picture-map-of-the-files)
3. [Prerequisites (what you need before you start)](#3-prerequisites-what-you-need-before-you-start)
4. [Step-by-step setup (the 4 commands you actually run)](#4-step-by-step-setup-the-4-commands-you-actually-run)
5. [After install: what changed on your computer](#5-after-install-what-changed-on-your-computer)
6. [The two magic shortcuts: `k` and `aw`](#6-the-two-magic-shortcuts-k-and-aw)
7. [Every file explained, one by one](#7-every-file-explained-one-by-one)
8. [Daily cheat sheet (copy / paste / print)](#8-daily-cheat-sheet-copy--paste--print)
9. [Troubleshooting (common problems and fixes)](#9-troubleshooting-common-problems-and-fixes)
10. [Glossary (words you will see)](#10-glossary-words-you-will-see)

---

## 1. What is this folder?

This is a **personal starter pack** for a developer or DevOps engineer who works
with **Kubernetes** (often called "k8s") and **AWS** (Amazon's cloud).

Instead of typing long commands like:

```
kubectl get pods -n my-namespace
```

…you will be able to type short ones like:

```
k p
```

…because this toolkit installs **shortcuts**, **helpers**, and the **tools
themselves** (kubectl, helm, terraform, the AWS CLI, etc.) on any Linux
workstation, configured the way you like it, in a few minutes.

Think of it as: **"one click and my whole work setup is ready."**

---

## 2. Big-picture map of the files

```
devops-deployment/
│
├── devops-deployment.sh   ← THE FRONT DOOR. You run this one script.
│
├── scripts/               ← Installers for individual tools.
│   ├── install-kubectl.sh
│   ├── install-k9s.sh
│   ├── install-helm.sh
│   ├── install-terraform.sh
│   ├── install-aws.sh
│   ├── install-brave.sh
│   ├── install-helium.sh
│   ├── install-bashtools.sh   ← installs the shortcuts & dotfiles
│   ├── install-fzf-setup.sh   ← opt-in fzf/cursor setup (menu option 9)
│   └── add_creds_to_aws.sh
│
├── custom/                ← Helper scripts copied to ~/.local/bin/scripts/
│   ├── aws/               ← AWS helpers (12 scripts)
│   │   ├── ami_finder.sh
│   │   ├── cert_expiry.sh
│   │   ├── cost_snapshot.sh
│   │   ├── cw_logs.sh
│   │   ├── ec2_connect.sh
│   │   ├── env_diff.sh
│   │   ├── iam_key_audit.sh
│   │   ├── local_s3_upload.sh
│   │   ├── s3_date_download.sh
│   │   ├── s3_local_copy.sh
│   │   ├── s3_local_objects.sh
│   │   └── sg_audit.sh
│   ├── k8s/               ← Kubernetes helpers (14 scripts)
│   │   ├── add_yaml_to_kconf.sh
│   │   ├── argocd_manager.sh
│   │   ├── base64_secret.sh
│   │   ├── decode_jwt.sh
│   │   ├── helm_lint.sh
│   │   ├── helm_manager.sh
│   │   ├── k_cleanup.sh
│   │   ├── k_doctor.sh
│   │   ├── k_events.sh
│   │   ├── k_forward.sh
│   │   ├── k_restart.sh
│   │   ├── k_top.sh
│   │   ├── kustomize_check.sh
│   │   └── yaml_lint.sh
│   ├── tf/                ← Terraform helpers
│   │   └── tf_state.sh
│   ├── misc/              ← General-purpose helpers
│   │   ├── docker_prune.sh
│   │   ├── fmt_yaml_json.sh
│   │   ├── fzf_setup.sh
│   │   ├── git_cleanup.sh
│   │   ├── nslookup_all.sh
│   │   ├── port_check.sh
│   │   ├── set_cursor.sh
│   │   └── ssh_key_audit.sh
│   ├── config             ← AWS SSO template (EDIT THIS with your details)
│   └── config.example     ← reference example of the above
│
├── bash-files/            ← Dotfiles copied into your home folder.
│   ├── bashrc                  ← runs every time you open a terminal
│   ├── bash_function           ← interactive helper functions
│   ├── bash_environment        ← aliases (short nicknames for commands)
│   ├── aws_environment   ← more aliases (SSM session shortcuts)
│   └── commands_environment    ← the `k`, `aw`, and `tf` shortcuts
│
├── tmp/                   ← temporary download folder (auto-cleaned)
├── ReadMe.md              ← short technical reference
└── GUIDE.md               ← THIS FILE (the friendly long version)
```

**One-line summary:** `devops-deployment.sh` is the boss. It calls the
installers in `scripts/`. The installers download tools. `install-bashtools.sh`
also copies everything in `custom/` and `bash-files/` into your home folder so
the shortcuts become available in every terminal.

---

## 3. Prerequisites (what you need before you start)

You need **all of these**. If any is missing, the install will tell you.

| Need | Why | How to check |
| --- | --- | --- |
| A **Linux** machine (x86_64 or arm64) | The installers download Linux binaries. | `uname -s` should print `Linux`. |
| **bash 4 or newer** | The interactive menu needs it (any modern Linux has this). | `bash --version` |
| A **terminal** open | That's where you type commands. | — |
| **`curl`** and **`wget`** | Used to download tools. | `curl --version` and `wget --version` |
| **`jq`** | Reads version numbers from GitHub. | `jq --version` |
| **`unzip`** | Unzips AWS CLI / terraform archives. | `unzip -v` |
| **Internet access** | Tools are downloaded from GitHub / AWS / HashiCorp. | `ping -c1 github.com` |

> **On macOS?** The menu runs there too, and the installers pick the right
> `darwin/arm64` downloads automatically (verified against kubectl, Helm, K9s,
> Terraform and fzf upstreams). Notes:
> - `./devops-deployment.sh` needs `brew install bash` once — the script then
>   re-launches itself under the newer bash automatically.
> - kubectl's installer installs **kubectx/kubens via Homebrew** on macOS
>   (the release binaries are Linux-only); AWS CLI uses Apple's universal
>   bundle and the SSM plugin comes from a brew cask.
> - **Brave and Helium** download Linux AppImages — those two options are
>   skipped with a message on macOS.

> Optional but recommended: **`fzf`** (fuzzy finder — powers the Alt+S / Ctrl+R
> shell search). It is installed automatically by `install-bashtools.sh`.
> The interactive menus in the scripts use a built-in arrow-key picker
> (`custom/misc/tui_select.sh`) and do **not** need fzf.

---

## 4. Step-by-step setup (the 4 commands you actually run)

Open a terminal and run below commands:

```bash
# 1. Put the folder in your home directory.
cp -r devops-deployment "$HOME"

# 2. Go inside it.
cd "$HOME/devops-deployment"

# 3. Make the main script runnable (only needed once).
chmod +x devops-deployment.sh

# 4. Run it.
./devops-deployment.sh
```

You will see an interactive menu like this:

```
 ██████╗ ███████╗██████╗ ██████╗  ██████╗ ██╗  ██╗
 ██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔═══██╗╚██╗██╔╝
 ██║  ██║███████╗██████╔╝██║  ██║██║   ██║ ╚███╔╝
 ██║  ██║╚════██║██╔══██╗██║  ██║██║   ██║ ██╔██╗
 ██████╔╝███████║██║  ██║██████╔╝╚██████╔╝██╔╝ ██╗
 ╚═════╝ ╚══════╝╚═╝  ╚═╝╚═════╝ ╚═════╝ ╚═╝  ╚═╝
 D E P L O Y M E N T

✓ prerequisites ready   os: linux · pkg: apt

 ▶  1  Install Kubectl       k8s CLI + kubectx/kubens
    2  Install K9s           k8s terminal UI
    3  Install Helm          k8s package manager
    4  Install Terraform     infrastructure as code
    5  Install AWS CLI       aws-cli v2 + SSM plugin
    6  Install Brave         browser (AppImage)
    7  Install Helium        browser (AppImage)
    8  Install Bash tools    dotfiles + aliases + k cmd
    9  Setup fzf + cursor    interactive search (opt-in)
   10  Install everything    runs 1-9 in order
   11  Install core only     skips browsers & fzf setup
   12  Install oc            OpenShift/kubectl client
   13  Config: stacks & cloud  choose k8s stack + cloud
   14  Update installed scripts  re-copy custom/ to ~/.local/bin

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
↑↓ navigate   ⏎ run   1-9 select   c config   u update   q quit
```

**Update installed scripts (item 14 / `u` / `update_scripts` alias):** re-copies
`~/devops-deployment/custom/` into `~/.local/bin/scripts/` so fixes made in
the repo reach the aliases (`k8screds`, `k_top`, `ssh_key_gen`, ...) without
re-running the full Bash-tools installer. Shows what changed first; never
deletes anything. Dotfiles (`~/.bashrc`, aliases) still come from option 8.

On the **first run** the script asks — with a multi-select picker — which
Kubernetes stack(s) you use (Rancher, OpenShift, GKE, EKS, AKS, plain) and
which cloud provider(s) (AWS, GCP, Azure, none). The answers are saved to
`~/devops-deployment/config.env`, shown as tailored notes after the
prerequisite checks (e.g. the `gke-gcloud-auth-plugin` reminder for GKE), and
can be changed any time via the menu's **Config** entry or the `c` key.

Navigate with the **arrow keys** (or `j`/`k`), press **Enter** to run the
highlighted entry, or type a digit (`1`-`9`) to jump straight to it — pressing
the same digit twice runs it. `g`/`Home` jump to the top, `G`/`End` to the
bottom, `q` quits. Options 10 and 11 ask for confirmation first.

> Note: option **9 is opt-in**. "Install everything" (10/11) does **not**
> enable the fzf keybindings or cursor styling — choose 9 separately only if
> you want them.

**Recommended for a first install:** highlight **11 — Install core only**
(arrow down to it, press Enter, confirm with `y`). This installs everything
except the fzf/cursor setup and the Brave/Helium browsers. When it finishes:

```bash
# 5. Close this terminal and open a new one (so the new shortcuts load).
exit
```

Open a fresh terminal. You are ready. ✅

> **Why reopen the terminal?** The shortcuts live in files that your terminal
> reads *only when it starts*. A new terminal = a fresh read = shortcuts
> available. You do not need to reboot the computer.

---

## 5. After install: what changed on your computer

Here is the full list:

| Where | What |
| --- | --- |
| `~/.local/bin/` | The actual tools (kubectl, helm, terraform, aws, k9s, fzf, brave, helium…). This folder was added to your `PATH`, so its programs are callable by name. |
| `~/.local/bin/scripts/` | The helper scripts from `custom/`, preserving the subfolder layout (`aws/`, `k8s/`, `tf/`, `misc/`). |
| `~/.local/bin/kubectl-versions/` | Every kubectl version you ever install, side by side. |
| `~/.local/bin/terraform-versions/` | Same idea, for terraform. |
| `~/.local/bin/s3-bucket-objects/` | Cache of S3 object listings, one file per bucket. |
| `~/.aws/config` | A symlink to `~/.local/bin/scripts/config`. **Edit this with your AWS SSO details.** |
| `~/.bashrc` | **Replaced.** The old one was saved as `~/.bashrc_bkp_<timestamp>`. |
| `~/.bash_function`, `~/.bash_environment`, `~/.aws_environment`, `~/.commands_environment` | **Created / replaced.** Old versions backed up with a `_bkp_<timestamp>` suffix. |
| `~/awsconfig/` | Empty folder. Put one `.txt` file per long-lived AWS credential profile here (optional — most people use SSO instead). |
| `~/k8sconfig/` | Empty folder. Put your Kubernetes context `.yaml` files here, then run `k8screds` to load them. |

> 💡 **About backups:** Every dotfile the installer overwrites is saved first
> with a name ending in `_bkp_` + the date and time. You can always find your
> old setup and restore it if needed.

---

## 6. The two magic shortcuts: `k` and `aw`

These are the stars of the show. They are **functions** (smart aliases) defined
in `~/.commands_environment`.

### `k` → talks to Kubernetes

Type `k` alone to see all options. A few favorites:

| You type | It runs | Meaning |
| --- | --- | --- |
| `k p` | `kubectl get pods` | List pods |
| `k svc` | `kubectl get svc` | List services |
| `k ns` | `kubectl get namespaces` | List namespaces |
| `k dep` | `kubectl get deployments` | List deployments |
| `k logs <pod>` | `kubectl logs <pod>` | Show a pod's logs |
| `k logs f <pod>` | `kubectl logs -f <pod>` | Follow logs live |
| `k ex <pod>` | `kubectl exec -it <pod>` | Shell into a pod |
| `k ctx` | (interactive) | Switch Kubernetes context |
| `k get svc -A` | `kubectl get svc -A` | Anything you type is passed through |

### `aw` → talks to AWS

Type `aw` alone to see all options. A few favorites:

| You type | It runs | Meaning |
| --- | --- | --- |
| `aw id` | `aws sts get-caller-identity` | "Who am I in AWS right now?" |
| `aw profiles` | `aws configure list-profiles` | List configured profiles |
| `aw s3` | `aws s3 ls` | List your S3 buckets |
| `aw ec2` | `aws ec2 describe-instances ...` | List EC2 machines (as a table) |
| `aw login` | (interactive) | AWS SSO login |
| `aw profile` | (interactive) | Set the default SSO profile |
| `aw s3 ls s3://bucket` | `aws s3 ls s3://bucket` | Passthrough again |

> **The rule to remember:** If `k` or `aw` does not have a built-in shortcut
> for what you want, it just forwards everything to the real tool. So you can
> never "break" a command by using `k` or `aw`.

---

## 7. Every file explained, one by one

### The front door

#### `devops-deployment.sh`
The menu. Asks you what to install, then calls the right installer(s). With the
recent update it uses a single list of tools, so adding a new tool later is a
one-line change. The menu is interactive (arrow keys / `j`/`k` / digits / Enter,
`q` to quit) and needs bash >= 4; without a proper terminal it falls back to the
classic numbered prompt with the same options. Bulk options 10/11 confirm before
they run.

---

### Installers (`scripts/`)

#### `install-kubectl.sh`
Downloads **kubectl** (the Kubernetes command-line tool) plus **kubectx** and
**kubens** (fast context/namespace switchers). Stores each version in
`~/.local/bin/kubectl-versions/` and points `~/.local/bin/kubectl` at the
selected one via a symlink — so you can keep many versions side by side.

#### `install-k9s.sh`
Downloads **k9s**, a text-based user interface for exploring Kubernetes. Think
"file manager, but for pods/services/etc."

#### `install-helm.sh`
Downloads **Helm**, the package manager for Kubernetes (used to install things
like ingress controllers or databases into a cluster).

#### `install-terraform.sh`
Downloads **Terraform** (Infrastructure as Code — define cloud resources in
text files). Same multi-version pattern as kubectl.

#### `install-aws.sh`
Downloads the **AWS CLI v2** and the **Session Manager plugin** (used for SSM
sessions into EC2 instances without SSH).

#### `install-oc.sh`
Downloads the OpenShift **`oc`** client from mirror.openshift.com (the
`latest` stream: Linux x86_64 and macOS). Selecting **OpenShift** in the
stack/cloud config also prints the `oc login` hint; the k8s scripts work the
same against OpenShift once you are logged in.

#### `install-brave.sh`
Downloads the **Brave browser** as a portable AppImage. If Brave is running
when you launch the install, it politely asks it to close (SIGTERM, with a
graceful fallback) instead of killing it abruptly.

#### `install-helium.sh`
Downloads the **Helium browser** as a portable AppImage. If Helium is running
when you launch the install, it politely asks it to close (SIGTERM, with a
graceful fallback) instead of killing it abruptly.

#### `install-bashtools.sh`
Installs **fzf** (the fuzzy finder behind the Alt+S / Ctrl+R shell search) and then
copies all the dotfiles and helper scripts into place. **Every file it
overwrites is backed up first** with a `_bkp_<timestamp>` suffix.

#### `add_creds_to_aws.sh`
(Optional.) If you keep long-lived AWS credentials as `.txt` files in
`~/awsconfig/`, this stitches them together into a single
`~/.aws/credentials.conf`. It refuses to run if the folder is empty (so it can
never wipe your credentials by accident) and locks the file to `chmod 600`.

#### `install-fzf-setup.sh`  (opt-in, menu option 9)
Enables the optional **fzf keybindings** and/or **cursor styling**. These are
**off by default** — the base `.bashrc` is never modified. Picking menu option
9 shows a sub-menu:

```
=== fzf / cursor setup (opt-in) ===
  1  - fzf keybindings only   (Alt+S / Ctrl+R / Ctrl+T / Alt+C)
  2  - Cursor styling only    (blinking red vertical bar)
  3  - Both
  4  - Remove (restore default behaviour)
```

> **TAB is never re-bound** — normal autocompletion keeps working. The command
> picker lives on **Alt+S**.

The installer appends a clearly marked, **idempotent** block to `~/.bashrc`
(between `# >>> devops-deployment fzf/cursor setup >>>` markers) that sources
the chosen helper script(s) on every new shell. Re-running with a different
option replaces the block; option `4` removes it. So you can safely try each
combination and undo.

---

### Helper scripts (`custom/`)

Scripts are organized by domain in subfolders. After install the subfolder
layout is preserved under `~/.local/bin/scripts/`, so e.g.
`custom/aws/s3_local_copy.sh` lands at
`~/.local/bin/scripts/aws/s3_local_copy.sh`. The aliases in
`.bash_environment` and the `k` / `aw` / `tf` dispatchers point at the new
locations.

#### `custom/aws/` — AWS helpers

##### `ami_finder.sh`
Find the latest AMI matching a name pattern, optionally across multiple
regions. Useful when bootstrapping EC2 / Launch Templates.

##### `cert_expiry.sh`
ACM certificate expiry report with color-coded flags (expired / critical /
warn). Optionally probes live TLS endpoints from a file of hostnames.

##### `cost_snapshot.sh`
Quick AWS Cost Explorer snapshot: today vs yesterday, MTD total, top 5
services and top 5 resources.

##### `cw_logs.sh`
CloudWatch Logs helper: tail live or search a time window with an optional
pattern filter. Arrow-key-picks the log group from a per-profile cache.

##### `ec2_connect.sh`
Browse running EC2 instances (Name, state, IP) via the arrow-key picker and start an SSM
session on the chosen one. Replaces hard-coded per-host aliases.

##### `env_diff.sh`
Diff the resources of two Kubernetes namespaces, or the buckets / instances
of two AWS profiles. Answers "why does prod work but stg doesn't?".
Usage: `env_diff.sh k8s <ns-a> <ns-b>` or `env_diff.sh aws <profile-a>
<profile-b> [region]`. Lives in `aws/` but supports both modes.

##### `iam_key_audit.sh`
IAM access-key auditor: reports every key with its age and last-used info,
flagging keys older than 90 days (configurable threshold).

##### `local_s3_upload.sh`
Upload a local file or folder to an S3 bucket. Interactive: pick the bucket
with the arrow-key picker, type the path, done.

##### `s3_date_download.sh`
Download S3 objects whose **filename contains a date**. First you pick the
**date format** used in your filenames, then the match mode:

- **Formats** — pick one up front (the raw digits are ambiguous, so you must
  declare the convention):
  - `YYYYMMDD` — e.g. `20260709` matches `Testfile20260709.zip`
  - `DDMMYYYY` — e.g. `09072026` matches `Testfile09072026.zip`
  - `YYYYMM`   — e.g. `202607`   matches `Report_202607.csv`
  - `MMYYYY`   — e.g. `072026`   matches `Report_072026.csv`
- **Exact date** — match keys containing that one date.
- **Date range** — match every key whose date falls between two dates,
  inclusive of both endpoints. Ranges work across formats too: a `YYYYMM`
  range `202607` to `202609` matches July, August and September.

It only ever matches a clean digit run of the chosen length (8 for day
formats, 6 for month formats), so longer numbers like
`backup1234567890.tar` are **not** falsely matched. Set `S3_DATE_FORMAT=1..4`
in the environment to pre-select a format and skip the prompt.

##### `s3_local_copy.sh`
Download one or more objects from an S3 bucket. Refreshes the cached object
list, then lets you multi-select what to download. Per-item error isolation:
one failure won't abort the rest.

##### `s3_local_objects.sh`
Just refresh and browse the object list of a bucket (no download). Handy to
"peek" at what is inside without pulling anything.

##### `sg_audit.sh`
Security-group auditor: finds wide-open ingress (0.0.0.0/0), unused SGs,
and rules referencing stale network interfaces.

#### `custom/k8s/` — Kubernetes helpers

##### `add_yaml_to_kconf.sh`
Takes every `.yaml` in `~/k8sconfig/` and registers it as a Kubernetes
context using **kconf**. Removes existing contexts first. Used through the
`k8screds` alias.

##### `argocd_manager.sh`
ArgoCD application manager. A dispatcher over the `argocd` CLI with an arrow-key picker
where it helps: list apps, sync, diff, history/rollback, logs, refresh,
manifest. Requires the `argocd` CLI. Run `argocd_manager.sh` (or `k argo`)
with no arguments to see all commands.

##### `base64_secret.sh`
base64 encode/decode helper + k8s Secret decoder. Usage: `b64 enc "value"`,
`b64 dec "<base64>"`, `b64 kget <secret-name> [namespace] [key]`.

##### `decode_jwt.sh`
Decode a JWT (base64url header.payload.signature) into pretty JSON.
Accepts a token as an argument or on stdin.

##### `helm_lint.sh`
Helm chart lint + render helper. Runs `helm lint` on a chart and optionally
`helm template`-renders it using a base values file auto-detected from
`base-files/`, `base/`, or passed via `-f`. Usage: `helm_lint.sh <chart>
--render`, or just `helm_lint.sh` to arrow-key-pick a chart.

##### `helm_template_test.sh`
Render a chart locally without installing: `helm_template_test.sh . -e stg`
runs `helm template test . -f values-stg.yaml`. With no `-e` it lists the
chart's `values*.yaml` files in an arrow-key picker (single file = used
automatically). Repeatable `-f other.yaml`, `--set key=value` and
`-n namespace` are passed through; output goes to stdout (`> out.yaml` to
save). Alias: `htest`.

##### `helm_manager.sh`
Helm release manager: arrow-key-pick a release across all namespaces, then
status / history / rollback / uninstall / get-values.

##### `k_cleanup.sh`
Cluster janitor: lists (and optionally deletes, with confirmation) the usual
clutter — Evicted/Failed pods, Completed Jobs, old ReplicaSets.

##### `k_doctor.sh`
One-shot pod doctor: describe + events + current/previous logs + restart
count and last termination reason. Great for "why is this pod crashing?".

##### `k_top.sh`
Pod resource hogs. Runs `kubectl top pods` and sorts by CPU (default) or
memory so the noisy neighbours surface immediately. fzf-picks the namespace
(or pass `-A`, `-n <ns>`), and a number argument limits the list (default 20).
Needs metrics-server on the cluster. (`k top mem`)

##### `k_restart.sh`
The safe deployment restart: fzf-pick a deployment across namespaces, see its
replica count, confirm, `rollout restart`, then watch `rollout status` until
it settles. `k_restart.sh <deploy> [ns]` for non-interactive use, `--list` to
just list. (`k restart`)

##### `k_forward.sh`
Port-forward manager. fzf-pick a service (it shows the service's ports) or a
pod, choose local/remote ports, and the forward runs in the background with a
registry so `list` shows what's active and `stop <pid|all>` cleans up. No more
forgotten port-forwards. (`k fwd`)

##### `k_events.sh`
Warning-events feed, newest first — what's actually going wrong on the
cluster. `-w` streams live (Ctrl+C to stop), `-a` includes Normal events,
`-n <ns>` scopes to a namespace, a number sets the count (default 30).
(`k ev`)

##### `kustomize_check.sh`
Validates a `kustomization.yaml`: checks required fields (`apiVersion`,
`kind`), verifies all `resources` and `components` referenced on disk
actually exist, and optionally runs `kustomize build` to confirm the bundle
renders. Usage: `kustomize_check.sh path/ --build`.

##### `yaml_lint.sh`
YAML sanity checker. Catches the three classes of mistakes that bite before
kubectl validates the resource: (1) **TAB characters** in indentation,
(2) **bad indentation / structure** (parsed + re-emitted to verify), (3)
**trailing whitespace**. Works with python3+PyYAML, `yq`, or a pure-bash
fallback. `--strict` makes trailing whitespace an error. Usage: `yaml_lint.sh
file.yaml`, `yaml_lint.sh dir/`, or `yaml_lint.sh --strict`.

#### `custom/tf/` — Terraform helpers

##### `tf_state.sh`
Terraform state explorer: list, find, show, count, browse (fzf) resources in
the current state file. Also supports force-unlock.

#### `custom/misc/` — General-purpose helpers

##### `docker_prune.sh`
Safe docker cleanup with a size preview before deletion. `--go` to actually
delete.

##### `fmt_yaml_json.sh`
Universal pretty-printer and converter for JSON / YAML. Auto-detects input
format and can convert between them. (Alias: `fmt`.)

##### `git_cleanup.sh`
Delete local git branches merged into the current branch. Shows the list
first, then asks for confirmation. `--force` to skip the prompt.

##### `nslookup_all.sh`
Resolve a hostname against several resolvers in parallel (system, Google,
Cloudflare, Route 53) to expose split-horizon / cache-poisoning issues.

##### `port_check.sh`
Batch TCP connectivity check. Reads `host:port` lines from a file or stdin
and reports reachability.

##### `ssh_key_audit.sh`
List SSH keys with fingerprints, and optionally purge stale known_hosts
entries. Aliases: `ssh_key_check` lists, `ssh_key_gen` generates.
`ssh_key_audit.sh gen` generates a new key interactively: an
arrow-key picker for the type (ed25519 recommended, ecdsa, rsa, dsa) and the
key size where it applies (rsa 2048/3072/4096, ecdsa 256/384/521), then
prompts for a comment (empty = no comment), a file name (cannot be empty) and
a directory (empty = `~/.ssh`). Refuses to silently overwrite an existing
key.

##### `fzf_setup.sh`  (opt-in)
Source-safe script that activates fzf keybindings in the current shell. Binds
**Alt+S** to a combined fuzzy-search menu (aliases + functions + history +
executables), **Ctrl+R** to history search, **Ctrl+T** to a file picker that
inserts the path at the cursor, and **Alt+C** to a directory picker that `cd`s
into the selection. **TAB is left untouched** for normal shell autocompletion.
(Alias: `fzf_setup`.) **Disabled by default** — enable it
permanently via menu option 9 (`install-fzf-setup.sh`), or run `fzf_setup` for
just the current terminal.

##### `set_cursor.sh`  (opt-in)
Source-safe script that sets the terminal cursor to a **blinking red vertical
bar** on xterm-compatible terminals. Use `set_cursor.sh --reset` to restore the
default cursor. (Alias: `set_cursor`.) **Disabled by default** — enable it
permanently via menu option 9, or run `set_cursor` for just the current terminal.

#### `config` / `config.example` (root of `custom/`)
`config` is the **AWS SSO template** copied to `~/.aws/config`. It ships with
placeholder values — **edit it** and put your real SSO start URL, account IDs
and role names. `config.example` is a clean reference copy you can compare
against. These stay at the root of `custom/` because the symlink
`~/.aws/config` → `~/.local/bin/scripts/config` must work.

---

### Dotfiles (`bash-files/`)

#### `bashrc`
Read by bash every time a terminal opens. It:
- puts `~/.local/bin` on your `PATH`,
- sources the other dotfiles,
- builds the colorful prompt (showing pwd, k8s context/namespace, git branch,
  and current AWS/SSO profile).

#### `bash_function`
Interactive helper functions: `kctx`, `kens` (switch context / namespace via
the arrow-key picker), `k8s_version`, `tf_version` (switch kubectl / terraform
version), `sso_login`, `sso_profile`, `ssm_session`, `aws_profile`, and the
prompt helpers `kubectl_prompt_info` / `git_branch_info`. All their pickers
share `__tui_pick`, which uses `tui_select.sh` and needs no fzf.

#### `bash_environment`
Aliases (nicknames) for the helper scripts, organized by domain (aws/, k8s/,
tf/, misc/), and some frequent destinations, e.g. `s3_date_download`,
`bin.` (cd to `~/.local/bin`), `deployment` (run the main menu).

#### `aws_environment`
Aliases for SSM sessions into specific environments (staging / GTE / prod).
Edit to match your own hostnames.

#### `commands_environment`
Defines the **`k`** (kubectl), **`aw`** (aws), and **`tf`** (terraform)
dispatcher functions described in section 6. Also includes shortcuts for the
new helpers: `k hlint` (helm lint), `k ylint` (yaml lint), `k kust`
(kustomize check), `k argo` (ArgoCD manager), `k b64` (base64/secret).

---

## 8. Daily cheat sheet (copy / paste / print)

```text
┌──────────────────────────────────────────────────────────────┐
│  FIRST TIME SETUP                                            │
│    cp -r devops-deployment ~                                 │
│    cd ~/devops-deployment && chmod +x devops-deployment.sh   │
│    ./devops-deployment.sh          # pick option 11          │
│    exit                            # reopen terminal         │
├──────────────────────────────────────────────────────────────┤
│  KUBERNETES  (type `k` for full list)                        │
│    k p                list pods                              │
│    k svc              list services                          │
│    k dep              list deployments                       │
│    k ns               list namespaces                        │
│    k logs f <pod>     follow a pod's logs                    │
│    k ex <pod>         shell into a pod                       │
│    k ctx              switch context (interactive)           │
│    k doc <pod>        pod doctor (events+logs+crash)         │
│    k cleanup          find/delete evicted pods, old RS       │
│    k top [mem]        top pods by CPU/mem (fzf ns picker)    │
│    k restart          rollout restart (fzf + rollout status) │
│    k fwd              port-forward manager (list/stop)       │
│    k ev [-w]          warning events (-w live, -a all)       │
│    k helm             helm release manager (fzf)             │
│    k hlint [chart]    helm lint + optional render            │
│    k ylint [path]     yaml lint: tabs / spacing / parse      │
│    k kust [path]      validate kustomization.yaml            │
│    k argo             argocd app manager                     │
│    k b64 enc "val"    base64 encode                          │
│    k b64 kget <sec>   decode k8s secret keys                 │
│    k8screds           reload k8s contexts from ~/k8sconfig   │
├──────────────────────────────────────────────────────────────┤
│  AWS  (type `aw` for full list)                              │
│    aw id              who am I in AWS                        │
│    aw profiles        list profiles                          │
│    aw s3              list buckets                           │
│    aw login           SSO login (interactive)                │
│    aw profile         set default SSO profile (interactive)  │
├──────────────────────────────────────────────────────────────┤
│  TERRAFORM  (type `tf` for full list)                        │
│    tf plan            terraform plan                         │
│    tf apply           terraform apply                        │
│    tf ws              list / pick workspace (TUI)            │
│    tf state list      browse terraform state                 │
│    tf fmt             format all .tf files                   │
├──────────────────────────────────────────────────────────────┤
│  S3 HELPERS                                                  │
│    local_s3_upload    upload a file/folder to S3             │
│    s3_local_copy      download objects from S3               │
│    s3_local_objects   browse a bucket's contents             │
│    s3_date_download   download by date / date range          │
├──────────────────────────────────────────────────────────────┤
│  OTHER SHORTCUTS (standalone aliases)                        │
│    argocd_mgr         argocd app manager                     │
│    helm_lint          helm lint + render                     │
│    yaml_lint          yaml tab/spacing checker               │
│    kustomize_check    validate kustomization.yaml            │
│    b64                base64 / secret helper                 │
│    fmt                pretty-print JSON / YAML (stdin)       │
│    env_diff           diff two namespaces or profiles        │
│    cost_snapshot      AWS cost explorer snapshot             │
│    sg_audit           security group auditor                 │
│    iam_key_audit      access key age audit                   │
│    cert_expiry        certificate expiry report              │
│    ami_finder         find latest AMI by pattern             │
│    cw_logs            cloudwatch logs tail/search            │
│    ec2_connect        SSM into a running instance (fzf)      │
│    docker_prune       safe docker cleanup (--go to delete)   │
│    git_cleanup        delete merged branches                 │
│    port_check         batch TCP connectivity check           │
│    nslookup_all       resolve hostname across resolvers      │
│    ssh_key_check      list keys · purge known_hosts          │
│    ssh_key_gen        generate a new SSH key (interactive)   │
│    fzf_setup          activate fzf bindings (opt-in)         │
│    set_cursor         blinking red cursor (opt-in)           │
├──────────────────────────────────────────────────────────────┤
│  OPT-IN: menu option 9 (install-fzf-setup.sh)                │
│    pick 1/2/3/4: completions / cursor / both / remove        │
│    once enabled, these bindings are live in every terminal:  │
│      Alt+S    fuzzy-pick aliases/functions/cmds              │
│      Ctrl+R   fuzzy-search command history                   │
│      Ctrl+T   fuzzy-pick file, insert path                   │
│      Alt+C    fuzzy-pick directory, cd into it               │
│      (TAB keeps its normal autocompletion behaviour)         │
├──────────────────────────────────────────────────────────────┤
│  VERSION SWITCHERS                                           │
│    k8s_version         pick kubectl version (interactive)    │
│    tf_version          pick terraform version (interactive)  │
└──────────────────────────────────────────────────────────────┘
```

---

## 9. Troubleshooting (common problems and fixes)

**"command not found: k / aw / kubectl / aws"**
→ You forgot to reopen the terminal after install. Close it and open a new one.

**The colorful prompt does not show the k8s context / AWS profile.**
→ You are not on a cluster / not logged in. Run `k ctx` to pick a context, and
`aw profile` + `aw login` for AWS.

**`s3_date_download` finds nothing for a date I know is there.**
→ The date must appear as an 8-digit run (`YYYYMMDD`) in the filename. If your
files use dashes (`2026-07-09`) the script will not match them yet. Rename or
extend the matcher.

**Install fails with "Unsupported architecture".**
→ You are on a CPU the scripts do not handle (only `x86_64` and `aarch64` are
supported). On an Apple Silicon Mac, run inside a Linux VM.

**`curl: (22) The requested URL returned an error: 404`.**
→ The version you typed does not exist for that tool. Re-run and press Enter
to take "latest" instead.

**Something went wrong and I want my old config back.**
→ Every overwritten dotfile was backed up. List them with:
`ls -la ~ | grep _bkp_` and copy any one back, e.g.
`cp ~/.bashrc_bkp_202607111045 ~/.bashrc`.

**`install-bashtools` complains a file already exists.**
→ It does not complain anymore — it backs the existing file up automatically
and then overwrites. If you see this message you are on an old copy of the
script; re-copy `devops-deployment` from this folder.

---

## 10. Glossary (words you will see)

| Word | Plain-English meaning |
| --- | --- |
| **Terminal** | The black window where you type commands. |
| **Shell (bash)** | The program inside the terminal that actually runs what you type. |
| **`PATH`** | The list of folders the shell searches when you type a command name. |
| **Alias** | A nickname for a command (e.g. `bin.` = `cd ~/.local/bin`). |
| **Function** | Like an alias, but smarter — can take arguments and make decisions. |
| **Dotfile** | A config file whose name starts with a dot (like `.bashrc`); hidden by default. |
| **kubectl** | The command-line tool for talking to Kubernetes. |
| **Context / Namespace** | Kubernetes concepts: "which cluster am I talking to" / "which section of it". |
| **S3** | Amazon's file-storage service. A "bucket" is a top-level folder in S3. |
| **SSO** | Single Sign-On — log in once with your browser, get temporary credentials. |
| **SSM** | AWS Systems Manager; here, used to open a shell on an EC2 instance without SSH. |
| **fzf** | A fast fuzzy picker — the thing that lets you search a list by typing partial text. |
| **Symlink** | A shortcut file that points at another file (e.g. `~/.aws/config` → `…/scripts/config`). |
| **Helm** | A package manager for Kubernetes. |
| **Terraform** | A tool to define cloud infrastructure as text files. |

---
