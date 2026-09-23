# devops-deployment

Personal dev-environment bootstrap toolkit.

## Usage

1. Copy the `devops-deployment` folder to the `$HOME` folder of the workstation.
2. `cd "$HOME/devops-deployment"`
3. Execute `./devops-deployment.sh`
4. Pick an option in the interactive menu: navigate with the **arrow keys**
   (or `j`/`k`), run with **Enter**, jump with digits `1`-`9` (press the same
   digit twice to run), `c` re-opens the stack/cloud config, `q` quits.
   Requires **bash >= 4**; in a non-interactive terminal it falls back to the
   classic numbered prompt with the same options.
   Item 14 (or the `u` key / `update_scripts` alias) re-copies updated
   helper scripts from the repo into `~/.local/bin/scripts/` without a full
   reinstall.
   On first run the script asks (multi-select) which Kubernetes stack(s)
   (Rancher / OpenShift / GKE / EKS / AKS / plain) and cloud provider(s)
   (AWS / GCP / Azure / none) you use, saving to `~/devops-deployment/config.env`
   and printing tailored notes (e.g. `gke-gcloud-auth-plugin` for GKE,
   `oc login` for OpenShift).
   On macOS the script re-execs itself under Homebrew bash (`brew install bash`)
   because `/bin/bash` is too old.
5. Once done, close and reopen all terminals.

## Notes

- `install-bashtools.sh` now backs up **every** dotfile it overwrites
  (`.bashrc`, `.bash_function`, `.bash_environment`, `.aws_environment`)
  as `<name>_bkp_<YYYYMMDDHHMM>` before replacing it. Earlier versions only
  backed up `.bashrc`.
- `.bashrc` sources `.bash_environment`, `.bash_function`, and
  `.aws_environment` only if they exist, so a fresh shell won't error
  when one is missing.
- The `custom/config` file is a **template** containing placeholder SSO
  details. After install, edit `$HOME/.local/bin/scripts/config` (symlinked
  from `~/.aws/config`) and replace the placeholders with your real SSO
  values. See `custom/config.example` for the format.

## Opt-in: fzf keybindings & cursor styling

Two helpers ship under `custom/misc/` but are **disabled by default** — they
only activate when you opt in, so people who want only autocompletion, only the
cursor, both, or neither are all served without touching the base dotfiles.

Enable them from the main menu (`./devops-deployment.sh`, **option 9 — Setup
fzf/cursor (opt-in)**) or by running the installer directly:

```
~/devops-deployment/scripts/install-fzf-setup.sh
```

The installer presents a sub-menu:

| Option | Effect |
| --- | --- |
| `1` | fzf keybindings only (Alt+S / Ctrl+R / Ctrl+T / Alt+C) |
| `2` | Cursor styling only (blinking red vertical bar) |
| `3` | Both |
| `4` | Remove (restore default behaviour) |

> **TAB is never re-bound.** Normal shell autocompletion keeps working; the
> command picker lives on **Alt+S** instead.

The installer appends a clearly marked, **idempotent** block to `~/.bashrc`
(between `# >>> devops-deployment fzf/cursor setup >>>` and
`# <<< ... <<<` markers) that sources the chosen script(s) on every new shell.
Re-running with a different option replaces the block; choosing `4` removes it.
The base `.bashrc` template is never modified.

You can also toggle either feature ad-hoc in the current shell without changing
`.bashrc`:

```
fzf_setup        # activate keybindings for this shell
set_cursor       # red cursor for this shell
set_cursor --reset
```


## Custom scripts (installed to `~/.local/bin/scripts/` by `install-bashtools.sh`)

Scripts are organized by domain. After install the subfolder layout is preserved
under `~/.local/bin/scripts/`, so e.g. `custom/aws/s3_local_copy.sh` lands at
`~/.local/bin/scripts/aws/s3_local_copy.sh`. The aliases in `.bash_environment`
and the `k` / `aw` / `tf` dispatchers already point at the new locations.

### `custom/aws/` — AWS helpers

| Script | Purpose |
| --- | --- |
| `ami_finder.sh`           | Find the latest AMI matching a name pattern, optionally across regions. |
| `cert_expiry.sh`          | ACM certificate expiry report + optional live TLS probes (color-coded). |
| `cost_snapshot.sh`        | AWS Cost Explorer snapshot: today / MTD / top services & resources. |
| `cw_logs.sh`              | CloudWatch Logs: live tail or time-window search (arrow-key picked group). |
| `ec2_connect.sh`          | Arrow-key-pick a running EC2 instance and start an SSM session on it. |
| `env_diff.sh`             | Diff resources between two namespaces (`k8s` mode) or two profiles (`aws` mode). Lives in `aws/` but supports both. |
| `iam_key_audit.sh`        | IAM access-key age + last-used audit; flags keys older than N days. |
| `local_s3_upload.sh`      | Upload a local file/folder to a selected S3 bucket (interactive). |
| `s3_date_download.sh`     | Download S3 objects whose key contains a date matching an exact date or an inclusive date range. Supports four filename date formats: `YYYYMMDD`, `DDMMYYYY`, `YYYYMM`, `MMYYYY`. Set `S3_DATE_FORMAT=1..4` to pre-select a format and skip the prompt. |
| `s3_local_copy.sh`        | Download objects from a selected S3 bucket (interactive, Space to multi-check). |
| `s3_local_objects.sh`     | Refresh + browse the cached object list of a bucket. |
| `sg_audit.sh`             | Security-group auditor: wide-open ingress, unused SGs, stale ENI refs. |

### `custom/k8s/` — Kubernetes helpers

