// Package ask holds the stack/cloud questions devops-deployment.sh asks
// once before the menu and again from its "Config" entry: the item lists
// and tokens_from_selection of items.go, and MultiSelect, the Go port of
// the --multi mode of custom/misc/tui_select.sh, which ask_stack_cloud()
// pipes K8S_STACK_ITEMS / CLOUD_ITEMS through.
//
// The port is deliberately smaller than the bash picker: no type-to-filter,
// no viewport/scrolling and no "accept the highlighted row when nothing is
// checked" fallback — the whole list is drawn and only what is checked is
// returned. What it does keep is the terminal handling the bash picker
// relies on: raw byte-at-a-time input via internal/term's Reader, the
// hide/show cursor pair around the loop, and erase-the-block before every
// exit.
//
// ask.go's AskStackCloud is the port of ask_stack_cloud() itself, and
// notes.go's PrintStackNotes the port of print_stack_notes(): the
// one-line-per-stack reminders shown after the prerequisite checks and
// again after a fresh config.env is saved.
package ask

import (
	"errors"
	"fmt"
	"io"
	"os"

	"devops-deployment-go/internal/term"
)

// ErrCancelled reports that the user backed out of the menu (Escape or q,
// or stdin ending), the way the bash picker exits 1 on cancel and
// ask_stack_cloud then keeps the existing config unchanged.
var ErrCancelled = errors.New("cancelled")

// MultiSelect is one interactive multi-choice question: a Title above the
// list, a Prompt line, and Items the user checks with Space. Run returns
// the checked items in list order.
type MultiSelect struct {
	Title  string
	Prompt string
	Items  []string
}

// ansiBoldReverse marks the row the cursor sits on; ansiReset ends it.
const (
	ansiBoldReverse = "\x1b[1;7m"
	ansiReset       = "\x1b[0m"
)

// erasePrefix is what the menu Draw (internal/menu/draw.go) writes ahead
// of every line and this picker must too: carriage return plus \e[2K,
// re-homing the column and wiping the line before it is (re)drawn.
const erasePrefix = "\r\x1b[2K"

// Run draws the menu on stdout and reads keystrokes from rd until the
// user accepts (Enter → the checked items, possibly an empty slice) or
// cancels (Escape/q or end of input → ErrCancelled).
//
// rd is the program's shared terminal Reader (term.NewReaderStdin), handed
// down sequentially: the caller keeps reading from the same instance after
// Run returns, so an escape window this picker leaves open — a lone ESC is
// the cancel key, and its 50ms window always expires — parks the next byte
// on the very Reader the caller reads next instead of stranding it on an
// abandoned one that would swallow a keystroke.
//
// stdin is switched to raw mode for the duration, mirroring the `read
// -rsn1` the bash picker does per keystroke, and both the termios settings
// and the cursor are restored by deferred calls on every return path — the
// same cleanup tui_select.sh gets from its EXIT trap. Non-interactive
// callers are turned away by MakeRaw itself: when fd 0 is not a terminal it
// returns an error before anything is drawn.
func (m *MultiSelect) Run(rd *term.Reader) ([]string, error) {
	out := os.Stdout

	state, err := term.MakeRaw(0)
	if err != nil {
		return nil, err
	}
	defer term.Restore(state)
	defer term.ShowCursor(out)
	term.HideCursor(out)

	cursor := 0
	checked := make(map[int]bool)
	lines := m.render(out, cursor, checked)

	// redraw wipes the block and paints it again from the top: render
	// leaves the cursor one line below the block, which is exactly the
	// position EraseBlock expects.
	redraw := func() {
		term.EraseBlock(lines)
		lines = m.render(out, cursor, checked)
	}

	for {
		key, err := rd.ReadKey()
		if err != nil {
			// The bash picker's `read ... <&3 || finish_cancel` treats a
			// failed read as a cancel — stdin reaching EOF (Ctrl-D on a
			// non-raw source, or a closed pipe) means there is nobody left
			// to answer. Real I/O errors propagate instead.
			term.EraseBlock(lines)
			if errors.Is(err, io.EOF) {
				return nil, ErrCancelled
			}
			return nil, err
		}

		switch {
		case key.Type == term.KeyUp || isRune(key, 'k'):
			if n := len(m.Items); n > 0 {
				cursor = (cursor - 1 + n) % n
			}
			redraw()
		case key.Type == term.KeyDown || isRune(key, 'j'):
			if n := len(m.Items); n > 0 {
				cursor = (cursor + 1) % n
			}
			redraw()
		case key.Type == term.KeySpace:
			if len(m.Items) > 0 {
				checked[cursor] = !checked[cursor]
			}
			redraw()
		case key.Type == term.KeyEnter:
			term.EraseBlock(lines)
			return m.selection(checked), nil
		case key.Type == term.KeyEscape || isRune(key, 'q') ||
			key.Type == term.KeyCtrlC || key.Type == term.KeyCtrlD:
			// Escape and q are this increment's cancel keys. Ctrl-C and
			// Ctrl-D come along because raw mode clears isig, so Ctrl-C
			// no longer raises SIGINT and Ctrl-D no longer EOFs: they
			// reach the loop as plain keystrokes, and letting them fall
			// through would strand a user who reached for the usual
			// abort. That is what tui_select.sh cancels on as well.
			term.EraseBlock(lines)
			return nil, ErrCancelled
		}
		// Anything else — Home/End, unbound escape sequences (KeyOther),
		// other printable runes — changes nothing, so it is swallowed
		// without a redraw.
	}
}

