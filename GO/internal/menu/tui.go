// tui.go is the Go port of the interactive half of devops-deployment.sh's
// TUI: tui_use_tui()'s gate, tui_loop()'s key loop, tui_run()'s action
// dispatch and fallback_numeric_menu()'s plain prompt. The drawing half —
// tui_geometry/tui_draw/tui_erase and setup_colors — lives in draw.go and
// colors.go, the action runner (run_menu_action + run_install) in
// runner.go.
//
// Where bash is only ever raw for the duration of one `read -rsn1`, the
// port keeps fd 0 in raw mode across the whole loop (term.MakeRaw) and
// hands the terminal back exactly where bash would have it: cooked for
// everything that prints or reads lines (RunAction, update-scripts.sh,
// ask.AskStackCloud), raw again for the single-keystroke pauses. Because
// raw mode also clears OPOST/onlcr, every newline printed while raw is
// written as "\r\n" — the two bytes the line discipline turns bash's bare
// "\n" printf output into — so the layout matches the bash TUI's.
//
// TuiLoop is inherently interactive — it needs a real terminal on fds 0
// and 1 — so, unlike FallbackMenu, it has no tests.

package menu

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"devops-deployment-go/internal/ask"
	"devops-deployment-go/internal/term"
)

// osExit is the exit FallbackMenu takes on an unparsable option — and the
// one TuiLoop's signal handler takes. It is a variable so tests can stub
// it and observe the exit instead of ending the test binary.
var osExit = os.Exit

// UseTUI ports tui_use_tui(): the interactive menu runs only when stdin
// and stdout are both terminals and the window — measured on stdout, the
// fd tui_geometry() probes — is at least 60 columns by 16 lines. The bash
// test reads TUI_LINES/TUI_COLS as tui_geometry() left them, columns
// capped at 78; the cap only lowers values above 78, which pass >= 60
// either way, so comparing the raw size is equivalent.
func UseTUI() bool {
	if !term.IsTTY(0) || !term.IsTTY(1) {
		return false
	}
	cols, lines, err := term.Size(os.Stdout.Fd())
	if err != nil {
		return false
	}
	return cols >= 60 && lines >= 16
}

