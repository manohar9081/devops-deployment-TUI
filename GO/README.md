# devops-deployment-go

`devops-deployment-go` is a Go port of the repo's bash entrypoint
`devops-deployment.sh`: it detects the OS, checks the installer
prerequisites, keeps `$HOME/.local/bin` on the `PATH`, creates the `tmp/`
scratch directory, asks the first-run stack/cloud questions into
`config.env`, and then presents the interactive tool menu (kubectl, k9s,
helm, terraform, AWS CLI, Brave, …).

## Embedded assets

The binary is self-contained: every runtime asset is embedded at build
time from `GO/assets/` (package `assets`, `//go:embed`):

- `assets/scripts/` — the bash installers (`install-<tool>.sh`,
  `update-scripts.sh`, `add_creds_to_aws.sh`);
- `assets/bash-files/` — the dotfiles (`bashrc`, `bash_function`,
  `bash_environment`, `aws_environment`, `commands_environment`);
- `assets/custom-scripts/` — the maintained helper scripts
  (`aws/ k8s/ tf/ misc/`, plus the AWS SSO `config` template).

`go build` therefore captures a new version of everything — a freshly
built binary carries the whole toolkit, and the repo folders are dev-time
sources only. At startup (the interactive menu path) the embedded tree is
materialized (`assets.Extract`) into a fresh per-run temp directory
(`$TMPDIR/devops-assets-<random>`): nothing is reused across runs, so a
binary upgrade always serves its own current installers. The directory is
removed right before the program exits; a session that ends through an
`os.Exit` path (an invalid fallback option, the TUI's Ctrl-C) or a crash
leaves one directory behind for the OS temp cleaner. The menu then
dispatches installers from the materialized `scripts/` directory and
exports the layout for the child scripts: `SCRIPT_DIR`,
`SCRIPT_TMPDIR` (`<root>/tmp`), `ROOT`, `BASHFILES_DIR` and `DEVOPS_SELF`
(the running executable, not symlink-resolved — install-bashtools.sh
copies that file onto the PATH as `devops` and, seeing it set, leaves the
root pin to the Go side). Files named `.DS_Store` are filtered out during
extraction and in the update sync, so Finder churn never shows up as an
update.

## Build and run

Requires a Go toolchain (module targets Go 1.22); stdlib only, no module
dependencies.

```sh
cd GO && go build -o devops-deployment-go .
./devops-deployment-go
```

`GO/.gitignore` already ignores the built binary. Cross-compile with e.g.
`GOOS=linux go build ./...` — the raw-mode/terminal code has per-OS
implementations (darwin, linux, other).

### Subcommands

Besides the interactive menu (the default, no arguments) the binary takes
two subcommands: `update` re-runs the menu's "Update installed scripts"
action (item 14 / the `u` key) non-interactively — no `config.env` load,
no prerequisite checks, no final `Done.` line — exiting with the updater's
status; `install` copies the binary that was run to `~/.local/bin/devops`
(creating the directory), which sets up the `devops` command from any
copy of the binary; `help` (also `-h` / `--help`) prints the usage. Any
other argument exits 1 with a one-line error plus the usage on stderr.

The update itself is pure Go: it extracts the embedded assets to a fresh
temp dir, syncs `custom-scripts/` from there into `~/.local/bin/scripts`
(see "Custom scripts" below) and removes the temp dir. When it finishes,
it also ensures the `devops` command against the checkout binary inside
the resolved root's GO module (`<root>/GO/devops-deployment-go`): a
missing `~/.local/bin/devops` is installed — so running the freshly built
binary's `update` re-creates a deleted command — a differing copy is
replaced atomically (temp file + rename) and a matching one is left
alone. It prints `devops: installed to <target>`,
`devops: refreshed from <source>` or `devops: already current.`; a failed
refresh only warns on stderr — the updater's status stays the exit code.
The interactive run (no arguments) performs the same ensure automatically
at startup, right before the menu: a missing or stale installed command
is re-installed or refreshed there too — silently when the command
already is the running binary or matches it byte-for-byte.

`install` follows the same semantics for the binary being run — the
canonical artifact `<root>/GO/devops-deployment-go` is what `update`
refreshes from, while `install` deploys whatever binary you ran, which is
honest because every build carries the complete embedded assets. It
prints `devops: installed to <target>`, `devops: refreshed from <self>`
or `devops: already current.`, or
`devops: skipped (already running as <target>)` when invoked as the
installed command itself; failures print
`devops: install failed: <error>` on stderr and exit 1. Running `install`
from the checkout also records the root pin (see "Deploying from
anywhere"), so the copy it leaves behind keeps resolving that checkout.

## Design decision: the installers stay bash

The Go binary is the front door, not a rewrite of everything behind it.
Every menu action dispatches to the bash installers via
`bash <scriptDir>/install-<tool>.sh`, with stdin/stdout/stderr inherited
so each script's own prompts and output reach the user unchanged. Since
the assets became embedded, the dispatch goes to the materialized copy of
the built-in `scripts/` in a fresh per-run temp dir — the repo checkout
is no longer needed at runtime; the child installers read the layout from
the exported `SCRIPT_DIR` / `SCRIPT_TMPDIR` / `ROOT` / `BASHFILES_DIR` /
`DEVOPS_SELF` environment (they fall back to self-located defaults when
run standalone from a checkout: the script's own dir and, two levels up,
the repo root).

## Deploying from anywhere

Like the bash script, the tool operates on a deployment root `<root>`:
`config.env` is read/written there and `tmp/` is created there — the
installers themselves run from the materialized embedded copy, so a root
without a GO module only limits those two, never the install actions.
Unlike the bash script — which hard-codes `~/devops-deployment` — the Go
port resolves `<root>` at startup, first match wins:

1. `DEVOPS_DEPLOYMENT_ROOT` — the environment variable, verbatim, even
   when it has no checkout of its own
2. the directory of the executable, when
   `GO/assets/scripts/install-kubectl.sh` exists under it (i.e. the
   binary sits at a checkout root)
3. the parent of the executable's directory, same marker file — the
   normal case, where the binary sits at
   `<repo-root>/GO/devops-deployment-go` and the repo root one level up
   matches
4. the pinned root — the absolute path stored in
   `~/.config/devops-deployment/path` (`DEVOPS_DEPLOYMENT_ROOT_PIN`
   overrides where that file lives), validated with the same marker file
   (`<pin>/GO/assets/scripts/install-kubectl.sh`); a stale or missing pin
   simply falls through
5. the current working directory, same marker file
6. the parent of the current working directory, same marker file
7. fallback: `$HOME/devops-deployment` (the bash-compatible default)

The marker validates the **repo root** — the checkout holding
`config.env`, `custom/` and `devops-deployment.sh` — never the GO module
directory itself: a `GO/` dir has no `GO/` of its own under it and cannot
match.

The pin keeps the installed `devops` command — a portable copy, written
by `install-bashtools.sh` (from `DEVOPS_SELF` when the Go menu runs it,
from `../GO/devops-deployment-go` when standalone) — pointed at its
checkout. When a binary-location or cwd candidate (2, 3, 5 or 6) wins,
the tool best-effort records the root in the pin file, so the copy keeps
resolving the checkout even when it runs from outside it; the
environment override, the pin itself and the fallback never write the
pin.

One startup line is an intentional addition vs the bash script: right
after `Detected OS:` the tool prints `Deployment root: <resolved path>`.

## First-run questions: Enter now proceeds

When no `config.env` exists yet, the stack/cloud questions behave
**intentionally differently from bash**: accepting a question with
nothing checked (`Enter` on an empty selection) no longer cancels — it
proceeds with the defaults (`rancher` for the stacks question, `none`
for the clouds question). Cancelling (`Esc`/`q`, or stdin ending) still
keeps the existing config exactly as before, printing
`Cancelled — config unchanged.`; nothing is saved unless both questions
are answered.

## Custom scripts

The helper scripts for the shell aliases are maintained inside the module
at `GO/assets/custom-scripts/` (seeded from the repo's legacy `custom/` —
edit and update them there). The update flow deploys them from the
binary's embedded copy:

- `u` in the TUI and menu item 14 (`Update installed scripts`), and the
  `update` subcommand, extract the embedded assets to a fresh temp dir
  and sync `custom-scripts/` from there into `~/.local/bin/scripts` via
  `internal/customsync`: new files are added, differing files are
  overwritten, exec bits are preserved, `.DS_Store` is filtered, and
  anything that exists only in the destination is never deleted. Each new
  or changed item is listed indented, followed by
  `Custom scripts: <n> item(s) updated.` (or
  `Custom scripts: already up to date.`). The sync source line names the
  embedded tree and the build the binary came from — the vcs revision
  when the build recorded one, `dev` otherwise.

There is no disk-source lookup anymore: the embedded tree always exists.
(`customsync.ResolveSource`, the pre-embedding directory probe with the
`DEVOPS_CUSTOM_SCRIPTS` override, is kept for direct use but is no longer
on the update path.)

## Differences vs the bash version

- **Single self-contained binary.** One compiled entrypoint with every
  runtime asset embedded, instead of a bash program requiring
  `bash >= 4` and the surrounding checkout (the associative arrays and
  `read -rsnN` handling are plain Go now). `go build` = new version.
- **Built-in multi-select.** The stack/cloud questions use an in-process
  multi-select picker, so `custom/misc/tui_select.sh` is no longer
  needed. (The bash `ask_stack_cloud` shells out to that helper.)
- **Empty accept proceeds.** The bash questions cancel when a question
  is accepted with nothing checked; the port takes the defaults
  instead — see "First-run questions" above. This is deliberate.
- **Deployment root is discovered, not hard-coded.** The bash script
  always assumes `~/devops-deployment`; the port resolves the root from
  the environment, the binary's location, the pin or the cwd — see
  "Deploying from anywhere" above. Its `Deployment root:` startup line is
  an addition vs bash.
- **Runner output is plain text.** The `▶` header, confirm prompt and
  `✓/✗` markers are unstyled, so they read the same under the TUI and
  the fallback menu.
- **No repo needed at runtime.** The installers stay bash but are
  embedded and materialized to the OS temp dir at startup — see
  "Embedded assets" above.

Everything else follows the bash script: same prerequisite checks
(`jq`, `unzip`, `wget`, installed via brew/apt/dnf/yum/pacman/zypper/
choco/scoop/winget), same `config.env` semantics, same
`$HOME/.local/bin` PATH block, same exit codes.

## Menu behavior

On startup, after the prerequisite checks and notes, the tool picks the
menu by the same gate the bash script uses (`tui_use_tui`): the
**full-screen TUI** runs only when stdin and stdout are both terminals
and the window is at least 60×16; otherwise the **fallback numeric
menu** is used. The TUI also degrades to the fallback if raw mode
cannot be entered.

### TUI key bindings

| Key               | Action                                             |
| ----------------- | -------------------------------------------------- |
| `↑` / `↓` or `j`/`k` | navigate                                        |
| `g` or `Home`     | jump to first entry                                |
| `G` or `End`      | jump to last entry                                 |
| `Enter` / `⏎`     | run the selected entry                             |
| `1`–`9`           | jump to entry N; run it if already selected        |
| `0`               | entry 10 (same jump/run rule)                      |
| `c`               | re-run the stack/cloud config questions            |
| `u`               | sync the embedded custom-scripts to `~/.local/bin/scripts` |
| `q` / `Q` / `Ctrl-D` | quit                                            |
| `Ctrl-C`          | restores the terminal and exits                    |

After every action the menu redraws (the action's status is ignored,
like bash's `run_menu_action "$sel" || true`).

### Fallback numeric menu

A plain `Select an option:` list (1–14) with an `Enter option:` prompt.
Options outside `1`–`14` print `Invalid option: '<option>'.` and exit 1.