// isRune reports whether key is the printable rune r.
func isRune(key term.KeyMsg, r rune) bool {
	return key.Type == term.KeyRune && key.Rune == r
}

// selection returns the checked items in list order. The slice is empty,
// not nil, when nothing is checked.
func (m *MultiSelect) selection(checked map[int]bool) []string {
	selected := make([]string, 0, len(checked))
	for i, item := range m.Items {
		if checked[i] {
			selected = append(selected, item)
		}
	}
	return selected
}

// render paints the whole menu block to w and returns how many lines it
// drew: Title, a blank line, Prompt, then one line per item — "> " or "  "
// for the cursor row, an "[x]"/"[ ]" checkbox and the item text. Every
// line ends in \n, so the cursor finishes one line below the block, the
// position tui_erase() documents ("cursor must be one line below it").
//
// Like the menu Draw (internal/menu/draw.go) every line is written as
// erasePrefix + content + "\n": carriage return to column 0, clear the
// line, then the content. The \r is not cosmetic — Run holds the terminal
// in raw mode, where OPOST is off and a bare \n is LF-only, so without it
// each line would start at the column where the previous one ended (the
// staircase the bash picker's per-line printf never shows). The \x1b[2K
// is cursor control, not color, and is written even when NO_COLOR keeps
// the highlight off.
//
// The cursor row is printed bold and reverse-video unless NO_COLOR is set
// or stdout is not a terminal; the check is against stdout rather than w
// because Run always draws to stdout, and a non-tty stdout (a pipe) must
// not be handed escape sequences.
func (m *MultiSelect) render(w io.Writer, cursor int, checked map[int]bool) int {
	color := os.Getenv("NO_COLOR") == "" && term.IsTTY(os.Stdout.Fd())

	fmt.Fprint(w, erasePrefix+m.Title+"\n")
	fmt.Fprint(w, erasePrefix+"\n")
	fmt.Fprint(w, erasePrefix+m.Prompt+"\n")

	for i, item := range m.Items {
		marker := " "
		if i == cursor {
			marker = ">"
		}
		box := "[ ]"
		if checked[i] {
			box = "[x]"
		}
		line := fmt.Sprintf("%s %s %s", marker, box, item)
		if i == cursor && color {
			line = ansiBoldReverse + line + ansiReset
		}
		fmt.Fprint(w, erasePrefix+line+"\n")
	}

	return 3 + len(m.Items)
}