| Script | Purpose |
| --- | --- |
| `add_yaml_to_kconf.sh`    | Load all `~/k8sconfig/*.yaml` into `kconf`. (Alias: `k8screds`.) |
| `argocd_manager.sh`       | **New.** ArgoCD app dispatcher: list / sync / diff / history / rollback / logs / refresh. Requires the `argocd` CLI. |
| `base64_secret.sh`        | base64 encode/decode + decode every key in a k8s Secret. (Alias: `b64`.) |
| `decode_jwt.sh`           | Decode a JWT (base64url) header/payload into pretty JSON. |
| `helm_lint.sh`            | **New.** `helm lint` + optional `helm template` render using a base-values file auto-detected from `base-files/`, `base/`, or passed via `-f`. |
| `helm_template_test.sh`  | Render a chart with `helm template test . -f values-<env>.yaml` without installing: `-e stg` picks `values-stg.yaml`, arrow-key pickers for chart/values, repeatable `-f`/`--set`/`-n`. |
| `helm_manager.sh`         | Pick a Helm release across namespaces (arrow keys); status/history/rollback/uninstall/get-values. |
| `k_cleanup.sh`            | Cluster janitor: list/delete Evicted pods, Completed Jobs, old ReplicaSets. |
| `k_doctor.sh`             | One-shot pod doctor: events + current/previous logs + restart count. No args = arrow-key pod picker. |
| `k_top.sh`                | Pod resource hogs: top pods by CPU/mem (arrow-key ns picker, `cpu`\|`mem`, `-A`, count). Needs metrics-server. |
| `k_restart.sh`            | Arrow-key-pick a deployment → confirm → `rollout restart` → live rollout status. `--list` to just list. |
| `k_forward.sh`            | Port-forward manager: pick svc/pod (arrow keys), background forward, `list`/`stop <pid|all>` to manage. |
| `k_events.sh`             | Warning-events feed, newest first. `-w` live stream, `-a` include Normal, `-n <ns>`, count arg. |
| `kustomize_check.sh`      | **New.** Validate `kustomization.yaml`: required fields, all `resources`/`components` exist on disk, optional `kustomize build`. |
| `yaml_lint.sh`            | **New.** YAML sanity check: tab-indent detection, trailing whitespace, structural parse. `--strict` makes trailing whitespace an error. |

### `custom/tf/` — Terraform helpers

| Script | Purpose |
| --- | --- |
| `tf_state.sh`             | Terraform state explorer: list / find / show / count / browse (arrow-key picker). |

### `custom/misc/` — General-purpose helpers

| Script | What it does |
| --- | --- |
| `tui_select.sh` | Shared arrow-key list picker (type-to-filter, Space multi-check, no fzf) used by the k8s/aws/tf scripts and shell functions. |

| Script | Purpose |
| --- | --- |
| `docker_prune.sh`         | Safe docker cleanup with a size preview; `--go` to actually delete. |
| `fmt_yaml_json.sh`        | Universal pretty-printer + JSON↔YAML converter. (Alias: `fmt`.) |
| `git_cleanup.sh`          | Delete local git branches merged into the current branch (with confirm). |
| `nslookup_all.sh`         | Resolve a hostname against several resolvers in parallel. |
| `port_check.sh`           | Batch TCP connectivity check from a list of `host:port` lines. |
| `ssh_key_audit.sh`        | List SSH keys with fingerprints; generate new keys interactively (`gen`: ed25519/ecdsa/rsa/dsa, key size, comment, file name + dir via arrow-key picker); purge stale known_hosts. Aliases: `ssh_key_check` (list), `ssh_key_gen` (generate). |
| `fzf_setup.sh`            | **Opt-in.** fzf keybindings (Alt+S=commands, Ctrl+R=history, Ctrl+T=files, Alt+C=cd). TAB untouched. Source-safe. (Alias: `fzf_setup`.) |
| `set_cursor.sh`           | **Opt-in.** Terminal cursor to blinking red vertical bar. `--reset` restores default. Source-safe. (Alias: `set_cursor`.) |

The `custom/config` and `custom/config.example` templates stay at the root of
`custom/` because `install-bashtools.sh` symlinks `~/.aws/config` →
`~/.local/bin/scripts/config`.

### `s3_date_download.sh` example session

```
$ s3_date_download
Fetching S3 buckets list...
> Select source bucket: my-bucket
Fetching object list for bucket: my-bucket ...

Select the date format used in the filenames:
  1 - YYYYMMDD   (e.g. 20260709)
  2 - DDMMYYYY   (e.g. 09072026)
  3 - YYYYMM     (e.g. 202607)
  4 - MMYYYY     (e.g. 072026)
Enter option [1]: 3
Using format: YYYYMM   (e.g. 202607)

Select download mode:
  1 - Exact date   (e.g. matches keys containing that one date)
  2 - Date range   (inclusive of both endpoints)
Enter option: 2
Enter start date (YYYYMM   (e.g. 202607)): 202607
Enter end date   (YYYYMM   (e.g. 202607)): 202609
Found 5 object(s) for range 20260700 .. 20260900 (YYYYMM   (e.g. 202607)).
> <fzf multi-select>
Enter destination directory: ./downloads
Download complete. Success: 5, Failed: 0 -> ./downloads
```

Tip: if every file in a bucket uses the same convention, set it once with
`export S3_DATE_FORMAT=3` (1=YYYYMMDD, 2=DDMMYYYY, 3=YYYYMM, 4=MMYYYY) and the
script skips the format prompt entirely.

Note before proceeding: 
Update line 129, 132, 135 in bash-files/bash_function file accordingly.