// TuiLoop ports tui_loop(): clear the screen, draw the menu and react to
// keystrokes until the user quits. osName and pkgMgr fill the status line,
// root is passed through to ask.AskStackCloud for the config entry, and
// scriptDir is the materialized embedded scripts/ directory the tool
// actions dispatch from (the bash SCRIPT_DIR). rd is the program's shared
// keystroke Reader (term.NewReaderStdin), owned sequentially: the key
// loop, the waitKey pauses and the config entry's pickers all read the
// same instance, so an escape window left open by one phase parks its
// follow-up byte where the next phase's read finds it — e.g. a lone ESC in
// the picker leaves its byte for the "press any key" pause, which then
// swallows no other key. Like bash's main scope, which runs tui_use_tui
// before tui_loop, callers are expected to have consulted UseTUI first:
// the loop needs a terminal on fd 0 to hold in raw mode.
func TuiLoop(osName, pkgMgr, root, scriptDir string, rd *term.Reader) {
	// Geometry once, like the main scope's tui_geometry() call.
	cols, _, showLogo := Geometry(os.Stdout)

	// state carries the termios to restore. It is re-stored after every
	// cooked excursion below — the newest capture is the right one to put
	// back — and the signal goroutine reads it while the loop runs, hence
	// the atomic pointer.
	var state atomic.Pointer[term.State]

	saved, err := term.MakeRaw(0)
	if err != nil {
		// No raw mode, no interactive loop: degrade to the numeric menu,
		// the way term.MakeRaw's contract asks its callers to.
		FallbackMenu(osName, pkgMgr, root, scriptDir)
		return
	}
	state.Store(saved)
	// Cleanup for every ordinary exit — quit, end of input, even a panic.
	// The signal handler cannot share it (os.Exit skips defers) and does
	// both steps itself; Restore is idempotent, so the two compose.
	defer func() {
		term.Restore(state.Load())
		term.ShowCursor(os.Stdout)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		<-sig
		term.Restore(state.Load())
		term.ShowCursor(os.Stdout)
		osExit(1)
	}()

	term.HideCursor(os.Stdout) // hidden while navigating, as in tui_loop
	term.ClearScreen(os.Stdout)

	colors := NewColors(ColorsEnabled(os.Stdout))
	n := len(Items)
	sel := 0
	blockHeight := Draw(os.Stdout, sel, cols, showLogo, osName, pkgMgr, colors)

	// redraw ports tui_redraw(): wipe the block and paint it again from
	// its top line, keeping blockHeight from Draw's return.
	redraw := func(sel int) {
		term.EraseBlock(blockHeight)
		blockHeight = Draw(os.Stdout, sel, cols, showLogo, osName, pkgMgr, colors)
	}

	// waitKey ports `read -rsn1 ignored || true`: swallow the single
	// keystroke — or the end of input — that ends a pause. Reading through
	// the shared rd is what makes it swallow the right key: a lone ESC in
	// a picker before the pause parks its escape-window byte right here.
	waitKey := func() {
		_, _ = rd.ReadKey()
	}

	// runSelection ports tui_run(): erase the block, announce the entry,
	// hand the terminal back for the action, take it again for the
	// any-key pause.
	runSelection := func(sel int) {
		term.EraseBlock(blockHeight)
		// bash prints this header's \n\n through a cooked tty, whose onlcr
		// adds the carriage returns; raw mode needs them spelled out, or
		// the action's first output line staircases.
		fmt.Printf("\r\x1b[2K%s▶%s %s%s%s\r\n\r\n",
			colors.BMagenta, colors.Reset, colors.Bold, Items[sel].Label, colors.Reset)
		term.ShowCursor(os.Stdout) // installer prompts need a visible cursor
		term.Restore(state.Load())
		// bash: `run_menu_action "$sel" || true` — the menu always comes
		// back, whatever the action reported. rd goes along so the
		// __config entry's pickers read the same Reader this loop reads.
		_ = RunAction(Items[sel].Action, root, scriptDir, rd)
		if ns, err := term.MakeRaw(0); err == nil {
			state.Store(ns)
		}
		term.HideCursor(os.Stdout)
		fmt.Printf("\r\n%s─ press any key to return to the menu ─%s", colors.Dim, colors.Reset)
		waitKey()
		fmt.Printf("\r\n")
		term.ClearScreen(os.Stdout)
	}

	// The key loop; Reader.ReadKey stands in for the script's `read -rsn1`
	// plus the windowed `read -rsn2 -t 0.05` that separates escape
	// sequences from a lone ESC. KeyEscape, KeySpace, KeyOther and
	// KeyCtrlC match no case arm of the bash case and are swallowed
	// without a redraw, exactly like there.
loop:
	for {
		key, err := rd.ReadKey()
		if err != nil {
			// bash: `IFS= read -rsn1 key || key='q'` — end of input quits.
			break loop
		}
		switch key.Type {
		case term.KeyUp: // [A
			sel = (sel - 1 + n) % n
			redraw(sel)
		case term.KeyDown: // [B
			sel = (sel + 1) % n
			redraw(sel)
		case term.KeyHome: // [H, 1~, 7~
			sel = 0
			redraw(sel)
		case term.KeyEnd: // [F, 4~, 8~
			sel = n - 1
			redraw(sel)
		case term.KeyEnter: // \n and \r
			runSelection(sel)
			redraw(sel)
		case term.KeyCtrlD: // \004
			break loop
		case term.KeyRune:
			switch key.Rune {
			case 'j', 'J':
				sel = (sel + 1) % n
				redraw(sel)
			case 'k', 'K':
				sel = (sel - 1 + n) % n
				redraw(sel)
			case 'g':
				sel = 0
				redraw(sel)
			case 'G':
				sel = n - 1
				redraw(sel)
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				// bash: `d=$(( key - 1 ))` — run when already selected,
				// jump to the entry otherwise.
				if d := int(key.Rune-'0') - 1; d == sel {
					runSelection(sel)
					redraw(sel)
				} else {
					sel = d
					redraw(sel)
				}
			case '0':
				// 0 is entry 10's second digit, so it targets index 9.
				if sel == 9 {
					runSelection(sel)
					redraw(sel)
				} else {
					sel = 9
					redraw(sel)
				}
			case 'c', 'C':
				term.EraseBlock(blockHeight)
				// ask_stack_cloud runs on a cooked terminal in bash — only
				// its picker's reads are raw — so the questions' and notes'
				// output goes through onlcr here too. MultiSelect itself
				// does a MakeRaw/Restore pair around the picker, which now
				// captures and puts back this cooked state. The picker
				// reads this loop's own shared Reader, so a lone ESC parks
				// its escape-window byte on rd and the waitKey below — the
				// same instance — drains it: the pause swallows no other
				// key.
				term.Restore(state.Load())
				if _, err := ask.AskStackCloud(root, rd); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
				if ns, err := term.MakeRaw(0); err == nil {
					state.Store(ns)
				}
				fmt.Printf("\r\n%spress any key to return%s", colors.Dim, colors.Reset)
				waitKey()
				term.ClearScreen(os.Stdout)
				redraw(sel)
			case 'u', 'U':
				term.EraseBlock(blockHeight)
				// bash: `bash "$SCRIPT_DIR/update-scripts.sh"` — a cooked
				// terminal with all three streams inherited; the exit
				// status is ignored. The shared UpdateScripts is pure Go
				// now: it syncs the embedded custom-scripts into
				// ~/.local/bin/scripts, no bash updater in between.
				term.Restore(state.Load())
				_ = UpdateScripts()
				if ns, err := term.MakeRaw(0); err == nil {
					state.Store(ns)
				}
				fmt.Printf("\r\n%spress any key to return%s", colors.Dim, colors.Reset)
				waitKey()
				term.ClearScreen(os.Stdout)
				redraw(sel)
			case 'q', 'Q':
				break loop
			}
		}
	}

	// Quit: bash's trailing `tui_erase` plus the `\e[?25h` its EXIT trap
	// stands for here; the deferred cleanup does the ShowCursor (and the
	// termios restore).
	term.EraseBlock(blockHeight)
}

// FallbackMenu ports fallback_numeric_menu(): the plain numeric prompt used
// when the terminal is missing or too small for the interactive layout.
// osName and pkgMgr are never printed here — the bash fallback shows
// neither — and sit in the signature only so both menus share one call
// site, along with the root/scriptDir RunAction needs. A choice runs
// straight through RunAction with no ▶ header, like bash's bare
// `run_menu_action "$sel" || true`.
func FallbackMenu(osName, pkgMgr, root, scriptDir string) {
	fmt.Println("Select an option:")
	for i, item := range Items {
		fmt.Printf("  %2d - %s\n", i+1, item.Label)
	}
	fmt.Println()
	// bash `read -rp` shows the prompt on standard error, not stdout.
	fmt.Fprint(os.Stderr, "Enter option: ")
	line, _ := readLine(os.Stdin)
	option := strings.TrimSpace(line)

	sel, ok := numericSelection(option)
	if !ok {
		fmt.Printf("Invalid option: '%s'.\n", option)
		osExit(1)
		// Unreachable under the real os.Exit; keeps the flow honest when
		// tests stub osExit out.
		return
	}
	// bash: `run_menu_action "$sel" || true`. nil Reader: this menu reads
	// lines, not keystrokes, so it has none to hand down — the __config
	// entry creates one if the questions turn out to be askable.
	_ = RunAction(Items[sel].Action, root, scriptDir, nil)
}

// numericSelection maps the option strings the bash case lists — exactly
// "1" through "14", so "01" or "1st" stay invalid — to the 0-based item
// index.
func numericSelection(option string) (int, bool) {
	for i := range Items {
		if option == strconv.Itoa(i+1) {
			return i, true
		}
	}
	return 0, false
}
